package syncservice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/open-horizon/edge-sync-service-client/client"

	"github.com/stolostron/multicluster-globalhub/manager/pkg/statistics"
	"github.com/stolostron/multicluster-globalhub/manager/pkg/statussyncer/transport2db/bundle"
	"github.com/stolostron/multicluster-globalhub/manager/pkg/statussyncer/transport2db/conflator"
	"github.com/stolostron/multicluster-globalhub/manager/pkg/statussyncer/transport2db/transport"
	"github.com/stolostron/multicluster-globalhub/pkg/compressor"
	"github.com/stolostron/multicluster-globalhub/pkg/constants"
)

type SyncServiceConfig struct {
	Protocol string
	CSSHost  string
	// int to uint16 and keep align with
	// https://pkg.go.dev/github.com/open-horizon/edge-sync-service-client/client#NewSyncServiceClient
	CSSPort int
	// keep align with
	// https://pkg.go.dev/github.com/open-horizon/edge-sync-service-client/client#SyncServiceClient.StartPollingForUpdates
	PollingInterval int
}

const (
	msgIDHeaderTokensLength       = 2
	compressionHeaderTokensLength = 2
	defaultCompressionType        = compressor.NoOp
)

var (
	errSyncServiceReadFailed  = errors.New("sync service error")
	errMessageIDWrongFormat   = errors.New("message ID format is bad")
	errMissingCompressionType = errors.New("compression type is missing from message description")
)

// NewSyncService creates a new instance of SyncService.
func NewSyncService(committerInterval time.Duration, syncServiceConfig *SyncServiceConfig,
	conflationManager *conflator.ConflationManager, statistics *statistics.Statistics,
	log logr.Logger,
) (*SyncService, error) {
	syncServiceClient := client.NewSyncServiceClient(syncServiceConfig.Protocol,
		syncServiceConfig.CSSHost, uint16(syncServiceConfig.CSSPort))

	syncServiceClient.SetOrgID("myorg")
	syncServiceClient.SetAppKeyAndSecret("user@myorg", "")

	// create committer
	committer, err := newCommitter(committerInterval, syncServiceClient,
		conflationManager.GetBundlesMetadata, log)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize sync service - %w", err)
	}

	ctx, cancelFunc := context.WithCancel(context.Background())

	return &SyncService{
		log:                    log,
		client:                 syncServiceClient,
		committer:              committer,
		compressorsMap:         make(map[compressor.CompressionType]compressor.Compressor),
		conflationManager:      conflationManager,
		statistics:             statistics,
		pollingInterval:        syncServiceConfig.PollingInterval,
		objectsMetaDataChan:    make(chan *client.ObjectMetaData),
		msgIDToRegistrationMap: make(map[string]*transport.BundleRegistration),
		ctx:                    ctx,
		cancelFunc:             cancelFunc,
	}, nil
}

// SyncService abstracts Sync Service client.
type SyncService struct {
	log               logr.Logger
	client            *client.SyncServiceClient
	committer         *committer
	compressorsMap    map[compressor.CompressionType]compressor.Compressor
	conflationManager *conflator.ConflationManager
	statistics        *statistics.Statistics

	pollingInterval        int
	objectsMetaDataChan    chan *client.ObjectMetaData
	msgIDToRegistrationMap map[string]*transport.BundleRegistration

	ctx        context.Context
	cancelFunc context.CancelFunc
	startOnce  sync.Once
	stopOnce   sync.Once
}

// Start function starts sync service.
func (s *SyncService) Start() {
	s.startOnce.Do(func() {
		go s.committer.start(s.ctx)
		go s.handleBundles(s.ctx)
	})
}

// Stop function stops sync service.
func (s *SyncService) Stop() {
	s.stopOnce.Do(func() {
		s.cancelFunc()
		close(s.objectsMetaDataChan)
	})
}

// Register function registers a msgID for sync service to know how to create the bundle, and use predicate.
func (s *SyncService) Register(registration *transport.BundleRegistration) {
	s.msgIDToRegistrationMap[registration.MsgID] = registration
}

func (s *SyncService) handleBundles(ctx context.Context) {
	// register for updates for spec bundles, this includes all types of spec bundles each with a different id.
	s.client.StartPollingForUpdates(constants.StatusBundle, s.pollingInterval, s.objectsMetaDataChan)

	for {
		select {
		case <-ctx.Done():
			return
		case objectMetadata := <-s.objectsMetaDataChan:
			var buffer bytes.Buffer
			if !s.client.FetchObjectData(objectMetadata, &buffer) {
				s.logError(errSyncServiceReadFailed, "failed to read bundle from sync service", objectMetadata)
				continue
			}

			// get msgID
			msgIDTokens := strings.Split(objectMetadata.ObjectID, ".") // object id is LH_ID.MSG_ID
			if len(msgIDTokens) != msgIDHeaderTokensLength {
				s.logError(errMessageIDWrongFormat, "expecting ObjectID of format LH_ID.MSG_ID", objectMetadata)
				continue
			}

			msgID := msgIDTokens[1]
			if _, found := s.msgIDToRegistrationMap[msgID]; !found {
				s.log.Info("no registration available, not sending bundle", "ObjectId",
					objectMetadata.ObjectID)
				continue // no one registered for this msg id
			}

			if !s.msgIDToRegistrationMap[msgID].Predicate() {
				s.log.Info("Predicate is false, not sending bundle", "ObjectId",
					objectMetadata.ObjectID)
				continue // registration predicate is false, do not send the update in the channel
			}

			receivedBundle := s.msgIDToRegistrationMap[msgID].CreateBundleFunc()
			if err := s.unmarshalPayload(receivedBundle, objectMetadata, buffer.Bytes()); err != nil {
				s.logError(err, "failed to get object payload", objectMetadata)
				continue
			}

			s.statistics.IncrementNumberOfReceivedBundles(receivedBundle)

			s.conflationManager.Insert(receivedBundle, newBundleMetadata(objectMetadata))

			if err := s.client.MarkObjectReceived(objectMetadata); err != nil {
				s.logError(err, "failed to report object received to sync service", objectMetadata)
			}
		}
	}
}

func (s *SyncService) logError(err error, errMsg string, objectMetaData *client.ObjectMetaData) {
	s.log.Error(err, errMsg, "ObjectID", objectMetaData.ObjectID, "ObjectType", objectMetaData.ObjectType,
		"ObjectDescription", objectMetaData.Description, "Version", objectMetaData.Version)
}

func (s *SyncService) unmarshalPayload(receivedBundle bundle.Bundle, objectMetaData *client.ObjectMetaData,
	payload []byte,
) error {
	compressionType := defaultCompressionType

	if objectMetaData.Description != "" {
		compressionTokens := strings.Split(objectMetaData.Description, ":") // obj desc is Content-Encoding:type
		if len(compressionTokens) != compressionHeaderTokensLength {
			return fmt.Errorf("invalid compression header (Description) - %w", errMissingCompressionType)
		}

		compressionType = compressor.CompressionType(compressionTokens[1])
	}

	decompressedPayload, err := s.decompressPayload(payload, compressionType)
	if err != nil {
		return fmt.Errorf("failed to decompress bundle bytes - %w", err)
	}

	if err := json.Unmarshal(decompressedPayload, receivedBundle); err != nil {
		return fmt.Errorf("failed to parse bundle - %w", err)
	}

	return nil
}

func (s *SyncService) decompressPayload(payload []byte, msgCompressorType compressor.CompressionType) ([]byte, error) {
	msgCompressor, found := s.compressorsMap[msgCompressorType]
	if !found {
		newCompressor, err := compressor.NewCompressor(msgCompressorType)
		if err != nil {
			return nil, fmt.Errorf("failed to create compressor: %w", err)
		}

		msgCompressor = newCompressor
		s.compressorsMap[msgCompressorType] = msgCompressor
	}

	decompressedBytes, err := msgCompressor.Decompress(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress message: %w", err)
	}

	return decompressedBytes, nil
}

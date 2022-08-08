package consumer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/open-horizon/edge-sync-service-client/client"

	helper "github.com/stolostron/hub-of-hubs/agent/pkg/helper"
	bundle "github.com/stolostron/hub-of-hubs/agent/pkg/spec/bundle"
	"github.com/stolostron/hub-of-hubs/pkg/compressor"
	"github.com/stolostron/hub-of-hubs/pkg/constants"
)

const (
	compressionHeaderTokensLength = 2
)

// SyncService abstracts Sync Service client.
type SyncService struct {
	log                             logr.Logger
	client                          *client.SyncServiceClient
	compressorsMap                  map[compressor.CompressionType]compressor.Compressor
	pollingInterval                 int
	bundlesMetaDataChan             chan *client.ObjectMetaData
	genericBundlesChan              chan *bundle.GenericBundle
	customBundleIDToRegistrationMap map[string]*bundle.CustomBundleRegistration
	// map from object key to metadata. size limited at all times.
	commitMap map[string]*client.ObjectMetaData

	ctx        context.Context
	cancelFunc context.CancelFunc
	startOnce  sync.Once
	stopOnce   sync.Once
	lock       sync.Mutex
}

// NewSyncService creates a new instance of SyncService.
func NewSyncService(log logr.Logger, environmentManager *helper.ConfigManager,
	genericBundlesUpdatesChan chan *bundle.GenericBundle) (*SyncService, error) {
	syncServiceConfig := environmentManager.SyncService
	syncServiceClient := client.NewSyncServiceClient(syncServiceConfig.Protocol,
		syncServiceConfig.ConsumerHost, uint16(syncServiceConfig.ConsumerPort))
	syncServiceClient.SetAppKeyAndSecret("user@myorg", "")

	ctx, cancelFunc := context.WithCancel(context.Background())

	return &SyncService{
		log:                             log,
		client:                          syncServiceClient,
		compressorsMap:                  make(map[compressor.CompressionType]compressor.Compressor),
		pollingInterval:                 syncServiceConfig.ConsumerPollingInterval,
		bundlesMetaDataChan:             make(chan *client.ObjectMetaData),
		genericBundlesChan:              genericBundlesUpdatesChan,
		customBundleIDToRegistrationMap: make(map[string]*bundle.CustomBundleRegistration),
		commitMap:                       make(map[string]*client.ObjectMetaData),
		ctx:                             ctx,
		cancelFunc:                      cancelFunc,
		lock:                            sync.Mutex{},
	}, nil
}

// Start function starts sync service.
func (s *SyncService) Start() {
	s.startOnce.Do(func() {
		go s.handleBundles(s.ctx)
	})
}

// Stop function stops sync service.
func (s *SyncService) Stop() {
	s.stopOnce.Do(func() {
		s.cancelFunc()
		close(s.bundlesMetaDataChan)
	})
}

// Register function registers a bundle ID to a CustomBundleRegistration.
func (s *SyncService) Register(msgID string, customBundleRegistration *bundle.CustomBundleRegistration) {
	s.customBundleIDToRegistrationMap[msgID] = customBundleRegistration
}

func (s *SyncService) handleBundles(ctx context.Context) {
	// register for updates for spec bundles and config objects, includes all types of spec bundles.
	s.client.StartPollingForUpdates(constants.SpecBundle, s.pollingInterval, s.bundlesMetaDataChan)

	for {
		select {
		case <-ctx.Done():
			return
		case objectMetaData := <-s.bundlesMetaDataChan:
			if err := s.handleBundle(objectMetaData); err != nil {
				s.logError(err, "failed to handle bundle", objectMetaData)
			}
		}
	}
}

func (s *SyncService) handleBundle(objectMetaData *client.ObjectMetaData) error {
	var buf bytes.Buffer
	if !s.client.FetchObjectData(objectMetaData, &buf) {
		return errors.New("sync service error")
	}
	// sync-service does not need to check for leafHubName since we assume actual selective-distribution.
	decompressedPayload, err := s.decompressPayload(buf.Bytes(), objectMetaData)
	if err != nil {
		return fmt.Errorf("failed to decompress bundle bytes - %w", err)
	}

	// if selective distribution was applied msgID contains "LH_ID." prefix. if not, trim returns string as is.
	msgID := strings.TrimPrefix(objectMetaData.ObjectID, fmt.Sprintf("%s.", objectMetaData.DestID))

	customBundleRegistration, found := s.customBundleIDToRegistrationMap[msgID]
	if !found { // received generic bundle
		if err := s.syncGenericBundle(decompressedPayload); err != nil {
			return fmt.Errorf("failed to sync generic bundle - %w", err)
		}
	} else {
		if err := s.SyncCustomBundle(customBundleRegistration, decompressedPayload); err != nil {
			return fmt.Errorf("failed to sync custom bundle - %w", err)
		}
	}
	// mark received
	if err := s.client.MarkObjectReceived(objectMetaData); err != nil {
		return fmt.Errorf("failed to report object received to sync service - %w", err)
	}

	return nil
}

func (s *SyncService) syncGenericBundle(payload []byte) error {
	receivedBundle := bundle.NewGenericBundle()
	if err := json.Unmarshal(payload, receivedBundle); err != nil {
		return fmt.Errorf("failed to parse bundle - %w", err)
	}

	s.genericBundlesChan <- receivedBundle

	return nil
}

// SyncCustomBundle writes a custom bundle to its respective syncer channel.
func (c *SyncService) SyncCustomBundle(customBundleRegistration *bundle.CustomBundleRegistration,
	payload []byte) error {
	receivedBundle := customBundleRegistration.InitBundlesResourceFunc()
	if err := json.Unmarshal(payload, &receivedBundle); err != nil {
		return fmt.Errorf("failed to parse custom bundle - %w", err)
	}
	customBundleRegistration.BundleUpdatesChan <- receivedBundle
	return nil
}

func (s *SyncService) logError(err error, errMsg string, objectMetaData *client.ObjectMetaData) {
	s.log.Error(err, errMsg, "ObjectID", objectMetaData.ObjectID, "ObjectType", objectMetaData.ObjectType,
		"ObjectDescription", objectMetaData.Description, "Version", objectMetaData.Version)
}

func (s *SyncService) decompressPayload(payload []byte, objectMetaData *client.ObjectMetaData) ([]byte, error) {
	compressionType, err := s.getObjectCompressionType(objectMetaData)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress message: %w", err)
	}

	msgCompressor, found := s.compressorsMap[compressionType]
	if !found {
		newCompressor, err := compressor.NewCompressor(compressionType)
		if err != nil {
			return nil, fmt.Errorf("failed to create compressor: %w", err)
		}

		msgCompressor = newCompressor
		s.compressorsMap[compressionType] = msgCompressor
	}

	decompressedBytes, err := msgCompressor.Decompress(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress message: %w", err)
	}

	return decompressedBytes, nil
}

func (s *SyncService) getObjectCompressionType(objectMetaData *client.ObjectMetaData,
) (compressor.CompressionType, error) {
	if objectMetaData.Description == "" { // obj desc is Content-Encoding:type
		return "", errors.New("compression type is missing from message description")
	}

	compressionTokens := strings.Split(objectMetaData.Description, ":")
	if len(compressionTokens) != compressionHeaderTokensLength {
		return "", errors.New("invalid compression header (Description)")
	}

	return compressor.CompressionType(compressionTokens[1]), nil
}

func (s *SyncService) GetGenericBundleChan() chan *bundle.GenericBundle {
	return s.genericBundlesChan
}

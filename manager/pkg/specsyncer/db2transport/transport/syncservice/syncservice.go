package syncservice

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	"github.com/open-horizon/edge-sync-service-client/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/transport"
	statussyncservice "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/transport/syncservice"
	"github.com/stolostron/multicluster-global-hub/pkg/compressor"
)

const (
	compressionHeader              = "Content-Encoding"
	syncServiceDestinationTypeEdge = "edge"
)

// NewSyncService returns a new instance of SyncService object.
func NewSyncService(compressor compressor.Compressor, syncServiceConfig *statussyncservice.SyncServiceConfig,
	log logr.Logger,
) (*SyncService, error) {
	syncServiceClient := client.NewSyncServiceClient(syncServiceConfig.Protocol,
		syncServiceConfig.CSSHost, uint16(syncServiceConfig.CSSPort))

	syncServiceClient.SetOrgID("myorg")
	syncServiceClient.SetAppKeyAndSecret("user@myorg", "")

	return &SyncService{
		log:        log,
		client:     syncServiceClient,
		compressor: compressor,
		msgChan:    make(chan *transport.Message),
		stopChan:   make(chan struct{}, 1),
	}, nil
}

// SyncService abstracts Open Horizon Sync Service usage.
type SyncService struct {
	log        logr.Logger
	client     *client.SyncServiceClient
	compressor compressor.Compressor
	msgChan    chan *transport.Message
	stopChan   chan struct{}
	startOnce  sync.Once
	stopOnce   sync.Once
}

// Start starts the sync service.
func (s *SyncService) Start() {
	s.startOnce.Do(func() {
		go s.distributeMessages()
	})
}

// Stop stops the sync service.
func (s *SyncService) Stop() {
	s.stopOnce.Do(func() {
		s.stopChan <- struct{}{}
		close(s.stopChan)
		close(s.msgChan)
	})
}

// SendAsync sends a message to the sync service asynchronously.
func (s *SyncService) SendAsync(destinationHubName string, id string, msgType string, version string, payload []byte) {
	message := &transport.Message{
		Destination: destinationHubName,
		ID:          id,
		MsgType:     msgType,
		Version:     version,
		Payload:     payload,
	}
	s.msgChan <- message
}

func (s *SyncService) distributeMessages() {
	for {
		select {
		case <-s.stopChan:
			return
		case msg := <-s.msgChan:
			objectMetaData := client.ObjectMetaData{
				ObjectID:    msg.ID,
				ObjectType:  msg.MsgType,
				Version:     msg.Version,
				Description: fmt.Sprintf("%s:%s", compressionHeader, s.compressor.GetType()),
			}

			if msg.Destination != transport.Broadcast { // only if specific to a hub, modify obj id
				objectMetaData.ObjectID = fmt.Sprintf("%s.%s", msg.Destination, msg.ID)
				objectMetaData.DestID = msg.Destination // if broadcast then empty, works as usual.
				objectMetaData.DestType = syncServiceDestinationTypeEdge
			}

			if err := s.client.UpdateObject(&objectMetaData); err != nil {
				s.log.Error(err, "Failed to update the object in the Cloud Sync Service")
				continue
			}

			compressedBytes, err := s.compressor.Compress(msg.Payload)
			if err != nil {
				s.log.Error(err, "Failed to compress payload", "CompressorType",
					s.compressor.GetType(), "MessageId", msg.ID, "MessageType", msg.MsgType, "Version",
					msg.Version)

				continue
			}

			reader := bytes.NewReader(compressedBytes)
			if err := s.client.UpdateObjectData(&objectMetaData, reader); err != nil {
				s.log.Error(err, "Failed to update the object data in the Cloud Sync Service")
				continue
			}

			s.log.Info("Message sent successfully", "MessageId", msg.ID, "MessageType", msg.MsgType,
				"DestinationID", objectMetaData.DestID, "DestinationType", objectMetaData.DestType,
				"Version", msg.Version)
		}
	}
}

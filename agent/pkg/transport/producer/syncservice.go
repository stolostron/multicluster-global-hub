package producer

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	"github.com/open-horizon/edge-sync-service-client/client"

	"github.com/stolostron/hub-of-hubs/agent/pkg/helper"
	"github.com/stolostron/hub-of-hubs/pkg/compressor"
)

const (
	// envVarSyncServiceProtocol = "SYNC_SERVICE_PROTOCOL"
	// envVarSyncServiceHost     = "SYNC_SERVICE_HOST"
	// envVarSyncServicePort     = "SYNC_SERVICE_PORT"
	compressionHeader = "Content-Encoding"
)

// SyncService abstracts Sync Service client.
type SyncServiceProducer struct {
	log                  logr.Logger
	client               *client.SyncServiceClient
	eventSubscriptionMap map[string]map[EventType]EventCallback
	compressor           compressor.Compressor
	msgChan              chan *Message
	stopChan             chan struct{}
	startOnce            sync.Once
	stopOnce             sync.Once
}

// NewSyncService creates a new instance of SyncService.
func NewSyncServiceProducer(compressor compressor.Compressor, log logr.Logger, env *helper.ConfigManager) (*SyncServiceProducer, error) {
	syncServiceClient := client.NewSyncServiceClient(env.SyncService.Protocol, env.SyncService.ProducerHost, uint16(env.SyncService.ProducerPort))
	syncServiceClient.SetAppKeyAndSecret("user@myorg", "")

	return &SyncServiceProducer{
		log:                  log,
		client:               syncServiceClient,
		eventSubscriptionMap: make(map[string]map[EventType]EventCallback),
		compressor:           compressor,
		msgChan:              make(chan *Message),
		stopChan:             make(chan struct{}, 1),
	}, nil
}

// Start function starts sync service.
func (s *SyncServiceProducer) Start() {
	s.startOnce.Do(func() {
		go s.sendMessages()
	})
}

// Stop function stops sync service.
func (s *SyncServiceProducer) Stop() {
	s.stopOnce.Do(func() {
		s.stopChan <- struct{}{}
		close(s.stopChan)
		close(s.msgChan)
	})
}

// Subscribe adds a callback to be delegated when a given event occurs for a message with the given ID.
func (s *SyncServiceProducer) Subscribe(messageID string, callbacks map[EventType]EventCallback) {
	s.eventSubscriptionMap[messageID] = callbacks
}

// SupportsDeltaBundles returns false. sync service doesn't support delta bundles.
func (s *SyncServiceProducer) SupportsDeltaBundles() bool {
	return false
}

// SendAsync function sends a message to the sync service asynchronously.
func (s *SyncServiceProducer) SendAsync(message *Message) {
	s.msgChan <- message
}

func (s *SyncServiceProducer) sendMessages() {
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

			// InvokeCallback(s.eventSubscriptionMap, msg.ID, DeliveryAttempt)
			s.eventSubscriptionMap[msg.ID][DeliveryAttempt]()

			if err := s.client.UpdateObject(&objectMetaData); err != nil {
				s.reportError(err, "Failed to update the object in the Edge Sync Service", msg)
				continue
			}

			compressedBytes, err := s.compressor.Compress(msg.Payload)
			if err != nil {
				s.reportError(err, "Failed to compress payload", msg)
				continue
			}

			reader := bytes.NewReader(compressedBytes)
			if err := s.client.UpdateObjectData(&objectMetaData, reader); err != nil {
				s.reportError(err, "Failed to update the object data in the Edge Sync Service", msg)
				continue
			}

			s.eventSubscriptionMap[msg.ID][DeliverySuccess]()
			s.log.Info("Message sent successfully", "MessageId", msg.ID, "MessageType", msg.MsgType,
				"Version", msg.Version)
		}
	}
}

func (s *SyncServiceProducer) reportError(err error, errorMsg string, msg *Message) {
	s.eventSubscriptionMap[msg.ID][DeliveryFailure]()
	s.log.Error(err, errorMsg, "CompressorType", s.compressor.GetType(), "MessageId", msg.ID,
		"MessageType", msg.MsgType, "Version", msg.Version)
}

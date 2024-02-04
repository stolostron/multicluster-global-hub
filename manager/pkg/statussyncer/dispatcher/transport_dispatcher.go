package dispatcher

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/handler"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/registration"
)

// Get message from transport, convert it to bundle and forward it to conflation manager.
type TransportDispatcher struct {
	log                 logr.Logger
	consumer            transport.Consumer
	bundleRegistrations map[string]*registration.BundleRegistration // msgID: BundleRegistration
	eventHandler        map[enum.EventType]handler.EventHandler
	statistics          *statistics.Statistics
	conflationManager   *conflator.ConflationManager
}

func NewTransportDispatcher(log logr.Logger, consumer transport.Consumer,
	conflationManager *conflator.ConflationManager, stats *statistics.Statistics,
) *TransportDispatcher {
	return &TransportDispatcher{
		log:                 log,
		consumer:            consumer,
		bundleRegistrations: make(map[string]*registration.BundleRegistration),
		eventHandler:        make(map[enum.EventType]handler.EventHandler),
		statistics:          stats,
		conflationManager:   conflationManager,
	}
}

func (d *TransportDispatcher) RegisterEventHandler(eventType enum.EventType, h handler.EventHandler) {
	d.eventHandler[eventType] = h
}

func (d *TransportDispatcher) BundleRegister(registration *registration.BundleRegistration) {
	d.bundleRegistrations[registration.MsgID] = registration
}

// Start function starts bundles status syncer.
func (d *TransportDispatcher) Start(ctx context.Context) error {
	d.log.Info("transport dispatcher starts dispatching received bundles...")

	go d.dispatch(ctx)

	<-ctx.Done() // blocking wait for stop event
	d.log.Info("stopped dispatching bundles")

	return nil
}

func (d *TransportDispatcher) dispatch(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-d.consumer.EventChan():
			//TODO: need to be intergrate to conflation in the next step
			d.process(evt)
		case message := <-d.consumer.MessageChan():

			// get msgID
			msgIDTokens := strings.Split(message.Key, ".") // object id is LH_ID.MSG_ID
			if len(msgIDTokens) != 2 {
				d.log.Error(errors.New("message ID format is bad"), "expecting MsgID format LH_ID.MSG_ID", "message", message)
				continue
			}

			msgID := msgIDTokens[1]
			if _, found := d.bundleRegistrations[msgID]; !found {
				// no one registered for this msg id
				d.log.Error(errors.New("msgID not found"), "no bundle-registration available", "message", message)
				continue
			}

			if !d.bundleRegistrations[msgID].Predicate() {
				d.log.Error(errors.New("predicate with false"),
					"bundle with false predicate", "message", message)
				continue // bundle-registration predicate is false, do not send the update in the channel
			}

			receivedBundle := d.bundleRegistrations[msgID].CreateBundleFunc()
			if err := json.Unmarshal(message.Payload, receivedBundle); err != nil {
				d.log.Error(errors.New("unmarshal error"),
					"parse message.payload error", "message", message)
				continue
			}

			d.statistics.IncrementNumberOfReceivedBundles(receivedBundle)
			// d.conflationManager.Insert(receivedBundle, NewBundleMetadata(message.TopicPartition.Partition,
			// 	message.TopicPartition.Offset))
			d.log.V(2).Info("forward received bundle to conflation", "messageID", msgID)
			d.conflationManager.Insert(receivedBundle, message.BundleStatus)
		}
	}
}

func (d *TransportDispatcher) process(evt cloudevents.Event) {
	h, ok := d.eventHandler[enum.EventType(evt.Type())]
	if !ok {
		d.log.Error(errors.New("not found type of the cloudevent"), "no handler available", "event", evt)
		return
	}

	err := h.ToDatabase(evt)
	if err != nil {
		d.log.Error(err, "failed to save event to database", "event", evt)
	}
}

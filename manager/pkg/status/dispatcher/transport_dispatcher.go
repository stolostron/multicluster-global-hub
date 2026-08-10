package dispatcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/cloudevents/sdk-go/protocol/kafka_confluent/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	cetypes "github.com/cloudevents/sdk-go/v2/types"
	"go.uber.org/zap"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// Get message from transport, convert it to bundle and forward it to conflation manager.
type TransportDispatcher struct {
	log               *zap.SugaredLogger
	consumer          transport.Consumer
	conflationManager *conflator.ConflationManager
	statistic         *statistics.Statistics
	// statusTopic is the manager's configured status-topic template
	// (e.g. "^gh-status.*"). When it contains '*', per-hub status topics
	// are in use and the broker-enforced topic name carries the hub
	// identity, so the dispatcher binds evt.Source() to the topic.
	statusTopic string
}

func AddTransportDispatcher(mgr ctrl.Manager, consumer transport.Consumer, managerConfig *configs.ManagerConfig,
	conflationManager *conflator.ConflationManager, stats *statistics.Statistics,
) error {
	statusTopic := ""
	if managerConfig.TransportConfig != nil && managerConfig.TransportConfig.KafkaCredential != nil {
		statusTopic = managerConfig.TransportConfig.KafkaCredential.StatusTopic
	}
	transportDispatcher := &TransportDispatcher{
		log:               logger.DefaultZapLogger(),
		consumer:          consumer,
		conflationManager: conflationManager,
		statistic:         stats,
		statusTopic:       statusTopic,
	}
	if err := mgr.Add(transportDispatcher); err != nil {
		return fmt.Errorf("failed to add transport dispatcher to runtime manager: %w", err)
	}
	return nil
}

// sourceMatchesTopic verifies that the self-asserted CloudEvent Source
// matches the broker-enforced Kafka topic the event arrived on. Each
// managed hub's KafkaUser only has Write ACL on its own per-hub status
// topic, so the topic name is the only trustworthy carrier of leaf-hub
// identity. Returns true when the binding holds or cannot be evaluated
// (shared-topic / non-Kafka transport).
func sourceMatchesTopic(evt *cloudevents.Event, statusTopic string) bool {
	if !strings.Contains(statusTopic, "*") {
		// Shared status topic (BYO or non-wildcard Strimzi config); the
		// broker provides no per-hub identity to bind against.
		return true
	}
	topic, err := cetypes.ToString(evt.Extensions()[kafka_confluent.KafkaTopicKey])
	if err != nil || topic == "" {
		// Non-Kafka transport (e.g. go-chan in tests) — nothing to bind.
		return true
	}
	if evt.Source() == "" {
		return false
	}
	template := strings.TrimPrefix(statusTopic, "^")
	expected := strings.ReplaceAll(template, "*", evt.Source())
	return topic == expected
}

// Start function starts bundles status syncer.
func (d *TransportDispatcher) Start(ctx context.Context) error {
	d.log.Info("transport dispatcher starts dispatching received events...")

	go d.dispatch(ctx)

	<-ctx.Done() // blocking wait for stop event
	d.log.Info("stopped dispatching events")

	return nil
}

func (d *TransportDispatcher) dispatch(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-d.consumer.EventChan():
			d.statistic.ReceivedEvent(evt)
			d.log.Debugf("received event: %s", evt)
			if !sourceMatchesTopic(evt, d.statusTopic) {
				sourceHash := sha256.Sum256([]byte(evt.Source()))
				d.log.Warnw("dropping status event: source does not match Kafka topic",
					"sourceHash", hex.EncodeToString(sourceHash[:8]))
				continue
			}
			d.conflationManager.Insert(evt)
		}
	}
}

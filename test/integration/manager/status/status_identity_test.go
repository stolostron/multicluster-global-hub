/*
Copyright Contributors to the Open Cluster Management project.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package status

import (
	"errors"
	"fmt"
	"time"

	kafka_confluent "github.com/cloudevents/sdk-go/protocol/kafka_confluent/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

// go test ./test/integration/manager/status -ginkgo.focus "StatusEventIdentity"
var _ = Describe("StatusEventIdentity", Ordered, func() {
	const (
		trustedHub = "identity-trusted-hub"
		topicHub   = "identity-topic-only-hub"
		victimHub  = "identity-victim-hub"
	)

	AfterEach(func() {
		db := database.GetGorm()
		Expect(db.Exec(
			`DELETE FROM status.leaf_hub_heartbeats WHERE leaf_hub_name IN ($1, $2, $3)`,
			trustedHub, topicHub, victimHub,
		).Error).To(Succeed(), "failed to clean up identity test heartbeat rows")
	})

	It("accepts status events when CloudEvent source matches the Kafka status topic hub", func() {
		version := eventversion.NewVersion()
		version.Incr()
		evt := ToKafkaStatusCloudEvent(
			fmt.Sprintf("gh-status.%s", trustedHub),
			trustedHub,
			string(enum.HubClusterHeartbeatType),
			version,
			generic.GenericObjectBundle{},
		)

		Expect(producer.SendEvent(ctx, *evt)).To(Succeed(),
			"trusted status event with matching topic and source should publish successfully")

		Eventually(func() error {
			var heartbeat models.LeafHubHeartbeat
			err := database.GetGorm().Where("leaf_hub_name = ?", trustedHub).First(&heartbeat).Error
			if err != nil {
				return fmt.Errorf("expected heartbeat row for trusted hub %q: %w", trustedHub, err)
			}
			return nil
		}, 30*time.Second, 100*time.Millisecond).Should(Succeed(),
			"trusted status event should be persisted for the topic-derived hub name")
	})

	It("drops status events when CloudEvent source does not match the Kafka status topic hub", func() {
		version := eventversion.NewVersion()
		version.Incr()
		evt := ToKafkaStatusCloudEvent(
			fmt.Sprintf("gh-status.%s", topicHub),
			victimHub,
			string(enum.HubClusterHeartbeatType),
			version,
			generic.GenericObjectBundle{},
		)

		Expect(producer.SendEvent(ctx, *evt)).To(Succeed(),
			"transport publish should succeed even when downstream identity validation will reject the event")

		Consistently(func() error {
			db := database.GetGorm()
			for _, hub := range []string{topicHub, victimHub} {
				var heartbeat models.LeafHubHeartbeat
				err := db.Where("leaf_hub_name = ?", hub).First(&heartbeat).Error
				if errors.Is(err, gorm.ErrRecordNotFound) {
					continue
				}
				if err != nil {
					return fmt.Errorf("query heartbeat for hub %q: %w", hub, err)
				}
				return fmt.Errorf("spoofed status event must not create heartbeat row for hub %q", hub)
			}
			return nil
		}, 30*time.Second, 200*time.Millisecond).Should(Succeed(),
			"spoofed status event must be dropped for both topic-derived and claimed hub names")
	})
})

func ToKafkaStatusCloudEvent(kafkaTopic, source, eventType string, version *eventversion.Version, data interface{}) *cloudevents.Event {
	evt := ToCloudEvent(source, eventType, version, data)
	evt.SetExtension(kafka_confluent.KafkaTopicKey, kafkaTopic)
	return evt
}

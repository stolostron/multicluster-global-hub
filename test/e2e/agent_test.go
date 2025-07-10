package tests

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/cloudevents/sdk-go/protocol/kafka_sarama/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/client"
	set "github.com/deckarep/golang-set"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	genericbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	sampleconfig "github.com/stolostron/multicluster-global-hub/samples/config"
)

const (
	STANDALONE_AGENT_NAMESPACE         = "open-cluster-management"
	STANDALONE_AGENT_CONSUMER_GROUP_ID = "standalone-agent-consumer-group"
	STANDALONE_AGENT_TOPIC             = "gh-status.standalone-agent"
)

var _ = Describe("Standalone Agent", Label("e2e-test-agent"), Ordered, func() {
	It("should receive the cluster event from the standalone agent", func() {
		agentCtx, agentCancel := context.WithCancel(ctx)
		defer agentCancel()

		transportSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: STANDALONE_AGENT_NAMESPACE,
				Name:      constants.GHTransportConfigSecret,
			},
		}
		err := globalHubClient.Get(agentCtx, runtimeclient.ObjectKeyFromObject(transportSecret), transportSecret)
		Expect(err).To(Succeed())

		bootstrapServer, saramaConfig, err := sampleconfig.GetSaramaConfigByClient(STANDALONE_AGENT_NAMESPACE, globalHubClient)
		if err != nil {
			log.Fatalf("failed to get sarama config: %v", err)
		}
		// if set this to false, it will consume message from beginning when restart the client,
		// otherwise it will consume message from the last committed offset.
		saramaConfig.Consumer.Offsets.AutoCommit.Enable = true
		saramaConfig.Consumer.Offsets.Initial = sarama.OffsetOldest

		receiver, err := kafka_sarama.NewConsumer([]string{bootstrapServer}, saramaConfig,
			STANDALONE_AGENT_CONSUMER_GROUP_ID, STANDALONE_AGENT_TOPIC)
		Expect(err).To(Succeed())

		c, err := cloudevents.NewClient(receiver, client.WithPollGoroutines(1), client.WithBlockingCallback())
		Expect(err).To(Succeed())

		clusters := set.NewSet()
		var mutex sync.Mutex
		go func() {
			err = c.StartReceiver(agentCtx, func(ctx context.Context, event cloudevents.Event) {
				if event.Type() == string(enum.ManagedClusterType) {
					mutex.Lock()
					defer mutex.Unlock()
					var bundle genericbundle.GenericBundle[clusterv1.ManagedCluster]
					if err := event.DataAs(&bundle); err != nil {
						log.Fatalf("failed to parse the clusters: %s", err)
					}
					for _, cluster := range append(bundle.Create, bundle.Update...) {
						clusters.Add(cluster.Name)
					}
				}
			})
			if err != nil {
				log.Fatalf("failed to start receiver: %s", err)
			}
		}()

		Eventually(func() error {
			cluster := "hub1"
			if !clusters.Contains(cluster) {
				return fmt.Errorf("should receive the cluster: %s from the standalone agent", cluster)
			}
			cluster = "hub2"
			if !clusters.Contains(cluster) {
				return fmt.Errorf("should receive the cluster: %s from the standalone agent", cluster)
			}
			return nil
		}, 3*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())
	})
})

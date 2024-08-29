package tests

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	kafka_confluent "github.com/cloudevents/sdk-go/protocol/kafka_confluent/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/client"
	set "github.com/deckarep/golang-set"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/config"
)

const (
	STANDALONE_NAMESPACE         = "open-cluster-management"
	STANDALONE_CONSUMER_GROUP_ID = "standalone-consumer-group"
	STANDALONE_TOPIC             = "gh-status.standalone"
)

var _ = Describe("Standalone Agent", Label("e2e-test-agent"), Ordered, func() {
	It("should receive the cluster event from the standalone agent", func() {
		agentCtx, agentCancel := context.WithCancel(ctx)
		defer agentCancel()

		transportSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: STANDALONE_NAMESPACE,
				Name:      constants.GHTransportConfigSecret,
			},
		}
		err := globalHubClient.Get(context.Background(), runtimeclient.ObjectKeyFromObject(transportSecret),
			transportSecret)
		Expect(err).To(Succeed())

		configMap, err := config.GetConfluentConfigMapByConfig(transportSecret, globalHubClient,
			STANDALONE_CONSUMER_GROUP_ID)
		Expect(err).To(Succeed())

		receiver, err := kafka_confluent.New(kafka_confluent.WithConfigMap(configMap),
			kafka_confluent.WithReceiverTopics([]string{STANDALONE_TOPIC}))
		Expect(err).To(Succeed())
		defer receiver.Close(ctx)

		c, err := cloudevents.NewClient(receiver, cloudevents.WithTimeNow(), cloudevents.WithUUIDs(),
			client.WithPollGoroutines(1), client.WithBlockingCallback())
		if err != nil {
			log.Fatalf("failed to create client, %v", err)
		}

		clusters := set.NewSet()
		var mutex sync.Mutex
		go func() {
			err = c.StartReceiver(agentCtx, func(ctx context.Context, event cloudevents.Event) {
				if event.Type() == string(enum.ManagedClusterType) {
					mutex.Lock()
					defer mutex.Unlock()
					var data []clusterv1.ManagedCluster
					if err := event.DataAs(&data); err != nil {
						log.Fatalf("failed to parse the clusters: %s", err)
					}
					for _, cluster := range data {
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

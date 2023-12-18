package kafka

import (
	"context"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	KafkaClusterName          = "kafka"
	DefaultGlobalHubKafkaUser = "global-hub-kafka-user"
)

var (
	readyCondition = "Ready"
	trueCondition  = "True"
	bootServer     = "kafka-kafka-bootstrap.multicluster-global-hub.svc:9092"
)

func UpdateKafkaClusterReady(client client.Client, ns string) error {
	statusKafkaCluster := &kafkav1beta2.Kafka{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      KafkaClusterName,
		},
		Status: &kafkav1beta2.KafkaStatus{
			Listeners: []kafkav1beta2.KafkaStatusListenersElem{
				{
					BootstrapServers: &bootServer,
				},
				{
					BootstrapServers: &bootServer,
					Certificates: []string{
						"cert",
					},
				},
			},
			Conditions: []kafkav1beta2.KafkaStatusConditionsElem{
				{
					Type:   &readyCondition,
					Status: &trueCondition,
				},
			},
		},
	}

	err := wait.PollImmediate(1*time.Second, 1*time.Minute, func() (bool, error) {
		existkafkaCluster := &kafkav1beta2.Kafka{}
		err := client.Get(context.Background(), types.NamespacedName{
			Name:      KafkaClusterName,
			Namespace: ns,
		}, existkafkaCluster)
		if err != nil {
			if errors.IsNotFound(err) {
				if e := client.Create(context.Background(), statusKafkaCluster); e != nil {
					klog.Errorf("Failed to create kafka cluster, error: %v", e)
					return false, nil
				}
			}
			klog.Errorf("Failed to get Kafka cluster, error:%v", err)
			return false, nil
		}
		existkafkaCluster.Status = &kafkav1beta2.KafkaStatus{
			Listeners: []kafkav1beta2.KafkaStatusListenersElem{
				{
					BootstrapServers: &bootServer,
				},
				{
					BootstrapServers: &bootServer,
					Certificates: []string{
						"cert",
					},
				},
			},
			Conditions: []kafkav1beta2.KafkaStatusConditionsElem{
				{
					Type:   &readyCondition,
					Status: &trueCondition,
				},
			},
		}
		err = client.Status().Update(context.Background(), existkafkaCluster)
		if err != nil {
			klog.Errorf("Failed to update Kafka cluster, error:%v", err)
			return false, nil
		}

		kafkaGlobalUserSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      DefaultGlobalHubKafkaUser,
			},
			Data: map[string][]byte{
				"user.crt": []byte("usercrt"),
				"user.key": []byte("userkey"),
			},
		}
		err = client.Get(context.Background(), types.NamespacedName{
			Name:      kafkaGlobalUserSecret.Name,
			Namespace: ns,
		}, kafkaGlobalUserSecret)

		if errors.IsNotFound(err) {
			e := client.Create(context.Background(), kafkaGlobalUserSecret)
			if e != nil {
				klog.Errorf("Failed to create Kafka secret, error:%v", e)
				return false, nil
			}
		} else if err != nil {
			klog.Errorf("Failed to get Kafka secret, error:%v", err)
			return false, nil
		}
		return true, nil
	})

	return err
}

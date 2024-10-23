package protocol

import (
	"context"
	"reflect"
	"testing"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	operatorv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	constants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var (
	mghName      = "mgh"
	mghNamespace = "default"
	now          = time.Now()
	reason       = "KafkaNotReady"
	message      = "Kafka cluster is not ready"
)

func TestNewStrimziTransporter(t *testing.T) {
	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mgh",
			Namespace: utils.GetDefaultNamespace(),
			Annotations: map[string]string{
				constants.CommunityCatalogSourceNameKey:      "test",
				constants.CommunityCatalogSourceNamespaceKey: "default",
			},
		},
		Spec: v1alpha4.MulticlusterGlobalHubSpec{
			DataLayerSpec: v1alpha4.DataLayerSpec{
				Postgres: v1alpha4.PostgresSpec{
					Retention: "2y",
				},
			},
		},
	}

	trans := NewStrimziTransporter(
		nil,
		mgh,
		WithCommunity(true),
		WithNamespacedName(types.NamespacedName{
			Name:      KafkaClusterName,
			Namespace: mgh.Namespace,
		}),
	)

	if trans.subCatalogSourceName != "test" {
		t.Errorf("catalogSource name should be test, but %v", trans.subCatalogSourceName)
	}

	if trans.subCatalogSourceNamespace != "default" {
		t.Errorf("catalogSource name should be default, but %v", trans.subCatalogSourceNamespace)
	}
}

func Test_strimziTransporter_updateMghKafkaComponent(t *testing.T) {
	tests := []struct {
		name          string
		mgh           *operatorv1alpha4.MulticlusterGlobalHub
		ready         bool
		kafkaReason   string
		kafkaMsg      string
		desiredStatus v1alpha4.StatusCondition
	}{
		{
			name: "kafka ready",
			mgh: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mghName,
					Namespace: mghNamespace,
				},
			},
			ready: true,
			desiredStatus: v1alpha4.StatusCondition{
				Kind:    "Kafka",
				Name:    config.COMPONENTS_KAFKA_NAME,
				Type:    config.COMPONENTS_AVAILABLE,
				Status:  config.CONDITION_STATUS_TRUE,
				Reason:  config.CONDITION_REASON_KAFKA_READY,
				Message: config.CONDITION_MESSAGE_KAFKA_READY,
			},
		},
		{
			name: "kafka not ready",
			mgh: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mghName,
					Namespace: mghNamespace,
				},
			},
			ready: false,
			desiredStatus: v1alpha4.StatusCondition{
				Kind:    "Kafka",
				Name:    config.COMPONENTS_KAFKA_NAME,
				Type:    config.COMPONENTS_AVAILABLE,
				Status:  config.CONDITION_STATUS_FALSE,
				Reason:  reason,
				Message: message,
			},
		},
		{
			name: "kafka not ready, mgh status not nil",
			mgh: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mghName,
					Namespace: mghNamespace,
				},
				Status: operatorv1alpha4.MulticlusterGlobalHubStatus{
					Components: map[string]operatorv1alpha4.StatusCondition{
						config.COMPONENTS_KAFKA_NAME: v1alpha4.StatusCondition{
							Kind:               "Kafka",
							Name:               config.COMPONENTS_KAFKA_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_FALSE,
							Reason:             reason,
							Message:            message,
							LastTransitionTime: metav1.Time{Time: now},
						},
					},
				},
			},
			ready: false,
			desiredStatus: v1alpha4.StatusCondition{
				Kind:    "Kafka",
				Name:    config.COMPONENTS_KAFKA_NAME,
				Type:    config.COMPONENTS_AVAILABLE,
				Status:  config.CONDITION_STATUS_FALSE,
				Reason:  reason,
				Message: message,
			},
		},
		{
			name: "kafka not ready, mgh status not nil",
			mgh: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mghName,
					Namespace: mghNamespace,
				},
				Status: operatorv1alpha4.MulticlusterGlobalHubStatus{
					Components: map[string]operatorv1alpha4.StatusCondition{
						config.COMPONENTS_KAFKA_NAME: v1alpha4.StatusCondition{
							Kind:               "Kafka",
							Name:               config.COMPONENTS_KAFKA_NAME,
							Type:               config.COMPONENTS_AVAILABLE,
							Status:             config.CONDITION_STATUS_FALSE,
							Reason:             reason,
							Message:            message,
							LastTransitionTime: metav1.Time{Time: now},
						},
					},
				},
			},
			kafkaReason: "kafka error reason",
			kafkaMsg:    "kafka error msg",
			ready:       false,
			desiredStatus: v1alpha4.StatusCondition{
				Kind:    "Kafka",
				Name:    config.COMPONENTS_KAFKA_NAME,
				Type:    config.COMPONENTS_AVAILABLE,
				Status:  config.CONDITION_STATUS_FALSE,
				Reason:  "kafka error reason",
				Message: "kafka error msg",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v1alpha4.AddToScheme(scheme.Scheme)
			apiextensionsv1.AddToScheme(scheme.Scheme)
			kafkav1beta2.AddToScheme(scheme.Scheme)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithStatusSubresource(&v1alpha4.MulticlusterGlobalHub{}).WithRuntimeObjects(tt.mgh).Build()

			k := &strimziTransporter{
				ctx: context.Background(),
				mgh: tt.mgh,
			}

			if err := k.updateMghKafkaComponent(tt.ready, fakeClient, tt.kafkaReason, tt.kafkaMsg); err != nil {
				t.Errorf("strimziTransporter.updateMghKafkaComponent() error = %v", err)
			}
			curmgh := &v1alpha4.MulticlusterGlobalHub{}
			err := fakeClient.Get(k.ctx, types.NamespacedName{
				Name:      mghName,
				Namespace: mghNamespace,
			}, curmgh)
			if err != nil {
				t.Errorf("failed to get mgh. error = %v", err)
			}
			tt.desiredStatus.LastTransitionTime = curmgh.Status.Components[config.COMPONENTS_KAFKA_NAME].LastTransitionTime
			if !reflect.DeepEqual(curmgh.Status.Components[config.COMPONENTS_KAFKA_NAME], tt.desiredStatus) {
				t.Errorf("mgh status is not right. mgh : %v, desired : %v",
					curmgh.Status.Components[config.COMPONENTS_KAFKA_NAME], tt.desiredStatus)
			}
		})
	}
}

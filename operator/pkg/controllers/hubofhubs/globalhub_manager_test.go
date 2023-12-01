package hubofhubs

import (
	"context"
	"testing"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/kafka"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/postgres"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakekube "k8s.io/client-go/kubernetes/fake"
)

func TestRestartManagerPod(t *testing.T) {
	ctx := context.Background()
	configNamespace := config.GetDefaultNamespace()

	tests := []struct {
		name        string
		initObjects []runtime.Object
		wantErr     bool
	}{
		{
			name:        "no manager pods",
			initObjects: []runtime.Object{},
			wantErr:     false,
		},
		{
			name: "has manager pods",
			initObjects: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: configNamespace,
						Name:      constants.ManagerDeploymentName + "xxx",
						Labels: map[string]string{
							"name": constants.ManagerDeploymentName,
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		kubeClient := fakekube.NewSimpleClientset(tt.initObjects...)
		t.Run(tt.name, func(t *testing.T) {
			if err := restartManagerPod(ctx, kubeClient); (err != nil) != tt.wantErr {
				t.Errorf("RestartmanagerPod() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_isMiddleWareChanged(t *testing.T) {

	tests := []struct {
		name                 string
		curMiddlewareConfig  *MiddlewareConfig
		pgConnectionCache    *postgres.PostgresConnection
		kafkaConnectionCache *kafka.KafkaConnection
		want                 bool
	}{
		{
			name:                "all nil",
			curMiddlewareConfig: nil,
			want:                false,
		},
		{
			name: "first start",
			curMiddlewareConfig: &MiddlewareConfig{
				PgConnection: &postgres.PostgresConnection{
					SuperuserDatabaseURI: "http:pgurl",
				},
			},
			want: false,
		},
		{
			name: "no change",
			curMiddlewareConfig: &MiddlewareConfig{
				PgConnection: &postgres.PostgresConnection{
					SuperuserDatabaseURI: "http:pgurl",
				},
				KafkaConnection: &kafka.KafkaConnection{
					BootstrapServer: "http:bs",
				},
			},
			pgConnectionCache: &postgres.PostgresConnection{
				SuperuserDatabaseURI: "http:pgurl",
			},
			kafkaConnectionCache: &kafka.KafkaConnection{
				BootstrapServer: "http:bs",
			},
			want: false,
		},
		{
			name: "pg conn change",
			curMiddlewareConfig: &MiddlewareConfig{
				PgConnection: &postgres.PostgresConnection{
					SuperuserDatabaseURI: "http:pgurl",
				},
				KafkaConnection: &kafka.KafkaConnection{
					BootstrapServer: "http:bs",
				},
			},
			pgConnectionCache: &postgres.PostgresConnection{
				SuperuserDatabaseURI: "http:pgurl-changed",
			},
			kafkaConnectionCache: &kafka.KafkaConnection{
				BootstrapServer: "http:bs",
			},
			want: true,
		},
		{
			name: "kafka conn change",
			curMiddlewareConfig: &MiddlewareConfig{
				PgConnection: &postgres.PostgresConnection{
					SuperuserDatabaseURI: "http:pgurl",
				},
				KafkaConnection: &kafka.KafkaConnection{
					BootstrapServer: "http:bs",
				},
			},
			pgConnectionCache: &postgres.PostgresConnection{
				SuperuserDatabaseURI: "http:pgurl",
			},
			kafkaConnectionCache: &kafka.KafkaConnection{
				BootstrapServer: "http:bs-change",
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pgConnectionCache = tt.pgConnectionCache
			kafkaConnectionCache = tt.kafkaConnectionCache
			if got := isMiddlewareChanged(tt.curMiddlewareConfig); got != tt.want {
				t.Errorf("isMiddleWareChanged() = %v, want %v", got, tt.want)
			}
		})
	}
}

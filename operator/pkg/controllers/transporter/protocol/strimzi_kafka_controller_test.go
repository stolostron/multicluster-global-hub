// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package protocol

import (
	"context"
	"strings"
	"testing"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func TestMulticlusterGlobalHubReconcilerStrimziResources(t *testing.T) {
	tests := []struct {
		name         string
		initObjects  []runtime.Object
		wantErr      bool
		requeueAfter time.Duration
	}{
		{
			name: "remove kafka resources",
			initObjects: []runtime.Object{
				&kafkav1beta2.Kafka{
					ObjectMeta: metav1.ObjectMeta{
						Name:      KafkaClusterName,
						Namespace: utils.GetDefaultNamespace(),
					},
				},
				&kafkav1beta2.KafkaUser{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kafkauser",
						Namespace: utils.GetDefaultNamespace(),
						Labels: map[string]string{
							constants.GlobalHubOwnerLabelKey: "global-hub",
						},
					},
				},
				&kafkav1beta2.KafkaTopic{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kafkatopic",
						Namespace: utils.GetDefaultNamespace(),
						Labels: map[string]string{
							constants.GlobalHubOwnerLabelKey: "global-hub",
						},
					},
				},
			},
		},
		{
			name: "remove kafka topics which has finalizer",
			initObjects: []runtime.Object{
				&kafkav1beta2.KafkaTopic{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kafkatopic",
						Namespace: utils.GetDefaultNamespace(),
						Finalizers: []string{
							"test-final",
						},
						Labels: map[string]string{
							constants.GlobalHubOwnerLabelKey: "global-hub",
						},
					},
				},
			},
			wantErr:      false,
			requeueAfter: 5 * time.Second,
		},
		{
			name: "remove subscription and csv",
			initObjects: []runtime.Object{
				&subv1alpha1.Subscription{
					ObjectMeta: metav1.ObjectMeta{
						Name:      DefaultKafkaSubName,
						Namespace: utils.GetDefaultNamespace(),
					},
					Status: subv1alpha1.SubscriptionStatus{
						InstalledCSV: "kafka-0.40.0",
					},
				},
				&subv1alpha1.ClusterServiceVersion{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kafka-0.40.0",
						Namespace: utils.GetDefaultNamespace(),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			kafkav1beta2.AddToScheme(scheme.Scheme)
			subv1alpha1.AddToScheme(scheme.Scheme)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.initObjects...).Build()
			kc := KafkaController{
				c: fakeClient,
				trans: &strimziTransporter{
					subName:          DefaultKafkaSubName,
					kafkaClusterName: "kafka",
				},
			}
			returnResult, err := kc.pruneStrimziResources(ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("Case:%v, MulticlusterGlobalHubReconciler.pruneStrimziResources() error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
			if returnResult.RequeueAfter != tt.requeueAfter {
				t.Errorf("Case:%v, MulticlusterGlobalHubReconciler.pruneStrimziResources() needRequeue = %v, wantRequeue %v", tt.name, returnResult.RequeueAfter, tt.requeueAfter)
			}
		})
	}
}

func TestSyncManagerTransportConn(t *testing.T) {
	tests := []struct {
		name        string
		transporter transport.Transporter
		wantRequeue bool
		wantErr     bool
	}{
		{
			name:        "non-strimzi transporter returns immediately without error",
			transporter: &mockTransporter{},
			wantRequeue: false,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			needRequeue, err := SyncManagerTransportConn(tt.transporter)
			if (err != nil) != tt.wantErr {
				t.Errorf("SyncManagerTransportConn() error = %v, wantErr %v", err, tt.wantErr)
			}
			if needRequeue != tt.wantRequeue {
				t.Errorf("SyncManagerTransportConn() needRequeue = %v, want %v", needRequeue, tt.wantRequeue)
			}
		})
	}
}

func TestSyncManagerTransportConnKafkaNotReady(t *testing.T) {
	resetTransportConnAfterTest(t)

	ctx := context.Background()
	ns := utils.GetDefaultNamespace()
	testScheme := newSyncManagerTransportConnScheme(t)
	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "globalhub", Namespace: ns},
	}
	notReadyType := "Ready"
	notReadyStatus := "False"
	notReadyReason := "NotReady"
	notReadyMessage := "Kafka cluster not ready"
	notReadyKafka := &kafkav1beta2.Kafka{
		ObjectMeta: metav1.ObjectMeta{Name: KafkaClusterName, Namespace: ns},
		Spec: &kafkav1beta2.KafkaSpec{
			Kafka: kafkav1beta2.KafkaSpecKafka{
				Listeners: []kafkav1beta2.KafkaSpecKafkaListenersElem{
					{Name: "tls", Port: 9093, Type: "nodeport", Tls: true},
				},
			},
		},
		Status: &kafkav1beta2.KafkaStatus{
			Conditions: []kafkav1beta2.KafkaStatusConditionsElem{
				{
					Type:    &notReadyType,
					Status:  &notReadyStatus,
					Reason:  &notReadyReason,
					Message: &notReadyMessage,
				},
			},
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(mgh, notReadyKafka).Build()
	trans := &strimziTransporter{
		ctx:                   ctx,
		mgh:                   mgh,
		kafkaClusterName:      KafkaClusterName,
		kafkaClusterNamespace: ns,
		manager:               &fakeManager{c: fakeClient},
	}

	needRequeue, err := SyncManagerTransportConn(trans)
	if err != nil {
		t.Fatalf("SyncManagerTransportConn() error = %v", err)
	}
	if !needRequeue {
		t.Fatal("SyncManagerTransportConn() should requeue when Kafka is not ready")
	}
}

func TestSyncManagerTransportConnMissingKafkaCluster(t *testing.T) {
	resetTransportConnAfterTest(t)

	ctx := context.Background()
	ns := utils.GetDefaultNamespace()
	testScheme := newSyncManagerTransportConnScheme(t)
	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "globalhub", Namespace: ns},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(mgh).Build()
	trans := &strimziTransporter{
		ctx:                   ctx,
		mgh:                   mgh,
		kafkaClusterName:      KafkaClusterName,
		kafkaClusterNamespace: ns,
		manager:               &fakeManager{c: fakeClient},
	}

	needRequeue, err := SyncManagerTransportConn(trans)
	if err != nil {
		t.Fatalf("SyncManagerTransportConn() error = %v", err)
	}
	if !needRequeue {
		t.Fatal("SyncManagerTransportConn() should requeue when Kafka cluster is missing")
	}
}

func TestSyncManagerTransportConnMissingUserCredential(t *testing.T) {
	resetTransportConnAfterTest(t)

	ctx := context.Background()
	ns := utils.GetDefaultNamespace()
	testScheme := newSyncManagerTransportConnScheme(t)
	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "globalhub", Namespace: ns},
	}
	bootServer := "kafka-kafka-bootstrap.example.svc:9092"
	kafkaCluster := readyKafkaCluster(ns, bootServer, "Ready", "True")
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(mgh, kafkaCluster).Build()
	trans := &strimziTransporter{
		ctx:                   ctx,
		mgh:                   mgh,
		kafkaClusterName:      KafkaClusterName,
		kafkaClusterNamespace: ns,
		manager:               &fakeManager{c: fakeClient},
	}

	_, err := SyncManagerTransportConn(trans)
	if err == nil {
		t.Fatal("SyncManagerTransportConn() expected error when global-hub user credential is missing")
	}
	if !strings.Contains(err.Error(), "failed to get manager transport connection") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSyncManagerTransportConnSuccess(t *testing.T) {
	resetTransportConnAfterTest(t)

	ctx := context.Background()
	ns := utils.GetDefaultNamespace()
	testScheme := newSyncManagerTransportConnScheme(t)
	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "globalhub", Namespace: ns},
		Spec: v1alpha4.MulticlusterGlobalHubSpec{
			DataLayerSpec: v1alpha4.DataLayerSpec{
				Kafka: v1alpha4.KafkaSpec{
					KafkaTopics: v1alpha4.KafkaTopics{
						SpecTopic:   "spec-custom",
						StatusTopic: "gh-status-*",
					},
				},
			},
		},
	}
	configClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(mgh).Build()
	if err := config.SetTransportConfig(ctx, configClient, mgh); err != nil {
		t.Fatalf("SetTransportConfig() error = %v", err)
	}

	bootServer := "kafka-kafka-bootstrap.example.svc:9092"
	kafkaCluster := readyKafkaCluster(ns, bootServer, "Ready", "True")
	kafkaUserSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: DefaultGlobalHubKafkaUserName, Namespace: ns},
		Data: map[string][]byte{
			"user.crt": []byte("usercrt"),
			"user.key": []byte("userkey"),
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(mgh, kafkaCluster, kafkaUserSecret).Build()
	trans := &strimziTransporter{
		ctx:                   ctx,
		mgh:                   mgh,
		kafkaClusterName:      KafkaClusterName,
		kafkaClusterNamespace: ns,
		manager:               &fakeManager{c: fakeClient},
	}

	needRequeue, err := SyncManagerTransportConn(trans)
	if err != nil {
		t.Fatalf("SyncManagerTransportConn() error = %v", err)
	}
	if needRequeue {
		t.Fatal("SyncManagerTransportConn() should not requeue when transport connection is ready")
	}

	conn := config.GetTransporterConn()
	if conn == nil {
		t.Fatal("expected transport connection to be set")
	}
	if conn.SpecTopic != "spec-custom" {
		t.Fatalf("SpecTopic = %q, want %q", conn.SpecTopic, "spec-custom")
	}
	if conn.StatusTopic != config.ManagerStatusTopic() {
		t.Fatalf("StatusTopic = %q, want %q", conn.StatusTopic, config.ManagerStatusTopic())
	}
	if conn.BootstrapServer != bootServer {
		t.Fatalf("BootstrapServer = %q, want %q", conn.BootstrapServer, bootServer)
	}
}

func newSyncManagerTransportConnScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	testScheme := runtime.NewScheme()
	if err := v1alpha4.AddToScheme(testScheme); err != nil {
		t.Fatalf("v1alpha4.AddToScheme() error = %v", err)
	}
	if err := corev1.AddToScheme(testScheme); err != nil {
		t.Fatalf("corev1.AddToScheme() error = %v", err)
	}
	if err := kafkav1beta2.AddToScheme(testScheme); err != nil {
		t.Fatalf("kafkav1beta2.AddToScheme() error = %v", err)
	}
	return testScheme
}

func readyKafkaCluster(ns, bootServer, readyType, readyStatus string) *kafkav1beta2.Kafka {
	clusterID := "test-cluster-id"
	return &kafkav1beta2.Kafka{
		ObjectMeta: metav1.ObjectMeta{Name: KafkaClusterName, Namespace: ns},
		Spec: &kafkav1beta2.KafkaSpec{
			Kafka: kafkav1beta2.KafkaSpecKafka{
				Listeners: []kafkav1beta2.KafkaSpecKafkaListenersElem{
					{Name: "tls", Port: 9093, Type: "nodeport", Tls: true},
				},
			},
		},
		Status: &kafkav1beta2.KafkaStatus{
			ClusterId: &clusterID,
			Listeners: []kafkav1beta2.KafkaStatusListenersElem{
				{
					BootstrapServers: &bootServer,
					Certificates:     []string{"cert"},
				},
			},
			Conditions: []kafkav1beta2.KafkaStatusConditionsElem{
				{Type: &readyType, Status: &readyStatus},
			},
		},
	}
}

func resetTransportConnAfterTest(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		config.SetTransporterConn(nil)
		config.SetTransporter(nil)
	})
}

type fakeManager struct {
	ctrl.Manager
	c client.Client
}

func (f *fakeManager) GetClient() client.Client { return f.c }

// mockTransporter implements transport.Transporter for testing.
type mockTransporter struct{}

func (m *mockTransporter) EnsureUser(clusterName string) (string, error) {
	return "", nil
}

func (m *mockTransporter) EnsureTopic(clusterName string) (*transport.ClusterTopic, error) {
	return nil, nil
}

func (m *mockTransporter) GetConnCredential(clusterName string) (*transport.KafkaConfig, error) {
	return nil, nil
}

func (m *mockTransporter) EnsureKafka() (bool, error) {
	return false, nil
}

func (m *mockTransporter) Prune(clusterName string) error {
	return nil
}

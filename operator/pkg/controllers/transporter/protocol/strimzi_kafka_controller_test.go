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
	ocv1 "github.com/operator-framework/operator-controller/api/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	operatorv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func restoreTransportConfigAfterTest(t *testing.T) {
	t.Helper()
	snapshot := config.SnapshotTransportConfigForTest()
	t.Cleanup(func() {
		config.RestoreTransportConfigForTest(snapshot)
	})
}

func TestApplyManagerKafkaTopics(t *testing.T) {
	restoreTransportConfigAfterTest(t)
	ctx := context.Background()
	testScheme := runtime.NewScheme()
	_ = operatorv1alpha4.AddToScheme(testScheme)
	_ = corev1.AddToScheme(testScheme)

	mgh := &operatorv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "globalhub",
			Namespace: utils.GetDefaultNamespace(),
		},
		Spec: operatorv1alpha4.MulticlusterGlobalHubSpec{
			DataLayerSpec: operatorv1alpha4.DataLayerSpec{
				Kafka: operatorv1alpha4.KafkaSpec{
					KafkaTopics: operatorv1alpha4.KafkaTopics{
						SpecTopic:   "spec1",
						StatusTopic: "gh-status.*",
					},
				},
			},
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(mgh).Build()
	if err := config.SetTransportConfig(ctx, fakeClient, mgh); err != nil {
		t.Fatalf("SetTransportConfig() error = %v", err)
	}

	conn := &transport.KafkaConfig{
		SpecTopic:   "gh-spec",
		StatusTopic: "gh-spec",
	}
	applyManagerKafkaTopics(conn)

	if conn.SpecTopic != "spec1" {
		t.Fatalf("SpecTopic = %q, want %q", conn.SpecTopic, "spec1")
	}
	if conn.StatusTopic != "^gh-status.*" {
		t.Fatalf("StatusTopic = %q, want %q", conn.StatusTopic, "^gh-status.*")
	}
}

func TestCreateManagerTransportSecretAppliesCustomSpecTopic(t *testing.T) {
	restoreTransportConfigAfterTest(t)
	ctx := context.Background()
	testScheme := runtime.NewScheme()
	_ = operatorv1alpha4.AddToScheme(testScheme)
	_ = corev1.AddToScheme(testScheme)
	_ = kafkav1beta2.AddToScheme(testScheme)

	ns := utils.GetDefaultNamespace()
	mgh := &operatorv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "globalhub",
			Namespace: ns,
		},
		Spec: operatorv1alpha4.MulticlusterGlobalHubSpec{
			DataLayerSpec: operatorv1alpha4.DataLayerSpec{
				Kafka: operatorv1alpha4.KafkaSpec{
					KafkaTopics: operatorv1alpha4.KafkaTopics{
						SpecTopic:   "spec1",
						StatusTopic: "gh-status.*",
					},
				},
			},
		},
	}
	readyCondition := "Ready"
	trueCondition := "True"
	bootServer := "kafka-kafka-bootstrap.example.svc:9092"
	kafkaCluster := readyKafkaCluster(ns, bootServer, readyCondition, trueCondition)
	kafkaUserSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: DefaultGlobalHubKafkaUserName, Namespace: ns},
		Data: map[string][]byte{
			"user.crt": []byte("usercrt"),
			"user.key": []byte("userkey"),
		},
	}
	clientCAKeySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: KafkaClusterName + "-clients-ca", Namespace: ns},
		Data:       map[string][]byte{"ca.key": []byte("cakey")},
	}
	clientCACertSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: KafkaClusterName + "-clients-ca-cert", Namespace: ns},
		Data:       map[string][]byte{"ca.crt": []byte("cacert")},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(
		mgh, kafkaCluster, kafkaUserSecret, clientCAKeySecret, clientCACertSecret,
	).Build()
	if err := config.SetTransportConfig(ctx, fakeClient, mgh); err != nil {
		t.Fatalf("SetTransportConfig() error = %v", err)
	}

	trans := &strimziTransporter{
		ctx:                   ctx,
		mgh:                   mgh,
		kafkaClusterName:      KafkaClusterName,
		kafkaClusterNamespace: ns,
		manager:               &fakeManager{c: fakeClient},
	}
	if err := SyncManagerTransportConfigSecret(ctx, mgh, trans, fakeClient); err != nil {
		t.Fatalf("SyncManagerTransportConfigSecret() error = %v", err)
	}

	secret := &corev1.Secret{}
	if err := fakeClient.Get(ctx, types.NamespacedName{
		Name:      constants.GHTransportConfigSecret,
		Namespace: ns,
	}, secret); err != nil {
		t.Fatalf("Get transport-config secret error = %v", err)
	}
	kafkaYAML := string(secret.Data["kafka.yaml"])
	if !strings.Contains(kafkaYAML, "topic.spec: spec1") {
		t.Fatalf("kafka.yaml missing customized spec topic: %s", kafkaYAML)
	}
	if !strings.Contains(kafkaYAML, "topic.status: ^gh-status.*") {
		t.Fatalf("kafka.yaml missing customized status topic: %s", kafkaYAML)
	}
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

func TestKafkaControllerRefreshUsesActiveStrimziTransporter(t *testing.T) {
	restoreTransportConfigAfterTest(t)

	ns := utils.GetDefaultNamespace()
	mgh1 := &operatorv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "globalhub", Namespace: ns},
		Spec: operatorv1alpha4.MulticlusterGlobalHubSpec{
			DataLayerSpec: operatorv1alpha4.DataLayerSpec{
				Kafka: operatorv1alpha4.KafkaSpec{
					KafkaTopics: operatorv1alpha4.KafkaTopics{SpecTopic: "spec1"},
				},
			},
		},
	}
	mgh2 := mgh1.DeepCopy()
	mgh2.Spec.DataLayerSpec.Kafka.KafkaTopics.SpecTopic = "spec2"

	trans1 := NewStrimziTransporter(nil, mgh1)
	kc := &KafkaController{trans: trans1}

	trans2 := NewStrimziTransporter(nil, mgh2)
	if trans2 == trans1 {
		t.Fatal("expected distinct Strimzi transporter instances")
	}

	if err := kc.refreshStrimziTransporter(); err != nil {
		t.Fatalf("refreshStrimziTransporter() error = %v", err)
	}
	if kc.trans != trans2 {
		t.Fatal("KafkaController should reconcile with the active Strimzi transporter")
	}
	if got := kc.trans.mgh.Spec.DataLayerSpec.Kafka.KafkaTopics.SpecTopic; got != "spec2" {
		t.Fatalf("SpecTopic = %q, want spec2", got)
	}
}

func TestActiveStrimziTransporter(t *testing.T) {
	restoreTransportConfigAfterTest(t)
	config.SetTransporter(nil)

	if _, err := activeStrimziTransporter(); err == nil {
		t.Fatal("activeStrimziTransporter() expected error when transporter is nil")
	}

	config.SetTransporter(&stubBYOTransporter{})
	if _, err := activeStrimziTransporter(); err == nil {
		t.Fatal("activeStrimziTransporter() expected error for non-Strimzi transporter")
	}

	mgh := &operatorv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "globalhub", Namespace: utils.GetDefaultNamespace()},
	}
	st := NewStrimziTransporter(nil, mgh)
	got, err := activeStrimziTransporter()
	if err != nil {
		t.Fatalf("activeStrimziTransporter() error = %v", err)
	}
	if got != st {
		t.Fatal("activeStrimziTransporter() should return the registered Strimzi transporter")
	}
}

type stubBYOTransporter struct{}

func (s *stubBYOTransporter) EnsureUser(string) (string, error) { return "", nil }

func (s *stubBYOTransporter) EnsureTopic(string) (*transport.ClusterTopic, error) {
	return &transport.ClusterTopic{}, nil
}

func (s *stubBYOTransporter) EnsureKafka() (bool, error) { return false, nil }

func (s *stubBYOTransporter) Prune(string) error { return nil }

func (s *stubBYOTransporter) GetConnCredential(string) (*transport.KafkaConfig, error) {
	return &transport.KafkaConfig{}, nil
}

func TestApplyManagerKafkaTopicsNilConn(t *testing.T) {
	applyManagerKafkaTopics(nil)
}

func TestSyncManagerTransportSecretIfReadyKafkaNotReady(t *testing.T) {
	restoreTransportConfigAfterTest(t)
	ctx := context.Background()
	testScheme := runtime.NewScheme()
	_ = operatorv1alpha4.AddToScheme(testScheme)
	_ = kafkav1beta2.AddToScheme(testScheme)

	ns := utils.GetDefaultNamespace()
	mgh := &operatorv1alpha4.MulticlusterGlobalHub{
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

	synced, err := syncManagerTransportSecretIfReady(ctx, mgh, trans, fakeClient)
	if err != nil {
		t.Fatalf("syncManagerTransportSecretIfReady() error = %v", err)
	}
	if synced {
		t.Fatal("syncManagerTransportSecretIfReady() should not sync when Kafka is not ready")
	}
}

func TestSyncManagerTransportSecretIfReadyRejectsIncompleteCA(t *testing.T) {
	restoreTransportConfigAfterTest(t)
	ctx := context.Background()
	testScheme := runtime.NewScheme()
	_ = operatorv1alpha4.AddToScheme(testScheme)
	_ = corev1.AddToScheme(testScheme)
	_ = kafkav1beta2.AddToScheme(testScheme)

	ns := utils.GetDefaultNamespace()
	mgh := &operatorv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "globalhub", Namespace: ns},
	}
	bootServer := "kafka-kafka-bootstrap.example.svc:9092"
	kafkaCluster := readyKafkaCluster(ns, bootServer, "Ready", "True")
	clientCAKeySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: KafkaClusterName + "-clients-ca", Namespace: ns},
		Data:       map[string][]byte{},
	}
	clientCACertSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: KafkaClusterName + "-clients-ca-cert", Namespace: ns},
		Data:       map[string][]byte{"ca.crt": []byte("cacert")},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(
		mgh, kafkaCluster, clientCAKeySecret, clientCACertSecret,
	).Build()
	trans := &strimziTransporter{
		ctx:                   ctx,
		mgh:                   mgh,
		kafkaClusterName:      KafkaClusterName,
		kafkaClusterNamespace: ns,
		manager:               &fakeManager{c: fakeClient},
	}

	_, err := syncManagerTransportSecretIfReady(ctx, mgh, trans, fakeClient)
	if err == nil {
		t.Fatal("syncManagerTransportSecretIfReady() expected error for incomplete CA secrets")
	}
	if !strings.Contains(err.Error(), "set Kafka client CA") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSyncManagerTransportSecretIfReadyWaitsForUserCredential(t *testing.T) {
	restoreTransportConfigAfterTest(t)
	ctx := context.Background()
	testScheme := runtime.NewScheme()
	_ = operatorv1alpha4.AddToScheme(testScheme)
	_ = corev1.AddToScheme(testScheme)
	_ = kafkav1beta2.AddToScheme(testScheme)

	ns := utils.GetDefaultNamespace()
	mgh := &operatorv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "globalhub", Namespace: ns},
	}
	bootServer := "kafka-kafka-bootstrap.example.svc:9092"
	kafkaCluster := readyKafkaCluster(ns, bootServer, "Ready", "True")
	clientCAKeySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: KafkaClusterName + "-clients-ca", Namespace: ns},
		Data:       map[string][]byte{"ca.key": []byte("cakey")},
	}
	clientCACertSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: KafkaClusterName + "-clients-ca-cert", Namespace: ns},
		Data:       map[string][]byte{"ca.crt": []byte("cacert")},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(
		mgh, kafkaCluster, clientCAKeySecret, clientCACertSecret,
	).Build()
	trans := &strimziTransporter{
		ctx:                   ctx,
		mgh:                   mgh,
		kafkaClusterName:      KafkaClusterName,
		kafkaClusterNamespace: ns,
		manager:               &fakeManager{c: fakeClient},
	}

	synced, err := syncManagerTransportSecretIfReady(ctx, mgh, trans, fakeClient)
	if err != nil {
		t.Fatalf("syncManagerTransportSecretIfReady() error = %v", err)
	}
	if synced {
		t.Fatal("syncManagerTransportSecretIfReady() should wait when global-hub user credential is missing")
	}
}

func TestCreateManagerTransportSecretUpdatesExistingTopics(t *testing.T) {
	restoreTransportConfigAfterTest(t)
	ctx := context.Background()
	testScheme := runtime.NewScheme()
	_ = operatorv1alpha4.AddToScheme(testScheme)
	_ = corev1.AddToScheme(testScheme)
	_ = kafkav1beta2.AddToScheme(testScheme)

	ns := utils.GetDefaultNamespace()
	mgh := &operatorv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "globalhub", Namespace: ns},
		Spec: operatorv1alpha4.MulticlusterGlobalHubSpec{
			DataLayerSpec: operatorv1alpha4.DataLayerSpec{
				Kafka: operatorv1alpha4.KafkaSpec{
					KafkaTopics: operatorv1alpha4.KafkaTopics{
						SpecTopic:   "spec-old",
						StatusTopic: "gh-status.*",
					},
				},
			},
		},
	}
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.GHTransportConfigSecret,
			Namespace: ns,
		},
		Data: map[string][]byte{"kafka.yaml": []byte("topic.spec: spec-old\n")},
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
	clientCAKeySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: KafkaClusterName + "-clients-ca", Namespace: ns},
		Data:       map[string][]byte{"ca.key": []byte("cakey")},
	}
	clientCACertSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: KafkaClusterName + "-clients-ca-cert", Namespace: ns},
		Data:       map[string][]byte{"ca.crt": []byte("cacert")},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(
		mgh, existingSecret, kafkaCluster, kafkaUserSecret, clientCAKeySecret, clientCACertSecret,
	).Build()
	if err := config.SetTransportConfig(ctx, fakeClient, mgh); err != nil {
		t.Fatalf("SetTransportConfig() error = %v", err)
	}

	mgh.Spec.DataLayerSpec.Kafka.KafkaTopics.SpecTopic = "spec-new"
	if err := config.SetTransportConfig(ctx, fakeClient, mgh); err != nil {
		t.Fatalf("SetTransportConfig() error = %v", err)
	}

	trans := &strimziTransporter{
		ctx:                   ctx,
		mgh:                   mgh,
		kafkaClusterName:      KafkaClusterName,
		kafkaClusterNamespace: ns,
		manager:               &fakeManager{c: fakeClient},
	}
	if err := SyncManagerTransportConfigSecret(ctx, mgh, trans, fakeClient); err != nil {
		t.Fatalf("SyncManagerTransportConfigSecret() error = %v", err)
	}

	secret := &corev1.Secret{}
	if err := fakeClient.Get(ctx, types.NamespacedName{
		Name:      constants.GHTransportConfigSecret,
		Namespace: ns,
	}, secret); err != nil {
		t.Fatalf("Get transport-config secret error = %v", err)
	}
	kafkaYAML := string(secret.Data["kafka.yaml"])
	if !strings.Contains(kafkaYAML, "topic.spec: spec-new") {
		t.Fatalf("kafka.yaml should reflect updated spec topic: %s", kafkaYAML)
	}
}

func TestGetManagerTransportConn(t *testing.T) {
	restoreTransportConfigAfterTest(t)
	ctx := context.Background()
	testScheme := runtime.NewScheme()
	_ = operatorv1alpha4.AddToScheme(testScheme)
	_ = corev1.AddToScheme(testScheme)
	_ = kafkav1beta2.AddToScheme(testScheme)

	ns := utils.GetDefaultNamespace()
	mgh := &operatorv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "globalhub", Namespace: ns},
		Spec: operatorv1alpha4.MulticlusterGlobalHubSpec{
			DataLayerSpec: operatorv1alpha4.DataLayerSpec{
				Kafka: operatorv1alpha4.KafkaSpec{
					KafkaTopics: operatorv1alpha4.KafkaTopics{
						SpecTopic:   "spec1",
						StatusTopic: "gh-status.*",
					},
					ConsumerGroupPrefix: "gh_",
				},
			},
		},
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
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(
		mgh, kafkaCluster, kafkaUserSecret,
	).Build()
	if err := config.SetTransportConfig(ctx, fakeClient, mgh); err != nil {
		t.Fatalf("SetTransportConfig() error = %v", err)
	}

	trans := &strimziTransporter{
		ctx:                   ctx,
		mgh:                   mgh,
		kafkaClusterName:      KafkaClusterName,
		kafkaClusterNamespace: ns,
		manager:               &fakeManager{c: fakeClient},
	}

	conn, err := getManagerTransportConn(trans)
	if err != nil {
		t.Fatalf("getManagerTransportConn() error = %v", err)
	}
	if conn == nil {
		t.Fatal("getManagerTransportConn() returned nil connection")
	}
	if conn.SpecTopic != "spec1" {
		t.Fatalf("SpecTopic = %q, want spec1", conn.SpecTopic)
	}
	if conn.StatusTopic != "^gh-status.*" {
		t.Fatalf("StatusTopic = %q, want ^gh-status.*", conn.StatusTopic)
	}
	if conn.ConsumerGroupID != "gh_global_hub" {
		t.Fatalf("ConsumerGroupID = %q, want gh_global_hub", conn.ConsumerGroupID)
	}
}

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
			_ = kafkav1beta2.AddToScheme(scheme.Scheme)
			_ = subv1alpha1.AddToScheme(scheme.Scheme)
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

func newKafkaControllerTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := kafkav1beta2.AddToScheme(s); err != nil {
		t.Fatalf("failed to add kafkav1beta2 to scheme: %v", err)
	}
	if err := subv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add subv1alpha1 to scheme: %v", err)
	}
	if err := ocv1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add ocv1 to scheme: %v", err)
	}
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	if err := rbacv1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add rbacv1 to scheme: %v", err)
	}
	return s
}

func TestPruneClusterExtensionResources_CEExists(t *testing.T) {
	ctx := context.Background()
	s := newKafkaControllerTestScheme(t)

	ce := &ocv1.ClusterExtension{
		ObjectMeta: metav1.ObjectMeta{Name: StrimziClusterExtensionName},
	}
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: StrimziInstallerSAName, Namespace: "test-ns"},
	}
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: StrimziInstallerCRBName},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(ce, sa, crb).Build()
	kc := KafkaController{
		c: c,
		trans: &strimziTransporter{
			olmVersion:       config.OLMVersionV1,
			kafkaClusterName: KafkaClusterName,
			mgh: &operatorv1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-ns"},
			},
		},
	}

	result, err := kc.pruneClusterExtensionResources(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 5*time.Second {
		t.Errorf("expected requeue after 5s when CE still exists, got %v", result.RequeueAfter)
	}

	// CE should be deleted (pending full removal)
	gotCE := &ocv1.ClusterExtension{}
	if err := c.Get(ctx, types.NamespacedName{Name: StrimziClusterExtensionName}, gotCE); err == nil {
		t.Error("expected CE to be deleted")
	}
}

func TestPruneClusterExtensionResources_CEGone(t *testing.T) {
	ctx := context.Background()
	s := newKafkaControllerTestScheme(t)

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: StrimziInstallerSAName, Namespace: "test-ns"},
	}
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: StrimziInstallerCRBName},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(sa, crb).Build()
	kc := KafkaController{
		c: c,
		trans: &strimziTransporter{
			olmVersion:       config.OLMVersionV1,
			kafkaClusterName: KafkaClusterName,
			mgh: &operatorv1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-ns"},
			},
		},
	}

	result, err := kc.pruneClusterExtensionResources(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue when CE is gone, got %v", result.RequeueAfter)
	}
	if !isResourceRemoved {
		t.Error("expected isResourceRemoved to be true")
	}

	// SA and CRB should be deleted
	if err := c.Get(ctx, types.NamespacedName{Name: StrimziInstallerSAName, Namespace: "test-ns"},
		&corev1.ServiceAccount{}); err == nil {
		t.Error("expected SA to be deleted")
	}
	if err := c.Get(ctx, types.NamespacedName{Name: StrimziInstallerCRBName},
		&rbacv1.ClusterRoleBinding{}); err == nil {
		t.Error("expected CRB to be deleted")
	}
}

func TestPruneClusterExtensionResources_AllGone(t *testing.T) {
	ctx := context.Background()
	s := newKafkaControllerTestScheme(t)

	c := fake.NewClientBuilder().WithScheme(s).Build()
	kc := KafkaController{
		c: c,
		trans: &strimziTransporter{
			olmVersion:       config.OLMVersionV1,
			kafkaClusterName: KafkaClusterName,
			mgh: &operatorv1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-ns"},
			},
		},
	}

	result, err := kc.pruneClusterExtensionResources(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue when all resources gone, got %v", result.RequeueAfter)
	}
	if !isResourceRemoved {
		t.Error("expected isResourceRemoved to be true")
	}
}

func TestPruneStrimziResources_OLMv1(t *testing.T) {
	ctx := context.Background()
	s := newKafkaControllerTestScheme(t)

	ce := &ocv1.ClusterExtension{
		ObjectMeta: metav1.ObjectMeta{Name: StrimziClusterExtensionName},
	}
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: StrimziInstallerSAName, Namespace: "test-ns"},
	}
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: StrimziInstallerCRBName},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(ce, sa, crb).Build()
	isResourceRemoved = false
	kc := KafkaController{
		c: c,
		trans: &strimziTransporter{
			olmVersion:       config.OLMVersionV1,
			kafkaClusterName: KafkaClusterName,
			mgh: &operatorv1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-ns"},
			},
		},
	}

	result, err := kc.pruneStrimziResources(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 5*time.Second {
		t.Errorf("expected requeue after 5s (CE still being removed), got %v", result.RequeueAfter)
	}
}

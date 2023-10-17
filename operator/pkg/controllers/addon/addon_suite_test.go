/*
Copyright 2022.
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

package addon_test

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operatorsv1 "github.com/operator-framework/operator-lifecycle-manager/pkg/package-server/apis/operators/v1"
	agentv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	clusterv1beta2 "open-cluster-management.io/api/cluster/v1beta2"
	workv1 "open-cluster-management.io/api/work/v1"
	chnv1 "open-cluster-management.io/multicloud-operators-channel/pkg/apis/apps/v1"
	placementrulesv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	appsubv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	appsubV1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/addon"
	hubofhubscontroller "github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/kafka"
	commonobjects "github.com/stolostron/multicluster-global-hub/pkg/objects"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg       *rest.Config
	k8sClient client.Client // You'll be using this client in your tests.
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Integration Suite")
}

var _ = BeforeSuite(func() {
	Expect(os.Setenv("POD_NAMESPACE", "default")).To(Succeed())
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "..", "pkg", "testdata", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	// add scheme
	err = operatorsv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = clusterv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = clusterv1beta1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = clusterv1beta2.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = workv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = addonv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = appsubv1.SchemeBuilder.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = appsubV1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = chnv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = placementrulesv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = globalhubv1alpha4.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = appsv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = agentv1.SchemeBuilder.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	prepareBeforeTest()

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		MetricsBindAddress: "0", // disable the metrics serving
		Scheme:             scheme.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	By("Add the addon install reconciler to the manager")
	err = (&addon.HoHAddonInstallReconciler{
		Client: k8sClient,
		Log:    ctrl.Log.WithName("addon install controller"),
	}).SetupWithManager(ctx, k8sManager)
	Expect(err).ToNot(HaveOccurred())

	kubeClient, err := kubernetes.NewForConfig(k8sManager.GetConfig())
	Expect(err).ToNot(HaveOccurred())
	electionConfig, err := getElectionConfig(kubeClient)
	Expect(err).ToNot(HaveOccurred())

	By("Add the addon controller to the manager")
	middlewareCfg := &hubofhubscontroller.MiddlewareConfig{
		KafkaConnection: &kafka.KafkaConnection{
			BootstrapServer: kafkaBootstrapServer,
			CACert:          base64.StdEncoding.EncodeToString([]byte(kafkaCA)),
			ClientCert:      "",
			ClientKey:       "",
		},
	}
	addonController, err := addon.NewHoHAddonController(k8sManager.GetConfig(), k8sClient,
		electionConfig, middlewareCfg, true, &corev1.ConfigMap{}, "info")
	Expect(err).ToNot(HaveOccurred())
	err = k8sManager.Add(addonController)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	// https://github.com/kubernetes-sigs/controller-runtime/issues/1571
	// Set 4 with random
	if err != nil {
		time.Sleep(4 * time.Second)
	}
	err = testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

const (
	MGHName              = "test-mgh"
	StorageSecretName    = operatorconstants.GHStorageSecretName
	TransportSecretName  = operatorconstants.GHTransportSecretName
	kafkaCA              = "foobar"
	kafkaBootstrapServer = "https://test-kafka.example.com"

	timeout  = time.Second * 60
	duration = time.Second * 10
	interval = time.Millisecond * 250
)

var mgh = &globalhubv1alpha4.MulticlusterGlobalHub{
	ObjectMeta: metav1.ObjectMeta{
		Name: MGHName,
	},
	Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{
		ImagePullSecret: "test-pull-secret",
		DataLayer:       globalhubv1alpha4.DataLayerConfig{},
	},
	Status: globalhubv1alpha4.MulticlusterGlobalHubStatus{
		Conditions: []metav1.Condition{
			{
				Type:   condition.CONDITION_TYPE_GLOBALHUB_READY,
				Status: metav1.ConditionTrue,
			},
		},
	},
}

func prepareBeforeTest() {
	// create a MGH instance
	By("By creating a new MGH instance")
	mgh.SetNamespace(config.GetDefaultNamespace())
	Expect(k8sClient.Create(ctx, mgh)).Should(Succeed())

	Expect(k8sClient.Status().Update(ctx, mgh)).Should(Succeed())

	err := condition.SetCondition(ctx, k8sClient, mgh,
		condition.CONDITION_TYPE_GLOBALHUB_READY,
		metav1.ConditionTrue,
		condition.CONDITION_REASON_GLOBALHUB_READY,
		condition.CONDITION_MESSAGE_GLOBALHUB_READY,
	)

	Expect(err).ShouldNot(HaveOccurred())

	// 	After creating this MGH instance, check that the MGH instance's Spec fields are failed with default values.
	mghLookupKey := types.NamespacedName{Namespace: config.GetDefaultNamespace(), Name: MGHName}
	config.SetHoHMGHNamespacedName(mghLookupKey)
	createdMGH := &globalhubv1alpha4.MulticlusterGlobalHub{}

	// get this newly created MGH instance, given that creation may not immediately happen.
	Eventually(func() bool {
		err := k8sClient.Get(ctx, mghLookupKey, createdMGH)
		return err == nil
	}, timeout, interval).Should(BeTrue())

	// set fake packagemenifestwork configuration
	By("By setting a fake packagemanifest configuration")
	addon.SetPackageManifestConfig("release-2.6", "advanced-cluster-management.v2.6.0",
		"stable-2.0", "multicluster-engine.v2.0.1",
		map[string]string{"multiclusterhub-operator": "example.com/registration-operator:test"},
		map[string]string{"registration-operator": "example.com/registration-operator:test"})

	By("By creating a fake image global hub pull secret")
	Expect(k8sClient.Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mgh.Spec.ImagePullSecret,
			Namespace: config.GetDefaultNamespace(),
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`{"test":"global hub pull secret"}`),
		},
		Type: corev1.SecretTypeDockerConfigJson,
	})).Should(Succeed())
}

func getElectionConfig(kubeClient *kubernetes.Clientset) (*commonobjects.LeaderElectionConfig, error) {
	cfg := &commonobjects.LeaderElectionConfig{
		LeaseDuration: 137,
		RenewDeadline: 107,
		RetryPeriod:   26,
	}

	configMap, err := kubeClient.CoreV1().ConfigMaps(config.GetDefaultNamespace()).Get(
		context.TODO(), operatorconstants.ControllerConfig, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}

	leaseDurationSec, err := strconv.Atoi(configMap.Data["leaseDuration"])
	if err != nil {
		return nil, err
	}

	renewDeadlineSec, err := strconv.Atoi(configMap.Data["renewDeadline"])
	if err != nil {
		return nil, err
	}

	retryPeriodSec, err := strconv.Atoi(configMap.Data["retryPeriod"])
	if err != nil {
		return nil, err
	}

	cfg.LeaseDuration = leaseDurationSec
	cfg.RenewDeadline = renewDeadlineSec
	cfg.RetryPeriod = retryPeriodSec
	return cfg, nil
}

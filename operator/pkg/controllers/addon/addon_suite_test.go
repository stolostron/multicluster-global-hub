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
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/addon"
	operatortrans "github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs/transport/transporter"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/test/pkg/kafka"
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
	runtimeScheme := config.GetRuntimeScheme()
	k8sClient, err = client.New(cfg, client.Options{Scheme: runtimeScheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	prepareBeforeTest()

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Metrics: metricsserver.Options{
			BindAddress: "0", // disable the metrics serving
		},
		Scheme:   runtimeScheme,
		NewCache: config.InitCache,
	})
	Expect(err).ToNot(HaveOccurred())

	By("Add the addon installer to the manager")
	err = (&addon.AddonInstaller{
		Client: k8sClient,
		Log:    ctrl.Log.WithName("addon install controller"),
	}).SetupWithManager(ctx, k8sManager)
	Expect(err).ToNot(HaveOccurred())

	kubeClient, err := kubernetes.NewForConfig(k8sManager.GetConfig())
	Expect(err).ToNot(HaveOccurred())
	err = config.LoadControllerConfig(ctx, kubeClient)
	Expect(err).ToNot(HaveOccurred())

	By("Add the addon controller to the manager")
	addonController, err := addon.NewAddonController(k8sManager.GetConfig(), k8sClient, &config.OperatorConfig{
		GlobalResourceEnabled: true,
		LogLevel:              "info",
		EnablePprof:           false,
	})
	Expect(err).ToNot(HaveOccurred())
	err = k8sManager.Add(addonController)
	Expect(err).ToNot(HaveOccurred())

	By("Create an external transport")
	trans := operatortrans.NewBYOTransporter(ctx, types.NamespacedName{
		Namespace: mgh.Namespace,
		Name:      constants.GHTransportSecretName,
	}, k8sClient)
	config.SetTransporter(trans)

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
	MGHName = "test-mgh"

	timeout  = time.Second * 60
	duration = time.Second * 10
	interval = time.Millisecond * 250
)

var mgh = &v1alpha4.MulticlusterGlobalHub{
	ObjectMeta: metav1.ObjectMeta{
		Name: MGHName,
	},
	Spec: v1alpha4.MulticlusterGlobalHubSpec{
		ImagePullSecret: "test-pull-secret",
		DataLayer:       v1alpha4.DataLayerConfig{},
	},
	Status: v1alpha4.MulticlusterGlobalHubStatus{
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
	mgh.SetNamespace(utils.GetDefaultNamespace())
	Expect(k8sClient.Create(ctx, mgh)).Should(Succeed())
	config.SetMGHNamespacedName(types.NamespacedName{
		Namespace: mgh.Namespace,
		Name:      mgh.Name,
	})

	Expect(k8sClient.Status().Update(ctx, mgh)).Should(Succeed())

	err := condition.SetCondition(ctx, k8sClient, mgh,
		condition.CONDITION_TYPE_GLOBALHUB_READY,
		metav1.ConditionTrue,
		condition.CONDITION_REASON_GLOBALHUB_READY,
		condition.CONDITION_MESSAGE_GLOBALHUB_READY,
	)

	Expect(err).ShouldNot(HaveOccurred())

	// 	After creating this MGH instance, check that the MGH instance's Spec fields are failed with default values.
	mghLookupKey := types.NamespacedName{Namespace: utils.GetDefaultNamespace(), Name: MGHName}
	config.SetMGHNamespacedName(mghLookupKey)
	createdMGH := &v1alpha4.MulticlusterGlobalHub{}

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
			Namespace: utils.GetDefaultNamespace(),
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`{"test":"global hub pull secret"}`),
		},
		Type: corev1.SecretTypeDockerConfigJson,
	})).Should(Succeed())

	By("By creating secret transport")
	kafka.CreateTestTransportSecret(k8sClient, mgh.Namespace)
	transporter := operatortrans.NewBYOTransporter(context.TODO(), types.NamespacedName{
		Namespace: mgh.Namespace,
		Name:      constants.GHTransportSecretName,
	}, k8sClient)
	config.SetTransporter(transporter)
}

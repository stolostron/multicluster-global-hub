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

package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/agent"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/agent/addon"
	operatortrans "github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/transporter/protocol"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg              *rest.Config
	runtimeClient    client.Client // You'll be using this client in your tests.
	testEnv          *envtest.Environment
	ctx              context.Context
	cancel           context.CancelFunc
	mgr              manager.Manager
	controllerOption config.ControllerOption
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
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths: []string{
				filepath.Join("..", "..", "..", "..", "..", "operator", "config", "crd", "bases"),
				filepath.Join("..", "..", "..", "..", "manifest", "crd"),
			},
			MaxTime: 1 * time.Minute,
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
	runtimeClient, err = client.New(cfg, client.Options{Scheme: runtimeScheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(runtimeClient).NotTo(BeNil())

	prepareBeforeTest()

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Metrics: metricsserver.Options{
			BindAddress: "0", // disable the metrics serving
		},
		Scheme:         runtimeScheme,
		NewCache:       config.InitCache,
		LeaderElection: false,
	})
	Expect(err).ToNot(HaveOccurred())
	existMgh := &globalhubv1alpha4.MulticlusterGlobalHub{}

	Eventually(func() bool {
		err := runtimeClient.Get(ctx, config.GetMGHNamespacedName(), existMgh)
		return err == nil
	}, timeout, interval).Should(BeTrue())

	mgr = k8sManager
	controllerOption = config.ControllerOption{
		Ctx:                   ctx,
		Manager:               mgr,
		MulticlusterGlobalHub: existMgh,
		OperatorConfig: &config.OperatorConfig{
			GlobalResourceEnabled: true,
			EnablePprof:           false,
		},
	}
	config.SetTransporterConn(&transport.KafkaConfig{
		ClusterID:         "fake",
		IsNewKafkaCluster: true,
	})

	By("start the addon manager and add addon controller to manager")
	_, err = addon.StartAddonManagerController(controllerOption)
	Expect(err).ToNot(HaveOccurred())
	_, err = addon.StartDefaultAgentController(controllerOption)
	Expect(err).ToNot(HaveOccurred())
	_, err = agent.StartLocalAgentController(controllerOption)
	Expect(err).ToNot(HaveOccurred())

	kubeClient, err := kubernetes.NewForConfig(k8sManager.GetConfig())
	Expect(err).ToNot(HaveOccurred())
	err = config.LoadControllerConfig(ctx, kubeClient)
	Expect(err).ToNot(HaveOccurred())

	By("Create an external transport")
	trans := operatortrans.NewBYOTransporter(ctx, types.NamespacedName{
		Namespace: mgh.Namespace,
		Name:      constants.GHTransportSecretName,
	}, runtimeClient)
	config.SetTransporter(trans)

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()
	Expect(k8sManager.GetCache().WaitForCacheSync(ctx)).To(BeTrue())
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

var mgh = &globalhubv1alpha4.MulticlusterGlobalHub{
	ObjectMeta: metav1.ObjectMeta{
		Name: MGHName,
	},
	Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{
		ImagePullSecret:     "test-pull-secret",
		DataLayerSpec:       globalhubv1alpha4.DataLayerSpec{},
		InstallAgentOnLocal: true,
	},
	Status: globalhubv1alpha4.MulticlusterGlobalHubStatus{
		Conditions: []metav1.Condition{
			{
				Type:   config.CONDITION_TYPE_GLOBALHUB_READY,
				Status: metav1.ConditionTrue,
			},
		},
	},
}

func prepareBeforeTest() {
	By("By creating a new MGH instance")
	mgh.SetNamespace(utils.GetDefaultNamespace())
	Expect(runtimeClient.Create(ctx, mgh)).Should(Succeed())
	config.SetMGHNamespacedName(types.NamespacedName{
		Namespace: mgh.Namespace,
		Name:      mgh.Name,
	})
	config.SetACMResourceReady(true)
	Expect(runtimeClient.Status().Update(ctx, mgh)).Should(Succeed())

	err := config.UpdateCondition(ctx, runtimeClient, config.GetMGHNamespacedName(),
		metav1.Condition{
			Type:    config.CONDITION_TYPE_GLOBALHUB_READY,
			Status:  config.CONDITION_STATUS_TRUE,
			Reason:  config.CONDITION_REASON_GLOBALHUB_READY,
			Message: config.CONDITION_MESSAGE_GLOBALHUB_READY,
		}, "")
	Expect(err).ShouldNot(HaveOccurred())

	// 	After creating this MGH instance, check that the MGH instance's Spec fields are failed with default values.
	mghLookupKey := types.NamespacedName{Namespace: utils.GetDefaultNamespace(), Name: MGHName}
	config.SetMGHNamespacedName(mghLookupKey)
	createdMGH := &globalhubv1alpha4.MulticlusterGlobalHub{}

	// get this newly created MGH instance, given that creation may not immediately happen.
	Eventually(func() bool {
		err := runtimeClient.Get(ctx, mghLookupKey, createdMGH)
		return err == nil
	}, timeout, interval).Should(BeTrue())

	// set fake packagemenifestwork configuration
	By("By setting a fake packagemanifest configuration")
	addon.SetPackageManifestConfig("release-2.6", "advanced-cluster-management.v2.6.0",
		"stable-2.0", "multicluster-engine.v2.0.1",
		map[string]string{"multiclusterhub-operator": "example.com/registration-operator:test"},
		map[string]string{"registration-operator": "example.com/registration-operator:test"})

	By("By creating a fake image global hub pull secret")
	Expect(runtimeClient.Create(ctx, &corev1.Secret{
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
	CreateTestTransportSecret(runtimeClient, mgh.Namespace)
	transporter := operatortrans.NewBYOTransporter(ctx, types.NamespacedName{
		Namespace: mgh.Namespace,
		Name:      constants.GHTransportSecretName,
	}, runtimeClient)
	config.SetTransporter(transporter)

	By("Create clustermanagementaddon instance")
	clusterManagementAddon := &addonv1alpha1.ClusterManagementAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name: constants.GHClusterManagementAddonName,
			Labels: map[string]string{
				constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
			},
		},
		Spec: addonv1alpha1.ClusterManagementAddOnSpec{
			InstallStrategy: addonv1alpha1.InstallStrategy{
				Type: "Manual",
			},
		},
	}
	Expect(runtimeClient.Create(ctx, clusterManagementAddon)).Should(Succeed())
}

func CreateTestTransportSecret(c client.Client, namespace string) error {
	// if namespace not exist, then create the namespace
	err := c.Create(context.TODO(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	err = c.Get(context.TODO(), types.NamespacedName{
		Namespace: namespace,
		Name:      constants.GHTransportSecretName,
	}, &corev1.Secret{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	} else if errors.IsNotFound(err) {
		return c.Create(context.TODO(), &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.GHTransportSecretName,
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"bootstrap_server": []byte("https://test-kafka.example.com"),
				"ca.crt":           []byte("foobar"),
				"client.crt":       []byte(""),
				"client.key":       []byte(""),
			},
		})
	}
	return nil
}

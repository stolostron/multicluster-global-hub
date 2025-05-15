package mgh

import (
	"reflect"
	"time"

	"github.com/cloudflare/cfssl/log"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"open-cluster-management.io/api/addon/v1alpha1"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/agent/addon"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
	testutils "github.com/stolostron/multicluster-global-hub/test/integration/utils"
)

var (
	timeout         = time.Second * 60
	interval        = time.Millisecond * 100
	hostedNamespace = "mc-hosted"
	mghNameSpace    = "test-mgh"
)

var workManager = v1alpha1.ClusterManagementAddOn{
	ObjectMeta: metav1.ObjectMeta{
		Name: "work-manager",
	},
	Spec: v1alpha1.ClusterManagementAddOnSpec{
		InstallStrategy: v1alpha1.InstallStrategy{
			Type: "Manual",
		},
	},
}

var proxy = v1alpha1.ClusterManagementAddOn{
	ObjectMeta: metav1.ObjectMeta{
		Name: "cluster-proxy",
	},
	Spec: v1alpha1.ClusterManagementAddOnSpec{
		InstallStrategy: v1alpha1.InstallStrategy{
			Type: "Manual",
		},
	},
}

var msa = v1alpha1.ClusterManagementAddOn{
	ObjectMeta: metav1.ObjectMeta{
		Name: "managed-serviceaccount",
	},
	Spec: v1alpha1.ClusterManagementAddOnSpec{
		InstallStrategy: v1alpha1.InstallStrategy{
			Type: "Manual",
		},
	},
}

var hostedMGH = globalhubv1alpha4.MulticlusterGlobalHub{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: mghNameSpace,
		Name:      "mgh",
		Finalizers: []string{
			"fz",
		},
	},
	Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{
		FeatureGates: []globalhubv1alpha4.FeatureGate{
			{
				Feature: globalhubv1alpha4.FeatureGateImportClusterInHosted,
				Mode:    globalhubv1alpha4.FeatureGateModeTypeEnable,
			},
		},
	},
	Status: globalhubv1alpha4.MulticlusterGlobalHubStatus{
		Conditions: []v1.Condition{
			metav1.Condition{
				Type:    config.CONDITION_TYPE_GLOBALHUB_READY,
				Status:  config.CONDITION_STATUS_TRUE,
				Reason:  config.CONDITION_REASON_GLOBALHUB_READY,
				Message: config.CONDITION_MESSAGE_GLOBALHUB_READY,
			},
		},
	},
}

// hosted: put the acm agent cluster control plane into acm hub cluster
// global hub -> managed-hub(local-cluster)

// go test ./test/integration/operator/controllers/agent -ginkgo.focus "other addons in hosted mode test" -v
var _ = Describe("other addons in hosted mode test", Ordered, func() {
	var hostedAddonReconciler *addon.HostedAgentController
	BeforeAll(func() {
		config.SetImportClusterInHosted(&hostedMGH)
		var err error

		hostedAddonReconciler = addon.NewHostedAgentController(mgr)
		Expect(mgr.GetClient().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: mghNameSpace,
			},
		})).To(Succeed())
		Expect(mgr.GetClient().Create(ctx, &hostedMGH)).To(Succeed())

		initOption := config.ControllerOption{
			MulticlusterGlobalHub: &hostedMGH,
			Manager:               mgr,
		}
		_, err = addon.StartHostedAgentController(initOption)
		Expect(err).Should(Succeed())

		config.SetACMResourceReady(true)

		mcHostedNamespace := &corev1.Namespace{ObjectMeta: v1.ObjectMeta{Name: hostedNamespace}}
		err = mgr.GetClient().Create(ctx, mcHostedNamespace)
		Expect(err).Should(Succeed())
		time.Sleep(1 * time.Second)

		workManagerDefault := workManager.DeepCopy()
		workManagerDefault.Namespace = hostedNamespace
		err = mgr.GetClient().Create(ctx, workManagerDefault)
		Expect(err).NotTo(HaveOccurred())

		proxyDefault := proxy.DeepCopy()
		proxyDefault.Namespace = hostedNamespace
		err = mgr.GetClient().Create(ctx, proxyDefault)
		Expect(err).NotTo(HaveOccurred())

		msaDefault := msa.DeepCopy()
		msaDefault.Namespace = hostedNamespace
		err = mgr.GetClient().Create(ctx, msaDefault)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		err := mgr.GetClient().Delete(ctx, &workManager)
		Expect(err).NotTo(HaveOccurred())
		err = mgr.GetClient().Delete(ctx, &proxy)
		Expect(err).NotTo(HaveOccurred())
		err = mgr.GetClient().Delete(ctx, &msa)
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() error {
			if err := testutils.DeleteMgh(ctx, mgr.GetClient(), &hostedMGH); err != nil {
				return err
			}
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: mghNameSpace,
				},
			}
			return mgr.GetClient().Delete(ctx, ns)
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	It("addons should be added the new config", func() {
		Eventually(func() bool {
			cma := &v1alpha1.ClusterManagementAddOn{}
			for addonName := range config.HostedAddonList {
				_, err := hostedAddonReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: hostedNamespace,
						Name:      addonName,
					},
				})
				if err != nil {
					log.Errorf("Failed to reconcile addon, err:%v", err)
					return false
				}

				err = mgr.GetClient().Get(ctx, types.NamespacedName{
					Namespace: hostedNamespace,
					Name:      addonName,
				}, cma)
				if err != nil {
					log.Errorf("Failed to list ClusterManagementAddOn")
					return false
				}

				found := false
				for _, ps := range cma.Spec.InstallStrategy.Placements {
					if reflect.DeepEqual(ps.PlacementRef, config.GlobalHubHostedAddonPlacementStrategy.PlacementRef) &&
						reflect.DeepEqual(ps.Configs, config.GlobalHubHostedAddonPlacementStrategy.Configs) {
						found = true
					}
				}
				if !found {
					log.Errorf("Can not found expected config in %v", cma.Spec.InstallStrategy)
					return false
				}
			}
			return true
		}, timeout, interval).Should(BeTrue())
	})

	It("addondeploymentconfig should be deleted after mgh deleting", func() {
		err := mgr.GetClient().Delete(ctx, &hostedMGH)
		Expect(err).NotTo(HaveOccurred())
		newMgh := hostedMGH.DeepCopy()
		newMgh.Annotations = nil
		config.SetImportClusterInHosted(newMgh)
		_, err = hostedAddonReconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{},
		})
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() bool {
			addonDeployConfig := &addonv1alpha1.AddOnDeploymentConfig{}
			if err := mgr.GetClient().Get(ctx, types.NamespacedName{
				Namespace: utils.GetDefaultNamespace(),
				Name:      "global-hub",
			}, addonDeployConfig); err != nil {
				if errors.IsNotFound(err) {
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue())
	})
})

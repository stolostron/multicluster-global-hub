package controllers

import (
	"context"
	"reflect"
	"time"

	"github.com/cloudflare/cfssl/log"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/mceaddons"
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

var mgh = v1alpha4.MulticlusterGlobalHub{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "test-mgh",
		Namespace: mghNameSpace,
	},
	Spec: v1alpha4.MulticlusterGlobalHubSpec{
		EnableMetrics: true,
	},
	Status: v1alpha4.MulticlusterGlobalHubStatus{
		Conditions: []metav1.Condition{
			{
				Type:   config.CONDITION_TYPE_GLOBALHUB_READY,
				Status: metav1.ConditionTrue,
			},
		},
	},
}

// go test ./test/integration/operator/controllers -ginkgo.focus "mce addons change config" -v
var _ = Describe("mceaddons", Ordered, func() {
	var mceAddonsReconciler *mceaddons.MceAddonsController
	BeforeAll(func() {
		var err error
		mceAddonsReconciler = mceaddons.NewMceAddonsController(runtimeManager)
		mceAddonsReconciler.SetupWithManager(runtimeManager)
		Expect(runtimeManager.GetClient().Create(context.Background(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: mghNameSpace,
			},
		})).To(Succeed())
		Expect(runtimeManager.GetClient().Create(context.Background(), &mgh)).To(Succeed())

		initOption := config.ControllerOption{
			MulticlusterGlobalHub: &mgh,
			Manager:               runtimeManager,
		}
		_, err = mceaddons.StartMceAddonsController(initOption)
		Expect(err).Should(Succeed())

		config.SetACMResourceReady(true)

		mcHostedNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: hostedNamespace}}
		err = runtimeManager.GetClient().Create(context.Background(), mcHostedNamespace)
		Expect(err).Should(Succeed())
		time.Sleep(1 * time.Second)

		workManagerDefault := workManager.DeepCopy()
		workManagerDefault.Namespace = hostedNamespace
		err = runtimeManager.GetClient().Create(context.Background(), workManagerDefault)
		Expect(err).NotTo(HaveOccurred())

		proxyDefault := proxy.DeepCopy()
		proxyDefault.Namespace = hostedNamespace
		err = runtimeManager.GetClient().Create(context.Background(), proxyDefault)
		Expect(err).NotTo(HaveOccurred())

		msaDefault := msa.DeepCopy()
		msaDefault.Namespace = hostedNamespace
		err = runtimeManager.GetClient().Create(context.Background(), msaDefault)
		Expect(err).NotTo(HaveOccurred())
	})
	AfterAll(func() {
		err := runtimeManager.GetClient().Delete(context.Background(), &workManager)
		Expect(err).NotTo(HaveOccurred())
		err = runtimeManager.GetClient().Delete(context.Background(), &proxy)
		Expect(err).NotTo(HaveOccurred())
		err = runtimeManager.GetClient().Delete(context.Background(), &msa)
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() error {
			if err := runtimeManager.GetClient().Delete(context.Background(), &mgh); err != nil {
				return err
			}
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: mghNameSpace,
				},
			}
			return runtimeManager.GetClient().Delete(context.Background(), ns)
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
	It("addons should be added the new config", func() {
		Eventually(func() bool {
			cma := &v1alpha1.ClusterManagementAddOn{}
			for addonName := range config.HostedAddonList {
				_, err := mceAddonsReconciler.Reconcile(context.Background(), reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: hostedNamespace,
						Name:      addonName,
					},
				})
				if err != nil {
					log.Errorf("Failed to reconcile addon, err:%v", err)
					return false
				}

				err = runtimeManager.GetClient().Get(context.Background(), types.NamespacedName{
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
})

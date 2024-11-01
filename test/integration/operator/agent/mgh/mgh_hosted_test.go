package mgh

import (
	"reflect"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/agent"
)

var (
	timeout         = time.Second * 60
	interval        = time.Millisecond * 100
	hostedNamespace = "mc-hosted"
	mghName         = "test-mgh"
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
		Namespace: mghName,
		Name:      "mgh",
		Annotations: map[string]string{
			"global-hub.open-cluster-management.io/import-cluster-in-hosted": "true",
		},
	},
	Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{},
}

// hosted: put the acm agent cluster control plane into acm hub cluster
// global hub -> managed-hub(local-cluster)

// go test ./test/integration/operator/agent -ginkgo.focus "other addons in hosted mode test" -v
var _ = Describe("other addons in hosted mode test", Ordered, func() {
	var hostedAddonReconciler *agent.HostedAddonsReconciler
	BeforeAll(func() {
		config.SetImportClusterInHosted(&hostedMGH)
		var err error
		hostedAddonReconciler, err = agent.AddHostedAddonsReconciler(mgr)
		Expect(err).NotTo(HaveOccurred())
		Expect(mgr.GetClient().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: mghName,
			},
		})).To(Succeed())
		Expect(mgr.GetClient().Create(ctx, &hostedMGH)).To(Succeed())

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
		err := mgr.GetClient().Delete(ctx, &hostedMGH)
		Expect(err).NotTo(HaveOccurred())
		err = mgr.GetClient().Delete(ctx, &workManager)
		Expect(err).NotTo(HaveOccurred())
		err = mgr.GetClient().Delete(ctx, &proxy)
		Expect(err).NotTo(HaveOccurred())
		err = mgr.GetClient().Delete(ctx, &msa)
		Expect(err).NotTo(HaveOccurred())
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
					klog.Errorf("Failed to reconcile addon, err:%v", err)
					return false
				}

				err = mgr.GetClient().Get(ctx, types.NamespacedName{
					Namespace: hostedNamespace,
					Name:      addonName,
				}, cma)
				if err != nil {
					klog.Errorf("Failed to list ClusterManagementAddOn")
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
					klog.Errorf("Can not found expected config in %v", cma.Spec.InstallStrategy)
					return false
				}
			}
			return true
		}, timeout, interval).Should(BeTrue())
	})
})

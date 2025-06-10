package agent

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

type Object interface {
	metav1.Object
	runtime.Object
}

var clusterAvailableCondition = metav1.Condition{
	Type:               "ManagedClusterConditionAvailable",
	Reason:             "ManagedClusterAvailable",
	Message:            "Managed cluster is available",
	Status:             "True",
	LastTransitionTime: metav1.Time{Time: time.Now()},
}

func prepareCluster(name string, labels, annotations map[string]string,
	claims []clusterv1.ManagedClusterClaim, conditions ...metav1.Condition,
) {
	cluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      labels,
			Annotations: annotations,
		},
	}
	Expect(runtimeClient.Create(ctx, cluster)).Should(Succeed())
	if len(conditions) != 0 || len(claims) != 0 {
		cluster.Status.Conditions = conditions
		cluster.Status.ClusterClaims = claims
		Expect(runtimeClient.Status().Update(ctx, cluster)).Should(Succeed())
	}

	err := runtimeClient.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	})
	if !errors.IsAlreadyExists(err) {
		Expect(err).Should(Succeed())
	}

	Expect(runtimeClient.Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-kafka-user", name),
			Namespace: config.GetMGHNamespacedName().Namespace,
		},
	})).Should(Succeed())
}

// go test ./test/integration/operator/controllers/agent -ginkgo.focus "none addon" -v
var _ = Describe("none addon", func() {
	It("Should not create addon in these cases", func() {
		By("By preparing a non-OCP with deployMode label Managed Clusters")
		clusterName1 := fmt.Sprintf("hub-non-ocp-%s", rand.String(6))
		prepareCluster(clusterName1,
			map[string]string{
				"vendor":                       "GCP",
				constants.GHDeployModeLabelKey: constants.GHDeployModeDefault,
			},
			map[string]string{},
			[]clusterv1.ManagedClusterClaim{},
			clusterAvailableCondition)

		By("By preparing an OCP with deployMode = none label Managed Clusters")
		clusterName2 := fmt.Sprintf("hub-ocp-mode-none-%s", rand.String(6))
		prepareCluster(clusterName2,
			map[string]string{
				"vendor": "OpenShift",
				// constants.GHDeployModeLabelKey: "",
			},
			map[string]string{},
			[]clusterv1.ManagedClusterClaim{},
			clusterAvailableCondition)

		By("By preparing an OCP with no condition Managed Clusters")
		clusterName3 := fmt.Sprintf("hub-ocp-no-condtion-%s", rand.String(6))
		prepareCluster(clusterName3,
			map[string]string{
				"vendor":                       "OpenShift",
				constants.GHDeployModeLabelKey: constants.GHDeployModeDefault,
			},
			map[string]string{},
			[]clusterv1.ManagedClusterClaim{},
		)

		By("By preparing a local cluster")
		clusterName4 := constants.LocalClusterName
		prepareCluster(clusterName4, map[string]string{
			"vendor":                       "OpenShift",
			constants.GHDeployModeLabelKey: constants.GHDeployModeDefault,
		},
			map[string]string{},
			[]clusterv1.ManagedClusterClaim{},
			clusterAvailableCondition)

		By("By preparing an OCP with deploy mode = Hosted without hosting cluster")
		clusterName5 := fmt.Sprintf("hub-ocp-mode-none-%s", rand.String(6))
		prepareCluster(clusterName5,
			map[string]string{
				"vendor":                       "OpenShift",
				constants.GHDeployModeLabelKey: constants.GHDeployModeHosted,
			},
			map[string]string{},
			[]clusterv1.ManagedClusterClaim{},
			clusterAvailableCondition)

		By("By checking the addon CR is is created in the cluster ns")
		addonList := &addonv1alpha1.ManagedClusterAddOnList{}
		checkCount := 0
		Eventually(func() error {
			err := runtimeClient.List(ctx, addonList, client.InNamespace(clusterName1),
				client.InNamespace(clusterName2), client.InNamespace(clusterName3),
				client.InNamespace(clusterName4), client.InNamespace(clusterName5))
			if err != nil {
				return err
			}

			if len(addonList.Items) != 0 {
				utils.PrettyPrint(addonList.Items)
				return fmt.Errorf("expected there is no addon, but got %#v", addonList)
			}
			if checkCount == 5 {
				return nil
			}
			checkCount++
			time.Sleep(1 * time.Second)
			return fmt.Errorf("check again %v", checkCount)
		}, timeout, interval).ShouldNot(HaveOccurred())
	})

	It("Should not create agent for the local-cluster", func() {
		clusterName := fmt.Sprintf("hub-%s", rand.String(6))
		By("By preparing an OCP Managed Clusters")
		prepareCluster(clusterName,
			map[string]string{"vendor": "OpenShift", "local-cluster": "true"},
			map[string]string{},
			[]clusterv1.ManagedClusterClaim{},
			clusterAvailableCondition)

		By("By checking the addon CR is is created in the cluster ns")
		addon := &addonv1alpha1.ManagedClusterAddOn{}
		Eventually(func() error {
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name:      constants.GHManagedClusterAddonName,
				Namespace: clusterName,
			}, addon)
		}, duration, interval).Should(HaveOccurred())
		Eventually(func() error {
			return runtimeClient.Delete(ctx, &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: clusterName,
				},
			})
		}, duration, interval).ShouldNot(HaveOccurred())
	})
})

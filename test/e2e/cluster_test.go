package tests

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

var _ = Describe("Managed Clusters", Label("e2e-test-cluster"), Ordered, func() {
	Context("Cluster Events", func() {
		It("sync the event to the global hub database", func() {
			By("Create the cluster event")
			cluster := managedClusters[0]
			hubName, _ := strings.CutSuffix(cluster.Name, "-cluster1")
			eventName := fmt.Sprintf("%s.event.17cd34e8c8b27fdc", cluster.Name)
			eventMessage := fmt.Sprintf("The managed cluster (%s) cannot connect to the hub cluster.", cluster.Name)
			clusterEvent := &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{
					Name:      eventName,
					Namespace: cluster.Name,
				},
				InvolvedObject: corev1.ObjectReference{
					Kind: constants.ManagedClusterKind,
					// TODO: the cluster namespace should be empty! but if not set the namespace,
					// it will throw the error: involvedObject.namespace: Invalid value: "": does not match event.namespace
					Namespace: cluster.Name,
					Name:      cluster.Name,
				},
				Reason:              "AvailableUnknown",
				Message:             eventMessage,
				ReportingController: "registration-controller",
				ReportingInstance:   "registration-controller-cluster-manager-registration-controller-6794cf54d9-j7lgm",
				Type:                "Warning",
			}

			hubClient, err := testClients.RuntimeClient(hubName, agentScheme)
			Expect(err).To(Succeed())
			Expect(hubClient.Create(ctx, clusterEvent, &client.CreateOptions{})).To(Succeed())

			By("Get the cluster event from database")
			Eventually(func() error {
				clusterEvent := models.ManagedClusterEvent{
					LeafHubName: hubName,
					EventName:   eventName,
				}
				err := db.Where(clusterEvent).First(&clusterEvent).Error
				if err != nil {
					return err
				}
				if clusterEvent.Message != eventMessage {
					return fmt.Errorf("want messsage %s, got %s", eventMessage, clusterEvent.Message)
				}
				return nil
			}, 3*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

			By("Delete the cluster event from the leafhub")
			Expect(hubClient.Delete(ctx, clusterEvent)).To(Succeed())
		})
	})

	// TODO: which case the test case want to cover, can we skip it, or move it into intergration test?
	Context("Cluster Managedcluster, should have some annotation", func() {
		It("create managedhub cluster, should have annotation", func() {
			By("Create the managed cluster")
			mh_name := "test-mc-annotation"
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name: mh_name,
			}}
			mh := &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: mh_name,
				},
				Spec: clusterv1.ManagedClusterSpec{
					HubAcceptsClient:     true,
					LeaseDurationSeconds: 60,
				},
			}
			globalhubAddon := &addonapiv1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.GHManagedClusterAddonName,
					Namespace: mh_name,
					Labels: map[string]string{
						constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
					},
				},
				Spec: addonapiv1alpha1.ManagedClusterAddOnSpec{},
			}
			Expect(globalHubClient.Create(ctx, ns)).To(Succeed())
			Expect(globalHubClient.Create(ctx, mh)).To(Succeed())
			Expect(globalHubClient.Create(ctx, globalhubAddon)).To(Succeed())

			Eventually(func() error {
				curMh := &clusterv1.ManagedCluster{}
				Expect(globalHubClient.Get(ctx, types.NamespacedName{
					Name: mh_name,
				}, curMh)).To(Succeed())

				if len(curMh.Annotations) == 0 {
					return fmt.Errorf("failed to add annotation to managedhub")
				}
				_, ok := curMh.GetAnnotations()[constants.AnnotationONMulticlusterHub]
				if !ok {
					return fmt.Errorf("failed to add annotation to managedhub, %v", curMh.GetAnnotations())
				}
				_, ok = curMh.GetAnnotations()[constants.AnnotationPolicyONMulticlusterHub]
				if !ok {
					return fmt.Errorf("failed to add annotation to managedhub%v", curMh.GetAnnotations())
				}
				return nil
			}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

			// remove the annotaiton, and they should be added
			Eventually(func() error {
				curMh := &clusterv1.ManagedCluster{}
				Expect(globalHubClient.Get(ctx, types.NamespacedName{
					Name: mh_name,
				}, curMh)).To(Succeed())
				curMh.Annotations = nil
				return globalHubClient.Update(ctx, curMh)
			}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

			Eventually(func() error {
				curMh := &clusterv1.ManagedCluster{}
				Expect(globalHubClient.Get(ctx, types.NamespacedName{
					Name: mh_name,
				}, curMh)).To(Succeed())

				if len(curMh.Annotations) == 0 {
					return fmt.Errorf("failed to add annotation to managedhub")
				}
				_, ok := curMh.GetAnnotations()[constants.AnnotationONMulticlusterHub]
				if !ok {
					return fmt.Errorf("failed to add annotation to managedhub, %v", curMh.GetAnnotations())
				}
				_, ok = curMh.GetAnnotations()[constants.AnnotationPolicyONMulticlusterHub]
				if !ok {
					return fmt.Errorf("failed to add annotation to managedhub%v", curMh.GetAnnotations())
				}
				return nil
			}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

			Expect(globalHubClient.Delete(ctx, mh)).To(Succeed())
			Expect(globalHubClient.Delete(ctx, globalhubAddon)).To(Succeed())
			Expect(globalHubClient.Delete(ctx, ns)).To(Succeed())
		})
	})

	Context("Cluster Managedcluster, should not have some annotation", func() {
		It("create managedhub cluster, should have annotation", func() {
			By("Create the managed cluster")
			mh_name := "test-mc-without-annotation"
			mh := &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: mh_name,
				},
				Spec: clusterv1.ManagedClusterSpec{
					HubAcceptsClient:     true,
					LeaseDurationSeconds: 60,
				},
			}
			Expect(globalHubClient.Create(ctx, mh)).To(Succeed())

			Consistently(func() error {
				curMh := &clusterv1.ManagedCluster{}
				Expect(globalHubClient.Get(ctx, types.NamespacedName{
					Name: mh_name,
				}, curMh)).To(Succeed())

				if len(curMh.Annotations) != 0 {
					return fmt.Errorf("should not add annotation to managedhub")
				}
				return nil
			}, 1*time.Minute, 1*time.Second).ShouldNot(HaveOccurred())

			Expect(globalHubClient.Delete(ctx, mh)).To(Succeed())
		})
	})
})

func updateClusterLabel(managedClusterName, labelStr string) error {
	leafhubName, _ := strings.CutSuffix(managedClusterName, "-cluster1")
	_, err := testClients.Kubectl(leafhubName, "label", "managedcluster", managedClusterName, labelStr)
	if err != nil {
		return err
	}
	return nil
}

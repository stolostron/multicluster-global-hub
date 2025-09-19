package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
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
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	CLUSTER_LABEL_KEY   = "label"
	CLUSTER_LABEL_VALUE = "test"
)

var _ = Describe("Managed Clusters", Label("e2e-test-cluster"), Ordered, func() {
	Context("Cluster Label", func() {
		It("add cluster label", func() {
			for _, managedCluster := range managedClusters {
				assertAddLabel(managedCluster, CLUSTER_LABEL_KEY, CLUSTER_LABEL_VALUE)
			}
		})

		It("remove cluster label", func() {
			for _, managedCluster := range managedClusters {
				assertRemoveLabel(managedCluster, CLUSTER_LABEL_KEY, CLUSTER_LABEL_VALUE)
			}
		})
	})

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

type patch struct {
	Op    string `json:"op" binding:"required"`
	Path  string `json:"path" binding:"required"`
	Value string `json:"value"`
}

func getManagedCluster(client *http.Client) ([]clusterv1.ManagedCluster, error) {
	managedClusterUrl := fmt.Sprintf("%s/global-hub-api/v1/managedclusters", testOptions.GlobalHub.Nonk8sApiServer)
	req, err := http.NewRequest("GET", managedClusterUrl, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("failed to close response body: %v", err)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var clusterList clusterv1.ManagedClusterList
	err = json.Unmarshal(body, &clusterList)
	if err != nil {
		return nil, err
	}

	return clusterList.Items, nil
}

func getManagedClusterByName(client *http.Client, managedClusterName string) (
	*clusterv1.ManagedCluster, error,
) {
	managedClusterUrl := fmt.Sprintf("%s/global-hub-api/v1/managedclusters",
		testOptions.GlobalHub.Nonk8sApiServer)
	req, err := http.NewRequest("GET", managedClusterUrl, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("failed to close response body: %v", err)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var managedClusterList clusterv1.ManagedClusterList
	err = json.Unmarshal(body, &managedClusterList)
	if err != nil {
		return nil, err
	}

	for _, managedCluster := range managedClusterList.Items {
		if managedCluster.Name == managedClusterName {
			return &managedCluster, nil
		}
	}

	return nil, nil
}

func assertAddLabel(cluster clusterv1.ManagedCluster, labelKey, labelVal string) {
	patches := []patch{
		{
			Op:    "add",
			Path:  "/metadata/labels/" + labelKey,
			Value: labelVal,
		},
	}

	By("Check the label is added")
	Eventually(func() error {
		err := updateClusterLabelByAPI(httpClient, patches, utils.GetClusterClaimID(&cluster, string(cluster.GetUID())))
		if err != nil {
			return err
		}
		managedCluster, err := getManagedClusterByName(httpClient, cluster.Name)
		if err != nil {
			return err
		}
		if managedCluster == nil {
			return fmt.Errorf("no managedcluster found")
		}
		if val, ok := managedCluster.Labels[labelKey]; ok {
			if labelVal == val {
				return nil
			}
		}
		return fmt.Errorf("failed to add label [%s: %s] to cluster %s, labels: %v", labelKey, labelVal, cluster.Name,
			cluster.Labels)
	}, 5*time.Minute, 10*time.Second).ShouldNot(HaveOccurred())
}

func assertRemoveLabel(cluster clusterv1.ManagedCluster, labelKey, labelVal string) {
	patches := []patch{
		{
			Op:    "remove",
			Path:  "/metadata/labels/" + labelKey,
			Value: labelVal,
		},
	}

	Eventually(func() error {
		err := updateClusterLabelByAPI(httpClient, patches, utils.GetClusterClaimID(&cluster, string(cluster.GetUID())))
		if err != nil {
			return err
		}
		managedCluster, err := getManagedClusterByName(httpClient, cluster.Name)
		if err != nil {
			return err
		}
		if val, ok := managedCluster.Labels[labelKey]; ok {
			if val == labelVal {
				return fmt.Errorf("the label %s:%s should be deleted from cluster %s", labelKey, labelVal, cluster.Name)
			}
		}
		return nil
	}, 5*time.Minute, 10*time.Second).ShouldNot(HaveOccurred())
}

func updateClusterLabelByAPI(client *http.Client, patches []patch, managedClusterID string) error {
	updateLabelUrl := fmt.Sprintf("%s/global-hub-api/v1/managedcluster/%s",
		testOptions.GlobalHub.Nonk8sApiServer, managedClusterID)
	// set method and body
	jsonBody, err := json.Marshal(patches)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("PATCH", updateLabelUrl, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}

	// add header
	req.Header.Add("Accept", "application/json")

	// do request
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			log.Printf("failed to close response body: %v", err)
		}
	}()
	return nil
}

func updateClusterLabel(managedClusterName, labelStr string) error {
	leafhubName, _ := strings.CutSuffix(managedClusterName, "-cluster1")
	_, err := testClients.Kubectl(leafhubName, "label", "managedcluster", managedClusterName, labelStr)
	if err != nil {
		return err
	}
	return nil
}

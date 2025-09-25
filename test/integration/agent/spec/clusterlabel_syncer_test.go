package spec

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/spec"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// go test ./test/integration/agent/spec -v -ginkgo.focus "ManagerClusterLabelBundle"
var _ = Describe("ManagerClusterLabelBundle", func() {
	It("sync managedclusterlabel bundle", func() {
		managedClusterName := "mc1"

		By("Create ManagedCluster on the managed hub")
		managedCluster := clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: managedClusterName,
				Labels: map[string]string{
					"vendor": "OpenShift",
				},
			},
			Spec: clusterv1.ManagedClusterSpec{
				HubAcceptsClient: true,
			},
		}
		Expect(runtimeClient.Create(ctx, &managedCluster)).NotTo(HaveOccurred())

		Eventually(func() error {
			mc := clusterv1.ManagedCluster{}
			return runtimeClient.Get(ctx, runtimeclient.ObjectKeyFromObject(&managedCluster), &mc)
		}, 5*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		By("Create ManagedClusterLabelBundle with labels")
		managedClusterLabelsSpecBundle := &spec.ManagedClusterLabelsSpecBundle{
			Objects:     []*spec.ManagedClusterLabelsSpec{},
			LeafHubName: leafHubName,
		}
		bundleObj := &spec.ManagedClusterLabelsSpec{
			ClusterName: managedClusterName,
			Labels: map[string]string{
				"test": "add",
			},
			DeletedLabelKeys: []string{},
			Version:          10,
			UpdateTimestamp:  time.Now(),
		}
		managedClusterLabelsSpecBundle.Objects = append(managedClusterLabelsSpecBundle.Objects, bundleObj)

		By("Send ManagedClusterLabelBundle by transport")
		payloadBytes, err := json.Marshal(managedClusterLabelsSpecBundle)
		Expect(err).NotTo(HaveOccurred())

		evt := utils.ToCloudEvent(constants.ManagedClustersLabelsMsgKey, constants.CloudEventGlobalHubClusterName,
			agentConfig.LeafHubName, payloadBytes)
		err = genericProducer.SendEvent(ctx, evt)
		Expect(err).NotTo(HaveOccurred())

		By("Check the managed cluster label")
		Eventually(func() error {
			mc := clusterv1.ManagedCluster{}
			err = runtimeClient.Get(ctx, runtimeclient.ObjectKeyFromObject(&managedCluster), &mc)
			if err != nil {
				return err
			}
			val := mc.GetLabels()["test"]
			if val == "add" {
				fmt.Println(mc.Labels)
				return nil
			}
			return fmt.Errorf("not found label on cluster { %s : %s}", "test", "add")
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})

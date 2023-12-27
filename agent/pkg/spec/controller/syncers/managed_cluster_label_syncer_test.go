package syncers_test

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/spec"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var _ = Describe("ManagerClusterLabel Bundle", func() {
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
		Expect(client.Create(ctx, &managedCluster)).NotTo(HaveOccurred())

		Eventually(func() error {
			mc := clusterv1.ManagedCluster{}
			return client.Get(ctx, runtimeclient.ObjectKeyFromObject(&managedCluster), &mc)
		}, 5*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		By("Create ManagedClusterLabelBundle with labels")
		managedClusterLabelsSpecBundle := &spec.ManagedClusterLabelsSpecBundle{
			Objects:     []*spec.ManagedClusterLabelsSpec{},
			LeafHubName: agentConfig.LeafHubName,
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
		err = producer.Send(ctx, &transport.Message{
			Source:  agentConfig.LeafHubName,
			Key:     constants.ManagedClustersLabelsMsgKey,
			MsgType: constants.SpecBundle,
			Payload: payloadBytes,
		})
		Expect(err).NotTo(HaveOccurred())

		By("Check the managed cluster label")
		Eventually(func() error {
			mc := clusterv1.ManagedCluster{}
			err = client.Get(ctx, runtimeclient.ObjectKeyFromObject(&managedCluster), &mc)
			if err != nil {
				return err
			}
			val := mc.GetLabels()["test"]
			if val == "add" {
				fmt.Println(mc.Labels)
				return nil
			}
			return fmt.Errorf("not found label { %s : %s}", "test", "add")
		}, 5*time.Second, 100*time.Microsecond).ShouldNot(HaveOccurred())
	})

	It("sync configmap bundle", func() {
		By("Create Config Bundle")
		baseBundle := bundle.NewBaseObjectsBundle()
		cm := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "hello",
				Namespace: "default",
			},
			Data: map[string]string{
				"hello": "world",
			},
		}
		baseBundle.AddObject(cm, uuid.New().String())

		By("Send Config Bundle by transport")
		payloadBytes, err := json.Marshal(baseBundle)
		Expect(err).NotTo(HaveOccurred())
		err = producer.Send(ctx, &transport.Message{
			Source:  transport.Broadcast,
			Key:     "Config",
			MsgType: constants.SpecBundle,
			Payload: payloadBytes,
		})
		Expect(err).NotTo(HaveOccurred())

		By("Check the configmap is synced")
		Eventually(func() error {
			syncedConfigMap := &corev1.ConfigMap{}
			return client.Get(ctx, runtimeclient.ObjectKeyFromObject(cm), syncedConfigMap)
		}, 5*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})

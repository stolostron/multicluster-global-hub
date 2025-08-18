// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package spec

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1beta2 "open-cluster-management.io/api/cluster/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var _ = Describe("managedclusterset controller", Ordered, func() {
	It("create the managedclusterset in kubernetes", func() {
		testManagedClusterSet := &clusterv1beta2.ManagedClusterSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-managedclusterset-1",
				Namespace: utils.GetDefaultNamespace(),
				Labels: map[string]string{
					constants.GlobalHubGlobalResourceLabel: "",
				},
			},
			Spec: clusterv1beta2.ManagedClusterSetSpec{
				ClusterSelector: clusterv1beta2.ManagedClusterSelector{
					SelectorType: clusterv1beta2.ExclusiveClusterSetLabel,
				},
			},
		}
		Expect(runtimeClient.Create(ctx, testManagedClusterSet, &client.CreateOptions{})).ToNot(HaveOccurred())
	})

	It("get the managedclusterset from postgres", func() {
		Eventually(func() error {
			rows, err := database.GetGorm().Raw("SELECT payload FROM spec.managedclustersets").Rows()
			if err != nil {
				return err
			}
			defer func() {
				if err := rows.Close(); err != nil {
					log.Printf("failed to close rows: %v", err)
				}
			}()
			for rows.Next() {
				var payload []byte
				err := rows.Scan(&payload)
				if err != nil {
					return err
				}
				gotManagedClusterSet := &clusterv1beta2.ManagedClusterSet{}
				if err := json.Unmarshal(payload, gotManagedClusterSet); err != nil {
					return err
				}
				if gotManagedClusterSet.Name == "test-managedclusterset-1" &&
					string(gotManagedClusterSet.Spec.ClusterSelector.SelectorType) ==
						string(clusterv1beta2.ExclusiveClusterSetLabel) {
					return nil
				}
			}
			return fmt.Errorf("not find managedclusterset in database")
		}, 1*time.Second).ShouldNot(HaveOccurred())
	})
})

// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1beta2 "open-cluster-management.io/api/cluster/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

var _ = Describe("managedclusterset controller", Ordered, func() {
	It("create the managedclusterset in kubernetes", func() {
		testManagedClusterSet := &clusterv1beta2.ManagedClusterSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-managedclusterset-1",
				Namespace: config.GetDefaultNamespace(),
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
		Expect(kubeClient.Create(ctx, testManagedClusterSet, &client.CreateOptions{})).ToNot(HaveOccurred())
	})

	It("get the managedclusterset from postgres", func() {
		Eventually(func() error {
			rows, err := postgresSQL.GetConn().Query(ctx,
				"SELECT payload FROM spec.managedclustersets")
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				gotManagedClusterSet := &clusterv1beta2.ManagedClusterSet{}
				if err := rows.Scan(gotManagedClusterSet); err != nil {
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

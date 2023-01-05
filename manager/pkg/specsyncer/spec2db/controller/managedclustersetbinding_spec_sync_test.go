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
)

var _ = Describe("managedclustersetbinding controller", Ordered, func() {
	It("create the spec.managedclustersetbinding table in database", func() {
		_, err := postgresSQL.GetConn().Exec(ctx, `
			CREATE SCHEMA IF NOT EXISTS spec;
			CREATE TABLE IF NOT EXISTS  spec.managedclustersetbindings (
				id uuid NOT NULL,
				payload jsonb NOT NULL,
				created_at timestamp without time zone DEFAULT now() NOT NULL,
				updated_at timestamp without time zone DEFAULT now() NOT NULL,
				deleted boolean DEFAULT false NOT NULL
			);
		`)
		Expect(err).ToNot(HaveOccurred())
	})

	It("create the managedclustersetbinding in kubernetes", func() {
		testManagedClusterSetBinding := &clusterv1beta2.ManagedClusterSetBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-managedclustersetbinding-1",
				Namespace: config.GetDefaultNamespace(),
			},
			Spec: clusterv1beta2.ManagedClusterSetBindingSpec{
				ClusterSet: "testing",
			},
		}
		Expect(kubeClient.Create(ctx, testManagedClusterSetBinding, &client.CreateOptions{})).ToNot(HaveOccurred())
	})

	It("get the managedclustersetbinding from postgres", func() {
		Eventually(func() error {
			rows, err := postgresSQL.GetConn().Query(ctx,
				"SELECT payload FROM spec.managedclustersetbindings")
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				gotManagedClusterSetBinding := &clusterv1beta2.ManagedClusterSetBinding{}
				if err := rows.Scan(gotManagedClusterSetBinding); err != nil {
					return err
				}
				if gotManagedClusterSetBinding.Name == "test-managedclustersetbinding-1" &&
					gotManagedClusterSetBinding.Spec.ClusterSet == "testing" {
					return nil
				}
			}
			return fmt.Errorf("not find managedclustersetbinding in database")
		}, 1*time.Second).ShouldNot(HaveOccurred())
	})
})

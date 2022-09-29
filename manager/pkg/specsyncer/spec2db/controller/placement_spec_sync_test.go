// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
)

var _ = Describe("placement controller", Ordered, func() {
	It("create the spec.placements table in database", func() {
		_, err := postgresSQL.GetConn().Exec(ctx, `
			CREATE SCHEMA IF NOT EXISTS spec;
			CREATE TABLE IF NOT EXISTS spec.placements (
				id uuid NOT NULL,
				payload jsonb NOT NULL,
				created_at timestamp without time zone DEFAULT now() NOT NULL,
				updated_at timestamp without time zone DEFAULT now() NOT NULL,
				deleted boolean DEFAULT false NOT NULL
			);
		`)
		Expect(err).ToNot(HaveOccurred())
	})

	It("create the placement in kubernetes", func() {
		testPlacement := &clusterv1beta1.Placement{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-placement-1",
				Namespace: config.GetDefaultNamespace(),
				Annotations: map[string]string{
					clusterv1beta1.PlacementDisableAnnotation: "true",
				},
			},
			Spec: clusterv1beta1.PlacementSpec{},
		}
		Expect(kubeClient.Create(ctx, testPlacement, &client.CreateOptions{})).ToNot(HaveOccurred())
	})

	It("get the placement from postgres", func() {
		Eventually(func() error {
			rows, err := postgresSQL.GetConn().Query(ctx,
				"SELECT payload FROM spec.placements")
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				placement := &clusterv1beta1.Placement{}
				if err := rows.Scan(placement); err != nil {
					return err
				}
				if placement.Name == "test-placement-1" && placement.Annotations == nil {
					return nil
				}
			}
			return fmt.Errorf("not find placement in database")
		}, 1*time.Second).ShouldNot(HaveOccurred())
	})
})

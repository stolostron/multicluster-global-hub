// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package dbsyncer_test

import (
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
)

var _ = Describe("PlacementRules", Ordered, func() {

	BeforeAll(func() {
		By("Create spec placementrule table in database")
		_, err := transportPostgreSQL.GetConn().Exec(ctx, `
			CREATE SCHEMA IF NOT EXISTS spec;
	
			CREATE TABLE IF NOT EXISTS  spec.placementrules (
				id uuid NOT NULL,
				payload jsonb NOT NULL,
				created_at timestamp without time zone DEFAULT now() NOT NULL,
				updated_at timestamp without time zone DEFAULT now() NOT NULL,
				deleted boolean DEFAULT false NOT NULL
			);
		`)
		Expect(err).ToNot(HaveOccurred())
	})

	It("Test placementrule is synced to the transport", func() {
		By("create placementrule")
		placementrule := &placementrulev1.PlacementRule{
			TypeMeta: metav1.TypeMeta{
				Kind:       "PlacementRule",
				APIVersion: "apps.open-cluster-management.io/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-placementrule",
				Namespace: "default",
			},
			Spec: placementrulev1.PlacementRuleSpec{},
		}
		payloadBytes, err := json.Marshal(placementrule)
		Expect(err).ToNot(HaveOccurred())

		_, err = transportPostgreSQL.GetConn().Exec(ctx,
			"INSERT INTO spec.placementrules (id,payload) VALUES($1, $2)",
			"0e0f7fc9-2dd4-42b5-b4a0-4a52f8fa8c83", &payloadBytes)
		Expect(err).ToNot(HaveOccurred())

		By("check the consumer can receive the bundle")
		Eventually(func() bool {
			_, ok := <-kafkaConsumer.GetGenericBundleChan()
			return ok
		}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())
	})
})

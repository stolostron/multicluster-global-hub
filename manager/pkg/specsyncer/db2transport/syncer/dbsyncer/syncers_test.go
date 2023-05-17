// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package dbsyncer_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var _ = Describe("Database to Transport Syncer", Ordered, func() {
	BeforeAll(func() {
		By("Create config table in spec schema")
		_, err := transportPostgreSQL.GetConn().Exec(ctx, `
			CREATE SCHEMA IF NOT EXISTS spec;

			CREATE TABLE IF NOT EXISTS  spec.configs (
				id uuid NOT NULL,
				payload jsonb NOT NULL,
				created_at timestamp without time zone DEFAULT now() NOT NULL,
				updated_at timestamp without time zone DEFAULT now() NOT NULL,
				deleted boolean DEFAULT false NOT NULL
			);
		`)
		Expect(err).ToNot(HaveOccurred())

		By("Create mangedcluster_label table in spec schema")
		_, err = transportPostgreSQL.GetConn().Exec(ctx, `
			CREATE TABLE IF NOT EXISTS  spec.managed_clusters_labels (
				id uuid NOT NULL,
				leaf_hub_name character varying(63) DEFAULT ''::character varying NOT NULL,
				managed_cluster_name character varying(63) NOT NULL,
				labels jsonb DEFAULT '{}'::jsonb NOT NULL,
				deleted_label_keys jsonb DEFAULT '[]'::jsonb NOT NULL,
				updated_at timestamp without time zone DEFAULT now() NOT NULL,
				version bigint DEFAULT 0 NOT NULL,
				CONSTRAINT managed_clusters_labels_version_check CHECK ((version >= 0))
			);
		`)
		Expect(err).ToNot(HaveOccurred())

		By("Create managedclusterset table in spec schema")
		_, err = transportPostgreSQL.GetConn().Exec(ctx, `
			CREATE TABLE IF NOT EXISTS  spec.managedclustersets (
				id uuid NOT NULL,
				payload jsonb NOT NULL,
				created_at timestamp without time zone DEFAULT now() NOT NULL,
				updated_at timestamp without time zone DEFAULT now() NOT NULL,
				deleted boolean DEFAULT false NOT NULL
			);
		`)
		Expect(err).ToNot(HaveOccurred())

		By("Create managedclustersetbinding table in spec schema")
		_, err = transportPostgreSQL.GetConn().Exec(ctx, `
			CREATE TABLE IF NOT EXISTS  spec.managedclustersetbindings (
				id uuid NOT NULL,
				payload jsonb NOT NULL,
				created_at timestamp without time zone DEFAULT now() NOT NULL,
				updated_at timestamp without time zone DEFAULT now() NOT NULL,
				deleted boolean DEFAULT false NOT NULL
			);
		`)
		Expect(err).ToNot(HaveOccurred())

		By("Create policy table in spec schema")
		_, err = transportPostgreSQL.GetConn().Exec(ctx, `
			CREATE TABLE IF NOT EXISTS  spec.policies (
				id uuid NOT NULL,
				payload jsonb NOT NULL,
				created_at timestamp without time zone DEFAULT now() NOT NULL,
				updated_at timestamp without time zone DEFAULT now() NOT NULL,
				deleted boolean DEFAULT false NOT NULL
			);
		`)
		Expect(err).ToNot(HaveOccurred())

		By("Create policy table in spec schema")
		_, err = transportPostgreSQL.GetConn().Exec(ctx, `
			CREATE TABLE IF NOT EXISTS  spec.policies (
				id uuid NOT NULL,
				payload jsonb NOT NULL,
				created_at timestamp without time zone DEFAULT now() NOT NULL,
				updated_at timestamp without time zone DEFAULT now() NOT NULL,
				deleted boolean DEFAULT false NOT NULL
			);
		`)
		Expect(err).ToNot(HaveOccurred())

		By("Create placementrule table in spec schema")
		_, err = transportPostgreSQL.GetConn().Exec(ctx, `
			CREATE TABLE IF NOT EXISTS  spec.placementrules (
				id uuid NOT NULL,
				payload jsonb NOT NULL,
				created_at timestamp without time zone DEFAULT now() NOT NULL,
				updated_at timestamp without time zone DEFAULT now() NOT NULL,
				deleted boolean DEFAULT false NOT NULL
			);
		`)
		Expect(err).ToNot(HaveOccurred())

		By("Create placementbinding table in spec schema")
		_, err = transportPostgreSQL.GetConn().Exec(ctx, `
			CREATE TABLE IF NOT EXISTS  spec.placementbindings (
				id uuid NOT NULL,
				payload jsonb NOT NULL,
				created_at timestamp without time zone DEFAULT now() NOT NULL,
				updated_at timestamp without time zone DEFAULT now() NOT NULL,
				deleted boolean DEFAULT false NOT NULL
			);
		`)
		Expect(err).ToNot(HaveOccurred())

		By("Create placement table in spec schema")
		_, err = transportPostgreSQL.GetConn().Exec(ctx, `
			CREATE TABLE IF NOT EXISTS  spec.placements (
				id uuid NOT NULL,
				payload jsonb NOT NULL,
				created_at timestamp without time zone DEFAULT now() NOT NULL,
				updated_at timestamp without time zone DEFAULT now() NOT NULL,
				deleted boolean DEFAULT false NOT NULL
			);
		`)
		Expect(err).ToNot(HaveOccurred())

		By("Create application table in spec schema")
		_, err = transportPostgreSQL.GetConn().Exec(ctx, `
			CREATE TABLE IF NOT EXISTS  spec.applications (
				id uuid NOT NULL,
				payload jsonb NOT NULL,
				created_at timestamp without time zone DEFAULT now() NOT NULL,
				updated_at timestamp without time zone DEFAULT now() NOT NULL,
				deleted boolean DEFAULT false NOT NULL
			);
		`)
		Expect(err).ToNot(HaveOccurred())

		By("Create channel table in spec schema")
		_, err = transportPostgreSQL.GetConn().Exec(ctx, `
			CREATE TABLE IF NOT EXISTS  spec.channels (
				id uuid NOT NULL,
				payload jsonb NOT NULL,
				created_at timestamp without time zone DEFAULT now() NOT NULL,
				updated_at timestamp without time zone DEFAULT now() NOT NULL,
				deleted boolean DEFAULT false NOT NULL
			);
		`)
		Expect(err).ToNot(HaveOccurred())

		By("Create subscription table in spec schema")
		_, err = transportPostgreSQL.GetConn().Exec(ctx, `
			CREATE TABLE IF NOT EXISTS  spec.subscriptions (
				id uuid NOT NULL,
				payload jsonb NOT NULL,
				created_at timestamp without time zone DEFAULT now() NOT NULL,
				updated_at timestamp without time zone DEFAULT now() NOT NULL,
				deleted boolean DEFAULT false NOT NULL
			);
		`)
		Expect(err).ToNot(HaveOccurred())
	})

	BeforeEach(func() {
		_, err := transportPostgreSQL.GetConn().Exec(ctx, "SELECT 1")
		fmt.Println("checking postgres...")
		Expect(err).ToNot(HaveOccurred())
	})

	It("Test config can be synced through transport", func() {
		By("create a config")
		_, err := transportPostgreSQL.GetConn().Exec(ctx,
			"INSERT INTO spec.configs (id,payload) VALUES($1, $2)",
			configUID, &configJSONBytes)
		Expect(err).ToNot(HaveOccurred())

		message := waitForChannel(genericConsumer.MessageChan())
		Expect(message.ID).Should(Equal("Config"))
		Expect(message.Payload).Should(ContainSubstring(configUID))
	})

	It("Test managedcluster labels can be synced through transport", func() {
		By("insert managed cluster labels to database")
		_, err := transportPostgreSQL.GetConn().Exec(ctx,
			`INSERT INTO spec.managed_clusters_labels (id, leaf_hub_name, managed_cluster_name, labels,
			deleted_label_keys, version, updated_at) values($1, $2, $3, $4::jsonb, $5::jsonb, 0, now())`,
			managedclusterUID, leafhubName, managedclusterName, labelsToAdd, labelKeysToRemove)
		Expect(err).ToNot(HaveOccurred())

		message := waitForChannel(genericConsumer.MessageChan())
		fmt.Printf("========== received ManagedClustersLabels: %s\n", message)
		Expect(message.ID).Should(Equal("ManagedClustersLabels"))
	})

	It("Test managedclusterset can be synced through transport", func() {
		By("create a managedclusterset")
		_, err := transportPostgreSQL.GetConn().Exec(ctx,
			"INSERT INTO spec.managedclustersets (id,payload) VALUES($1, $2)",
			managedclustersetUID, &managedclustersetJSONBytes)
		Expect(err).ToNot(HaveOccurred())

		message := waitForChannel(genericConsumer.MessageChan())
		fmt.Printf("========== received ManagedClusterSets: %s\n", message)
		Expect(message.ID).Should(Equal("ManagedClusterSets"))
		Expect(message.Payload).Should(ContainSubstring(managedclustersetUID))
	})

	It("Test managedclustersetbinding can be synced through transport", func() {
		By("create a managedclustersetbinding")
		_, err := transportPostgreSQL.GetConn().Exec(ctx,
			"INSERT INTO spec.managedclustersetbindings (id,payload) VALUES($1, $2)",
			managedclustersetbindingUID, &managedclustersetbindingJSONBytes)
		Expect(err).ToNot(HaveOccurred())

		message := waitForChannel(genericConsumer.MessageChan())
		fmt.Printf("========== received ManagedClusterSetBindings: %s\n", message)
		Expect(message.ID).Should(Equal("ManagedClusterSetBindings"))
		Expect(message.Payload).Should(ContainSubstring(managedclustersetbindingUID))
	})

	It("Test policy can be synced through transport", func() {
		By("create a policy")
		_, err := transportPostgreSQL.GetConn().Exec(ctx,
			"INSERT INTO spec.policies (id,payload) VALUES($1, $2)",
			policyUID, &policyJSONBytes)
		Expect(err).ToNot(HaveOccurred())

		message := waitForChannel(genericConsumer.MessageChan())
		fmt.Printf("========== received policy: %s\n", message)
		Expect(message.ID).Should(Equal("Policies"))
		Expect(message.Payload).Should(ContainSubstring(policyUID))
	})

	It("Test placementrule can be synced through transport", func() {
		By("create a placementrule")
		_, err := transportPostgreSQL.GetConn().Exec(ctx,
			"INSERT INTO spec.placementrules (id,payload) VALUES($1, $2)",
			placementruleUID, &placementruleJSONBytes)
		Expect(err).ToNot(HaveOccurred())

		message := waitForChannel(genericConsumer.MessageChan())
		fmt.Printf("========== received placementrule: %s\n", message)
		Expect(message.Payload).Should(ContainSubstring(placementruleUID))
	})

	It("Test placementbinding can be synced through transport", func() {
		By("create a placementbinding")
		_, err := transportPostgreSQL.GetConn().Exec(ctx,
			"INSERT INTO spec.placementbindings (id,payload) VALUES($1, $2)",
			placementbindingUID, &placementbindingJSONBytes)
		Expect(err).ToNot(HaveOccurred())

		message := waitForChannel(genericConsumer.MessageChan())
		fmt.Printf("========== received placementbinding: %s\n", message)
		Expect(message.Payload).Should(ContainSubstring(placementbindingUID))
	})

	It("Test placement can be synced through transport", func() {
		By("create a placement")
		_, err := transportPostgreSQL.GetConn().Exec(ctx,
			"INSERT INTO spec.placements (id,payload) VALUES($1, $2)",
			placementUID, &placementJSONBytes)
		Expect(err).ToNot(HaveOccurred())

		message := waitForChannel(genericConsumer.MessageChan())
		fmt.Printf("========== received placement: %s\n", message)
		Expect(message.Payload).Should(ContainSubstring(placementUID))
	})

	It("Test application can be synced through transport", func() {
		By("create a application")
		_, err := transportPostgreSQL.GetConn().Exec(ctx,
			"INSERT INTO spec.applications (id,payload) VALUES($1, $2)",
			applicationUID, &applicationJSONBytes)
		Expect(err).ToNot(HaveOccurred())

		message := waitForChannel(genericConsumer.MessageChan())
		fmt.Printf("========== received application: %s\n", message)
		Expect(message.Payload).Should(ContainSubstring(applicationUID))
	})

	It("Test subscription can be synced through transport", func() {
		By("create a subscription")
		_, err := transportPostgreSQL.GetConn().Exec(ctx,
			"INSERT INTO spec.subscriptions (id,payload) VALUES($1, $2)",
			subscriptionUID, &subscriptionJSONBytes)
		Expect(err).ToNot(HaveOccurred())

		message := waitForChannel(genericConsumer.MessageChan())
		fmt.Printf("========== received subscription: %s\n", message)
		Expect(message.Payload).Should(ContainSubstring(subscriptionUID))
	})

	It("Test channel can be synced through transport", func() {
		By("create a channel")
		_, err := transportPostgreSQL.GetConn().Exec(ctx,
			"INSERT INTO spec.channels (id,payload) VALUES($1, $2)",
			channelUID, &channelJSONBytes)
		Expect(err).ToNot(HaveOccurred())

		message := waitForChannel(genericConsumer.MessageChan())
		fmt.Printf("========== received channel: %s\n", message)
		Expect(message.Payload).Should(ContainSubstring(channelUID))
	})
})

// waitForChannel genericConsumer.MessageChan() with timeout
func waitForChannel(ch chan *transport.Message) *transport.Message {
	select {
	case msg := <-ch:
		return msg
	case <-time.After(10 * time.Second):
		fmt.Println("timeout waiting for message from  transport consumer channel")
		return nil
	}
}

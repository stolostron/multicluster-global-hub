// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package dbsyncer_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

	It("Test config can be synced through transport", func() {
		By("create a config")
		_, err := transportPostgreSQL.GetConn().Exec(ctx,
			"INSERT INTO spec.configs (id,payload) VALUES($1, $2)",
			configUID, &configJSONBytes)
		Expect(err).ToNot(HaveOccurred())

		By("check the consumer can receive the config from bundle")
		configUnstrObj := &unstructured.Unstructured{}
		// decode JSON into unstructured.Unstructured
		_, _, err = unstructured.UnstructuredJSONScheme.Decode(configJSONBytes, nil, configUnstrObj)
		Expect(err).ToNot(HaveOccurred())

		// retrieve bundle from bundle channel
		<-consumer.GetMessageChan()

		// fmt.Println("========================================")
		// fmt.Printf("%s", event)

		// genericBundle := <-kafkaConsumer.GetGenericBundleChan()
		// fmt.Printf("========== received bundle: %+v\n", genericBundle)
		// fmt.Printf("========== received bundle objects length: %d\n", len(genericBundle.Objects))
		// fmt.Printf("========== received bundle deleted objects length: %d\n", len(genericBundle.DeletedObjects))
		// Expect(len(genericBundle.Objects)).Should(Equal(1))
		// Expect(len(genericBundle.DeletedObjects)).Should(Equal(0))
		// fmt.Printf("========== want object: %+v\n", configUnstrObj)
		// fmt.Printf("========== got object: %+v\n", genericBundle.Objects[0])
		// Expect(configUnstrObj).Should(Equal(genericBundle.Objects[0]))
	})

	// It("Test managedcluster labels can be synced through transport", func() {
	// 	By("insert managed cluster labels to database")

	// 	_, err := transportPostgreSQL.GetConn().Exec(ctx,
	// 		`INSERT INTO spec.managed_clusters_labels (id, leaf_hub_name, managed_cluster_name, labels,
	// 		deleted_label_keys, version, updated_at) values($1, $2, $3, $4::jsonb, $5::jsonb, 0, now())`,
	// 		managedclusterUID, leafhubName, managedclusterName, labelsToAdd, labelKeysToRemove)

	// 	By("check the consumer can receive the managedcluster labels from bundle")
	// 	configUnstrObj := &unstructured.Unstructured{}
	// 	// decode JSON into unstructured.Unstructured
	// 	_, _, err = unstructured.UnstructuredJSONScheme.Decode(configJSONBytes, nil, configUnstrObj)
	// 	Expect(err).ToNot(HaveOccurred())

	// 	// retrieve bundle from bundle channel
	// 	customBundle := <-customBundleUpdatesChan
	// 	managedClusterLabelsBundle, ok := customBundle.(*specbundle.ManagedClusterLabelsSpecBundle)
	// 	Expect(ok).Should(BeTrue())
	// 	fmt.Printf("========== received managed cluster labels bundle for leafhub: %s\n", managedClusterLabelsBundle.LeafHubName)
	// 	fmt.Printf("========== received managed cluster labels bundle objects length: %d\n",
	// 		len(managedClusterLabelsBundle.Objects))
	// 	Expect(len(managedClusterLabelsBundle.Objects)).Should(Equal(1))
	// 	Expect(managedClusterLabelsBundle.LeafHubName).Should(Equal(leafhubName))
	// 	managedClusterLabelSpec := managedClusterLabelsBundle.Objects[0]
	// 	fmt.Printf("========== got object: %+v\n", managedClusterLabelSpec)
	// 	Expect(managedClusterLabelSpec.ClusterName).Should(Equal(managedclusterName))
	// 	Expect(managedClusterLabelSpec.Labels).Should(Equal(labelsToAdd))
	// 	Expect(managedClusterLabelSpec.DeletedLabelKeys).Should(Equal(labelKeysToRemove))
	// })

	// It("Test managedclusterset can be synced through transport", func() {
	// 	By("create a managedclusterset")
	// 	_, err := transportPostgreSQL.GetConn().Exec(ctx,
	// 		"INSERT INTO spec.managedclustersets (id,payload) VALUES($1, $2)",
	// 		managedclustersetUID, &managedclustersetJSONBytes)
	// 	Expect(err).ToNot(HaveOccurred())

	// 	By("check the consumer can receive the managedclusterset from bundle")
	// 	managedclustersetUnstrObj := &unstructured.Unstructured{}
	// 	// decode JSON into unstructured.Unstructured
	// 	_, _, err = unstructured.UnstructuredJSONScheme.Decode(managedclustersetJSONBytes, nil, managedclustersetUnstrObj)
	// 	Expect(err).ToNot(HaveOccurred())

	// 	// retrieve bundle from bundle channel
	// 	genericBundle := <-kafkaConsumer.GetGenericBundleChan()
	// 	fmt.Printf("========== received bundle: %+v\n", genericBundle)
	// 	fmt.Printf("========== received bundle objects length: %d\n", len(genericBundle.Objects))
	// 	fmt.Printf("========== received bundle deleted objects length: %d\n", len(genericBundle.DeletedObjects))
	// 	Expect(len(genericBundle.Objects)).Should(Equal(1))
	// 	Expect(len(genericBundle.DeletedObjects)).Should(Equal(0))
	// 	fmt.Printf("========== want object: %+v\n", managedclustersetUnstrObj)
	// 	fmt.Printf("========== got object: %+v\n", genericBundle.Objects[0])
	// 	Expect(managedclustersetUnstrObj).Should(Equal(genericBundle.Objects[0]))
	// })

	// It("Test managedclustersetbinding can be synced through transport", func() {
	// 	By("create a managedclustersetbinding")
	// 	_, err := transportPostgreSQL.GetConn().Exec(ctx,
	// 		"INSERT INTO spec.managedclustersetbindings (id,payload) VALUES($1, $2)",
	// 		managedclustersetbindingUID, &managedclustersetbindingJSONBytes)
	// 	Expect(err).ToNot(HaveOccurred())

	// 	By("check the consumer can receive the managedclustersetbinding from bundle")
	// 	managedclustersetbindingUnstrObj := &unstructured.Unstructured{}
	// 	// decode JSON into unstructured.Unstructured
	// 	_, _, err = unstructured.UnstructuredJSONScheme.Decode(
	// 		managedclustersetbindingJSONBytes, nil, managedclustersetbindingUnstrObj)
	// 	Expect(err).ToNot(HaveOccurred())

	// 	// retrieve bundle from bundle channel
	// 	genericBundle := <-kafkaConsumer.GetGenericBundleChan()
	// 	fmt.Printf("========== received bundle: %+v\n", genericBundle)
	// 	fmt.Printf("========== received bundle objects length: %d\n", len(genericBundle.Objects))
	// 	fmt.Printf("========== received bundle deleted objects length: %d\n", len(genericBundle.DeletedObjects))
	// 	Expect(len(genericBundle.Objects)).Should(Equal(1))
	// 	Expect(len(genericBundle.DeletedObjects)).Should(Equal(0))
	// 	fmt.Printf("========== want object: %+v\n", managedclustersetbindingUnstrObj)
	// 	fmt.Printf("========== got object: %+v\n", genericBundle.Objects[0])
	// 	Expect(managedclustersetbindingUnstrObj).Should(Equal(genericBundle.Objects[0]))
	// })

	// It("Test policy can be synced through transport", func() {
	// 	By("create a policy")
	// 	_, err := transportPostgreSQL.GetConn().Exec(ctx,
	// 		"INSERT INTO spec.policies (id,payload) VALUES($1, $2)",
	// 		policyUID, &policyJSONBytes)
	// 	Expect(err).ToNot(HaveOccurred())

	// 	By("check the consumer can receive the policy from bundle")
	// 	policyUnstrObj := &unstructured.Unstructured{}
	// 	// decode JSON into unstructured.Unstructured
	// 	_, _, err = unstructured.UnstructuredJSONScheme.Decode(policyJSONBytes, nil, policyUnstrObj)
	// 	Expect(err).ToNot(HaveOccurred())

	// 	// retrieve bundle from bundle channel
	// 	genericBundle := <-kafkaConsumer.GetGenericBundleChan()
	// 	fmt.Printf("========== received bundle: %+v\n", genericBundle)
	// 	fmt.Printf("========== received bundle objects length: %d\n", len(genericBundle.Objects))
	// 	fmt.Printf("========== received bundle deleted objects length: %d\n", len(genericBundle.DeletedObjects))
	// 	Expect(len(genericBundle.Objects)).Should(Equal(1))
	// 	Expect(len(genericBundle.DeletedObjects)).Should(Equal(0))
	// 	fmt.Printf("========== want object: %+v\n", policyUnstrObj)
	// 	fmt.Printf("========== got object: %+v\n", genericBundle.Objects[0])
	// 	Expect(policyUnstrObj).Should(Equal(genericBundle.Objects[0]))
	// })

	// It("Test placementrule can be synced through transport", func() {
	// 	By("create a placementrule")
	// 	_, err := transportPostgreSQL.GetConn().Exec(ctx,
	// 		"INSERT INTO spec.placementrules (id,payload) VALUES($1, $2)",
	// 		placementruleUID, &placementruleJSONBytes)
	// 	Expect(err).ToNot(HaveOccurred())

	// 	By("check the consumer can receive the placement from bundle")
	// 	placementruleUnstrObj := &unstructured.Unstructured{}
	// 	// decode JSON into unstructured.Unstructured
	// 	_, _, err = unstructured.UnstructuredJSONScheme.Decode(placementruleJSONBytes, nil, placementruleUnstrObj)
	// 	Expect(err).ToNot(HaveOccurred())

	// 	// retrieve bundle from bundle channel
	// 	genericBundle := <-kafkaConsumer.GetGenericBundleChan()
	// 	fmt.Printf("========== received bundle: %+v\n", genericBundle)
	// 	fmt.Printf("========== received bundle objects length: %d\n", len(genericBundle.Objects))
	// 	fmt.Printf("========== received bundle deleted objects length: %d\n", len(genericBundle.DeletedObjects))
	// 	Expect(len(genericBundle.Objects)).Should(Equal(1))
	// 	Expect(len(genericBundle.DeletedObjects)).Should(Equal(0))
	// 	fmt.Printf("========== want object: %+v\n", placementruleUnstrObj)
	// 	fmt.Printf("========== got object: %+v\n", genericBundle.Objects[0])
	// 	Expect(placementruleUnstrObj).Should(Equal(genericBundle.Objects[0]))
	// })

	// It("Test placementbinding can be synced through transport", func() {
	// 	By("create a placementbinding")
	// 	_, err := transportPostgreSQL.GetConn().Exec(ctx,
	// 		"INSERT INTO spec.placementbindings (id,payload) VALUES($1, $2)",
	// 		placementbindingUID, &placementbindingJSONBytes)
	// 	Expect(err).ToNot(HaveOccurred())

	// 	By("check the consumer can receive the placement from bundle")
	// 	placementbindingUnstrObj := &unstructured.Unstructured{}
	// 	// decode JSON into unstructured.Unstructured
	// 	_, _, err = unstructured.UnstructuredJSONScheme.Decode(placementbindingJSONBytes, nil, placementbindingUnstrObj)
	// 	Expect(err).ToNot(HaveOccurred())

	// 	// retrieve bundle from bundle channel
	// 	genericBundle := <-kafkaConsumer.GetGenericBundleChan()
	// 	fmt.Printf("========== received bundle: %+v\n", genericBundle)
	// 	fmt.Printf("========== received bundle objects length: %d\n", len(genericBundle.Objects))
	// 	fmt.Printf("========== received bundle deleted objects length: %d\n", len(genericBundle.DeletedObjects))
	// 	Expect(len(genericBundle.Objects)).Should(Equal(1))
	// 	Expect(len(genericBundle.DeletedObjects)).Should(Equal(0))
	// 	fmt.Printf("========== want object: %+v\n", placementbindingUnstrObj)
	// 	fmt.Printf("========== got object: %+v\n", genericBundle.Objects[0])
	// 	Expect(placementbindingUnstrObj).Should(Equal(genericBundle.Objects[0]))
	// })

	// It("Test placement can be synced through transport", func() {
	// 	By("create a placement")
	// 	_, err := transportPostgreSQL.GetConn().Exec(ctx,
	// 		"INSERT INTO spec.placements (id,payload) VALUES($1, $2)",
	// 		placementUID, &placementJSONBytes)
	// 	Expect(err).ToNot(HaveOccurred())

	// 	By("check the consumer can receive the placement from bundle")
	// 	placementUnstrObj := &unstructured.Unstructured{}
	// 	// decode JSON into unstructured.Unstructured
	// 	_, _, err = unstructured.UnstructuredJSONScheme.Decode(placementJSONBytes, nil, placementUnstrObj)
	// 	Expect(err).ToNot(HaveOccurred())

	// 	// retrieve bundle from bundle channel
	// 	genericBundle := <-kafkaConsumer.GetGenericBundleChan()
	// 	fmt.Printf("========== received bundle: %+v\n", genericBundle)
	// 	fmt.Printf("========== received bundle objects length: %d\n", len(genericBundle.Objects))
	// 	fmt.Printf("========== received bundle deleted objects length: %d\n", len(genericBundle.DeletedObjects))
	// 	Expect(len(genericBundle.Objects)).Should(Equal(1))
	// 	Expect(len(genericBundle.DeletedObjects)).Should(Equal(0))
	// 	fmt.Printf("========== want object: %+v\n", placementUnstrObj)
	// 	fmt.Printf("========== got object: %+v\n", genericBundle.Objects[0])
	// 	Expect(placementUnstrObj).Should(Equal(genericBundle.Objects[0]))
	// })

	// It("Test application can be synced through transport", func() {
	// 	By("create a application")
	// 	_, err := transportPostgreSQL.GetConn().Exec(ctx,
	// 		"INSERT INTO spec.applications (id,payload) VALUES($1, $2)",
	// 		applicationUID, &applicationJSONBytes)
	// 	Expect(err).ToNot(HaveOccurred())

	// 	By("check the consumer can receive the application from bundle")
	// 	applicationUnstrObj := &unstructured.Unstructured{}
	// 	// decode JSON into unstructured.Unstructured
	// 	_, _, err = unstructured.UnstructuredJSONScheme.Decode(applicationJSONBytes, nil, applicationUnstrObj)
	// 	Expect(err).ToNot(HaveOccurred())

	// 	// retrieve bundle from bundle channel
	// 	genericBundle := <-kafkaConsumer.GetGenericBundleChan()
	// 	fmt.Printf("========== received bundle: %+v\n", genericBundle)
	// 	fmt.Printf("========== received bundle objects length: %d\n", len(genericBundle.Objects))
	// 	fmt.Printf("========== received bundle deleted objects length: %d\n", len(genericBundle.DeletedObjects))
	// 	Expect(len(genericBundle.Objects)).Should(Equal(1))
	// 	Expect(len(genericBundle.DeletedObjects)).Should(Equal(0))
	// 	fmt.Printf("========== want object: %+v\n", applicationUnstrObj)
	// 	fmt.Printf("========== got object: %+v\n", genericBundle.Objects[0])
	// 	Expect(applicationUnstrObj).Should(Equal(genericBundle.Objects[0]))
	// })

	// It("Test subscription can be synced through transport", func() {
	// 	By("create a subscription")
	// 	_, err := transportPostgreSQL.GetConn().Exec(ctx,
	// 		"INSERT INTO spec.subscriptions (id,payload) VALUES($1, $2)",
	// 		subscriptionUID, &subscriptionJSONBytes)
	// 	Expect(err).ToNot(HaveOccurred())

	// 	By("check the consumer can receive the subscription from bundle")
	// 	subscriptionUnstrObj := &unstructured.Unstructured{}
	// 	// decode JSON into unstructured.Unstructured
	// 	_, _, err = unstructured.UnstructuredJSONScheme.Decode(subscriptionJSONBytes, nil, subscriptionUnstrObj)
	// 	Expect(err).ToNot(HaveOccurred())

	// 	// retrieve bundle from bundle channel
	// 	genericBundle := <-kafkaConsumer.GetGenericBundleChan()
	// 	fmt.Printf("========== received bundle: %+v\n", genericBundle)
	// 	fmt.Printf("========== received bundle objects length: %d\n", len(genericBundle.Objects))
	// 	fmt.Printf("========== received bundle deleted objects length: %d\n", len(genericBundle.DeletedObjects))
	// 	Expect(len(genericBundle.Objects)).Should(Equal(1))
	// 	Expect(len(genericBundle.DeletedObjects)).Should(Equal(0))
	// 	fmt.Printf("========== want object: %+v\n", subscriptionUnstrObj)
	// 	fmt.Printf("========== got object: %+v\n", genericBundle.Objects[0])
	// 	Expect(subscriptionUnstrObj).Should(Equal(genericBundle.Objects[0]))
	// })

	// It("Test channel can be synced through transport", func() {
	// 	By("create a channel")
	// 	_, err := transportPostgreSQL.GetConn().Exec(ctx,
	// 		"INSERT INTO spec.channels (id,payload) VALUES($1, $2)",
	// 		channelUID, &channelJSONBytes)
	// 	Expect(err).ToNot(HaveOccurred())

	// 	By("check the consumer can receive the channel from bundle")
	// 	channelUnstrObj := &unstructured.Unstructured{}
	// 	// decode JSON into unstructured.Unstructured
	// 	_, _, err = unstructured.UnstructuredJSONScheme.Decode(channelJSONBytes, nil, channelUnstrObj)
	// 	Expect(err).ToNot(HaveOccurred())

	// 	// retrieve bundle from bundle channel
	// 	genericBundle := <-kafkaConsumer.GetGenericBundleChan()
	// 	fmt.Printf("========== received bundle: %+v\n", genericBundle)
	// 	fmt.Printf("========== received bundle objects length: %d\n", len(genericBundle.Objects))
	// 	fmt.Printf("========== received bundle deleted objects length: %d\n", len(genericBundle.DeletedObjects))
	// 	Expect(len(genericBundle.Objects)).Should(Equal(1))
	// 	Expect(len(genericBundle.DeletedObjects)).Should(Equal(0))
	// 	fmt.Printf("========== want object: %+v\n", channelUnstrObj)
	// 	fmt.Printf("========== got object: %+v\n", genericBundle.Objects[0])
	// 	Expect(channelUnstrObj).Should(Equal(genericBundle.Objects[0]))
	// })
})

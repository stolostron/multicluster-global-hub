// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package dbsyncer_test

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var _ = Describe("Database to Transport Syncer", Ordered, func() {
	var db *gorm.DB
	BeforeEach(func() {
		db = database.GetGorm()
		err := db.Exec("SELECT 1").Error
		fmt.Println("checking postgres...")
		Expect(err).ToNot(HaveOccurred())
	})

	It("Test managedcluster labels can be synced through transport", func() {
		By("insert managed cluster labels to database")
		labelPayload, err := json.Marshal(labelsToAdd)
		Expect(err).Should(Succeed())

		labelKeysToRemovePayload, err := json.Marshal(labelKeysToRemove)
		Expect(err).Should(Succeed())

		err = db.Create(&models.ManagedClusterLabel{
			ID:                 managedclusterUID,
			LeafHubName:        leafhubName,
			ManagedClusterName: managedclusterName,
			Labels:             labelPayload,
			DeletedLabelKeys:   labelKeysToRemovePayload,
			Version:            0,
		}).Error
		Expect(err).ToNot(HaveOccurred())

		message := waitForChannel(genericConsumer.MessageChan())
		fmt.Printf("========== received ManagedClustersLabels: %s\n", message)
		Expect(message.ID).Should(Equal("ManagedClustersLabels"))
	})

	It("Test managedclusterset can be synced through transport", func() {
		By("create a managedclusterset")
		err := db.Exec(
			"INSERT INTO spec.managedclustersets (id, payload) VALUES(?, ?)",
			managedclustersetUID, managedclustersetJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		message := waitForChannel(genericConsumer.MessageChan())
		fmt.Printf("========== received ManagedClusterSets: %s\n", message)
		Expect(message.ID).Should(Equal("ManagedClusterSets"))
		Expect(message.Payload).Should(ContainSubstring(managedclustersetUID))
	})

	It("Test managedclustersetbinding can be synced through transport", func() {
		By("create a managedclustersetbinding")
		err := db.Exec("INSERT INTO spec.managedclustersetbindings (id,payload) VALUES(?, ?)",
			managedclustersetbindingUID, &managedclustersetbindingJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		message := waitForChannel(genericConsumer.MessageChan())
		fmt.Printf("========== received ManagedClusterSetBindings: %s\n", message)
		Expect(message.ID).Should(Equal("ManagedClusterSetBindings"))
		Expect(message.Payload).Should(ContainSubstring(managedclustersetbindingUID))
	})

	It("Test policy can be synced through transport", func() {
		By("create a policy")
		err := db.Exec(
			"INSERT INTO spec.policies (id,payload) VALUES(?, ?)",
			policyUID, &policyJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		message := waitForChannel(genericConsumer.MessageChan())
		fmt.Printf("========== received policy: %s\n", message)
		Expect(message.ID).Should(Equal("Policies"))
		Expect(message.Payload).Should(ContainSubstring(policyUID))
	})

	It("Test placementrule can be synced through transport", func() {
		By("create a placementrule")
		err := db.Exec(
			"INSERT INTO spec.placementrules (id,payload) VALUES(?, ?)",
			placementruleUID, &placementruleJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		message := waitForChannel(genericConsumer.MessageChan())
		fmt.Printf("========== received placementrule: %s\n", message)
		Expect(message.Payload).Should(ContainSubstring(placementruleUID))
	})

	It("Test placementbinding can be synced through transport", func() {
		By("create a placementbinding")
		err := db.Exec(
			"INSERT INTO spec.placementbindings (id,payload) VALUES(?, ?)",
			placementbindingUID, &placementbindingJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		message := waitForChannel(genericConsumer.MessageChan())
		fmt.Printf("========== received placementbinding: %s\n", message)
		Expect(message.Payload).Should(ContainSubstring(placementbindingUID))
	})

	It("Test placement can be synced through transport", func() {
		By("create a placement")
		err := db.Exec(
			"INSERT INTO spec.placements (id,payload) VALUES(?, ?)",
			placementUID, &placementJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		message := waitForChannel(genericConsumer.MessageChan())
		fmt.Printf("========== received placement: %s\n", message)
		Expect(message.Payload).Should(ContainSubstring(placementUID))
	})

	It("Test application can be synced through transport", func() {
		By("create a application")
		err := db.Exec(
			"INSERT INTO spec.applications (id,payload) VALUES(?, ?)",
			applicationUID, &applicationJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		message := waitForChannel(genericConsumer.MessageChan())
		fmt.Printf("========== received application: %s\n", message)
		Expect(message.Payload).Should(ContainSubstring(applicationUID))
	})

	It("Test subscription can be synced through transport", func() {
		By("create a subscription")
		err := db.Exec(
			"INSERT INTO spec.subscriptions (id,payload) VALUES(?, ?)",
			subscriptionUID, &subscriptionJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		message := waitForChannel(genericConsumer.MessageChan())
		fmt.Printf("========== received subscription: %s\n", message)
		Expect(message.Payload).Should(ContainSubstring(subscriptionUID))
	})

	It("Test channel can be synced through transport", func() {
		By("create a channel")
		err := db.Exec(
			"INSERT INTO spec.channels (id,payload) VALUES(?, ?)",
			channelUID, &channelJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		message := waitForChannel(genericConsumer.MessageChan())
		fmt.Printf("========== received channel: %s\n", message)
		Expect(message.Payload).Should(ContainSubstring(channelUID))
	})

	// It("Test managed cluster labels syncer", func() {
	// 	Eventually(func() error {
	// 		var managedClusterLabel models.ManagedClusterLabel
	// 		err := db.First(&managedClusterLabel).Error
	// 		if err != nil {
	// 			return err
	// 		}

	// 		deletedKeys := []string{}
	// 		err = json.Unmarshal(managedClusterLabel.DeletedLabelKeys, deletedKeys)
	// 		if err != nil {
	// 			return err
	// 		}
	// 		if len(deletedKeys) > 0 {
	// 			fmt.Println("deletedKeys", deletedKeys)
	// 			return nil
	// 		}
	// 		return fmt.Errorf("the labels haven't been synced")
	// 	}, 20*time.Second, 1*time.Second).ShouldNot(HaveOccurred())
	// })

	// 		By("Sync managed cluster labels")
	// 		Eventually(func() error {
	// 			var managedClusterLabels []models.ManagedClusterLabel
	// 			err := db.Where(&models.ManagedClusterLabel{
	// 				LeafHubName: leafHubName,
	// 			}).Find(&managedClusterLabels).Error
	// 			if err != nil {
	// 				return err
	// 			}
	// 			fmt.Println("managedClusterLabels------------", managedClusterLabels)
	// 			if len(managedClusterLabels) == 3 {
	// 				return nil
	// 			}
	// 			return fmt.Errorf("the labels haven't been synced")
	// 		}, 20*time.Second, 1*time.Second).ShouldNot(HaveOccurred())
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

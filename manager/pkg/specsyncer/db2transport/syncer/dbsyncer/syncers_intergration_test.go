// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package dbsyncer_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
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

	It("test resources can be synced through transport", func() {
		ExpectedMessageIDs := make(map[string]string)

		By("ManagedClusterLabels")
		ExpectedMessageIDs["ManagedClustersLabels"] = ""

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

		By("Resend hubinfo")
		ExpectedMessageIDs["Resync"] = constants.HubClusterInfoMsgKey

		By("ManagedClusterSet")
		ExpectedMessageIDs["ManagedClusterSets"] = managedclustersetUID
		err = db.Exec("INSERT INTO spec.managedclustersets (id, payload) VALUES(?, ?)",
			managedclustersetUID, managedclustersetJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		By("ManagedClustersetBinding")
		ExpectedMessageIDs["ManagedClusterSetBindings"] = managedclustersetbindingUID
		err = db.Exec("INSERT INTO spec.managedclustersetbindings (id,payload) VALUES(?, ?)", managedclustersetbindingUID,
			&managedclustersetbindingJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		By("Policy")
		ExpectedMessageIDs["Policies"] = policyUID
		err = db.Exec("INSERT INTO spec.policies (id,payload) VALUES(?, ?)", policyUID, &policyJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		By("Placementrule")
		ExpectedMessageIDs["PlacementRules"] = placementruleUID
		err = db.Exec("INSERT INTO spec.placementrules (id,payload) VALUES(?, ?)", placementruleUID,
			&placementruleJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		By("Placementbinding")
		ExpectedMessageIDs["PlacementBindings"] = placementbindingUID
		err = db.Exec("INSERT INTO spec.placementbindings (id,payload) VALUES(?, ?)", placementbindingUID,
			&placementbindingJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		By("Placement")
		ExpectedMessageIDs["Placements"] = placementUID
		err = db.Exec(
			"INSERT INTO spec.placements (id,payload) VALUES(?, ?)", placementUID, &placementJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		By("Application")
		ExpectedMessageIDs["Applications"] = applicationUID
		err = db.Exec(
			"INSERT INTO spec.applications (id,payload) VALUES(?, ?)", applicationUID, &applicationJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		By("Subscription")
		ExpectedMessageIDs["Subscriptions"] = subscriptionUID
		err = db.Exec("INSERT INTO spec.subscriptions (id,payload) VALUES(?, ?)", subscriptionUID,
			&subscriptionJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		By("Channel")
		ExpectedMessageIDs["Channels"] = channelUID
		err = db.Exec("INSERT INTO spec.channels (id,payload) VALUES(?, ?)", channelUID, &channelJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		By("verify the result from transport")
		Eventually(func() error {
			message := waitForChannel(genericConsumer.MessageChan())
			if val, ok := ExpectedMessageIDs[message.Key]; ok && strings.Contains(string(message.Payload), val) {
				fmt.Println("receive the expected message", message.Key)
				delete(ExpectedMessageIDs, message.Key)
			} else if !ok {
				fmt.Printf("get an unexpected message %s: %v \n", message.Key, message)
			}
			if len(ExpectedMessageIDs) > 0 {
				return fmt.Errorf("missing the message: %s", ExpectedMessageIDs)
			}
			return nil
		}, 15*time.Second, 1*time.Second).Should(Succeed())
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
})

// waitForChannel genericConsumer.MessageChan() with timeout
func waitForChannel(ch chan *transport.Message) *transport.Message {
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case msg := <-ch:
		return msg
	case <-timer.C:
		fmt.Println("timeout waiting for message from  transport consumer channel")
		return nil
	}
}

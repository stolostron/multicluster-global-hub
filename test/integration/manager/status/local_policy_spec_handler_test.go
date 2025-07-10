package status

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

// go test ./test/integration/manager/status -v -ginkgo.focus "LocalPolicySpecHandler"
var _ = Describe("LocalPolicySpecHandler", Ordered, func() {
	var version *eventversion.Version
	var policyID1 string
	var policyID2 string
	var leafHubName string

	BeforeEach(func() {
		if version == nil {
			version = eventversion.NewVersion()
		}
		if policyID1 == "" {
			policyID1 = uuid.New().String()
		}
		if policyID2 == "" {
			policyID2 = uuid.New().String()
		}

		leafHubName = "hub1"
	})

	It("should be able to sync local policy event", func() {
		By("Create event")
		version.Incr()

		bundle := generic.GenericBundle[policiesv1.Policy]{}
		policy1 := &policiesv1.Policy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testLocalPolicy1",
				Namespace: "default",
				UID:       types.UID(policyID1),
			},
			Spec: policiesv1.PolicySpec{
				Disabled: false,
			},
		}
		policy2 := &policiesv1.Policy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testLocalPolicy2",
				Namespace: "default",
				UID:       types.UID(policyID2),
			},
			Spec: policiesv1.PolicySpec{},
		}
		bundle.Create = []policiesv1.Policy{*policy1, *policy2}

		evt := ToCloudEvent(leafHubName, string(enum.LocalPolicySpecType), version, bundle)

		By("Sync event with transport")
		err := producer.SendEvent(ctx, *evt)
		version.Next()
		Expect(err).Should(Succeed())

		By("Check the leaf hubs table")
		Eventually(func() error {
			db := database.GetGorm()

			items := []models.LocalSpecPolicy{}
			if err := db.Where("leaf_hub_name = ?", leafHubName).Find(&items).Error; err != nil {
				return err
			}

			count := 0
			for _, item := range items {
				fmt.Println("PolicySpec Creating", item.LeafHubName, item.PolicyID, item.Payload)
				if item.PolicyID == policyID1 || item.PolicyID == policyID2 {
					count++
				}
			}
			if count != 2 {
				return fmt.Errorf("not found expected resource on the table, found %d", count)
			}
			return nil
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	It("should be able to update local policy specs", func() {
		By("Create update event")
		version.Incr()

		bundle := generic.GenericBundle[policiesv1.Policy]{}
		bundle.Update = []policiesv1.Policy{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testLocalPolicy1",
					Namespace: "default",
					UID:       types.UID(policyID1),
				},
				Spec: policiesv1.PolicySpec{
					Disabled: true,
				},
			},
		}
		evt := ToCloudEvent(leafHubName, string(enum.LocalPolicySpecType), version, bundle)

		By("Sync event with transport")
		err := producer.SendEvent(ctx, *evt)
		version.Next()
		Expect(err).Should(Succeed())

		By("Check the policy spec was updated")
		Eventually(func() error {
			db := database.GetGorm()

			var item models.LocalSpecPolicy
			if err := db.Where("leaf_hub_name = ? AND policy_id = ?", leafHubName, policyID1).First(&item).Error; err != nil {
				return err
			}

			policy := &policiesv1.Policy{}
			err := json.Unmarshal(item.Payload, policy)
			if err != nil {
				return err
			}

			if policy.Spec.Disabled {
				return nil
			}
			return fmt.Errorf("policy should be disabled")
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	It("should handle resync policy spec bundle", func() {
		By("Create resync event")
		version.Incr()

		bundle := generic.GenericBundle[policiesv1.Policy]{}
		bundle.ResyncMetadata = []generic.ObjectMetadata{
			{
				ID:        policyID1,
				Namespace: "default",
				Name:      "testLocalPolicy1",
			},
		}

		evt := ToCloudEvent(leafHubName, string(enum.LocalPolicySpecType), version, bundle)

		By("Sync event with transport")
		err := producer.SendEvent(ctx, *evt)
		version.Next()
		Expect(err).Should(Succeed())

		By("Check that policy2 is deleted")
		Eventually(func() error {
			db := database.GetGorm()

			var count int64
			if err := db.Model(&models.LocalSpecPolicy{}).Where("leaf_hub_name = ?", leafHubName).Count(&count).Error; err != nil {
				return err
			}

			items := []models.LocalSpecPolicy{}
			if err := db.Where("leaf_hub_name = ?", leafHubName).Find(&items).Error; err != nil {
				return err
			}

			reservedPolicy1 := false
			reservedPolicy2 := false
			for _, item := range items {
				if item.PolicyID == policyID1 {
					reservedPolicy1 = true
				}
				if item.PolicyID == policyID2 {
					reservedPolicy2 = true
				}
			}

			if !reservedPolicy1 {
				return fmt.Errorf("policy1 not found")
			}
			if reservedPolicy2 {
				return fmt.Errorf("policy2 should not be found")
			}

			return nil
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	// // delete policy1
	// It("should handle delete policy spec bundle", func() {
	// 	version.Incr()

	// 	bundle := generic.GenericBundle[policiesv1.Policy]{}
	// 	bundle.Delete = []generic.ObjectMetadata{
	// 		{
	// 			ID:        policyID1,
	// 			Namespace: "default",
	// 			Name:      "testLocalPolicy1",
	// 		},
	// 	}

	// 	evt := ToCloudEvent(leafHubName, string(enum.LocalPolicySpecType), version, bundle)

	// 	By("Sync event with transport")
	// 	err := producer.SendEvent(ctx, *evt)
	// 	version.Next()
	// 	Expect(err).Should(Succeed())

	// 	By("Check that policy1 is deleted")
	// 	Eventually(func() error {
	// 		db := database.GetGorm()

	// 		var count int64
	// 		if err := db.Model(&models.LocalSpecPolicy{}).Where("leaf_hub_name = ?", leafHubName).Count(&count).Error; err != nil {
	// 			return err
	// 		}

	// 		items := []models.LocalSpecPolicy{}
	// 		if err := db.Where("leaf_hub_name = ?", leafHubName).Find(&items).Error; err != nil {
	// 			return err
	// 		}

	// 		for _, item := range items {
	// 			if item.PolicyID == policyID1 {
	// 				return fmt.Errorf("policy1 should be deleted")
	// 			}
	// 		}
	// 		return nil
	// 	}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	// })
})

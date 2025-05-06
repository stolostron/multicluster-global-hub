// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package spec

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

var (
	leafhubName        = "hub1"
	ExpectedMessageIDs = map[string]string{
		"ManagedClustersLabels":     "",
		"ManagedClusterSets":        managedclustersetUID,
		"ManagedClusterSetBindings": managedclustersetbindingUID,
		"Policies":                  policyUID,
		"PlacementRules":            placementruleUID,
		"PlacementBindings":         placementbindingUID,
		"Placements":                placementUID,
		"Applications":              applicationUID,
		"Subscriptions":             subscriptionUID,
		"Channels":                  channelUID,
	}
)

type agentSyncer struct {
	eventType string
}

func newGenericAgentSyncer(eventType string) *agentSyncer {
	return &agentSyncer{eventType: eventType}
}

func (s *agentSyncer) Sync(ctx context.Context, evt *cloudevents.Event) error {
	payload := evt.Data()
	expectedResourceId := ExpectedMessageIDs[s.eventType]
	if strings.Contains(string(payload), expectedResourceId) {
		fmt.Println("agent spec sync the resource from manager: ", s.eventType)
		delete(ExpectedMessageIDs, s.eventType)
	}
	return nil
}

// go test ./test/integration/manager/spec -v -ginkgo.focus "Database to Transport Syncer"
var _ = Describe("Database to Transport Syncer", Ordered, func() {
	var db *gorm.DB
	BeforeEach(func() {
		db = database.GetGorm()
		err := db.Exec("SELECT 1").Error
		fmt.Println("checking postgres...")
		Expect(err).ToNot(HaveOccurred())
	})

	It("test resources can be synced through transport", func() {
		for eventType := range ExpectedMessageIDs {
			agentDispatcher.RegisterSyncer(eventType, newGenericAgentSyncer(eventType))
		}

		By("ManagedClusterLabels")
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

		By("ManagedClusterSet")
		err = db.Exec("INSERT INTO spec.managedclustersets (id, payload) VALUES(?, ?)",
			managedclustersetUID, managedclustersetJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		By("ManagedClustersetBinding")
		err = db.Exec("INSERT INTO spec.managedclustersetbindings (id,payload) VALUES(?, ?)", managedclustersetbindingUID,
			&managedclustersetbindingJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		By("Policy")
		err = db.Exec("INSERT INTO spec.policies (id,payload) VALUES(?, ?)", policyUID, &policyJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		By("Placementrule")
		err = db.Exec("INSERT INTO spec.placementrules (id,payload) VALUES(?, ?)", placementruleUID,
			&placementruleJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		By("Placementbinding")
		err = db.Exec("INSERT INTO spec.placementbindings (id,payload) VALUES(?, ?)", placementbindingUID,
			&placementbindingJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		By("Placement")
		err = db.Exec(
			"INSERT INTO spec.placements (id,payload) VALUES(?, ?)", placementUID, &placementJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		By("Application")
		err = db.Exec(
			"INSERT INTO spec.applications (id,payload) VALUES(?, ?)", applicationUID, &applicationJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		By("Subscription")
		err = db.Exec("INSERT INTO spec.subscriptions (id,payload) VALUES(?, ?)", subscriptionUID,
			&subscriptionJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		By("Channel")
		err = db.Exec("INSERT INTO spec.channels (id,payload) VALUES(?, ?)", channelUID, &channelJSONBytes).Error
		Expect(err).ToNot(HaveOccurred())

		By("Verify the result from transport")
		Eventually(func() error {
			if len(ExpectedMessageIDs) > 0 {
				return fmt.Errorf("not receive expect message: %s", ExpectedMessageIDs)
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(Succeed())
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

var (
	managedclusterUID  = uuid.New().String()
	managedclusterName = "mc1"
	labelsToAdd        = map[string]string{
		"foo": "bar",
		"env": "dev",
	}
)

var labelKeysToRemove = []string{
	"goo",
	"haa",
}

var (
	managedclustersetUID       = uuid.New().String()
	managedclustersetJSONBytes = []byte(fmt.Sprintf(`{
"apiVersion": "cluster.open-cluster-management.io/v1beta2",
"kind": "ManagedClusterSet",
"metadata": {
"name": "test-managedclusterset-1",
"namespace": "default",
"creationTimestamp": null,
"labels": {
	"global-hub.open-cluster-management.io/global-resource": ""
},
"annotations": {
"global-hub.open-cluster-management.io/origin-ownerreference-uid": "%s"
}
},
"spec": {
"clusterSelector": {
"selectorType": "ExclusiveClusterSetLabel"
}
},
"status": {
"conditions": []
}
}`, managedclustersetUID))
)

var (
	managedclustersetbindingUID       = uuid.New().String()
	managedclustersetbindingJSONBytes = []byte(fmt.Sprintf(`{
"apiVersion": "cluster.open-cluster-management.io/v1beta2",
"kind": "ManagedClusterSetBinding",
"metadata": {
"name": "test-managedclustersetbinding-1",
"namespace": "default",
"creationTimestamp": null,
"labels": {
	"global-hub.open-cluster-management.io/global-resource": ""
},
"annotations": {
"global-hub.open-cluster-management.io/origin-ownerreference-uid": "%s"
}
},
"spec": {
"clusterSet": "test-managedclusterset-1"
},
"status": {
"conditions": []
}
}`, managedclustersetbindingUID))
)

var (
	policyUID       = uuid.New().String()
	policyJSONBytes = []byte(fmt.Sprintf(`{
"apiVersion": "policy.open-cluster-management.io/v1",
"kind": "Policy",
"metadata": {
"name": "test-policy-1",
"namespace": "default",
"creationTimestamp": null,
"annotations": {
"global-hub.open-cluster-management.io/origin-ownerreference-uid": "%s",
"policy.open-cluster-management.io/standards": "NIST SP 800-53",
"policy.open-cluster-management.io/categories": "AU Audit and Accountability",
"policy.open-cluster-management.io/controls": "AU-3 Content of Audit Records"
},
"labels": {
"env": "production",
"global-hub.open-cluster-management.io/global-resource": ""
}
},
"spec": {
"disabled": false,
"policy-templates": []
},
"status": {}
}`, policyUID))
)

var (
	placementruleUID       = uuid.New().String()
	placementruleJSONBytes = []byte(fmt.Sprintf(`{
"apiVersion": "apps.open-cluster-management.io/v1",
"kind": "PlacementRule",
"metadata": {
"name": "test-placementrule-1",
"namespace": "default",
"creationTimestamp": null,
"labels": {
"global-hub.open-cluster-management.io/global-resource": ""
},
"annotations": {
"global-hub.open-cluster-management.io/origin-ownerreference-uid": "%s"
}
},
"spec": {},
"status": {}
}`, placementruleUID))
)

var (
	placementbindingUID       = uuid.New().String()
	placementbindingJSONBytes = []byte(fmt.Sprintf(`{
"apiVersion": "policy.open-cluster-management.io/v1",
"kind": "PlacementBinding",
"metadata": {
"name": "test-placementbinding-1",
"namespace": "default",
"creationTimestamp": null,
"labels": {
	"global-hub.open-cluster-management.io/global-resource": ""
	},
"annotations": {
"global-hub.open-cluster-management.io/origin-ownerreference-uid": "%s"
}
},
"placementRef": {
"name": "test-placementrule-1",
"kind": "PlacementRule",
"apiGroup": "apps.open-cluster-management.io"
},
"subjects": [
{
"name": "test-policy-1",
"kind": "Policy",
"apiGroup": "policy.open-cluster-management.io"
}
],
"status": {}
}`, placementbindingUID))
)

var (
	placementUID       = uuid.New().String()
	placementJSONBytes = []byte(fmt.Sprintf(`{
"apiVersion": "cluster.open-cluster-management.io/v1beta1",
"kind": "Placement",
"metadata": {
"name": "test-placement-1",
"namespace": "default",
"creationTimestamp": null,
"annotations": {
"global-hub.open-cluster-management.io/origin-ownerreference-uid": "%s"
},
"labels": {
"env": "production",
"global-hub.open-cluster-management.io/global-resource": ""
}
},
"spec": {
"prioritizerPolicy": {
"mode": "Additive"
}
},
"status": {
	"numberOfSelectedClusters": 0,
	"conditions": []
}
}`, placementUID))
)

var (
	applicationUID       = uuid.New().String()
	applicationJSONBytes = []byte(fmt.Sprintf(`{
"apiVersion": "app.k8s.io/v1beta1",
"kind": "Application",
"metadata": {
"name": "test-application-1",
"namespace": "default",
"labels": {
	"global-hub.open-cluster-management.io/global-resource": ""
	},
"annotations": {
"global-hub.open-cluster-management.io/origin-ownerreference-uid": "%s"
},
"creationTimestamp": null
},
"spec": {
"componentKinds": [
{
"group": "apps.open-cluster-management.io",
"kind": "Subscription"
}
],
"descriptor": {},
"selector": {
"matchExpressions": [
{
"key": "app",
"operator": "In",
"values": [
"helloworld-app"
]
}
]
}
},
"status": {}
}`, applicationUID))
)

var (
	channelUID       = uuid.New().String()
	channelJSONBytes = []byte(fmt.Sprintf(`{
"apiVersion": "apps.open-cluster-management.io/v1",
"kind": "Channel",
"metadata": {
"name": "test-channel-1",
"namespace": "default",
"labels": {
	"global-hub.open-cluster-management.io/global-resource": ""
	},
"annotations": {
"global-hub.open-cluster-management.io/origin-ownerreference-uid": "%s"
},
"creationTimestamp": null
},
"spec": {
"type": "GitHub",
"pathname": "https://github.com/open-cluster-management/application-samples.git"
},
"status": {}
}`, channelUID))
)

var (
	subscriptionUID       = uuid.New().String()
	subscriptionJSONBytes = []byte(fmt.Sprintf(`{
"apiVersion": "apps.open-cluster-management.io/v1",
"kind": "Subscription",
"metadata": {
"annotations": {
"global-hub.open-cluster-management.io/origin-ownerreference-uid": "%s",
"apps.open-cluster-management.io/git-branch": "main",
"apps.open-cluster-management.io/git-path": "bar",
"apps.open-cluster-management.io/reconcile-option": "merge"
},
"labels": {
"app": "bar",
"app.kubernetes.io/part-of": "bar",
"apps.open-cluster-management.io/reconcile-rate": "medium",
"global-hub.open-cluster-management.io/global-resource": ""
},
"name": "test-subscription-1",
"namespace": "default",
"creationTimestamp": null
},
"spec": {
"channel": "git-application-samples-ns/git-application-samples",
"placement": {
"placementRef": {
	"kind": "PlacementRule",
	"name": "test-placement-1"
}
}
},
"status": {
"lastUpdateTime": null,
"ansiblejobs": {}
}
}`, subscriptionUID))
)

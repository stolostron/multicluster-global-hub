package localpolicies

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/test/pkg/testpostgres"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	ctx    context.Context
	cancel context.CancelFunc
)

func TestMain(m *testing.M) {

	ctx, cancel = context.WithCancel(context.Background())
	testPostgres, err := testpostgres.NewTestPostgres()
	if err != nil {
		fmt.Printf("Error creating test PostgreSQL: %v\n", err)
		os.Exit(1)
	}

	defer func() {
		err := testPostgres.Stop()
		if err != nil {
			fmt.Printf("Error closing test PostgreSQL: %v\n", err)
			os.Exit(1)
		}
	}()

	err = testpostgres.InitDatabase(testPostgres.URI)
	if err != nil {
		fmt.Printf("Error initializing test database: %v\n", err)
		os.Exit(1)
	}

	exitCode := m.Run()

	os.Exit(exitCode)
}

func TestRootPolicyEventHandler(t *testing.T) {

	h := NewLocalRootPolicyEventHandler()
	assert.Equal(t, enum.LocalRootPolicyEventType, h.eventType)

	evt := cloudevents.NewEvent()
	evt.SetSource("hub1")
	evt.SetType(string(enum.LocalRootPolicyEventType))
	evt.SetExtension("kafkatopic", "event")
	evt.SetExtension("kafkaoffset", "8")
	evt.SetExtension("kafkapartition", "0")
	evt.SetExtension("kafkamessagekey", "hub1")
	evt.SetExtension("extversion", "0.1")
	evt.SetData(cloudevents.ApplicationJSON, []event.RootPolicyEvent{
		event.RootPolicyEvent{
			BaseEvent: event.BaseEvent{
				EventName:      "policy-limitrange.17b0db23b941f40b",
				EventNamespace: "local-policy-namespace",
				Message: `Policy local-policy-namespace/policy-limitrange was propagated to cluster 
				kind-hub1-cluster1/kind-hub1-cluster1`,
				Reason: "PolicyPropagation",
				Count:  1,
				Source: corev1.EventSource{
					Component: "policy-propagator",
				},
				CreatedAt: metav1.NewTime(time.Now()),
			},
			PolicyID:   "13b2e003-2bdf-4c82-9bdf-f1aa7ccf608d",
			Compliance: "NonCompliant",
		}})

	err := h.ToDatabase(evt)
	assert.Nil(t, err)

	db := database.GetGorm()
	var rootPolicyEvents []models.LocalRootPolicyEvent

	err = db.Find(&rootPolicyEvents).Error
	assert.Nil(t, err)

	assert.Equal(t, "policy-limitrange.17b0db23b941f40b", rootPolicyEvents[0].EventName)
}

func TestReplicatedPolicyEventHandler(t *testing.T) {

	h := NewLocalReplicatedPolicyEventHandler()
	assert.Equal(t, enum.LocalReplicatedPolicyEventType, h.eventType)

	evt := cloudevents.NewEvent()
	evt.SetSource("hub1")
	evt.SetType(string(enum.LocalRootPolicyEventType))
	evt.SetExtension("kafkatopic", "event")
	evt.SetExtension("kafkaoffset", "9")
	evt.SetExtension("kafkapartition", "0")
	evt.SetExtension("kafkamessagekey", "hub1")
	evt.SetExtension("extversion", "0.1")
	evt.SetData(cloudevents.ApplicationJSON, []event.ReplicatedPolicyEvent{
		event.ReplicatedPolicyEvent{
			BaseEvent: event.BaseEvent{
				EventName:      "local-policy-namespace.policy-limitrange.17b0db2427432200",
				EventNamespace: "kind-hub1-cluster1",
				Message: `NonCompliant; violation - limitranges [container-mem-limit-range] not found
				 in namespace default`,
				Reason: "PolicyStatusSync",
				Count:  1,
				Source: corev1.EventSource{
					Component: "policy-status-history-sync",
				},
				CreatedAt: metav1.NewTime(time.Now()),
			},
			PolicyID:   "13b2e003-2bdf-4c82-9bdf-f1aa7ccf608d",
			ClusterID:  "f302ce61-98e7-4d63-8dd2-65951e32fd95",
			Compliance: "NonCompliant",
		}})

	err := h.ToDatabase(evt)
	assert.Nil(t, err)

	db := database.GetGorm()
	var replicatedPolicyEvents []models.LocalClusterPolicyEvent

	err = db.Find(&replicatedPolicyEvents).Error
	assert.Nil(t, err)

	assert.Equal(t, "local-policy-namespace.policy-limitrange.17b0db2427432200", replicatedPolicyEvents[0].EventName)
}

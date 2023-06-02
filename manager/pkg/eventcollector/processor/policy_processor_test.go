package processor

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

var _ = Describe("configmaps to database controller", func() {
	const testSchema = "event"
	const testTable = "local_policies"

	BeforeEach(func() {
		By("Creating test table in the database")
		_, err := pool.Exec(ctx, `
			CREATE SCHEMA IF NOT EXISTS event;
			CREATE SCHEMA IF NOT EXISTS local_status;
			DO $$ BEGIN
				CREATE TYPE local_status.compliance_type AS ENUM (
					'compliant',
					'non_compliant',
					'unknown'
				);
			EXCEPTION
				WHEN duplicate_object THEN null;
			END $$;
			CREATE TABLE IF NOT EXISTS event.local_policies (
				policy_id uuid NOT NULL,
				cluster_id uuid NOT NULL,
				leaf_hub_name character varying(63) NOT NULL,
				message text,
				reason text,
				count integer NOT NULL DEFAULT 0,
				source jsonb,
				count integer NOT NULL DEFAULT 0,
				created_at timestamp without time zone DEFAULT now() NOT NULL,
				compliance local_status.compliance_type NOT NULL,
				CONSTRAINT local_policies_unique_constraint UNIQUE (policy_id, cluster_id, count)
			);
		`)
		Expect(err).ToNot(HaveOccurred())

		By("Check whether the table is created")
		Eventually(func() error {
			var tables []PGTable
			if err := g2.Table("pg_tables").Find(&tables).Error; err != nil {
				return err
			}

			for _, table := range tables {
				if table.Schemaname == testSchema && table.Tablename == testTable {
					return nil
				}
			}

			return fmt.Errorf("failed to create test table %s.%s", testSchema, testTable)
		}, 1*time.Second).ShouldNot(HaveOccurred())
	})

	It("sync the events to database", func() {
		By("Create a policy processor")
		policyProcessor := NewPolicyProcessor(ctx, &offsetManagerMock{})

		By("Create a kube.enhancedenvet")
		e := &kube.EnhancedEvent{
			Event: corev1.Event{
				Message: "foovar",
				ObjectMeta: metav1.ObjectMeta{
					Name:      "event1",
					Namespace: "default",
				},
				Reason: "foovar",
				Source: corev1.EventSource{
					Component: "foovar",
				},
				LastTimestamp: metav1.NewTime(time.Now()),
			},
			InvolvedObject: kube.EnhancedObjectReference{
				ObjectReference: corev1.ObjectReference{
					Kind:      string(policyv1.Kind),
					Name:      "managed-cluster-policy",
					Namespace: "cluster1",
				},
				Labels: map[string]string{
					constants.PolicyEventRootPolicyIdLabelKey: "37c9a640-af05-4bea-9dcc-1873e86bebcd",
					constants.PolicyEventClusterIdLabelKey:    "47c9a640-af05-4bea-9dcc-1873e86bebcd",
					constants.PolicyEventComplianceLabelKey:   string(policyv1.Compliant),
				},
			},
		}

		By("Process the event")
		policyProcessor.Process(e, &EventOffset{
			Topic:     "event",
			Offset:    0,
			Partition: 0,
		})

		By("Check whether the event is synced to the database")
		Eventually(func() error {
			var localPolicyEvents []models.LocalClusterPolicyEvent

			err := g2.Find(&localPolicyEvents).Error
			if err != nil {
				return err
			}

			for _, localPolicyEvent := range localPolicyEvents {
				if localPolicyEvent.PolicyID == "37c9a640-af05-4bea-9dcc-1873e86bebcd" &&
					localPolicyEvent.ClusterID == "47c9a640-af05-4bea-9dcc-1873e86bebcd" &&
					localPolicyEvent.Compliance == "compliant" {
					return nil
				}
			}
			return fmt.Errorf("not find event in database")
		}, 10*time.Second).ShouldNot(HaveOccurred())
	})
})

type PGTable struct {
	Schemaname  string
	Tablename   string
	Tableowner  string
	Tablespace  string
	Hasindexes  bool
	Hasrules    bool
	Hastriggers bool
}

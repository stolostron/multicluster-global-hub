package controller

import (
	"fmt"
	"time"

	"github.com/go-co-op/gocron"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/processes/cronjob/task"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

// go test ./test/integration/controller -v -ginkgo.focus "LocalComplianceHistory"
var _ = Describe("LocalComplianceHistory", Ordered, func() {
	It("sync the data from the event.local_policies to the history.local_compliance", func() {
		By("Create the data to the local_status.compliance table")
		err := db.Exec(`
		INSERT INTO "local_status"."compliance" ("policy_id", "cluster_name", "leaf_hub_name", "error",
		 "compliance", "cluster_id") VALUES
		('00000000-0000-0000-0000-000000000001', 'managedcluster-1', 'hub3', 'none', 'compliant',
		 '00000003-0000-0000-0000-000000000001'),
		('00000000-0000-0000-0000-000000000001', 'managedcluster-2', 'hub3', 'none', 'compliant', 
		'00000003-0000-0000-0000-000000000002'),
		('00000000-0000-0000-0000-000000000001', 'managedcluster-3', 'hub3', 'none', 'compliant', 
		'00000003-0000-0000-0000-000000000003'),
		('00000000-0000-0000-0000-000000000001', 'managedcluster-4', 'hub3', 'none', 'compliant', 
		'00000003-0000-0000-0000-000000000004'),
		('00000000-0000-0000-0000-000000000001', 'managedcluster-5', 'hub3', 'none', 'compliant', 
		'00000003-0000-0000-0000-000000000005');
		`).Error
		Expect(err).ToNot(HaveOccurred())

		By("Sync the local_status.compliance to history.local_compliance")
		By("Create the sync job")
		s := gocron.NewScheduler(time.UTC)
		complianceJob, err := s.Every(1).Day().DoWithJobDetails(task.LocalComplianceHistory, ctx)
		Expect(err).ToNot(HaveOccurred())
		fmt.Println("set local compliance job", "scheduleAt", complianceJob.ScheduledAtTime())
		s.StartAsync()
		defer s.Clear()

		By("Check whether the data is synced to the history.local_compliance table")
		Eventually(func() error {
			rows, err := db.Raw(`
			SELECT policy_id, cluster_id, compliance, compliance_date, compliance_changed_frequency
			FROM history.local_compliance`).Rows()
			if err != nil {
				return err
			}
			defer func() {
				if err := rows.Close(); err != nil {
					fmt.Printf("failed to close rows: %v\n", err)
				}
			}()

			syncCount := 0
			fmt.Println("found the following compliance history:")
			for rows.Next() {
				var policy_id, cluster_id, compliance string
				var compliance_date time.Time
				var compliance_changed_frequency int
				err := rows.Scan(&policy_id, &cluster_id, &compliance,
					&compliance_date, &compliance_changed_frequency)
				if err != nil {
					return err
				}
				fmt.Println(policy_id, cluster_id, compliance, compliance_date, compliance_changed_frequency)
				syncCount++
			}
			if syncCount != 5 {
				return fmt.Errorf("expected 5 items, but got %d in the local compliance history", syncCount)
			}
			return nil
		}, 10*time.Second, 2*time.Second).ShouldNot(HaveOccurred())

		By("Check whether the job log is created")
		Eventually(func() error {
			rows, err := db.Raw(`SELECT start_at, end_at, name, total, inserted, offsets, error FROM
				history.local_compliance_job_log`).Rows()
			if err != nil {
				return err
			}
			defer func() {
				if err := rows.Close(); err != nil {
					fmt.Printf("failed to close rows: %v\n", err)
				}
			}()

			logCount := 0
			fmt.Println("found the following compliance history job log:")
			for rows.Next() {
				var name, errMessage string
				var total, inserted, offsets int64
				var startAt, endAt time.Time
				err := rows.Scan(&startAt, &endAt, &name, &total, &inserted, &offsets, &errMessage)
				if err != nil {
					return err
				}
				logCount += 1
				format := "2006-01-02 15:04:05"
				fmt.Println(">>", startAt.Format(format), endAt.Format(format), name, total,
					inserted, offsets, errMessage)
			}
			if logCount < 1 {
				return fmt.Errorf("table history.local_compliance_job_log records are not synced")
			}
			return nil
		}, 10*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})

	It("Update the history compliance by event1", func() {
		By("Create the data to the event.local_policies table")
		policyEvent := models.LocalReplicatedPolicyEvent{
			BaseLocalPolicyEvent: models.BaseLocalPolicyEvent{
				EventName:   "default.rootpolicy-1.managedcluster-1.37695",
				PolicyID:    "00000000-0000-0000-0000-000000000001",
				LeafHubName: "hub3",
				Compliance:  "non_compliant",
				Message:     "Compliant; notification - limitranges container-mem-limit-range found as specified in namespace default",
				Reason:      "PolicyStatusSync",
				CreatedAt:   time.Now(),
			},
			ClusterID: "00000003-0000-0000-0000-000000000001",
		}
		err := db.Create(&policyEvent).Error
		Expect(err).ToNot(HaveOccurred())

		By("Check whether the data is updated to the history.local_compliance table")
		Eventually(func() error {
			compliance, frequency, err := findHistory("00000000-0000-0000-0000-000000000001",
				"00000003-0000-0000-0000-000000000001")
			if err != nil {
				return err
			}
			if compliance == "non_compliant" && frequency == 1 {
				return nil
			}
			return fmt.Errorf("expected non_compliant: 1, but got %s: %d", compliance, frequency)
		}, 10*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})

	It("Update the history compliance by event2", func() {
		By("Create the data to the event.local_policies table")
		policyEvent := models.LocalReplicatedPolicyEvent{
			BaseLocalPolicyEvent: models.BaseLocalPolicyEvent{
				EventName:   "default.rootpolicy-1.managedcluster-1.37696",
				PolicyID:    "00000000-0000-0000-0000-000000000001",
				LeafHubName: "hub3",
				Compliance:  "compliant",
				Message:     "Compliant; notification - limitranges container-mem-limit-range found as specified in namespace default",
				Reason:      "PolicyStatusSync",
				CreatedAt:   time.Now(),
			},
			ClusterID: "00000003-0000-0000-0000-000000000001",
		}
		err := db.Create(&policyEvent).Error
		Expect(err).ToNot(HaveOccurred())

		By("Check whether the data is updated to the history.local_compliance table")
		Eventually(func() error {
			compliance, frequency, err := findHistory("00000000-0000-0000-0000-000000000001",
				"00000003-0000-0000-0000-000000000001")
			if err != nil {
				return err
			}
			if compliance == "non_compliant" && frequency == 2 {
				return nil
			}
			return fmt.Errorf("expected non_compliant: 2, but got %s: %d", compliance, frequency)
		}, 10*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})

	It("Update the history compliance by event3", func() {
		By("Create the data to the event.local_policies table")
		policyEvent := models.LocalReplicatedPolicyEvent{
			BaseLocalPolicyEvent: models.BaseLocalPolicyEvent{
				EventName:   "default.rootpolicy-1.managedcluster-1.37697",
				PolicyID:    "00000000-0000-0000-0000-000000000001",
				LeafHubName: "hub3",
				Compliance:  "unknown",
				Message:     "Compliant; notification - limitranges container-mem-limit-range found as specified in namespace default",
				Reason:      "PolicyStatusSync",
				CreatedAt:   time.Now(),
			},
			ClusterID: "00000003-0000-0000-0000-000000000001",
		}
		err := db.Create(&policyEvent).Error
		Expect(err).ToNot(HaveOccurred())

		By("Check whether the data is updated to the history.local_compliance table")
		Eventually(func() error {
			compliance, frequency, err := findHistory("00000000-0000-0000-0000-000000000001",
				"00000003-0000-0000-0000-000000000001")
			if err != nil {
				return err
			}
			if compliance == "unknown" && frequency == 3 {
				return nil
			}
			return fmt.Errorf("expected unknown: 3, but got %s: %d", compliance, frequency)
		}, 10*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})
})

func findHistory(policyId, clusterId string) (string, int, error) {
	rows, err := db.Raw(`
			SELECT policy_id, cluster_id, compliance, compliance_date, compliance_changed_frequency
			FROM history.local_compliance`).Rows()
	if err != nil {
		return "", 0, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			fmt.Printf("failed to close rows: %v\n", err)
		}
	}()

	for rows.Next() {
		var policy_id, cluster_id, compliance string
		var compliance_date time.Time
		var compliance_changed_frequency int
		err := rows.Scan(&policy_id, &cluster_id, &compliance,
			&compliance_date, &compliance_changed_frequency)
		if err != nil {
			return "", 0, err
		}
		if policy_id == policyId && cluster_id == clusterId {
			fmt.Println(policy_id, cluster_id, compliance, compliance_date, compliance_changed_frequency)
			return compliance, compliance_changed_frequency, nil
		}
	}
	return "", 0, nil
}

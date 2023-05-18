package task_test

import (
	"fmt"
	"time"

	"github.com/go-co-op/gocron"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/cronjob/task"
)

var _ = Describe("sync the compliance data", Ordered, func() {
	const targetSchema = "local_status"
	const sourceTable = "compliance"
	const targetTable = "compliance_history"

	BeforeAll(func() {
		By("Creating test table in the database")
		conn, err := pool.Acquire(ctx)
		Expect(err).ToNot(HaveOccurred())
		defer conn.Release()
		_, err = conn.Exec(ctx, `
			CREATE SCHEMA IF NOT EXISTS local_status;
			CREATE SCHEMA IF NOT EXISTS status;
			DO $$ BEGIN
				CREATE TYPE local_status.compliance_type AS ENUM (
					'compliant',
					'non_compliant',
					'unknown'
				);
			EXCEPTION
				WHEN duplicate_object THEN null;
			END $$;
			DO $$ BEGIN
				CREATE TYPE status.error_type AS ENUM (
					'disconnected',
					'none'
				);
			EXCEPTION
				WHEN duplicate_object THEN null;
			END $$;
			CREATE TABLE IF NOT EXISTS local_status.compliance (
				id uuid NOT NULL,
				cluster_name character varying(63) NOT NULL,
				leaf_hub_name character varying(63) NOT NULL,
				error status.error_type NOT NULL,
				compliance local_status.compliance_type NOT NULL,
				cluster_id uuid
			);
			CREATE TABLE IF NOT EXISTS local_status.compliance_history (
				id uuid NOT NULL,
				cluster_id uuid NOT NULL,
				updated_at timestamp without time zone DEFAULT now() NOT NULL,
				compliance_date DATE DEFAULT (CURRENT_DATE - INTERVAL '1 day') NOT NULL,
				compliance local_status.compliance_type NOT NULL,
				compliance_changed_frequency integer NOT NULL DEFAULT 0
			);
			CREATE TABLE IF NOT EXISTS local_status.compliance_history_job_log (
				name varchar(63) NOT NULL,
				start_at timestamp NOT NULL DEFAULT now(),
				end_at timestamp NOT NULL DEFAULT now(),
				total int8,
				inserted int8,
				offsets int8, 
				error TEXT
			);`)
		Expect(err).ToNot(HaveOccurred())
		By("Check whether the tables are created")
		Eventually(func() error {
			rows, err := conn.Query(ctx, "SELECT * FROM pg_tables")
			if err != nil {
				return err
			}
			defer rows.Close()
			expectedTableSet := map[string]bool{
				sourceTable: true,
				targetTable: true,
			}

			for rows.Next() {
				columnValues, _ := rows.Values()
				schema := columnValues[0]
				table := columnValues[1]
				if schema == targetSchema && expectedTableSet[table.(string)] {
					delete(expectedTableSet, table.(string))
				}
			}
			if len(expectedTableSet) > 0 {
				return fmt.Errorf("tables %v are not created", expectedTableSet)
			}
			return nil
		}, 10*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})

	It("sync the data from the source table to the target", func() {
		By("Create the data to the source table")
		_, err := pool.Exec(ctx, `
			INSERT INTO local_status.compliance (id, cluster_name, leaf_hub_name, error, compliance, cluster_id) VALUES
			('f8c4479f-fec5-44d8-8060-da9a92d5e138', 'local_cluster1', 'leaf1', 'none', 'compliant', 
			'0cd723ab-4649-42e8-b8aa-aa094ccf06b4'),
			('f8c4479f-fec5-44d8-8060-da9a92d5e139', 'local_cluster2', 'leaf2', 'none', 'compliant', 
			'0cd723ab-4649-42e8-b8aa-aa094ccf06b4'),
			('f8c4479f-fec5-44d8-8060-da9a92d5e137', 'local_cluster3', 'leaf3', 'none', 'compliant', 
			'0cd723ab-4649-42e8-b8aa-aa094ccf06b4');
		`)
		Expect(err).ToNot(HaveOccurred())

		By("Create the sync job")
		s := gocron.NewScheduler(time.UTC)
		complianceJob, err := s.Every(1).Second().Tag("LocalCompliance").DoWithJobDetails(
			task.SyncLocalCompliance, ctx, pool, true)
		Expect(err).ToNot(HaveOccurred())
		fmt.Println("set local compliance job", "scheduleAt", complianceJob.ScheduledAtTime())
		s.StartAsync()
		defer s.Clear()

		By("Check whether the data is copied to the target table")
		Eventually(func() error {
			rows, err := pool.Query(ctx, "SELECT id, cluster_id, compliance FROM local_status.compliance_history")
			if err != nil {
				return err
			}
			defer rows.Close()
			syncCount := 0
			expectRecordMap := map[string]string{
				"f8c4479f-fec5-44d8-8060-da9a92d5e138": "compliant",
				"f8c4479f-fec5-44d8-8060-da9a92d5e139": "compliant",
				"f8c4479f-fec5-44d8-8060-da9a92d5e137": "compliant",
			}
			for rows.Next() {
				var id, cluster_id, compliance string
				err := rows.Scan(&id, &cluster_id, &compliance)
				if err != nil {
					return err
				}
				fmt.Println("found record", "id", id, "cluster_id", cluster_id, "compliance", compliance)
				if status, ok := expectRecordMap[id]; ok && status == compliance {
					syncCount++
					delete(expectRecordMap, id)
				}
			}
			if syncCount != 3 {
				return fmt.Errorf("table local_status.compliance_history records are not synced")
			}
			return nil
		}, 10*time.Second, 2*time.Second).ShouldNot(HaveOccurred())

		By("Check whether the job log is created")
		Eventually(func() error {
			rows, err := pool.Query(ctx, `SELECT start_at, end_at, name, total, inserted, offsets, error FROM 
			local_status.compliance_history_job_log`)
			if err != nil {
				return err
			}
			defer rows.Close()

			logCount := 0
			for rows.Next() {
				var name, errMessage string
				var total, inserted, offsets int64
				var startAt, endAt time.Time
				err := rows.Scan(&startAt, &endAt, &name, &total, &inserted, &offsets, &errMessage)
				if err != nil {
					return err
				}
				logCount += 1
				fmt.Println("found log", "startAt", startAt.Format("2006-01-02 15:04:05"),
					"endAt", endAt.Format("2006-01-02 15:04:05"), "name", name, "total", total, "inserted",
					inserted, "offsets", offsets, "err", errMessage)
			}
			if logCount < 1 {
				return fmt.Errorf("table local_status.compliance_history_job_log records are not synced")
			}
			return nil
		}, 10*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})
})

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
				compliance local_status.compliance_type NOT NULL
			);
			CREATE TABLE IF NOT EXISTS local_status.compliance_history (
					id uuid NOT NULL,
					cluster_name character varying(63) NOT NULL,
					leaf_hub_name character varying(63) NOT NULL,
					updated_at timestamp without time zone DEFAULT now() NOT NULL,
					compliance local_status.compliance_type NOT NULL
			);
		`)
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
			INSERT INTO local_status.compliance (id, cluster_name, leaf_hub_name, error, compliance) VALUES
			('f8c4479f-fec5-44d8-8060-da9a92d5e138', 'local_cluster1', 'leaf1', 'none', 'compliant'),
			('f8c4479f-fec5-44d8-8060-da9a92d5e138', 'local_cluster2', 'leaf2', 'none', 'compliant'),
			('f8c4479f-fec5-44d8-8060-da9a92d5e138', 'local_cluster3', 'leaf3', 'none', 'compliant');
		`)
		Expect(err).ToNot(HaveOccurred())

		By("Create the sync job")
		s := gocron.NewScheduler(time.UTC)
		complianceJob, err := s.Every(1).Second().Tag("LocalCompliance").DoWithJobDetails(
			task.SyncLocalCompliance, ctx, pool)
		Expect(err).ToNot(HaveOccurred())
		fmt.Println("set local compliance job", "scheduleAt", complianceJob.ScheduledAtTime())
		s.StartAsync()
		defer s.Clear()

		By("Check whether the data is copied to the target table")
		Eventually(func() error {
			rows, err := pool.Query(ctx, "SELECT * FROM local_status.compliance_history")
			if err != nil {
				return err
			}
			defer rows.Close()
			expectedRecordsSet := map[string]bool{
				"local_cluster1": true,
				"local_cluster2": true,
				"local_cluster3": true,
			}
			syncCount := 0
			for rows.Next() {
				columnValues, _ := rows.Values()
				clusterName := columnValues[1]
				fmt.Println("found record", "clusterName", clusterName)
				if expectedRecordsSet[clusterName.(string)] {
					syncCount++
					delete(expectedRecordsSet, clusterName.(string))
				}
			}
			if len(expectedRecordsSet) > 0 && syncCount != 3 {
				return fmt.Errorf("table local_status.compliance_history records %v are not created", expectedRecordsSet)
			}
			return nil
		}, 10*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})
})

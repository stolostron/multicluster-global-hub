package controller

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-co-op/gocron"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/processes/cronjob/task"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

var _ = Describe("data retention job", Ordered, func() {
	expiredPartitionTables := map[string]bool{}
	minPartitionTables := map[string]bool{}

	now := time.Now()
	currentMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	// Use 12 months retention to avoid race condition with scheduler_test which uses 18 months.
	// This ensures our test's "expired" partitions (13 months ago) won't be deleted by
	// the scheduler's data-retention job (which only deletes partitions >18 months old).
	retentionMonth := 12

	minTime := currentMonth.AddDate(0, -retentionMonth, 0)
	expirationTime := minTime.AddDate(0, -1, 0)
	maxTime := currentMonth.AddDate(0, 1, 0)

	BeforeAll(func() {
		fmt.Println("Time", "max", maxTime, "min", minTime, "expiredTime", expirationTime)
		By("Creating expired partition table in the database")
		for _, tableName := range task.PartitionTables {
			err := createPartitionTable(tableName, expirationTime)
			Expect(err).ToNot(HaveOccurred())
			expiredPartitionTables[fmt.Sprintf("%s_%s", tableName, expirationTime.Format(task.PartitionDateFormat))] = false
		}

		By("Create the min partition table in the database")
		for _, tableName := range task.PartitionTables {
			err := createPartitionTable(tableName, minTime)
			Expect(err).ToNot(HaveOccurred())
			minPartitionTables[fmt.Sprintf("%s_%s", tableName, minTime.Format(task.PartitionDateFormat))] = false
		}

		By("Check whether the expired/min tables are created")
		Eventually(func() error {
			var tables []models.Table
			result := db.Raw("SELECT schemaname as schema_name, tablename as table_name FROM pg_tables").Find(&tables)
			if result.Error != nil {
				return result.Error
			}
			for _, table := range tables {
				gotTable := fmt.Sprintf("%s.%s", table.Schema, table.Table)
				// expired partition tables
				if _, ok := expiredPartitionTables[gotTable]; ok {
					fmt.Println("the expired partition table is created: ", gotTable)
					expiredPartitionTables[gotTable] = true
				}
				// min partition tables
				if _, ok := minPartitionTables[gotTable]; ok {
					fmt.Println("the min partition table is created: ", gotTable)
					minPartitionTables[gotTable] = true
				}
			}
			for key, val := range expiredPartitionTables {
				if !val {
					return fmt.Errorf("table %s is not created", key)
				}
			}
			for key, val := range minPartitionTables {
				if !val {
					return fmt.Errorf("table %s is not created", key)
				}
			}
			return nil
		}, 10*time.Second, 1*time.Second).ShouldNot(HaveOccurred())

		By("Create soft deleted recorded in the database")
		for _, tableName := range task.RetentionTables {
			err := createRetentionData(tableName, expirationTime)
			Expect(err).ToNot(HaveOccurred())
		}

		for _, table := range task.RetentionTables {
			By(fmt.Sprintf("Check whether the record was created in table %s", table))
			Eventually(func() error {
				rows, err := db.Raw(fmt.Sprintf(`SELECT leaf_hub_name, deleted_at FROM %s WHERE DELETED_AT <= '%s'`,
					table, expirationTime.Format(task.TimeFormat))).Rows()
				if err != nil {
					return fmt.Errorf("error reading from table %s due to: %v", table, err)
				}
				defer func() {
					if err := rows.Close(); err != nil {
						fmt.Printf("failed to close rows: %v\n", err)
					}
				}()

				if !rows.Next() {
					return fmt.Errorf("The record was not exists in table %s due to: %v", table, err)
				}

				fmt.Println("the deleted record is created: ", table)
				return nil
			}, 10*time.Second, 1*time.Second).Should(BeNil())
		}
	})

	It("the data retention job should work", func() {
		By("Create the data retention job")
		s := gocron.NewScheduler(time.UTC)
		_, err := s.Every(1).Week().DoWithJobDetails(task.DataRetention, ctx, retentionMonth)
		Expect(err).ToNot(HaveOccurred())
		s.StartAsync()
		defer s.Clear()

		By("Check whether the expired tables are deleted")
		Eventually(func() error {
			var tables []models.Table
			result := db.Raw("SELECT schemaname as schema_name, tablename as table_name FROM pg_tables").Find(&tables)
			if result.Error != nil {
				return result.Error
			}

			count := 0
			for _, table := range tables {
				gotTable := fmt.Sprintf("%s.%s", table.Schema, table.Table)
				if _, ok := expiredPartitionTables[gotTable]; ok {
					fmt.Println("deleting the expired partition table: ", gotTable)
					count++
				}
			}
			if count > 0 {
				return fmt.Errorf("the expired tables hasn't been deleted")
			}
			return nil
		}, 10*time.Second, 1*time.Second).ShouldNot(HaveOccurred())

		for _, table := range task.RetentionTables {
			By(fmt.Sprintf("Check whether the record were deleted in table %s", table))
			Eventually(func() error {
				rows, err := db.Raw(fmt.Sprintf(`SELECT leaf_hub_name, deleted_at FROM %s WHERE DELETED_AT <= '%s'`,
					table, expirationTime.Format(task.TimeFormat))).Rows()
				if err != nil {
					return fmt.Errorf("error reading from table %s due to: %v", table, err)
				}
				defer func() {
					if err := rows.Close(); err != nil {
						fmt.Printf("failed to close rows: %v\n", err)
					}
				}()
				if rows.Next() {
					return fmt.Errorf("The record was not exists in table %s due to: %v", table, err)
				}

				fmt.Println("deleting the expired record in table: ", table)
				return nil
			}, 10*time.Second, 1*time.Second).Should(BeNil())
		}
	})

	It("the data retention should log the job execution", func() {
		db := database.GetGorm()
		logs := []models.DataRetentionJobLog{}

		Eventually(func() error {
			result := db.Find(&logs)
			if result.Error != nil {
				return result.Error
			}
			if len(logs) < 6 {
				return fmt.Errorf("the logs are not enough")
			}
			return nil
		}, 10*time.Second, 1*time.Second).ShouldNot(HaveOccurred())
		fmt.Println("Time", "Min", minTime.Format(task.PartitionDateFormat), "Max", maxTime.Format(task.PartitionDateFormat))
		for _, log := range logs {
			fmt.Printf("table_name(%s) | min(%s) | max(%s) | min_deletion(%s) \n",
				log.Name, log.MinPartition, log.MaxPartition, log.MinDeletion.Format(task.DateFormat))
			for _, tableName := range task.PartitionTables {
				if log.Name == tableName {
					Expect(log.MinPartition).To(ContainSubstring(minTime.Format(task.PartitionDateFormat)))
					Expect(log.MaxPartition).To(ContainSubstring(maxTime.Format(task.PartitionDateFormat)))
				}
			}
		}
	})
})

func createPartitionTable(tableName string, date time.Time) error {
	db := database.GetGorm()
	result := db.Exec(`SELECT create_monthly_range_partitioned_table(?, ?)`, tableName, date.Format(task.DateFormat))
	if result.Error != nil {
		return fmt.Errorf("failed to create partition table %s: %w", tableName, result.Error)
	}
	return nil
}

func createRetentionData(tableName string, date time.Time) error {
	var result *gorm.DB

	db := database.GetGorm()

	switch tableName {
	case "status.managed_clusters":
		cluster := &clusterv1.ManagedCluster{
			ObjectMeta: v1.ObjectMeta{
				Name: "cluster1",
				UID:  types.UID(uuid.New().String()),
			},
			Spec: clusterv1.ManagedClusterSpec{
				HubAcceptsClient:     true,
				LeaseDurationSeconds: 60,
			},
		}
		payload, _ := json.Marshal(cluster)

		result = db.Exec(
			fmt.Sprintf(`INSERT INTO status.managed_clusters (leaf_hub_name, cluster_id, payload, error, 
				created_at, updated_at, deleted_at) VALUES ('leafhub1', '%s', '%s', 'none', '%s', '%s', '%s')`,
				string(cluster.UID), payload, date.Format(task.TimeFormat), date.Format(task.TimeFormat), date.Format(task.TimeFormat)))

	case "status.leaf_hubs":
		result = db.Exec(
			fmt.Sprintf(`INSERT INTO status.leaf_hubs (leaf_hub_name, cluster_id, payload, created_at, updated_at, deleted_at) 
			VALUES ('leafhub1','a71a6b5c-8361-4f50-9890-3de9e2df0b1c', '{"consoleURL": "https://leafhub1.com", "leafHubName": "leafhub1"}', '%s', '%s', '%s')`,
				date.Format(task.TimeFormat), date.Format(task.TimeFormat), date.Format(task.TimeFormat)))

	case "local_spec.policies":
		policy := policyv1.Policy{
			ObjectMeta: v1.ObjectMeta{
				Name:      "policy1",
				Namespace: "default",
				UID:       types.UID(uuid.New().String()),
			},
			Spec: policyv1.PolicySpec{},
		}
		payload, _ := json.Marshal(policy)

		result = db.Exec(
			fmt.Sprintf(`INSERT INTO local_spec.policies (policy_id, leaf_hub_name, payload, created_at, 
				updated_at, deleted_at) VALUES ('%s', 'leafhub1', '%s', '%s', '%s', '%s')`, policy.UID, payload,
				date.Format(task.TimeFormat), date.Format(task.TimeFormat), date.Format(task.TimeFormat)))
	}
	if result.Error != nil {
		return fmt.Errorf("failed to create retention data in table %s due to: %w", tableName, result.Error)
	}
	return nil
}

package task

import (
	"fmt"
	"time"

	"github.com/go-co-op/gocron"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

var _ = Describe("data retention job", Ordered, func() {
	expiredPartitionTables := map[string]bool{}
	currentTime := time.Now()
	minTime := currentTime.AddDate(0, -(retentionMonth - 1), 0)
	maxTime := currentTime.AddDate(0, 1, 0)

	BeforeAll(func() {
		By("Creating expired partition table in the database")
		for _, tableName := range partitionTables {
			err := createPartitionTable(tableName, currentTime.AddDate(0, -retentionMonth, 0))
			Expect(err).ToNot(HaveOccurred())

			expiredPartitionTables[fmt.Sprintf("%s_%s", tableName,
				currentTime.AddDate(0, -retentionMonth, 0).Format(partitionDateFormat))] = false
		}

		By("Create the min partition table need to be deleted")
		for _, tableName := range partitionTables {
			err := createPartitionTable(tableName, minTime)
			Expect(err).ToNot(HaveOccurred())
		}

		By("Check whether the expired tables are created")
		Eventually(func() error {
			var tables []models.Table
			result := db.Raw("SELECT schemaname as schema_name, tablename as table_name FROM pg_tables").Find(&tables)
			if result.Error != nil {
				return result.Error
			}
			for _, table := range tables {
				gotTable := fmt.Sprintf("%s.%s", table.Schema, table.Table)
				if _, ok := expiredPartitionTables[gotTable]; ok {
					fmt.Println("the expected partition table is created: ", gotTable)
					expiredPartitionTables[gotTable] = true
				}
			}

			for key, val := range expiredPartitionTables {
				if !val {
					return fmt.Errorf("table %s is not created", key)
				}
			}
			return nil
		}, 10*time.Second, 1*time.Second).ShouldNot(HaveOccurred())
	})

	It("the data retention job should work", func() {
		By("Create the data retention job")
		s := gocron.NewScheduler(time.UTC)
		_, err := s.Every(1).Week().DoWithJobDetails(DataRetention, ctx, pool)
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
		for _, log := range logs {
			fmt.Printf("table_name(%s) | min(%s) | max(%s) | min_deletion(%s) \n", log.Name, log.MinPartition,
				log.MaxPartition, log.MinDeletion.Format(dateFormat))
			for _, tableName := range partitionTables {
				if log.Name == tableName {
					Expect(log.MinPartition).To(ContainSubstring(minTime.Format(partitionDateFormat)))
					Expect(log.MaxPartition).To(ContainSubstring(maxTime.Format(partitionDateFormat)))
				}
			}
		}
	})
})

func createPartitionTable(tableName string, date time.Time) error {
	db := database.GetGorm()
	result := db.Exec(`SELECT create_monthly_range_partitioned_table(?, ?)`, tableName, date.Format(dateFormat))
	if result.Error != nil {
		return fmt.Errorf("failed to create partition table %s: %w", tableName, result.Error)
	}
	return nil
}

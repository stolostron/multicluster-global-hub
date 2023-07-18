package task

import (
	"fmt"
	"time"

	"github.com/go-co-op/gocron"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("data retention job", Ordered, func() {
	expiredPartitionTables := map[string]bool{}
	currentTime := time.Now()

	BeforeAll(func() {
		By("Creating test table in the database")
		for _, tableName := range partitionTables {
			table := fmt.Sprintf("%s_%s", tableName, currentTime.AddDate(0, -retentionMonth, 0).Format(partitionDateFormat))
			expiredPartitionTables[table] = false
		}

		By("Create the expired partition table need to be deleted")
		for _, tableName := range partitionTables {
			if err := createPartitionTable(tableName, currentTime.AddDate(0, -retentionMonth, 0),
				currentTime.AddDate(0, -retentionMonth+1, 0)); err != nil {
				log.Error(err, "failed to create partition table")
				return
			}
		}

		By("Check whether the tables are created")
		Eventually(func() error {
			rows, err := db.Raw("SELECT schemaname, tablename FROM pg_tables").Rows()
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var schema, table string
				if err := rows.Scan(&schema, &table); err != nil {
					return err
				}
				gotTable := fmt.Sprintf("%s.%s", schema, table)
				if _, ok := expiredPartitionTables[gotTable]; ok {
					fmt.Println("get expected partition table: ", gotTable)
					expiredPartitionTables[gotTable] = true
				}
			}
			for table, exist := range expiredPartitionTables {
				if !exist {
					return fmt.Errorf("table %s is not created", table)
				}
			}
			return nil
		}, 10*time.Second, 1*time.Second).ShouldNot(HaveOccurred())
	})

	It("create the partition table", func() {
		By("Create the data retention job")
		s := gocron.NewScheduler(time.UTC)
		_, err := s.Every(1).Week().DoWithJobDetails(DataRetention, ctx, pool)
		Expect(err).ToNot(HaveOccurred())
		s.StartAsync()
		defer s.Clear()

		By("Check whether the tables are deleted")
		Eventually(func() error {
			rows, err := db.Raw("SELECT schemaname, tablename FROM pg_tables").Rows()
			if err != nil {
				return err
			}
			defer rows.Close()
			count := 0
			for rows.Next() {
				var schema, table string
				if err := rows.Scan(&schema, &table); err != nil {
					return err
				}
				gotTable := fmt.Sprintf("%s.%s", schema, table)
				if _, ok := expiredPartitionTables[gotTable]; ok {
					fmt.Println("deleting partition table: ", gotTable)
					count++
				}
			}
			if count > 0 {
				return fmt.Errorf("the partition table hasn't been deleted")
			}
			return nil
		}, 10*time.Second, 1*time.Second).ShouldNot(HaveOccurred())
	})
})

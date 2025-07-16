package migration_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

var _ = Describe("Database Schema Verification", func() {
	It("should list all tables in the 'spec' schema to verify initialization", func() {
		db := database.GetGorm()
		Expect(db).NotTo(BeNil())

		type TableResult struct {
			TableName string `gorm:"column:table_name"`
		}
		var results []TableResult

		// Query the information schema for tables in the 'spec' schema
		err := db.Raw("SELECT table_name FROM information_schema.tables WHERE table_schema = ?", "spec").Scan(&results).Error
		Expect(err).NotTo(HaveOccurred())

		fmt.Println("Tables found in 'spec' schema:")
		found := false
		for _, result := range results {
			fmt.Println("- ", result.TableName)
			if result.TableName == "leaf_hubs" {
				found = true
			}
		}
		Expect(found).To(BeTrue(), "Expected to find 'leaf_hubs' table in 'spec' schema, but it was not found.")
	})
})

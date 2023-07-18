package testpostgres

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

func InitDatabase(uri string) error {
	err := database.InitGormInstance(&database.DatabaseConfig{
		URL:      uri,
		Dialect:  database.PostgresDialect,
		PoolSize: 1,
	})
	if err != nil {
		return err
	}

	db := database.GetGorm()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("failed to get current dir: no caller information")
	}
	dirname := filepath.Dir(currentFile)
	dirname = filepath.Dir(dirname)
	dirname = filepath.Dir(dirname)
	dirname = filepath.Dir(dirname)

	sqlDir := filepath.Join(dirname, "operator", "pkg", "controllers", "hubofhubs", "database")
	files, err := os.ReadDir(sqlDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		filePath := filepath.Join(sqlDir, file.Name())
		fileContent, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}

		result := db.Exec(string(fileContent))
		if result.Error != nil {
			return result.Error
		}
		fmt.Printf("script %s executed successfully.\n", file.Name())
	}

	// create partition tables
	partitionDateFormat := "2006_01"
	dateFormat := "2006-01-02"
	partitionTables := []string{
		"event.local_policies",
		"event.local_root_policies",
	}
	currentTime := time.Now()
	for _, tableName := range partitionTables {
		for i := 0; i <= 5; i++ {
			month := currentTime.AddDate(0, i, 0)
			startDate := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, month.Location())
			endDate := startDate.AddDate(0, 1, 0)

			partitionTableName := fmt.Sprintf("%s_%s", tableName, startDate.Format(partitionDateFormat))
			db := database.GetGorm()
			if result := db.Exec(fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')",
				partitionTableName, tableName, startDate.Format(dateFormat), endDate.Format(dateFormat))); result.Error != nil {
				return fmt.Errorf("failed to create partition table %s: %w", tableName, result.Error)
			}
		}
	}

	return nil
}

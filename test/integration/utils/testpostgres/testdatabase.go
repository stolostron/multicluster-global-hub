package testpostgres

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

func InitDatabase(uri string) error {
	err := database.InitGormInstance(&database.DatabaseConfig{
		URL:      uri,
		Dialect:  database.PostgresDialect,
		PoolSize: 10,
	})
	if err != nil {
		return err
	}

	db := database.GetGorm()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("failed to get current dir: no caller information")
	}
	dirname := strings.Replace(currentFile, "test/integration/utils/testpostgres/testdatabase.go", "", 1)

	sqlDir := filepath.Join(dirname, "operator", "pkg", "controllers", "storage", "database")
	files, err := os.ReadDir(sqlDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		filePath := filepath.Join(sqlDir, file.Name())
		if file.Name() == "5.privileges.sql" {
			continue
		}
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

	sqlDir = filepath.Join(dirname, "operator", "pkg", "controllers", "storage", "upgrade")
	upgradeFiles, err := os.ReadDir(sqlDir)
	if err != nil {
		return err
	}
	for _, file := range upgradeFiles {
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

	return nil
}

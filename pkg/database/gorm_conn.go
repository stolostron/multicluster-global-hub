package database

import (
	"database/sql"
	"fmt"
	"net/url"
	"sync"

	_ "github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const PostgresDialect = "postgres"

var (
	gormDB   *gorm.DB
	gormOnce sync.Once
	// Direct database connection.
	// It is used:
	// - to setup/close connection because GORM V2 removed gorm.Close()
	// - to work with pq.CopyIn because connection returned by GORM V2 gorm.DB() in "not the same"
	sqlDB *sql.DB
)

type DatabaseConfig struct {
	URL        string
	Dialect    string
	CaCertPath string
	PoolSize   int
}

func InitGormInstance(config *DatabaseConfig) error {
	var err error
	if config.Dialect != PostgresDialect {
		return fmt.Errorf("unsupported database dialect: %s", config.Dialect)
	}
	urlObj, err := completePostgres(config.URL, config.CaCertPath)
	if err != nil {
		return err
	}
	gormOnce.Do(func() {
		sqlDB, err = sql.Open(config.Dialect, urlObj.String())
		if err != nil {
			panic(err)
		}
		sqlDB.SetMaxOpenConns(config.PoolSize)
		gormDB, err = gorm.Open(postgres.New(postgres.Config{
			Conn:                 sqlDB,
			PreferSimpleProtocol: true,
		}), &gorm.Config{
			PrepareStmt:          false,
			FullSaveAssociations: false,
		})
		if err != nil {
			panic(err)
		}
	})
	return err
}

func GetGorm() *gorm.DB {
	if gormDB == nil {
		panic("gormDB is not initialized")
	}
	return gormDB
}

// Close the sql.DB connection
func CloseGorm() error {
	if sqlDB != nil {
		return sqlDB.Close()
	} else {
		return fmt.Errorf("sqlDB is not initialized")
	}
}

func completePostgres(postgresUri string, caCertPath string) (*url.URL, error) {
	urlObj, err := url.Parse(postgresUri)
	if err != nil {
		return nil, err
	}
	// only support verify-ca or disable(for test)
	query := urlObj.Query()
	if query.Get("sslmode") == "verify-ca" && utils.Validate(caCertPath) {
		query.Set("sslrootcert", caCertPath)
		urlObj.RawQuery = query.Encode()
	}
	return urlObj, nil
}

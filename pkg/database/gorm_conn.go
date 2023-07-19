package database

import (
	"database/sql"
	"fmt"
	"net/url"
	"sync"

	_ "github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	ctrl "sigs.k8s.io/controller-runtime"

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
	log   = ctrl.Log.WithName("addon-controller")
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
			log.Error(err, "failed to open database connection")
			return
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
			log.Error(err, "failed to open gorm connection")
			return
		}
	})
	return err
}

func GetGorm() *gorm.DB {
	if gormDB == nil {
		log.Error(nil, "gorm connection is not initialized")
		return nil
	}
	return gormDB
}

// Close the sql.DB connection
func CloseGorm() {
	err := sqlDB.Close()
	if err != nil {
		log.Error(err, "failed to close database connection")
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
	} else {
		query.Add("sslmode", "disable")
	}
	urlObj.RawQuery = query.Encode()
	return urlObj, nil
}

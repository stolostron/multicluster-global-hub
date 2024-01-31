package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"sync"

	_ "github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const PostgresDialect = "postgres"

var (
	IsBackupEnabled bool

	gormDB   *gorm.DB
	gormOnce sync.Once
	// Direct database connection.
	// It is used:
	// - to setup/close connection because GORM V2 removed gorm.Close()
	// - to work with pq.CopyIn because connection returned by GORM V2 gorm.DB() in "not the same"
	sqlDB *sql.DB
	log   = ctrl.Log.WithName("database-controller")

	//This conn is used for advisory lock
	//The advisory lock should use a same connection in a connection pool.
	//Detail: https://engineering.qubecinema.com/2019/08/26/unlocking-advisory-locks.html
	conn *sql.Conn
	ctx  = context.Background()
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
	gormOnce.Do(func() {
		gormDB, sqlDB, err = NewGormConn(config)
		fmt.Println("set max connection==============:", config.PoolSize)
		sqlDB.SetMaxOpenConns(config.PoolSize)
	})
	if err != nil {
		return err
	}
	if IsBackupEnabled {
		go func() {
			conn, err = sqlDB.Conn(ctx)
		}()
	}
	return err
}

func NewGormConn(config *DatabaseConfig) (*gorm.DB, *sql.DB, error) {
	var err error
	if config.Dialect != PostgresDialect {
		return nil, nil, fmt.Errorf("unsupported database dialect: %s", config.Dialect)
	}
	urlObj, err := completePostgres(config.URL, config.CaCertPath)
	if err != nil {
		return nil, nil, err
	}

	sqlDBConn, err := sql.Open(config.Dialect, urlObj.String())
	if err != nil {
		log.Error(err, "failed to open database connection")
		return nil, nil, err
	}
	gormDBconn, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDBConn,
		PreferSimpleProtocol: true,
	}), &gorm.Config{
		PrepareStmt:          false,
		FullSaveAssociations: false,
	})
	if err != nil {
		log.Error(err, "failed to open gorm connection")
		return nil, nil, err
	}
	return gormDBconn, sqlDBConn, nil
}

func GetGorm() *gorm.DB {
	if gormDB == nil {
		log.Error(nil, "gorm connection is not initialized")
		return nil
	}
	return gormDB
}

func GetConn() *sql.Conn {
	if conn == nil {
		log.Error(nil, "sql connection is not initialized")
		return nil
	}
	return conn
}

func GetSqlDb() *sql.DB {
	if sqlDB == nil {
		log.Error(nil, "sqlDb connection is not initialized")
		return nil
	}
	return sqlDB
}

func Lock(lockConn *sql.Conn) error {
	if !IsBackupEnabled {
		return nil
	}
	log.V(2).Info("Add db lock")
	defer log.V(2).Info("db locked")
	_, err := lockConn.ExecContext(ctx, "select pg_advisory_lock($1)", constants.LockId)
	return err
}

func Unlock(lockConn *sql.Conn) {
	if !IsBackupEnabled {
		return
	}
	log.V(2).Info("unlock db")
	defer log.V(2).Info("db unlocked")
	err := retry.OnError(retry.DefaultRetry, func(err error) bool {
		if err != nil {
			klog.V(2).Infof("unlock failed, retry unlock. err: %s", err)
			return true
		}
		return false
	},
		func() error {
			_, err := lockConn.ExecContext(ctx, "select pg_advisory_unlock($1)", constants.LockId)
			return err
		})
	if err != nil {
		log.Error(err, "Failed to unlock db")
	}
}

// Close the sql.DB connection
func CloseGorm(sqlConn *sql.DB) {
	if sqlConn != nil {
		err := sqlConn.Close()
		if err != nil {
			log.Error(err, "failed to close database connection")
		}
	}
}

func completePostgres(postgresUri string, caCertPath string) (*url.URL, error) {
	urlObj, err := url.Parse(postgresUri)
	if err != nil {
		return nil, err
	}
	// only support verify-ca or disable(for test)
	query := urlObj.Query()
	_, ok := utils.Validate(caCertPath)
	if query.Get("sslmode") == "verify-ca" && ok {
		query.Set("sslrootcert", caCertPath)
	} else {
		query.Add("sslmode", "disable")
	}
	urlObj.RawQuery = query.Encode()
	return urlObj, nil
}

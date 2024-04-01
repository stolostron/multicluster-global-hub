package database_test

import (
	"context"
	"fmt"
	"testing"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/test/pkg/testpostgres"
)

var (
	testPostgres   *testpostgres.TestPostgres
	databaseConfig *database.DatabaseConfig
)

func TestConnectionPool(t *testing.T) {
	var err error
	testPostgres, err = testpostgres.NewTestPostgres()
	assert.Nil(t, err)

	databaseConfig = &database.DatabaseConfig{
		URL:      testPostgres.URI,
		Dialect:  database.PostgresDialect,
		PoolSize: 6,
	}
	err = database.InitGormInstance(databaseConfig)
	assert.Nil(t, err)

	err = testpostgres.InitDatabase(testPostgres.URI)
	assert.Nil(t, err)

	db := database.GetGorm()
	sqlDB, err := db.DB()
	assert.Nil(t, err)

	stats := sqlDB.Stats()
	assert.Equal(t, databaseConfig.PoolSize, stats.MaxOpenConnections)

	fmt.Println("--------------------------------------------------------")
	fmt.Println("stats.OpenConnections: ", stats.OpenConnections)
	fmt.Println("stats.MaxOpenConnections: ", stats.MaxOpenConnections)
	fmt.Println("stats.InUse: ", stats.InUse)
	fmt.Println("stats.Idle: ", stats.Idle)
}

func TestTwoInstanceCanGetLockWhenReleased(t *testing.T) {
	database.IsBackupEnabled = true
	err := database.InitGormInstance(databaseConfig)
	assert.Nil(t, err)

	err = testpostgres.InitDatabase(testPostgres.URI)
	assert.Nil(t, err)
	_, sqlDb, err := database.NewGormConn(databaseConfig)
	assert.Nil(t, err)

	newConn, err := sqlDb.Conn(context.Background())
	assert.Nil(t, err)

	default_conn := database.GetConn()
	err = database.Lock(default_conn)
	assert.Nil(t, err)
	database.Unlock(default_conn)

	err = database.Lock(newConn)
	assert.Nil(t, err)
	database.Unlock(newConn)
	database.IsBackupEnabled = false
}

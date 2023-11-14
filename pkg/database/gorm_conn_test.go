package database_test

import (
	"fmt"
	"testing"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/test/pkg/testpostgres"
	"github.com/stretchr/testify/assert"
)

func TestConnectionPool(t *testing.T) {
	testPostgres, err := testpostgres.NewTestPostgres()
	assert.Nil(t, err)

	databaseConfig := &database.DatabaseConfig{
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

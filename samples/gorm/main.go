package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/samples/config"
)

func main() {
	secret, err := config.GetStorageSecret()
	if err != nil {
		fmt.Println(err)
		return
	}

	caCert := secret.Data["ca.crt"]
	// ca.crt to file - only for testing
	postgreSSlRootCert := "/tmp/postgres_ca.crt"
	err = os.WriteFile(postgreSSlRootCert, caCert, 0o644)
	if err != nil {
		log.Fatal(err)
	}

	connStr := string(secret.Data["database_uri"])
	// need to replace internal host to external
	err = database.InitGormInstance(&database.DatabaseConfig{
		URL:        connStr,
		Dialect:    database.PostgresDialect,
		CaCertPath: postgreSSlRootCert,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer database.CloseGorm(database.GetSqlDb())

	g2 := database.GetGorm()

	err = g2.Exec("SELECT 1").Error
	if err != nil {
		log.Fatal(err)
	}

	// Retrieve data from the table
	var localCompliances []LocalCompliance
	result := g2.Find(&localCompliances)
	if result.Error != nil {
		panic(result.Error)
	}

	// Use the localCompliances slice which contains the query results
	for _, lc := range localCompliances {
		fmt.Println(lc.PolicyID, lc.ClusterID, lc.ComplianceDate, lc.Compliance, lc.ComplianceChangedFreq)
	}
}

type LocalCompliance struct {
	PolicyID              string `gorm:"type:uuid;primaryKey"`
	ClusterID             string `gorm:"type:uuid;primaryKey"`
	ComplianceDate        time.Time
	Compliance            string `gorm:"type:local_status.compliance_type"`
	ComplianceChangedFreq int    `gorm:"column:compliance_changed_frequency;default:0"`
}

func (c *LocalCompliance) TableName() string {
	return "history.local_compliance"
}

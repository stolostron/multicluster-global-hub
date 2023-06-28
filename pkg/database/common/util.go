package common

import (
	"log"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

func GetDatabaseCompliance(PolicyCompliance string) database.ComplianceStatus {
	// algin with the database enum values
	status := database.Unknown
	switch PolicyCompliance {
	case string(policyv1.Compliant):
		status = database.Compliant
	case string(policyv1.NonCompliant):
		status = database.NonCompliant
	default:
		log.Printf("unknown compliance status: %s", PolicyCompliance)
	}
	return status
}

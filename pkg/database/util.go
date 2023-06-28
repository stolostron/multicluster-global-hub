package database

import (
	"log"

	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

func GetDatabaseCompliance(PolicyCompliance string) ComplianceStatus {
	// algin with the database enum values
	status := Unknown
	switch PolicyCompliance {
	case string(policyv1.Compliant):
		status = Compliant
	case string(policyv1.NonCompliant):
		status = NonCompliant
	default:
		log.Printf("unknown compliance status: %s", PolicyCompliance)
	}
	return status
}

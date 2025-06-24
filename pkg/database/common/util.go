package common

import (
	"go.uber.org/zap"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

func GetDatabaseCompliance(PolicyCompliance string, log *zap.SugaredLogger) database.ComplianceStatus {
	// algin with the database enum values
	status := database.Unknown
	switch PolicyCompliance {
	case string(policyv1.Compliant):
		status = database.Compliant
	case string(policyv1.NonCompliant):
		status = database.NonCompliant
	case string(policyv1.Pending):
		status = database.Pending
	default:
		log.Infof("unknown compliance status: %s", PolicyCompliance)
	}
	return status
}

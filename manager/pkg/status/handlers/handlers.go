package handlers

import (
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	clustermigration "github.com/stolostron/multicluster-global-hub/manager/pkg/status/handlers/clustermigartion"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/handlers/managedcluster"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/handlers/managedhub"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/handlers/policy"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/handlers/security"
)

func RegisterHandlers(mgr ctrl.Manager, cmr *conflator.ConflationManager) {
	// managed hub
	managedhub.RegisterHubClusterHeartbeatHandler(cmr)
	managedhub.RegsiterHubClusterInfoHandler(cmr)

	// managed cluster
	managedcluster.RegisterManagedClusterHandler(mgr.GetClient(), cmr)
	managedcluster.RegisterManagedClusterEventHandler(cmr)

	// managed cluster migration
	clustermigration.RegisterManagedClusterMigrationHandler(mgr, cmr)

	// local policy
	policy.RegisterLocalPolicySpecHandler(cmr)
	policy.RegisterLocalPolicyComplianceHandler(cmr)
	policy.RegisterLocalPolicyCompleteHandler(cmr)
	policy.RegisterLocalRootPolicyEventHandler(cmr)
	policy.RegisterLocalReplicatedPolicyEventHandler(cmr)

	// security
	security.RegisterSecurityAlertCountsHandler(cmr)
}

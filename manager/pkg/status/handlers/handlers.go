package handlers

import (
	"fmt"

	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	clustermigration "github.com/stolostron/multicluster-global-hub/manager/pkg/status/handlers/clustermigartion"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/handlers/generic"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/handlers/managedcluster"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/handlers/managedhub"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/handlers/policy"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/handlers/security"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
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

	// local placementrule
	generic.RegisterGenericHandler[*placementrulev1.PlacementRule](
		cmr,
		string(enum.LocalPlacementRuleSpecType),
		conflator.LocalPlacementRulesSpecPriority,
		enum.CompleteStateMode,
		fmt.Sprintf("%s.%s", database.LocalSpecSchema, database.PlacementRulesTableName))

	// security
	security.RegisterSecurityAlertCountsHandler(cmr)
}

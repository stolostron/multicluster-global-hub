package handlers

import (
	"fmt"

	clustersv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
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

func RegisterHandlers(mgr ctrl.Manager, cmr *conflator.ConflationManager, enableGlobalResource bool) {
	// managed hub
	managedhub.RegisterHubClusterHeartbeatHandler(cmr)
	managedhub.RegsiterHubClusterInfoHandler(cmr)

	// managed cluster
	managedcluster.RegisterManagedClusterHandler(mgr.GetClient(), cmr)
	managedcluster.RegisterManagedClusterEventHandler(cmr)

	// managed cluster migration
	clustermigration.RegisterManagedClusterMigrationHandler(mgr, cmr)

	// local policy
	policy.RegisterLocalPolicySpecHandler(mgr.GetClient(), cmr)
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

	if enableGlobalResource {
		// global policy
		policy.RegisterPolicyComplianceHandler(cmr)
		policy.RegisterPolicyCompleteHandler(cmr)
		policy.RegisterPolicyDeltaComplianceHandler(cmr)
		policy.RegisterPolicyMiniComplianceHandler(cmr)

		// placementRule
		generic.RegisterGenericHandler[*placementrulev1.PlacementRule](
			cmr,
			string(enum.PlacementRuleSpecType),
			conflator.PlacementRulePriority,
			enum.CompleteStateMode,
			fmt.Sprintf("%s.%s", database.StatusSchema, database.PlacementRulesTableName))

		generic.RegisterGenericHandler[*clustersv1beta1.Placement](
			cmr,
			string(enum.PlacementSpecType),
			conflator.PlacementPriority,
			enum.CompleteStateMode,
			fmt.Sprintf("%s.%s", database.StatusSchema, database.PlacementsTableName))

		generic.RegisterGenericHandler[*clustersv1beta1.PlacementDecision](
			cmr,
			string(enum.PlacementDecisionType),
			conflator.PlacementDecisionPriority,
			enum.CompleteStateMode,
			fmt.Sprintf("%s.%s", database.StatusSchema, database.PlacementDecisionsTableName))

		generic.RegisterGenericHandler[*appsv1alpha1.SubscriptionReport](
			cmr,
			string(enum.SubscriptionReportType),
			conflator.SubscriptionReportPriority,
			enum.CompleteStateMode,
			fmt.Sprintf("%s.%s", database.StatusSchema, database.SubscriptionReportsTableName))

		generic.RegisterGenericHandler[*appsv1alpha1.SubscriptionStatus](
			cmr,
			string(enum.SubscriptionStatusType),
			conflator.SubscriptionStatusPriority,
			enum.CompleteStateMode,
			fmt.Sprintf("%s.%s", database.StatusSchema, database.SubscriptionStatusesTableName))
	}
}

package handler

import (
	"fmt"

	clustersv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

func RegisterHandlers(cmr *conflator.ConflationManager, enableGlobalResource bool) {
	NewHubClusterHeartbeatHandler().RegisterHandler(cmr)
	NewHubClusterInfoHandler().RegisterHandler(cmr)

	// managed cluster
	NewManagedClusterHandler().RegisterHandler(cmr)

	// local policy
	NewLocalPolicySpecHandler().RegisterHandler(cmr)
	NewLocalPolicyComplianceHandler().RegisterHandler(cmr)
	NewLocalPolicyCompleteHandler().RegisterHandler(cmr)

	// event
	NewLocalReplicatedPolicyEventHandler().RegisterHandler(cmr)
	NewLocalRootPolicyEventHandler().RegisterHandler(cmr)
	NewManagedClusterEventHandler().RegisterHandler(cmr)

	// local placementrule
	NewGenericHandler[*placementrulev1.PlacementRule](
		string(enum.LocalPlacementRuleSpecType),
		conflator.LocalPlacementRulesSpecPriority,
		enum.CompleteStateMode,
		fmt.Sprintf("%s.%s", database.LocalSpecSchema, database.PlacementRulesTableName),
	).RegisterHandler(cmr)

	// security
	NewSecurityAlertCountsHandler().RegisterHandler(cmr)

	if enableGlobalResource {
		NewPolicyComplianceHandler().RegisterHandler(cmr)
		NewPolicyCompleteHandler().RegisterHandler(cmr)
		NewPolicyDeltaComplianceHandler().RegisterHandler(cmr)
		NewPolicyMiniComplianceHandler().RegisterHandler(cmr)

		NewGenericHandler[*placementrulesv1.PlacementRule](
			string(enum.PlacementRuleSpecType),
			conflator.PlacementRulePriority,
			enum.CompleteStateMode,
			fmt.Sprintf("%s.%s", database.StatusSchema, database.PlacementRulesTableName)).RegisterHandler(cmr)

		NewGenericHandler[*clustersv1beta1.Placement](
			string(enum.PlacementSpecType),
			conflator.PlacementPriority,
			enum.CompleteStateMode,
			fmt.Sprintf("%s.%s", database.StatusSchema, database.PlacementsTableName)).RegisterHandler(cmr)

		NewGenericHandler[*clustersv1beta1.PlacementDecision](
			string(enum.PlacementDecisionType),
			conflator.PlacementDecisionPriority,
			enum.CompleteStateMode,
			fmt.Sprintf("%s.%s", database.StatusSchema, database.PlacementDecisionsTableName)).RegisterHandler(cmr)

		NewGenericHandler[*appsv1alpha1.SubscriptionReport](
			string(enum.SubscriptionReportType),
			conflator.SubscriptionReportPriority,
			enum.CompleteStateMode,
			fmt.Sprintf("%s.%s", database.StatusSchema, database.SubscriptionReportsTableName)).RegisterHandler(cmr)

		NewGenericHandler[*appsv1alpha1.SubscriptionStatus](
			string(enum.SubscriptionStatusType),
			conflator.SubscriptionStatusPriority,
			enum.CompleteStateMode,
			fmt.Sprintf("%s.%s", database.StatusSchema, database.SubscriptionStatusesTableName)).RegisterHandler(cmr)
	}
}

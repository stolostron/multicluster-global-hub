package dbsyncer

import (
	"fmt"

	placementrulesv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

func NewPlacementRuleHandler() conflator.Handler {
	return NewGenericHandler[*placementrulesv1.PlacementRule](
		string(enum.PlacementRuleSpecType),
		conflator.PlacementRulePriority,
		metadata.CompleteStateMode,
		fmt.Sprintf("%s.%s", database.StatusSchema, database.PlacementRulesTableName))
}

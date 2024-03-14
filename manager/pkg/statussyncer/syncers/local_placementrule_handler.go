package dbsyncer

import (
	"fmt"

	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

func NewLocalPlacementRuleSpecHandler() conflator.Handler {
	return NewGenericHandler[*placementrulev1.PlacementRule](
		string(enum.LocalPlacementRuleSpecType),
		conflator.LocalPlacementRulesSpecPriority,
		enum.CompleteStateMode,
		fmt.Sprintf("%s.%s", database.LocalSpecSchema, database.PlacementRulesTableName))
}

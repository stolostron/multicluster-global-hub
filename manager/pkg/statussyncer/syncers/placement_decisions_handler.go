package dbsyncer

import (
	"fmt"

	clustersv1beta1 "open-cluster-management.io/api/cluster/v1beta1"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

func NewPlacementDecisionHandler() conflator.Handler {
	return NewGenericHandler[*clustersv1beta1.PlacementDecision](
		string(enum.PlacementDecisionType),
		conflator.PlacementDecisionPriority,
		metadata.CompleteStateMode,
		fmt.Sprintf("%s.%s", database.StatusSchema, database.PlacementDecisionsTableName))
}

package dbsyncer

import (
	"fmt"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

func NewPlacementDecisionHandler() Handler {
	return NewGenericHandler(
		string(enum.PlacementDecisionType),
		conflator.PlacementDecisionPriority,
		metadata.CompleteStateMode,
		fmt.Sprintf("%s.%s", database.StatusSchema, database.PlacementDecisionsTableName))
}

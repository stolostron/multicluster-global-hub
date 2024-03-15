package dbsyncer

import (
	"fmt"

	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

func NewSubscriptionReportHandler() conflator.Handler {
	return NewGenericHandler[*appsv1alpha1.SubscriptionReport](
		string(enum.SubscriptionReportType),
		conflator.SubscriptionReportPriority,
		enum.CompleteStateMode,
		fmt.Sprintf("%s.%s", database.StatusSchema, database.SubscriptionReportsTableName))
}

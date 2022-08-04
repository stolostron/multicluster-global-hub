package hubofhubs

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hubofhubsv1alpha1 "github.com/stolostron/hub-of-hubs/operator/apis/hubofhubs/v1alpha1"
)

type ICondition interface {
	GetConditions() []metav1.Condition
	SetConditions([]metav1.Condition)
}

const (
	CONDITION_STATUS_TRUE    = "True"
	CONDITION_STATUS_FALSE   = "False"
	CONDITION_STATUS_UNKNOWN = "Unknown"
)

// NOTE: the status of ResourceFound can only be True; otherwise there is no condition
const (
	CONDITION_TYPE_RESOURCE_FOUND    = "ResourceFound"
	CONDITION_REASON_RESOURCE_FOUND  = "ResourceFound"
	CONDITION_MESSAGE_RESOURCE_FOUND = "Resource found"
)

func (reconcile *ConfigReconciler) setConditionResourceFound(ctx context.Context, config *hubofhubsv1alpha1.Config) error {
	if !reconcile.containsCondition(ctx, config, CONDITION_REASON_RESOURCE_FOUND) {
		return AppendCondition(ctx, reconcile.Client, config, CONDITION_TYPE_RESOURCE_FOUND,
			CONDITION_STATUS_TRUE, CONDITION_REASON_RESOURCE_FOUND, CONDITION_MESSAGE_RESOURCE_FOUND)
	}
	return nil
}

// NOTE: the status of DatabaseInitialized can be True or False
const (
	CONDITION_TYPE_DATABASE_INIT    = "DatabaseInitialized"
	CONDITION_REASON_DATABASE_INIT  = "DatabaseInitialized"
	CONDITION_MESSAGE_DATABASE_INIT = "Database has been initialized"
)

func (reconcile *ConfigReconciler) setConditionDatabaseInit(ctx context.Context, config *hubofhubsv1alpha1.Config, status metav1.ConditionStatus) error {
	if !reconcile.containsCondition(ctx, config, CONDITION_REASON_DATABASE_INIT) {
		return AppendCondition(ctx, reconcile.Client, config, CONDITION_TYPE_DATABASE_INIT,
			status, CONDITION_REASON_DATABASE_INIT, CONDITION_MESSAGE_DATABASE_INIT)
	} else {
		currentStatus := reconcile.getConditionStatus(ctx, config, CONDITION_TYPE_DATABASE_INIT)
		if currentStatus != status {
			err := reconcile.deleteCondition(ctx, config, CONDITION_TYPE_DATABASE_INIT, CONDITION_REASON_DATABASE_INIT)
			if err != nil {
				return err
			}
			return AppendCondition(ctx, reconcile.Client, config, CONDITION_TYPE_DATABASE_INIT, status,
				CONDITION_REASON_DATABASE_INIT, CONDITION_MESSAGE_DATABASE_INIT)
		}
	}
	return nil
}

func (reconcile *ConfigReconciler) deleteCondition(ctx context.Context, config *hubofhubsv1alpha1.Config, typeName string, reason string) error {
	newConditions := make([]metav1.Condition, 0)
	for _, condition := range config.Status.Conditions {
		if condition.Type != typeName && condition.Reason != reason {
			newConditions = append(newConditions, condition)
		}
	}
	config.Status.Conditions = newConditions
	err := reconcile.Client.Status().Update(ctx, config)
	if err != nil {
		return fmt.Errorf("hubofhubs config status condition update failed: %v", err)
	}
	return nil
}

func (reconciler *ConfigReconciler) getConditionStatus(ctx context.Context, config *hubofhubsv1alpha1.Config,
	typeName string,
) metav1.ConditionStatus {
	var output metav1.ConditionStatus = CONDITION_STATUS_UNKNOWN
	for _, condition := range config.Status.Conditions {
		if condition.Type == typeName {
			output = condition.Status
		}
	}
	return output
}

func (reconciler *ConfigReconciler) containsCondition(ctx context.Context,
	config *hubofhubsv1alpha1.Config, reason string,
) bool {
	output := false
	for _, condition := range config.Status.Conditions {
		if condition.Reason == reason {
			output = true
		}
	}
	return output
}

func (reconcile *ConfigReconciler) containConditionStatus(ctx context.Context,
	config *hubofhubsv1alpha1.Config, typeName string, status metav1.ConditionStatus,
) bool {
	output := false
	for _, condition := range config.Status.Conditions {
		if condition.Type == typeName && condition.Status == status {
			output = true
		}
	}
	return output
}

func AppendCondition(ctx context.Context, reconcilerClient client.Client, object client.Object, typeName string,
	stats metav1.ConditionStatus, reason string, message string,
) error {
	conditions, ok := (object).(ICondition)
	if ok {
		condition := metav1.Condition{Type: typeName, Status: stats, Reason: reason, Message: message, LastTransitionTime: metav1.Time{Time: time.Now()}}
		conditions.SetConditions(append(conditions.GetConditions(), condition))
		err := reconcilerClient.Status().Update(ctx, object)
		if err != nil {
			return fmt.Errorf("custom status condition update failed: %v", err)
		}
	} else {
		return fmt.Errorf("status condition cannot be set, resource does not support Conditions")
	}
	return nil
}

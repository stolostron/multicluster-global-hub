package utils

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ICondition interface {
	GetConditions() []metav1.Condition
	SetConditions([]metav1.Condition)
}

func AppendCondition(ctx context.Context, reconcilerClient client.Client, object client.Object, typeName string,
	stats metav1.ConditionStatus, reason string, message string) error {
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

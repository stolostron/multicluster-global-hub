package status

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

type StatusReconciler struct {
	log logr.Logger
	client.Client
}

func NewStatusReconciler(c client.Client) *StatusReconciler {
	return &StatusReconciler{
		log:    ctrl.Log.WithName("global-hub-status"),
		Client: c,
	}
}

func (r *StatusReconciler) Reconcile(ctx context.Context, mgh *v1alpha4.MulticlusterGlobalHub,
	reconcileErr error,
) error {
	// skip status updating if deleting the mgh
	if mgh.DeletionTimestamp != nil {
		return nil
	}

	// update the ready condition
	readyCond := metav1.Condition{
		Type:               config.CONDITION_TYPE_GLOBALHUB_READY,
		Status:             metav1.ConditionTrue,
		Reason:             config.CONDITION_REASON_GLOBALHUB_READY,
		Message:            config.CONDITION_MESSAGE_GLOBALHUB_READY,
		LastTransitionTime: metav1.Time{Time: time.Now()},
	}
	if reconcileErr != nil {
		readyCond.Status = metav1.ConditionFalse
		readyCond.Reason = config.CONDITION_REASON_GLOBALHUB_FAILED
		readyCond.Message = reconcileErr.Error()
	}
	if err := config.UpdateCondition(ctx, r.Client, mgh, readyCond); err != nil {
		return err
	}

	// update the topic condition
	topicMessage := fmt.Sprintf("The topics is parsed: spec(%s), status(%s)", config.GetSpecTopic(),
		config.FuzzyStatusTopic())
	if err := config.SetConditionTopic(ctx, r.Client, mgh, config.CONDITION_STATUS_TRUE, topicMessage); err != nil {
		return err
	}

	// dataRetention should at least be 1 month, otherwise it will deleted the current month partitions and records
	// update the MGH status and message of the condition if they are not set or changed
	months, err := utils.ParseRetentionMonth(mgh.Spec.DataLayer.Postgres.Retention)
	if err == nil {
		if months < 1 {
			months = 1
		}
		msg := fmt.Sprintf("The data will be kept in the database for %d months.", months)
		if err := config.SetConditionDataRetention(ctx, r.Client, mgh, config.CONDITION_STATUS_TRUE, msg); err != nil {
			return err
		}
	} else {
		klog.Info("failed to parse the retention month", "message", err.Error())
	}

	// update status of the global hub manager deployment
	if err := r.updateDeploymentStatus(ctx, mgh, config.CONDITION_TYPE_MANAGER_AVAILABLE,
		operatorconstants.GHManagerDeploymentName); err != nil {
		return err
	}

	if config.IsACMResourceReady() {
		// update status of the global hub grafana deployment
		if err := r.updateDeploymentStatus(ctx, mgh, config.CONDITION_TYPE_GRAFANA_AVAILABLE,
			operatorconstants.GHGrafanaDeploymentName); err != nil {
			return err
		}
	}
	return nil
}

func (r *StatusReconciler) updateDeploymentStatus(ctx context.Context,
	mgh *v1alpha4.MulticlusterGlobalHub, conditionType string, deployName string,
) error {
	desiredCondition := metav1.Condition{
		Type:               conditionType,
		Status:             metav1.ConditionFalse,
		Reason:             "DeploymentIsNotReady",
		Message:            "Deployment is not ready",
		LastTransitionTime: metav1.Time{Time: time.Now()},
	}

	deployment := &appsv1.Deployment{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      deployName,
		Namespace: mgh.Namespace,
	}, deployment)
	if err != nil {
		desiredCondition.Message = err.Error()
	} else {
		for _, cond := range deployment.Status.Conditions {
			if cond.Type == appsv1.DeploymentAvailable {
				desiredCondition.Status = metav1.ConditionStatus(cond.Status)
				desiredCondition.Reason = cond.Reason
				desiredCondition.Message = cond.Message
				desiredCondition.LastTransitionTime = cond.LastTransitionTime
			}
		}
	}

	r.log.V(2).Info("updating deployment status", "name", deployName, "message", desiredCondition.Message)
	if err := config.UpdateCondition(ctx, r.Client, mgh, desiredCondition); err != nil {
		return err
	}
	return nil
}

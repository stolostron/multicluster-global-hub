package hubofhubs

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
)

// this controller is responsible for updating the status of the global hub mgh cr
type GlobalHubConditionReconciler struct {
	client.Client
	Log logr.Logger
}

func (r *GlobalHubConditionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log.V(2).Info("reconciling global hub status condition", "namespace", req.Namespace, "name", req.Name)

	mgh := &globalhubv1alpha4.MulticlusterGlobalHub{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Name:      req.Name,
		Namespace: req.Namespace,
	}, mgh); err != nil && errors.IsNotFound(err) {
		// mgh not found, ignore
		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	// update status of the global hub manager deployment
	if err := r.updateDeploymentStatus(ctx, mgh, condition.CONDITION_TYPE_MANAGER_AVAILABLE,
		operatorconstants.GHManagerDeploymentName); err != nil {
		return ctrl.Result{}, err
	}

	// update status of the global hub grafana deployment
	if err := r.updateDeploymentStatus(ctx, mgh, condition.CONDITION_TYPE_GRAFANA_AVAILABLE,
		operatorconstants.GHGrafanaDeploymentName); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *GlobalHubConditionReconciler) updateDeploymentStatus(ctx context.Context,
	mgh *globalhubv1alpha4.MulticlusterGlobalHub, conditionType string, deployName string,
) error {
	desiredCondition := metav1.Condition{
		Type:               conditionType,
		Status:             metav1.ConditionFalse,
		Reason:             "DeploymentIsNotReady",
		Message:            "Deployment is not ready",
		LastTransitionTime: metav1.Time{Time: time.Now()},
	}

	deployment := &appsv1.Deployment{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Name:      deployName,
		Namespace: mgh.Namespace,
	}, deployment); err != nil && errors.IsNotFound(err) {
		// deployment not found, ignore
		r.Log.Info("deployment not found", "name", deployName, "namespace", mgh.Namespace)
	} else if err != nil {
		return err
	}

	for _, cond := range deployment.Status.Conditions {
		if cond.Type == appsv1.DeploymentAvailable {
			desiredCondition.Status = metav1.ConditionStatus(cond.Status)
			desiredCondition.Reason = cond.Reason
			desiredCondition.Message = cond.Message
			desiredCondition.LastTransitionTime = cond.LastTransitionTime
		}
	}

	r.Log.V(2).Info("updating deployment status", "name", deployName, "message", desiredCondition.Message)
	if err := condition.UpdateCondition(ctx, r.Client, mgh, desiredCondition); err != nil {
		return err
	}
	return nil
}

func (r *GlobalHubConditionReconciler) SetupWithManager(mgr manager.Manager) error {
	mghPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}
	return ctrl.NewControllerManagedBy(mgr).
		Named("conditionController").
		For(&globalhubv1alpha4.MulticlusterGlobalHub{}, builder.WithPredicates(mghPred)).
		Watches(&appsv1.Deployment{},
			handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &globalhubv1alpha4.MulticlusterGlobalHub{})).
		Complete(r)
}

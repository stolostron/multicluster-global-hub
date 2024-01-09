// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

// this controller is outside the global resource.
func AddManagedHubClusterController(mgr ctrl.Manager) error {
	clusterPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return targetManagedHub(e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return targetManagedHub(e.ObjectNew) &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion()
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return targetManagedHub(e.Object)
		},
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.ManagedCluster{}, builder.WithPredicates(clusterPred)).
		Complete(&managedHubController{
			log:           ctrl.Log.WithName("managed-hub-controller"),
			client:        mgr.GetClient(),
			finalizerName: constants.GlobalHubCleanupFinalizer,
		})
}

type managedHubController struct {
	client        client.Client
	log           logr.Logger
	finalizerName string
}

func (r *managedHubController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := r.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	cluster := &clusterv1.ManagedCluster{}
	err := r.client.Get(ctx, request.NamespacedName, cluster)
	// the managed hub is deleted
	if err != nil && errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	// the managed hub is deleting then delete the data from database and then remove the finalizer
	if !cluster.DeletionTimestamp.IsZero() || !clusterAvailable(cluster) {
		logger.Info("remove database resources from the hub cluster", "hubAvailable", clusterAvailable(cluster))
		if err := deleteHubResources(cluster); err != nil {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, err
		}

		logger.Info("remove finalizer from the hub cluster")
		if controllerutil.RemoveFinalizer(cluster, r.finalizerName) {
			if err = r.client.Update(ctx, cluster); err != nil {
				return ctrl.Result{RequeueAfter: 5 * time.Second}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// if the managed hub is created/updated, then add the finalizer to it
	if controllerutil.AddFinalizer(cluster, r.finalizerName) {
		logger.Info("add finalizer to the hub cluster")
		if err := r.client.Update(ctx, cluster); err != nil {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, err
		}
	}

	return ctrl.Result{}, nil
}

func clusterAvailable(cluster *clusterv1.ManagedCluster) bool {
	if cluster.Status.Conditions == nil {
		return false
	}
	for _, condition := range cluster.Status.Conditions {
		if condition.Type == clusterv1.ManagedClusterConditionAvailable && condition.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

func deleteHubResources(cluster *clusterv1.ManagedCluster) error {
	db := database.GetGorm()
	return db.Transaction(func(tx *gorm.DB) error {
		// soft delete the clusters from the current hub
		e := tx.Where(&models.ManagedCluster{
			LeafHubName: cluster.Name,
		}).Delete(&models.ManagedCluster{}).Error
		if e != nil {
			return e
		}

		// soft delete the hub from database
		e = tx.Where(&models.LeafHub{
			LeafHubName: cluster.Name,
		}).Delete(&models.LeafHub{}).Error
		if e != nil {
			return e
		}

		// soft delete the policy from the current hub
		e = tx.Where(&models.LocalSpecPolicy{
			LeafHubName: cluster.Name,
		}).Delete(&models.LocalSpecPolicy{}).Error
		if e != nil {
			return e
		}

		// delete the compliance from the current hub
		e = tx.Where(&models.LocalStatusCompliance{
			LeafHubName: cluster.Name,
		}).Delete(&models.LocalStatusCompliance{}).Error
		return e
	})
}

func targetManagedHub(obj client.Object) bool {
	return obj.GetLabels()["vendor"] == "OpenShift" &&
		obj.GetLabels()["openshiftVersion"] != "3" &&
		obj.GetName() != "local-cluster"
}

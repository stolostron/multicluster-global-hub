// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/api/errors"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

// this controller is outside the global resource.
func AddManagedHubController(mgr ctrl.Manager) error {
	clusterPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return !filterManagedHub(e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return !filterManagedHub(e.ObjectNew) &&
				e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration()
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return !filterManagedHub(e.Object)
		},
	}
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.ManagedCluster{}).
		WithEventFilter(clusterPred).
		Complete(&managedHubReconciler{
			client:        mgr.GetClient(),
			log:           ctrl.Log.WithName("managedhub-syncer"),
			finalizerName: constants.GlobalHubCleanupFinalizer,
		}); err != nil {
		return fmt.Errorf("failed to add managed cluster set controller to the manager: %w", err)
	}
	return nil
}

type managedHubReconciler struct {
	client        client.Client
	log           logr.Logger
	finalizerName string
}

func (r *managedHubReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	cluster := &clusterv1.ManagedCluster{}

	err := r.client.Get(ctx, request.NamespacedName, cluster)
	// the managed hub is deleted
	if err != nil && errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	// the managed hub is deleting then delete the data from database and then remove the finalizer
	if !cluster.DeletionTimestamp.IsZero() {
		db := database.GetGorm()
		err = db.Transaction(func(tx *gorm.DB) error {
			e := tx.Where(&models.ManagedCluster{
				LeafHubName: cluster.Name,
			}).Delete(&models.ManagedCluster{}).Error
			if e != nil {
				return e
			}
			e = tx.Where(&models.LeafHub{
				LeafHubName: cluster.Name,
			}).Delete(&models.LeafHub{}).Error
			if e != nil {
				return e
			}
			e = tx.Where(&models.LocalSpecPolicy{
				LeafHubName: cluster.Name,
			}).Delete(&models.LocalSpecPolicy{}).Error
			return e
		})
		if err != nil {
			return ctrl.Result{}, err
		}
		reqLogger.Info("remove finalizer from the cluster")
		if controllerutil.RemoveFinalizer(cluster, r.finalizerName) {
			err = r.client.Update(ctx, cluster)
		}
		return ctrl.Result{}, err
	}

	// if the managed hub is created/updated, then add the finalizer to it
	added := controllerutil.AddFinalizer(cluster, r.finalizerName)
	if !added {
		reqLogger.Info("add finalizer to the cluster")
		err = r.client.Update(ctx, cluster)
	}
	return ctrl.Result{}, err
}

func filterManagedHub(obj client.Object) bool {
	return obj.GetLabels()["vendor"] != "OpenShift" ||
		obj.GetLabels()["openshiftVersion"] == "3" ||
		obj.GetName() == "local-cluster"
}

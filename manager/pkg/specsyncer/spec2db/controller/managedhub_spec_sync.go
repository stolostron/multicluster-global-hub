// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/api/errors"
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
func AddManagedHubController(mgr ctrl.Manager) error {
	clusterPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return !filterManagedHub(e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return !filterManagedHub(e.ObjectNew) &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion()
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return !filterManagedHub(e.Object)
		},
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.ManagedCluster{}, builder.WithPredicates(clusterPred)).
		Complete(&managedHubReconciler{
			log:           ctrl.Log.WithName("managedhub-syncer"),
			client:        mgr.GetClient(),
			finalizerName: constants.GlobalHubCleanupFinalizer,
		})
}

type managedHubReconciler struct {
	client        client.Client
	log           logr.Logger
	finalizerName string
}

func (r *managedHubReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.V(2).Info("reconcile managed hub")

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
			if e != nil {
				return e
			}
			e = tx.Where(&models.LocalStatusCompliance{
				LeafHubName: cluster.Name,
			}).Delete(&models.LocalStatusCompliance{}).Error
			return e
		})
		if err != nil {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, err
		}
		reqLogger.V(2).Info("remove finalizer from the cluster")
		if controllerutil.RemoveFinalizer(cluster, r.finalizerName) {
			if err = r.client.Update(ctx, cluster); err != nil {
				return ctrl.Result{RequeueAfter: 5 * time.Second}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// if the managed hub is created/updated, then add the finalizer to it
	add := controllerutil.AddFinalizer(cluster, r.finalizerName)
	if add {
		reqLogger.V(2).Info("add finalizer to the cluster")
		err = r.client.Update(ctx, cluster)
		if err != nil {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, err
		}
	}
	return ctrl.Result{}, nil
}

func filterManagedHub(obj client.Object) bool {
	return obj.GetLabels()["vendor"] != "OpenShift" ||
		obj.GetLabels()["openshiftVersion"] == "3" ||
		obj.GetName() == "local-cluster"
}

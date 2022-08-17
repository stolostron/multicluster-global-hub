// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	pgx "github.com/jackc/pgx/v4"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db"
)

type genericSpecToDBReconciler struct {
	client         client.Client
	log            logr.Logger
	specDB         db.SpecDB
	tableName      string
	finalizerName  string
	createInstance func() client.Object
	cleanStatus    func(client.Object)
	areEqual       func(client.Object, client.Object) bool
}

const (
	requeuePeriodSeconds = 5
)

func (r *genericSpecToDBReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	instanceUID, instance, err := r.processCR(ctx, request, reqLogger)
	if err != nil {
		reqLogger.Error(err, "Reconciliation failed")
		return ctrl.Result{Requeue: true, RequeueAfter: requeuePeriodSeconds * time.Second}, err
	}

	if instance == nil {
		return ctrl.Result{}, nil
	}

	instanceInTheDatabase, err := r.processInstanceInTheDatabase(ctx, instance, instanceUID, reqLogger)
	if err != nil {
		reqLogger.Error(err, "Reconciliation failed")
		return ctrl.Result{Requeue: true, RequeueAfter: requeuePeriodSeconds * time.Second}, err
	}

	if !r.areEqual(instance, instanceInTheDatabase) {
		reqLogger.Info("Mismatch between hub and the database, updating the database")

		if err := r.specDB.UpdateSpecObject(ctx, r.tableName, instanceUID, &instance); err != nil {
			reqLogger.Error(err, "Reconciliation failed")

			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, err
}

func (r *genericSpecToDBReconciler) processCR(ctx context.Context, request ctrl.Request,
	log logr.Logger,
) (string, client.Object, error) {
	instance := r.createInstance()

	err := r.client.Get(ctx, request.NamespacedName, instance)
	if apierrors.IsNotFound(err) {
		// the instance on hub was deleted, update all the matching instances in the database as deleted
		return "", nil, r.deleteFromTheDatabase(ctx, request.Name, request.Namespace, log)
	}

	if err != nil {
		return "", nil, fmt.Errorf("failed to get the instance from hub: %w", err)
	}

	if isInstanceBeingDeleted(instance) {
		return "", nil, r.removeFinalizerAndDelete(ctx, instance, log)
	}

	err = r.addFinalizer(ctx, instance, log)

	return string(instance.GetUID()), r.cleanInstance(instance), err
}

func isInstanceBeingDeleted(instance client.Object) bool {
	return !instance.GetDeletionTimestamp().IsZero()
}

func (r *genericSpecToDBReconciler) removeFinalizerAndDelete(ctx context.Context, instance client.Object,
	log logr.Logger,
) error {
	if !controllerutil.ContainsFinalizer(instance, r.finalizerName) {
		return nil
	}

	log.Info("Removing an instance from the database")

	// the policy is being deleted, update all the matching policies in the database as deleted
	if err := r.deleteFromTheDatabase(ctx, instance.GetName(), instance.GetNamespace(), log); err != nil {
		return fmt.Errorf("failed to delete an instance from the database: %w", err)
	}

	log.Info("Removing finalizer")
	controllerutil.RemoveFinalizer(instance, r.finalizerName)

	if err := r.client.Update(ctx, instance); err != nil {
		return fmt.Errorf("failed to remove a finalizer: %w", err)
	}

	return nil
}

func (r *genericSpecToDBReconciler) addFinalizer(ctx context.Context, instance client.Object, log logr.Logger) error {
	if controllerutil.ContainsFinalizer(instance, r.finalizerName) {
		return nil
	}

	log.Info("Adding finalizer")
	controllerutil.AddFinalizer(instance, r.finalizerName)

	if err := r.client.Update(ctx, instance); err != nil {
		return fmt.Errorf("failed to add a finalizer: %w", err)
	}

	return nil
}

func (r *genericSpecToDBReconciler) processInstanceInTheDatabase(ctx context.Context, instance client.Object,
	instanceUID string, log logr.Logger,
) (client.Object, error) {
	instanceInTheDatabase := r.createInstance()
	err := r.specDB.QuerySpecObject(ctx, r.tableName, instanceUID, &instanceInTheDatabase)

	if errors.Is(err, pgx.ErrNoRows) {
		log.Info("The instance with the current UID does not exist in the database, inserting...")

		if err := r.specDB.InsertSpecObject(ctx, r.tableName, instanceUID, &instance); err != nil {
			return nil, err
		}

		log.Info("The instance has been inserted into the database")

		return instance, nil // the instance in the database is identical to the instance we just inserted
	}

	if err != nil {
		return nil, err
	}

	return instanceInTheDatabase, nil
}

func (r *genericSpecToDBReconciler) cleanInstance(instance client.Object) client.Object {
	instance.SetUID("")
	instance.SetResourceVersion("")
	instance.SetManagedFields(nil)
	instance.SetFinalizers(nil)
	instance.SetGeneration(0)
	instance.SetOwnerReferences(nil)
	// instance.SetClusterName("")

	delete(instance.GetAnnotations(), "kubectl.kubernetes.io/last-applied-configuration")

	r.cleanStatus(instance)

	return instance
}

func (r *genericSpecToDBReconciler) deleteFromTheDatabase(ctx context.Context, name, namespace string,
	log logr.Logger,
) error {
	log.Info("Instance was deleted, update the deleted field in the database")

	if err := r.specDB.DeleteSpecObject(ctx, r.tableName, name, namespace); err != nil {
		return err
	}

	log.Info("Instance has been updated as deleted in the database")

	return nil
}

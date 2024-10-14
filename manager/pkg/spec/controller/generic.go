// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/specdb"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

const (
	requeueDuration = 5 * time.Second
)

func GlobalResourcePredicate() predicate.Predicate {
	p, _ := predicate.LabelSelectorPredicate(metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      constants.GlobalHubGlobalResourceLabel,
				Operator: metav1.LabelSelectorOpExists,
			},
		},
	})
	return p
}

type genericSpecController struct {
	client         client.Client
	log            logr.Logger
	specDB         specdb.SpecDB
	tableName      string
	finalizerName  string
	createInstance func() client.Object
	cleanObject    func(client.Object)
	areEqual       func(client.Object, client.Object) bool
}

func (r *genericSpecController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	// add/remove finalizer from the resource
	instance := r.createInstance()

	err := r.client.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// the instance on hub was deleted, update all the matching instances in the database as deleted
			if err := r.specDB.DeleteSpecObject(ctx, r.tableName, request.Name, request.Namespace); err != nil {
				return ctrl.Result{Requeue: true, RequeueAfter: requeueDuration}, err
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{Requeue: true, RequeueAfter: requeueDuration}, fmt.Errorf("failed to get the instance: %w", err)
	}

	// deleting
	if !instance.GetDeletionTimestamp().IsZero() {
		if err := r.removeFinalizerAndDelete(ctx, instance, reqLogger); err != nil {
			return ctrl.Result{Requeue: true, RequeueAfter: requeueDuration}, err
		}
	}

	// updating
	if err := r.addFinalizer(ctx, instance, reqLogger); err != nil {
		return ctrl.Result{Requeue: true, RequeueAfter: requeueDuration}, fmt.Errorf("failed to add finalzier: %w", err)
	}

	instanceUID := string(instance.GetUID())
	instance = r.cleanInstance(instance)

	instanceInDatabase, err := r.insertInstanceToDatabase(ctx, instance, instanceUID, reqLogger)
	if err != nil {
		return ctrl.Result{Requeue: true, RequeueAfter: requeuePeriodSeconds * time.Second}, err
	}

	if !r.areEqual(instance, instanceInDatabase) {
		reqLogger.Info("Mismatch between hub and the database, updating the database")

		if err := r.specDB.UpdateSpecObject(ctx, r.tableName, instanceUID, &instance); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, err
}

func (r *genericSpecController) removeFinalizerAndDelete(ctx context.Context, instance client.Object,
	log logr.Logger,
) error {
	if !controllerutil.ContainsFinalizer(instance, r.finalizerName) {
		return nil
	}

	log.Info("Removing an instance from the database")

	// the policy is being deleted, update all the matching policies in the database as deleted
	if err := r.specDB.DeleteSpecObject(ctx, r.tableName, instance.GetName(), instance.GetNamespace()); err != nil {
		return fmt.Errorf("failed to delete an instance from the database: %w", err)
	}

	log.Info("Removing finalizer")
	controllerutil.RemoveFinalizer(instance, r.finalizerName)

	if err := r.client.Update(ctx, instance); err != nil {
		return fmt.Errorf("failed to remove a finalizer: %w", err)
	}

	return nil
}

func (r *genericSpecController) addFinalizer(ctx context.Context, instance client.Object, log logr.Logger) error {
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

func (r *genericSpecController) insertInstanceToDatabase(ctx context.Context, instance client.Object,
	instanceUID string, log logr.Logger,
) (client.Object, error) {
	instanceInTheDatabase := r.createInstance()
	err := r.specDB.QuerySpecObject(ctx, r.tableName, instanceUID, &instanceInTheDatabase)

	if errors.Is(err, sql.ErrNoRows) {
		log.V(2).Info("The instance with the current UID does not exist in the database, inserting...")

		if err := r.specDB.InsertSpecObject(ctx, r.tableName, instanceUID, &instance); err != nil {
			return nil, err
		}

		log.V(2).Info("The instance has been inserted into the database")

		return instance, nil // the instance in the database is identical to the instance we just inserted
	}

	if err != nil {
		return nil, err
	}

	return instanceInTheDatabase, nil
}

func (r *genericSpecController) cleanInstance(instance client.Object) client.Object {
	// instance.SetUID("")
	instance.SetResourceVersion("")
	instance.SetManagedFields(nil)
	instance.SetFinalizers(nil)
	instance.SetGeneration(0)
	instance.SetOwnerReferences(nil)
	// instance.SetClusterName("")

	delete(instance.GetAnnotations(), "kubectl.kubernetes.io/last-applied-configuration")

	r.cleanObject(instance)

	return instance
}

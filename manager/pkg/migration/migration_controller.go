// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"open-cluster-management.io/managed-serviceaccount/apis/authentication/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	klusterletConfigNamePrefix = "migration-"
	bootstrapSecretNamePrefix  = "bootstrap-"
)

var log = logger.DefaultZapLogger()

// ClusterMigrationController reconciles a ManagedClusterMigration object
type ClusterMigrationController struct {
	client.Client
	transport.Producer
	BootstrapSecret       *corev1.Secret
	importClusterInHosted bool
	migrationTopic        string
	// string is MCM ID
	migrationEventProgress map[string]*MigrationEventProgress
}

func NewMigrationController(client client.Client, producer transport.Producer,
	importClusterInHosted bool, migrationTopic string,
) *ClusterMigrationController {
	return &ClusterMigrationController{
		Client:                client,
		Producer:              producer,
		importClusterInHosted: importClusterInHosted,
		migrationTopic:        migrationTopic,
	}
}

// SetupWithManager sets up the controller with the Manager.
func (m *ClusterMigrationController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named("migration-ctrl").
		For(&migrationv1alpha1.ManagedClusterMigration{}).
		Watches(&v1beta1.ManagedServiceAccount{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Name:      obj.GetName(), // the msa name = migration name
							Namespace: utils.GetDefaultNamespace(),
						},
					},
				}
			}),
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					return false
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					return false
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					e.Object.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
					labels := e.Object.GetLabels()
					if value, ok := labels["owner"]; ok {
						if value == strings.ToLower(constants.ManagedClusterMigrationKind) {
							return !e.DeleteStateUnknown
						}
						return false
					}
					return false
				},
			})).
		Watches(&corev1.Secret{}, &handler.EnqueueRequestForObject{},
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					return e.Object.GetLabels()[constants.LabelKeyIsManagedServiceAccount] == "true"
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					if e.ObjectNew.GetLabels()[constants.LabelKeyIsManagedServiceAccount] == "true" {
						return e.ObjectOld.GetResourceVersion() != e.ObjectNew.GetResourceVersion()
					}
					return false
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					// case 1: the secret is deleted by user. In this case, the secret will be created by managedserviceaccount
					// so we do not need to handle it in deleteFunc. instead, we handle it in createFunc.
					// case 2: the secret is deleted by managedserviceaccount. we will handle it in managedserviceaccount deleteFunc.
					return false
				},
			})).
		Complete(m)
}

func (m *ClusterMigrationController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.Infof("reconcile managed cluster migration %v", req)
	// TODO: we only allow to have one migration processing at a time.
	// if there are multiple migrations, we need to wait for the first one to finish.
	// we need to put the migrations into a queue and provide the message in that CR to tell the user
	mcm := &migrationv1alpha1.ManagedClusterMigration{}
	// the migration name is the same as managedserviceaccount and the secret
	err := m.Get(ctx, types.NamespacedName{Namespace: utils.GetDefaultNamespace(), Name: req.Name}, mcm)
	if apierrors.IsNotFound(err) {
		// If the custom resource is not found then it usually means that it was deleted or not created
		// In this way, we will stop the reconciliation
		log.Info("managedclustermigration resource not found. Ignoring since object must be deleted")
		return ctrl.Result{}, nil
	}
	if err != nil {
		log.Errorf("failed to get managedclustermigration %v", err)
		return ctrl.Result{}, err
	}

	// add finalizer if resources is not being deleted
	if !controllerutil.ContainsFinalizer(mcm, constants.ManagedClusterMigrationFinalizer) {
		controllerutil.AddFinalizer(mcm, constants.ManagedClusterMigrationFinalizer)
	}
	if err := m.Update(ctx, mcm); err != nil {
		return ctrl.Result{}, err
	}

	// validating
	requeue, err := m.validating(ctx, mcm)
	if err != nil {
		return ctrl.Result{}, err
	}
	if requeue {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// initializing
	requeue, err = m.initializing(ctx, mcm)
	if err != nil {
		return ctrl.Result{}, err
	}
	if requeue {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// registering
	requeue, err = m.registering(ctx, mcm)
	if err != nil {
		return ctrl.Result{}, err
	}
	if requeue {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// deploying
	requeue, err = m.deploying(ctx, mcm)
	if err != nil {
		return ctrl.Result{}, err
	}
	if requeue {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// completed
	requeue, err = m.completed(ctx, mcm)
	if err != nil {
		return ctrl.Result{}, err
	}
	if requeue {
		return ctrl.Result{RequeueAfter: deleteInterval}, nil
	}

	// Remove finalizer when all stages have been successfully pruned
	if !mcm.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(mcm, constants.ManagedClusterMigrationFinalizer) {
			controllerutil.RemoveFinalizer(mcm, constants.ManagedClusterMigrationFinalizer)
			if err := m.Update(ctx, mcm); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (m *ClusterMigrationController) deleteManagedServiceAccount(ctx context.Context,
	migration *migrationv1alpha1.ManagedClusterMigration,
) error {
	msa := &v1beta1.ManagedServiceAccount{}
	if err := m.Get(ctx, types.NamespacedName{
		Name:      migration.Name,
		Namespace: migration.Spec.To,
	}, msa); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return m.Delete(ctx, msa)
}

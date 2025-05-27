// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"open-cluster-management.io/managed-serviceaccount/apis/authentication/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/configs"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	migrationbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
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
	BootstrapSecret *corev1.Secret
	managerConfigs  *configs.ManagerConfig
}

func NewMigrationController(client client.Client, producer transport.Producer,
	managerConfig *configs.ManagerConfig,
) *ClusterMigrationController {
	return &ClusterMigrationController{
		Client:         client,
		Producer:       producer,
		managerConfigs: managerConfig,
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

	// get the current migration
	mcm, err := m.getCurrentMigration(ctx, req)
	if err != nil {
		log.Errorf("failed to get managedclustermigration %v", err)
		return ctrl.Result{}, err
	}
	if mcm == nil {
		log.Infof("no desired managedclustermigration found")
		return ctrl.Result{}, nil
	}

	log.Debugf("current migration: %s", mcm.Name)
	// add finalizer if resources is not being deleted
	if !controllerutil.ContainsFinalizer(mcm, constants.ManagedClusterMigrationFinalizer) {
		controllerutil.AddFinalizer(mcm, constants.ManagedClusterMigrationFinalizer)
	}
	if err := m.Update(ctx, mcm); err != nil {
		return ctrl.Result{}, err
	}

	// validating
	err = m.validating(ctx, mcm)
	if err != nil {
		return ctrl.Result{}, err
	}

	// initializing
	requeue, err := m.initializing(ctx, mcm)
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
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
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

// getCurrentMigration returns the current migration object.
// It returns nil if no migration is in progress.
// It select the oldest migration object if there are multiple migrations.
// For other migrations which waiting for migrating, it will update the condition and phase to pending.
func (m *ClusterMigrationController) getCurrentMigration(ctx context.Context,
	req ctrl.Request,
) (*migrationv1alpha1.ManagedClusterMigration, error) {
	migrationList := &migrationv1alpha1.ManagedClusterMigrationList{}
	err := m.List(ctx, migrationList)
	if err != nil {
		return nil, err
	}
	var desiredMigration *migrationv1alpha1.ManagedClusterMigration
	var pendingMigrations []migrationv1alpha1.ManagedClusterMigration
	for _, migration := range migrationList.Items {
		// if the current migration is deleted, we need to return it
		if !migration.GetDeletionTimestamp().IsZero() && req.Name == migration.Name {
			return &migration, nil
		}
		// skip the migration which is completed or failed
		if migration.Status.Phase == migrationv1alpha1.PhaseCompleted {
			continue
		}
		// skip the migration which is failed and cleaned
		if migration.Status.Phase == migrationv1alpha1.PhaseFailed &&
			meta.FindStatusCondition(migration.Status.Conditions, migrationv1alpha1.ConditionTypeCleaned) != nil {
			continue
		}

		// if the migration has the finalizer, it means that the migration is in progress
		if controllerutil.ContainsFinalizer(&migration, constants.ManagedClusterMigrationFinalizer) {
			desiredMigration = &migration
			continue
		}

		if desiredMigration == nil {
			desiredMigration = &migration
		} else if migration.CreationTimestamp.Before(&desiredMigration.CreationTimestamp) {
			pendingMigrations = append(pendingMigrations, *desiredMigration)
			desiredMigration = &migration
		} else {
			pendingMigrations = append(pendingMigrations, migration)
		}
	}
	if desiredMigration == nil {
		return nil, nil
	}
	if desiredMigration.Status.Phase == migrationv1alpha1.PhasePending {
		// if the migration is in pending, update the condition
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := m.Client.Get(ctx, client.ObjectKeyFromObject(desiredMigration), desiredMigration); err != nil {
				return err
			}

			if err := m.UpdateCondition(ctx,
				desiredMigration,
				migrationv1alpha1.ConditionTypeStarted,
				metav1.ConditionTrue,
				"migrationInProgress",
				"Migration is in progress",
			); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			log.Errorf("failed to update the %s condition: %v", desiredMigration.Name, err)
		}
	}

	for _, mcm := range pendingMigrations {
		e := m.UpdateConditionWithRetry(ctx,
			&mcm,
			migrationv1alpha1.ConditionTypeStarted,
			metav1.ConditionFalse,
			"waitOtherMigrationCompleted",
			fmt.Sprintf("Wait for other migration <%s> to be completed", desiredMigration.Name),
			migrationStageTimeout,
		)
		if e != nil {
			log.Errorf("failed to update the %s condition: %v", mcm.Name, e)
		}
	}
	return desiredMigration, nil
}

// sendEventToDestinationHub:
// 1. only send the msa info to allow auto approve if KlusterletAddonConfig is nil -> registering
// 2. if KlusterletAddonConfig is not nil -> deploying
func (m *ClusterMigrationController) sendEventToDestinationHub(ctx context.Context,
	migration *migrationv1alpha1.ManagedClusterMigration, stage string, managedClusters []string,
) error {
	// if the target cluster is local cluster, then the msaNamespace is open-cluster-management-agent-addon
	isLocalCluster := false
	managedCluster := &clusterv1.ManagedCluster{}
	if err := m.Client.Get(ctx, types.NamespacedName{
		Name: migration.Spec.To,
	}, managedCluster); err != nil {
		return err
	}
	if managedCluster.Labels[constants.LocalClusterName] == "true" {
		isLocalCluster = true
	}
	log.Debugf("%s is %v", migration.Spec.To, isLocalCluster)

	// default managedserviceaccount addon namespace
	msaNamespace := "open-cluster-management-agent-addon"
	if m.managerConfigs.ImportClusterInHosted && !isLocalCluster {
		// hosted mode, the  managedserviceaccount addon namespace
		msaNamespace = "open-cluster-management-global-hub-agent-addon"
	}
	msaInstallNamespaceAnnotation := "global-hub.open-cluster-management.io/managed-serviceaccount-install-namespace"
	// if user specifies the managedserviceaccount addon namespace, then use it
	if val, ok := migration.Annotations[msaInstallNamespaceAnnotation]; ok {
		msaNamespace = val
	}
	managedClusterMigrationToEvent := &migrationbundle.ManagedClusterMigrationToEvent{
		MigrationId:                           string(migration.GetUID()),
		Stage:                                 stage,
		ManagedServiceAccountName:             migration.Name,
		ManagedServiceAccountInstallNamespace: msaNamespace,
		ManagedClusters:                       managedClusters,
	}

	payloadToBytes, err := json.Marshal(managedClusterMigrationToEvent)
	if err != nil {
		return fmt.Errorf("failed to marshal managed cluster migration to event(%v) - %w",
			managedClusterMigrationToEvent, err)
	}

	eventType := constants.MigrationTargetMsgKey
	evt := utils.ToCloudEvent(eventType, constants.CloudEventGlobalHubClusterName, migration.Spec.To, payloadToBytes)
	if err := m.Producer.SendEvent(ctx, evt); err != nil {
		return fmt.Errorf("failed to sync managedclustermigration event(%s) from source(%s) to destination(%s) - %w",
			eventType, constants.CloudEventGlobalHubClusterName, migration.Spec.To, err)
	}
	return nil
}

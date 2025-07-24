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
	mcm, err := m.selectAndPrepareMigration(ctx, req)
	if err != nil {
		log.Errorf("failed to get managedclustermigration %v", err)
		return ctrl.Result{}, err
	}
	if mcm == nil {
		log.Info("no desired managedclustermigration found")
		return ctrl.Result{}, nil
	}

	log.Infof("processing migration instance: %s, phase: %s", mcm.Name, mcm.Status.Phase)

	// add the finalizer if the migration is not being deleted
	if mcm.DeletionTimestamp == nil {
		if controllerutil.AddFinalizer(mcm, constants.ManagedClusterMigrationFinalizer) {
			if err := m.Update(ctx, mcm); err != nil {
				log.Errorf("failed to add finalizer: %v", err)
				return ctrl.Result{}, err
			}
			// initializing the migration status for the instance
			AddMigrationStatus(string(mcm.GetUID()))
		}
	}

	// Process migration phases with centralized phase transition logic
	phases := []struct {
		name    string
		handler func(context.Context, *migrationv1alpha1.ManagedClusterMigration) (bool, error)
	}{
		{"validating", m.validating},
		{"initializing", m.initializing},
		{"deploying", m.deploying},
		{"registering", m.registering},
		{"cleaning", m.cleaning},
	}

	for _, phase := range phases {
		if requeue, err := phase.handler(ctx, mcm); err != nil {
			log.Errorf("error in phase %s: %v", phase.name, err)
			return ctrl.Result{}, err
		} else if requeue {
			log.Infof("requeuing migration %s in phase %s", mcm.Name, phase.name)
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
	}

	// clean up the finalizer
	if mcm.DeletionTimestamp != nil {
		if controllerutil.RemoveFinalizer(mcm, constants.ManagedClusterMigrationFinalizer) {
			if updateErr := m.Update(ctx, mcm); updateErr != nil {
				log.Errorf("failed to remove finalizer: %v", updateErr)
			}
			RemoveMigrationStatus(string(mcm.GetUID()))
		}
	}

	return ctrl.Result{}, nil
}

// sendEventToDestinationHub sends migration event to the destination hub with validation.
// It creates the appropriate bundle based on the migration stage and sends it via the transport.
func (m *ClusterMigrationController) sendEventToDestinationHub(ctx context.Context,
	migration *migrationv1alpha1.ManagedClusterMigration, stage string, managedClusters []string,
) error {
	log.Debugf("sending event to destination hub %s for migration %s in stage %s",
		migration.Spec.To, migration.Name, stage)
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

	targetHubBundle := &migrationbundle.MigrationTargetHubBundle{
		MigrationId:     string(migration.GetUID()),
		Stage:           stage,
		ManagedClusters: managedClusters,
	}

	// if the registered status is false, and the current stage is cleaning, then the migration will be rolled back
	if stage == migrationv1alpha1.PhaseCleaning &&
		!meta.IsStatusConditionTrue(migration.Status.Conditions, migrationv1alpha1.ConditionTypeRegistered) {
		targetHubBundle.Rollback = true
	}

	// require the msa info when initializing or cleaning
	if stage == migrationv1alpha1.PhaseInitializing || stage == migrationv1alpha1.PhaseCleaning {

		installNamespace, err := m.getManagedServiceAccountAddonInstallNamespace(ctx, migration)
		if err != nil {
			return fmt.Errorf("failed to get the managedserviceaccount addon installNamespace: %v", err)
		}

		targetHubBundle.ManagedServiceAccountName = migration.Name
		targetHubBundle.ManagedServiceAccountInstallNamespace = installNamespace
	}

	payloadToBytes, err := json.Marshal(targetHubBundle)
	if err != nil {
		return fmt.Errorf("failed to marshal managed cluster migration to event(%v) - %w",
			targetHubBundle, err)
	}

	eventType := constants.MigrationTargetMsgKey
	evt := utils.ToCloudEvent(eventType, constants.CloudEventGlobalHubClusterName, migration.Spec.To, payloadToBytes)
	if err := m.Producer.SendEvent(ctx, evt); err != nil {
		return fmt.Errorf("failed to sync managedclustermigration event(%s) from source(%s) to destination(%s) - %w",
			eventType, constants.CloudEventGlobalHubClusterName, migration.Spec.To, err)
	}
	return nil
}

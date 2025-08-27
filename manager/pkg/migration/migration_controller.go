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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
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

var (
	// the following timeouts are accumulated from the start of the migration
	migratingTimeout   time.Duration
	cleaningTimeout    time.Duration
	rollbackingTimeout time.Duration
	registeringTimeout time.Duration
)

var log = logger.DefaultZapLogger()

// ClusterMigrationController reconciles a ManagedClusterMigration object
type ClusterMigrationController struct {
	client.Client
	transport.Producer
	EventRecorder record.EventRecorder
	Scheme        *runtime.Scheme
}

func NewMigrationController(client client.Client, producer transport.Producer,
	managerConfig *configs.ManagerConfig, eventRecorder record.EventRecorder,
) *ClusterMigrationController {
	return &ClusterMigrationController{
		Client:        client,
		Producer:      producer,
		EventRecorder: eventRecorder,
	}
}

// SetupWithManager sets up the controller with the Manager.
func (m *ClusterMigrationController) SetupWithManager(mgr ctrl.Manager) error {
	m.Scheme = mgr.GetScheme()
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

	log.Infof("processing migration instance: %s", mcm.Name)

	// add the finalizer if the migration is not being deleted
	if mcm.DeletionTimestamp == nil {
		if controllerutil.AddFinalizer(mcm, constants.ManagedClusterMigrationFinalizer) {
			if err := m.Update(ctx, mcm); err != nil {
				log.Errorf("failed to add finalizer: %v", err)
				return ctrl.Result{}, err
			}
		}
		// initializing the migration status for the instance
		AddMigrationStatus(string(mcm.GetUID()))
	}

	// setup custom timeouts from config
	if err := m.SetupMigrationStageTimeout(mcm); err != nil {
		log.Errorf("failed to setup timeouts from config: %v", err)
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

	// deploying
	requeue, err = m.deploying(ctx, mcm)
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

	// rollbacking
	requeue, err = m.rollbacking(ctx, mcm)
	if err != nil {
		return ctrl.Result{}, err
	}
	if requeue {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// cleaning
	requeue, err = m.cleaning(ctx, mcm)
	if err != nil {
		return ctrl.Result{}, err
	}
	if requeue {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// clean up the finalizer
	if mcm.DeletionTimestamp != nil {
		RemoveMigrationStatus(string(mcm.GetUID()))
		if controllerutil.RemoveFinalizer(mcm, constants.ManagedClusterMigrationFinalizer) {
			if updateErr := m.Update(ctx, mcm); updateErr != nil {
				log.Errorf("failed to remove finalizer: %v", updateErr)
			}
		}
		log.Infof("clean up migration status for migrationId: %s", mcm.GetUID())
	}
	return ctrl.Result{}, nil
}

// sendEventToTargetHub:
// 1. only send the msa info to allow auto approve if KlusterletAddonConfig is nil -> registering
// 2. if KlusterletAddonConfig is not nil -> deploying
func (m *ClusterMigrationController) sendEventToTargetHub(ctx context.Context,
	migration *migrationv1alpha1.ManagedClusterMigration, stage string, managedClusters []string,
	rollbackStage string,
) error {
	// if the target cluster is local cluster, then the msaNamespace is open-cluster-management-agent-addon
	isLocalCluster := false
	managedCluster := &clusterv1.ManagedCluster{}
	if err := m.Get(ctx, types.NamespacedName{
		Name: migration.Spec.To,
	}, managedCluster); err != nil {
		return err
	}
	if managedCluster.Labels[constants.LocalClusterName] == "true" {
		isLocalCluster = true
	}
	log.Debugf("%s is %v", migration.Spec.To, isLocalCluster)

	managedClusterMigrationToEvent := &migrationbundle.MigrationTargetBundle{
		MigrationId:               string(migration.GetUID()),
		Stage:                     stage,
		ManagedClusters:           managedClusters,
		RollbackStage:             rollbackStage,
		ManagedServiceAccountName: migration.Name,
		// the timeout in agent part should less than manager part,
		// the event in agent need time to send event to manager
		RegisteringTimeoutMinutes: int((registeringTimeout - 2*time.Minute).Minutes()),
	}

	// namespace
	installNamespace, err := m.getManagedServiceAccountAddonInstallNamespace(ctx, migration)
	if err != nil {
		return err
	}
	managedClusterMigrationToEvent.ManagedServiceAccountInstallNamespace = installNamespace

	payloadToBytes, err := json.Marshal(managedClusterMigrationToEvent)
	if err != nil {
		return fmt.Errorf("failed to marshal managed cluster migration to event(%v) - %w",
			managedClusterMigrationToEvent, err)
	}

	eventType := constants.MigrationTargetMsgKey
	evt := utils.ToCloudEvent(eventType, constants.CloudEventGlobalHubClusterName, migration.Spec.To, payloadToBytes)
	if err := m.SendEvent(ctx, evt); err != nil {
		return fmt.Errorf("failed to sync managedclustermigration event(%s) from source(%s) to destination(%s) - %w",
			eventType, constants.CloudEventGlobalHubClusterName, migration.Spec.To, err)
	}
	return nil
}

// SetupMigrationStageTimeout reads the SupportedConfigs field and sets custom timeouts
func (m *ClusterMigrationController) SetupMigrationStageTimeout(mcm *migrationv1alpha1.ManagedClusterMigration) error {
	// Check if StageTimeout is specified in SupportedConfigs
	if mcm.Spec.SupportedConfigs != nil && mcm.Spec.SupportedConfigs.StageTimeout != nil {
		// Set the timeout value to all timeout variables
		migratingTimeout = mcm.Spec.SupportedConfigs.StageTimeout.Duration
		registeringTimeout = mcm.Spec.SupportedConfigs.StageTimeout.Duration
		cleaningTimeout = mcm.Spec.SupportedConfigs.StageTimeout.Duration
		rollbackingTimeout = mcm.Spec.SupportedConfigs.StageTimeout.Duration

		log.Infof("set migration: %v timeouts to %v", mcm.Name, migratingTimeout)
	} else {
		migratingTimeout = 5 * time.Minute
		rollbackingTimeout = migratingTimeout
		registeringTimeout = 12 * time.Minute
		cleaningTimeout = registeringTimeout
	}

	return nil
}

func getTimeout(stage string) time.Duration {
	switch stage {
	case migrationv1alpha1.PhaseRollbacking:
		return rollbackingTimeout
	case migrationv1alpha1.PhaseRegistering:
		return registeringTimeout
	case migrationv1alpha1.PhaseCleaning:
		return cleaningTimeout
	default:
		return migratingTimeout
	}
}

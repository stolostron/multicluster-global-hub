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
	"k8s.io/apimachinery/pkg/types"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"open-cluster-management.io/managed-serviceaccount/apis/authentication/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	// Check if migration is in final state (completed or failed)
	if mcm.Status.Phase == migrationv1alpha1.PhaseCompleted || mcm.Status.Phase == migrationv1alpha1.PhaseFailed {
		log.Infof("migration %s is in final state: %s", mcm.Name, mcm.Status.Phase)
		return ctrl.Result{}, nil
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

	// cleaning
	requeue, err = m.cleaning(ctx, mcm)
	if err != nil {
		return ctrl.Result{}, err
	}
	if requeue {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Handle deletion
	if !mcm.DeletionTimestamp.IsZero() {
		// Let the cleaning phase handle finalizer removal
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
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

	managedClusterMigrationToEvent := &migrationbundle.ManagedClusterMigrationToEvent{
		MigrationId:     string(migration.GetUID()),
		Stage:           stage,
		ManagedClusters: managedClusters,
	}

	// require the msa info when initializing or cleaning
	if stage == migrationv1alpha1.PhaseInitializing || stage == migrationv1alpha1.PhaseCleaning {
		// get the namespace from the managedserviceaccount addon
		msa := addonapiv1alpha1.ManagedClusterAddOn{ObjectMeta: metav1.ObjectMeta{
			Name:      "managed-serviceaccount",
			Namespace: migration.Spec.To, // target hub
		}}
		err := m.Client.Get(ctx, client.ObjectKeyFromObject(&msa), &msa)
		if err != nil {
			return fmt.Errorf("failed to get the managedserviceaccount %s/%s: %v", msa.Namespace, msa.Name, err)
		}
		if msa.Status.Namespace == "" {
			return fmt.Errorf("the status.namespace of managedserviceaccount %s/%s is not ready", msa.Namespace, msa.Name)
		}
		msaNamespace := msa.Status.Namespace
		msaInstallNamespaceAnnotation := "global-hub.open-cluster-management.io/managed-serviceaccount-install-namespace"
		// if user specifies the managedserviceaccount addon namespace, then use it
		if val, ok := migration.Annotations[msaInstallNamespaceAnnotation]; ok {
			msaNamespace = val
		}
		managedClusterMigrationToEvent.ManagedServiceAccountName = migration.Name
		managedClusterMigrationToEvent.ManagedServiceAccountInstallNamespace = msaNamespace
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

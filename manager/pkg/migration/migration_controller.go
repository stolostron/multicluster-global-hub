// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"strings"
	"time"

	klusterletv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
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
}

func NewMigrationController(client client.Client, producer transport.Producer,
	importClusterInHosted bool,
) *ClusterMigrationController {
	return &ClusterMigrationController{
		Client:                client,
		Producer:              producer,
		importClusterInHosted: importClusterInHosted,
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

	if !mcm.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(mcm, constants.ManagedClusterMigrationFinalizer) {
			if err := m.deleteManagedServiceAccount(ctx, mcm); err != nil {
				return ctrl.Result{}, err
			}

			controllerutil.RemoveFinalizer(mcm, constants.ManagedClusterMigrationFinalizer)
			if err := m.Update(ctx, mcm); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// add finalizer
	if !controllerutil.ContainsFinalizer(mcm, constants.ManagedClusterMigrationFinalizer) {
		controllerutil.AddFinalizer(mcm, constants.ManagedClusterMigrationFinalizer)
	}
	if err := m.Update(ctx, mcm); err != nil {
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

	// TODO: registering, deploying, completed
	return ctrl.Result{}, nil
}

func (m *ClusterMigrationController) generateKlusterletConfig(req ctrl.Request) *klusterletv1alpha1.KlusterletConfig {
	return &klusterletv1alpha1.KlusterletConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: klusterletConfigNamePrefix + req.Namespace,
		},
		Spec: klusterletv1alpha1.KlusterletConfigSpec{
			BootstrapKubeConfigs: operatorv1.BootstrapKubeConfigs{
				Type: operatorv1.LocalSecrets,
				LocalSecrets: operatorv1.LocalSecretsConfig{
					KubeConfigSecrets: []operatorv1.KubeConfigSecret{
						{
							Name: bootstrapSecretNamePrefix + req.Namespace,
						},
					},
				},
			},
		},
	}
}

func (m *ClusterMigrationController) generateBootstrapSecret(kubeconfig *clientcmdapi.Config,
	req ctrl.Request,
) (*corev1.Secret, error) {
	// Serialize the kubeconfig to YAML
	kubeconfigBytes, err := clientcmd.Write(*kubeconfig)
	if err != nil {
		return nil, err
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bootstrapSecretNamePrefix + req.Namespace,
			Namespace: "multicluster-engine",
		},
		Data: map[string][]byte{
			"kubeconfig": kubeconfigBytes,
		},
	}, nil
}

func (m *ClusterMigrationController) generateKubeconfig(ctx context.Context, req ctrl.Request) (*clientcmdapi.Config, error) {
	// get the secret which is generated by msa
	desiredSecret := &corev1.Secret{}
	if err := m.Client.Get(ctx, types.NamespacedName{
		Name:      req.Name,
		Namespace: req.Namespace,
	}, desiredSecret); err != nil {
		return nil, err
	}
	// fetch the managed cluster to get url
	managedcluster := &clusterv1.ManagedCluster{}
	if err := m.Client.Get(ctx, types.NamespacedName{
		Name: req.Namespace,
	}, managedcluster); err != nil {
		return nil, err
	}

	config := clientcmdapi.NewConfig()
	config.Clusters[req.Namespace] = &clientcmdapi.Cluster{
		Server:                   managedcluster.Spec.ManagedClusterClientConfigs[0].URL,
		CertificateAuthorityData: desiredSecret.Data["ca.crt"],
	}
	config.AuthInfos["user"] = &clientcmdapi.AuthInfo{
		Token: string(desiredSecret.Data["token"]),
	}
	config.Contexts["default-context"] = &clientcmdapi.Context{
		Cluster:  req.Namespace,
		AuthInfo: "user",
	}
	config.CurrentContext = "default-context"

	return config, nil
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

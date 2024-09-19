// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"

	klusterletv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/utils/ptr"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	"open-cluster-management.io/managed-serviceaccount/apis/authentication/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// MigrationReconciler reconciles a ManagedClusterMigration object
type MigrationReconciler struct {
	manager.Manager
	client.Client
	BootstrapSecret *corev1.Secret // only for test. should be deleted during integration
}

func NewMigrationReconciler(mgr manager.Manager) *MigrationReconciler {
	return &MigrationReconciler{
		Manager: mgr,
		Client:  mgr.GetClient(),
	}
}

const (
	klusterletConfigNamePrefix = "migration-"
	bootstrapSecretNamePrefix  = "bootstrap-"
)

// SetupWithManager sets up the controller with the Manager.
func (m *MigrationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named("migration-controller").
		Owns(&migrationv1alpha1.ManagedClusterMigration{}).
		Watches(&v1beta1.ManagedServiceAccount{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				if !obj.GetDeletionTimestamp().IsZero() {
					// trigger to recreate the msa
					return []reconcile.Request{
						{
							NamespacedName: types.NamespacedName{
								Name:      obj.GetName(),
								Namespace: constants.GHDefaultNamespace,
							},
						},
					}
				}
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Name:      obj.GetName(),
							Namespace: obj.GetNamespace(),
						},
					},
				}
			}),
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					return false
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					ownerReferences := e.ObjectNew.GetOwnerReferences()
					for _, reference := range ownerReferences {
						if kind := reference.Kind; kind == constants.ManagedClusterMigrationKind {
							return e.ObjectOld.GetResourceVersion() != e.ObjectNew.GetResourceVersion()
						}
					}
					return false
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					ownerReferences := e.Object.GetOwnerReferences()
					for _, reference := range ownerReferences {
						if kind := reference.Kind; kind == constants.ManagedClusterMigrationKind {
							return !e.DeleteStateUnknown
						}
					}
					return false
				},
			})).
		Complete(m)
}

func (m *MigrationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	if req.Namespace == constants.GHDefaultNamespace {
		// create managedserviceaccount
		migration := &migrationv1alpha1.ManagedClusterMigration{}
		err := m.Get(ctx, req.NamespacedName, migration)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// If the custom resource is not found then it usually means that it was deleted or not created
				// In this way, we will stop the reconciliation
				log.Info("managedclustermigration resource not found. Ignoring since object must be deleted")
				return ctrl.Result{}, nil
			}
			// Error reading the object - requeue the request.
			log.Error(err, "Failed to get managedclustermigration")
			return ctrl.Result{}, err
		}
		if err := m.ensureManagedServiceAccount(ctx, migration); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		// create kubeconfig based on the secret of managedserviceaccount
		kubeconfig, err := m.generateKubeconfig(ctx, req)
		if err != nil {
			return ctrl.Result{}, err
		}
		m.BootstrapSecret, err = m.generateBootstrapSecret(kubeconfig, req)
		if err != nil {
			return ctrl.Result{}, err
		}
		m.generateKlusterConfig(req)
		// send the kubeconfig to managedclustermigration.From
	}
	return ctrl.Result{}, nil
}

func (m *MigrationReconciler) generateKlusterConfig(req ctrl.Request) *klusterletv1alpha1.KlusterletConfig {
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
						// need remove one when import controller
						{
							Name: bootstrapSecretNamePrefix + req.Namespace,
						},
					},
				},
			},
		},
	}
}

func (m *MigrationReconciler) generateBootstrapSecret(kubeconfig *clientcmdapi.Config,
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

func (m *MigrationReconciler) generateKubeconfig(ctx context.Context, req ctrl.Request) (*clientcmdapi.Config, error) {
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

func (m *MigrationReconciler) ensureManagedServiceAccount(ctx context.Context,
	migration *migrationv1alpha1.ManagedClusterMigration,
) error {
	// create a desired msa
	desiredMSA := &v1beta1.ManagedServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      migration.GetName(),
			Namespace: migration.Spec.To,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         migrationv1alpha1.GroupVersion.String(),
					Kind:               constants.ManagedClusterMigrationKind,
					Name:               migration.GetName(),
					UID:                migration.GetUID(),
					BlockOwnerDeletion: ptr.To(true),
					Controller:         ptr.To(true),
				},
			},
		},
	}

	existingMSA := &v1beta1.ManagedServiceAccount{}
	err := m.Client.Get(ctx, types.NamespacedName{
		Name:      migration.GetName(),
		Namespace: migration.Spec.To,
	}, existingMSA)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return m.Client.Create(ctx, desiredMSA)
		}
		return err
	}
	return nil
}

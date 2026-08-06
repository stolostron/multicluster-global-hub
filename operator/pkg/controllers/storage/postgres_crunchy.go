package storage

import (
	"context"
	"fmt"
	"time"

	postgresv1beta1 "github.com/crunchydata/postgres-operator/pkg/apis/postgres-operator.crunchydata.com/v1beta1"
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	ocv1 "github.com/operator-framework/operator-controller/api/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorutils "github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	postgresAdminUsername    = "postgres"
	postgresReadonlyUsername = "global-hub-readonly-user" // #nosec G101
)

const (
	CrunchyClusterExtensionName = "crunchy-postgres-operator"
	CrunchyInstallerSAName      = "crunchy-postgres-installer"
	CrunchyInstallerCRBName     = "crunchy-postgres-installer"
)

var (
	SubscriptionName = "crunchy-postgres-operator"
	// prod postgres variables
	channel                = "v5"
	installPlanApproval    = subv1alpha1.ApprovalAutomatic
	packageName            = "crunchy-postgres-operator"
	catalogSourceName      = "certified-operators"
	catalogSourceNamespace = "openshift-marketplace"

	// community postgres variables
	communityChannel           = "v5"
	communityPackageName       = "postgresql"
	communityCatalogSourceName = "community-operators"

	replicas3 int32 = 3

	// postgres storage size: 25Gi should be enough for 18 months data
	// 5 managed hubs with 300 managed cluster each and 50 policies per managed hub cluster
	storageSize = "25Gi"
)

// EnsureCrunchyPostgresSub verifies resources needed for Crunchy Postgres are created
func EnsureCrunchyPostgresSub(ctx context.Context, c client.Client, mgh *globalhubv1alpha4.MulticlusterGlobalHub) error {
	// Generate sub config from mcgh CR
	subConfig := &subv1alpha1.SubscriptionConfig{
		NodeSelector: mgh.Spec.NodeSelector,
		Tolerations:  mgh.Spec.Tolerations,
	}

	existSub, err := operatorutils.GetSubscriptionByName(ctx, c, mgh.Namespace, SubscriptionName)
	if err != nil {
		return err
	}

	if existSub == nil {
		// Sub is nil so create a new one
		return c.Create(ctx, NewCrunchySubscription(mgh, subConfig, operatorutils.IsCommunityMode()))
	}

	// Apply Crunchy Postgres sub
	calcSub := ExpectedSubscription(existSub, subConfig, operatorutils.IsCommunityMode())
	if !equality.Semantic.DeepEqual(existSub.Spec, calcSub.Spec) {
		return c.Update(ctx, calcSub)
	}
	return nil
}

// EnsureCrunchyPostgresClusterExtension creates the ServiceAccount, ClusterRoleBinding, and
// ClusterExtension for Crunchy Postgres using OLMv1
func EnsureCrunchyPostgresClusterExtension(ctx context.Context, c client.Client,
	mgh *globalhubv1alpha4.MulticlusterGlobalHub,
) error {
	// ensure installer ServiceAccount
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CrunchyInstallerSAName,
			Namespace: mgh.Namespace,
			Labels: map[string]string{
				constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
			},
		},
	}
	if err := c.Get(ctx, types.NamespacedName{
		Name: sa.Name, Namespace: sa.Namespace,
	}, &corev1.ServiceAccount{}); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("get crunchy installer ServiceAccount %s/%s: %w", sa.Namespace, sa.Name, err)
		}
		if err := c.Create(ctx, sa); err != nil {
			return fmt.Errorf("create crunchy installer ServiceAccount %s/%s: %w", sa.Namespace, sa.Name, err)
		}
	}

	// ensure ClusterRoleBinding with cluster-admin
	expectedSubjects := []rbacv1.Subject{
		{
			Kind:      rbacv1.ServiceAccountKind,
			Name:      CrunchyInstallerSAName,
			Namespace: mgh.Namespace,
		},
	}
	expectedRoleRef := rbacv1.RoleRef{
		APIGroup: rbacv1.GroupName,
		Kind:     "ClusterRole",
		Name:     "cluster-admin",
	}
	existingCRB := &rbacv1.ClusterRoleBinding{}
	if err := c.Get(ctx, types.NamespacedName{Name: CrunchyInstallerCRBName}, existingCRB); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("get crunchy installer ClusterRoleBinding %q: %w", CrunchyInstallerCRBName, err)
		}
		crb := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: CrunchyInstallerCRBName,
				Labels: map[string]string{
					constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
				},
			},
			Subjects: expectedSubjects,
			RoleRef:  expectedRoleRef,
		}
		if err := c.Create(ctx, crb); err != nil {
			return fmt.Errorf("create crunchy installer ClusterRoleBinding %q: %w", CrunchyInstallerCRBName, err)
		}
	} else if !equality.Semantic.DeepEqual(existingCRB.Subjects, expectedSubjects) ||
		existingCRB.RoleRef != expectedRoleRef {
		// RoleRef is immutable — delete and requeue to recreate
		if err := c.Delete(ctx, existingCRB); err != nil {
			return fmt.Errorf("delete crunchy installer ClusterRoleBinding %q: %w", CrunchyInstallerCRBName, err)
		}
		return fmt.Errorf("deleted crunchy installer ClusterRoleBinding due to drift, requeueing to recreate")
	}

	// ensure ClusterExtension
	chName, pkgName := channel, packageName
	if operatorutils.IsCommunityMode() {
		chName = communityChannel
		pkgName = communityPackageName
	}
	expectedCE := &ocv1.ClusterExtension{
		ObjectMeta: metav1.ObjectMeta{
			Name: CrunchyClusterExtensionName,
			Labels: map[string]string{
				constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
			},
		},
		Spec: ocv1.ClusterExtensionSpec{
			Namespace: mgh.Namespace,
			ServiceAccount: ocv1.ServiceAccountReference{
				Name: CrunchyInstallerSAName,
			},
			Source: ocv1.SourceConfig{
				SourceType: ocv1.SourceTypeCatalog,
				Catalog: &ocv1.CatalogFilter{
					PackageName: pkgName,
					Channels:    []string{chName},
				},
			},
		},
	}

	existingCE := &ocv1.ClusterExtension{}
	if err := c.Get(ctx, types.NamespacedName{Name: expectedCE.Name}, existingCE); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("get crunchy ClusterExtension %q: %w", expectedCE.Name, err)
		}
		if err := c.Create(ctx, expectedCE); err != nil {
			return fmt.Errorf("create crunchy ClusterExtension %q: %w", expectedCE.Name, err)
		}
		return nil
	}

	var existingPkg string
	if existingCE.Spec.Source.Catalog != nil {
		existingPkg = existingCE.Spec.Source.Catalog.PackageName
	}
	expectedPkg := expectedCE.Spec.Source.Catalog.PackageName

	// packageName and namespace are immutable — delete and let next reconcile recreate
	if existingPkg != expectedPkg || existingCE.Spec.Namespace != expectedCE.Spec.Namespace {
		if err := c.Delete(ctx, existingCE); err != nil {
			return fmt.Errorf("delete crunchy ClusterExtension %q: %w", existingCE.Name, err)
		}
		return fmt.Errorf("deleted crunchy ClusterExtension due to immutable field change, requeueing to recreate")
	}

	// Compare only the fields we manage to avoid hot loops from server-defaulted fields
	needsUpdate := existingCE.Spec.ServiceAccount.Name != expectedCE.Spec.ServiceAccount.Name
	needsUpdate = needsUpdate || !equality.Semantic.DeepEqual(
		existingCE.Spec.Source.Catalog.Channels, expectedCE.Spec.Source.Catalog.Channels,
	)

	if needsUpdate {
		patch := client.MergeFrom(existingCE.DeepCopy())
		existingCE.Spec.ServiceAccount = expectedCE.Spec.ServiceAccount
		if expectedCE.Spec.Source.Catalog != nil && existingCE.Spec.Source.Catalog != nil {
			existingCE.Spec.Source.Catalog.Channels = expectedCE.Spec.Source.Catalog.Channels
		}
		if err := c.Patch(ctx, existingCE, patch); err != nil {
			return fmt.Errorf("patch crunchy ClusterExtension %q: %w", existingCE.Name, err)
		}
	}
	return nil
}

// PruneCrunchyClusterExtensionResources deletes the Crunchy ClusterExtension, installer CRB, and SA.
// Returns true when all resources are fully removed.
func PruneCrunchyClusterExtensionResources(ctx context.Context, c client.Client, mghNamespace string) (bool, error) {
	ce := &ocv1.ClusterExtension{}
	err := c.Get(ctx, types.NamespacedName{Name: CrunchyClusterExtensionName}, ce)
	if err != nil && !errors.IsNotFound(err) {
		return false, fmt.Errorf("get crunchy ClusterExtension %q: %w", CrunchyClusterExtensionName, err)
	}
	if err == nil {
		log.Infof("Delete crunchy ClusterExtension %v", ce.Name)
		if err := c.Delete(ctx, ce); err != nil && !errors.IsNotFound(err) {
			return false, fmt.Errorf("delete crunchy ClusterExtension %q: %w", ce.Name, err)
		}
		// requeue until the CE is fully removed so OLMv1 can use the installer SA for finalizer work
		return false, nil
	}

	// CE is gone — safe to remove installer CRB and SA
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: CrunchyInstallerCRBName},
	}
	if err := c.Delete(ctx, crb); err != nil && !errors.IsNotFound(err) {
		return false, fmt.Errorf("delete crunchy installer ClusterRoleBinding %q: %w", CrunchyInstallerCRBName, err)
	}

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CrunchyInstallerSAName,
			Namespace: mghNamespace,
		},
	}
	if err := c.Delete(ctx, sa); err != nil && !errors.IsNotFound(err) {
		return false, fmt.Errorf("delete crunchy installer ServiceAccount %s/%s: %w", mghNamespace, CrunchyInstallerSAName, err)
	}

	log.Infof("crunchy ClusterExtension and installer resources deleted")
	return true, nil
}

// EnsureCrunchyPostgres verifies PostgresCluster operand is created
func EnsureCrunchyPostgres(ctx context.Context, c client.Client) (*config.PostgresConnection, error) {
	// store crunchy postgres connection
	var pgConnection *config.PostgresConnection
	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, 10*time.Minute, true,
		func(ctx context.Context) (bool, error) {
			postgresCluster := &postgresv1beta1.PostgresCluster{}
			err := c.Get(ctx, types.NamespacedName{
				Name:      config.PostgresName,
				Namespace: utils.GetDefaultNamespace(),
			}, postgresCluster)
			if err != nil && errors.IsNotFound(err) {
				if err := c.Create(ctx, NewPostgresCluster(config.PostgresName, utils.GetDefaultNamespace())); err != nil {
					log.Info("waiting the postgres cluster to be ready...", "message", err.Error())
					return false, nil
				}
			}

			pgConnection, err = config.GetPGConnectionFromBuildInPostgres(ctx, c)
			if err != nil {
				log.Info("waiting the postgres connection credential to be ready...", "message", err.Error())
				return false, nil
			}
			return true, nil
		})

	return pgConnection, err
}

// NewCrunchySubscription returns an CrunchyPostgres subscription with desired default values
func NewCrunchySubscription(m *globalhubv1alpha4.MulticlusterGlobalHub, c *subv1alpha1.SubscriptionConfig,
	community bool,
) *subv1alpha1.Subscription {
	chName, pkgName, catSourceName := channel, packageName, catalogSourceName
	if community {
		chName = communityChannel
		pkgName = communityPackageName
		catSourceName = communityCatalogSourceName
	}
	labels := map[string]string{
		"installer.name":                 m.GetName(),
		"installer.namespace":            m.GetNamespace(),
		constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
	}
	sub := &subv1alpha1.Subscription{
		TypeMeta: metav1.TypeMeta{
			APIVersion: subv1alpha1.SubscriptionCRDAPIVersion,
			Kind:       subv1alpha1.SubscriptionKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      SubscriptionName,
			Namespace: m.GetNamespace(),
			Labels:    labels,
		},
		Spec: &subv1alpha1.SubscriptionSpec{
			Channel:                chName,
			InstallPlanApproval:    installPlanApproval,
			Package:                pkgName,
			CatalogSource:          catSourceName,
			CatalogSourceNamespace: catalogSourceNamespace,
			Config:                 c,
		},
	}

	return sub
}

// ExpectedSubscription returns a subscription by modifying the spec of an existing subscription based on overrides
func ExpectedSubscription(existingSubscription *subv1alpha1.Subscription, config *subv1alpha1.SubscriptionConfig,
	community bool,
) *subv1alpha1.Subscription {
	copy := existingSubscription.DeepCopy()
	copy.ManagedFields = nil
	copy.TypeMeta = metav1.TypeMeta{
		APIVersion: subv1alpha1.SubscriptionCRDAPIVersion,
		Kind:       subv1alpha1.SubscriptionKind,
	}

	chName, pkgName, catSourceName := channel, packageName, catalogSourceName
	if community {
		chName = communityChannel
		pkgName = communityPackageName
		catSourceName = communityCatalogSourceName
	}

	copy.Spec = &subv1alpha1.SubscriptionSpec{
		Channel:                chName,
		InstallPlanApproval:    installPlanApproval,
		Package:                pkgName,
		CatalogSource:          catSourceName,
		CatalogSourceNamespace: catalogSourceNamespace,
		Config:                 config,
	}

	// if updating channel must remove startingCSV
	if copy.Spec.Channel != existingSubscription.Spec.Channel {
		copy.Spec.StartingCSV = ""
	}

	return copy
}

// NewPostgreCluster returns a postgres cluster with desired default values
func NewPostgresCluster(name, namespace string) *postgresv1beta1.PostgresCluster {
	return &postgresv1beta1.PostgresCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: postgresv1beta1.PostgresClusterSpec{
			PostgresVersion: 14,
			Users: []postgresv1beta1.PostgresUserSpec{
				{
					Name:      postgresv1beta1.PostgresIdentifier(config.PostgresSuperUser),
					Databases: []postgresv1beta1.PostgresIdentifier{"hoh"},
				},
				{
					// create a readonly user for grafana view the data
					Name:      postgresv1beta1.PostgresIdentifier(config.PostgresGuestUser),
					Databases: []postgresv1beta1.PostgresIdentifier{"hoh"},
					Options:   "LOGIN",
				},
			},
			Patroni: &postgresv1beta1.PatroniSpec{
				DynamicConfiguration: map[string]interface{}{
					"postgresql": map[string]interface{}{
						"parameters": map[string]interface{}{
							"max_wal_size":  "3GB",
							"wal_recycle":   true,
							"wal_init_zero": false,
						},
					},
				},
			},
			InstanceSets: []postgresv1beta1.PostgresInstanceSetSpec{
				{
					Name:     "pgha1",
					Replicas: &replicas3,
					DataVolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse(storageSize),
							},
						},
					},
				},
			},
			Backups: postgresv1beta1.Backups{
				PGBackRest: postgresv1beta1.PGBackRestArchive{
					Repos: []postgresv1beta1.PGBackRestRepo{
						{
							Name: "repo1",
							Volume: &postgresv1beta1.RepoPVC{
								VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
									AccessModes: []corev1.PersistentVolumeAccessMode{
										corev1.ReadWriteOnce,
									},
									Resources: corev1.VolumeResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceStorage: resource.MustParse(storageSize),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

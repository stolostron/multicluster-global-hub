package storage

import (
	"context"
	"time"

	postgresv1beta1 "github.com/crunchydata/postgres-operator/pkg/apis/postgres-operator.crunchydata.com/v1beta1"
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
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

// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package postgres

import (
	postgresv1beta1 "github.com/crunchydata/postgres-operator/pkg/apis/postgres-operator.crunchydata.com/v1beta1"
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
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

	// default names
	PostgresName                = "postgres"
	PostgresGuestUser           = "guest"
	PostgresGuestUserSecretName = PostgresName + "-" + "pguser" + "-" + PostgresGuestUser
	PostgresSuperUser           = "postgres"
	PostgresSuperUserSecretName = PostgresName + "-" + "pguser" + "-" + PostgresSuperUser
	PostgresCertName            = PostgresName + "-cluster-cert"

	replicas3 int32 = 3
	// need append "?sslmode=verify-ca" to the end of the uri to access postgres
	PostgresURIWithSslmode = "?sslmode=verify-ca"
	// postgres storage size: 30Gi should be enough for 18 months data
	// 5 managed hubs with 300 managed cluster each and 50 policies per managed hub cluster
	storageSize = "30Gi"
)

type PostgresConnection struct {
	// super user connection
	// it is used for initiate the database
	SuperuserDatabaseURI string
	// readonly user connection
	// it is used for read the database by the grafana
	ReadonlyUserDatabaseURI string
	// ca certificate
	CACert []byte
}

// NewSubscription returns an CrunchyPostgres subscription with desired default values
func NewSubscription(m *globalhubv1alpha4.MulticlusterGlobalHub, c *subv1alpha1.SubscriptionConfig,
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

// RenderSubscription returns a subscription by modifying the spec of an existing subscription based on overrides
func RenderSubscription(existingSubscription *subv1alpha1.Subscription, config *subv1alpha1.SubscriptionConfig,
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

// NewPostgres returns a postgres cluster with desired default values
func NewPostgres(name, namespace string) *postgresv1beta1.PostgresCluster {
	return &postgresv1beta1.PostgresCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: postgresv1beta1.PostgresClusterSpec{
			PostgresVersion: 14,
			Users: []postgresv1beta1.PostgresUserSpec{
				{
					Name:      postgresv1beta1.PostgresIdentifier(PostgresSuperUser),
					Databases: []postgresv1beta1.PostgresIdentifier{"hoh"},
				},
				{
					// create a readonly user for grafana view the data
					Name:      postgresv1beta1.PostgresIdentifier(PostgresGuestUser),
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
						Resources: corev1.ResourceRequirements{
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
									Resources: corev1.ResourceRequirements{
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

package hubofhubs

import (
	"context"
	"fmt"

	postgresv1beta1 "github.com/crunchydata/postgres-operator/pkg/apis/postgres-operator.crunchydata.com/v1beta1"
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/postgres"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
)

const (
	secretNilErrorMsg        = "postgres secret %s is nil"
	postgresAdminUsername    = "postgres"
	postgresReadonlyUsername = "global-hub-readonly-user" // #nosec G101
	postgresCA               = "multicluster-global-hub-postgres-ca"
)

type postgresCredential struct {
	postgresAdminUsername        string
	postgresAdminUserPassword    string
	postgresReadonlyUsername     string
	postgresReadonlyUserPassword string
}

var partialPostgresURI = "@multicluster-global-hub-postgres." +
	config.GetDefaultNamespace() + ".svc:5432/hoh?sslmode=verify-ca"

// EnsureCrunchyPostgresSubscription verifies resources needed for Crunchy Postgres are created
func (r *MulticlusterGlobalHubReconciler) EnsureCrunchyPostgresSubscription(ctx context.Context,
	mgh *globalhubv1alpha4.MulticlusterGlobalHub,
) error {
	postgresSub, err := utils.GetSubscriptionByName(ctx, r.Client, postgres.SubscriptionName)
	if err != nil {
		return err
	}

	// Generate sub config from mcgh CR
	subConfig := &subv1alpha1.SubscriptionConfig{
		NodeSelector: mgh.Spec.NodeSelector,
		Tolerations:  mgh.Spec.Tolerations,
	}

	createSub := false
	if postgresSub == nil {
		// Sub is nil so create a new one
		postgresSub = postgres.NewSubscription(mgh, subConfig, utils.IsCommunityMode())
		createSub = true
	}

	// Apply Crunchy Postgres sub
	calcSub := postgres.RenderSubscription(postgresSub, subConfig, utils.IsCommunityMode())
	if createSub {
		err = r.Client.Create(ctx, calcSub)
	} else {
		if !equality.Semantic.DeepEqual(postgresSub.Spec, calcSub.Spec) {
			err = r.Client.Update(ctx, calcSub)
		}
	}
	if err != nil {
		return fmt.Errorf("error updating subscription %s: %w", calcSub.Name, err)
	}

	return nil
}

// EnsureCrunchyPostgres verifies PostgresCluster operand is created
func (r *MulticlusterGlobalHubReconciler) EnsureCrunchyPostgres(ctx context.Context) error {
	postgresCluster := &postgresv1beta1.PostgresCluster{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      postgres.PostgresName,
		Namespace: config.GetDefaultNamespace(),
	}, postgresCluster)
	if err != nil && errors.IsNotFound(err) {
		return r.Client.Create(ctx, postgres.NewPostgres(postgres.PostgresName, config.GetDefaultNamespace()))
	}
	return err
}

// WaitForPostgresReady waits for postgres to be ready and returns a postgres connection
func (r *MulticlusterGlobalHubReconciler) WaitForPostgresReady(ctx context.Context) (
	*postgres.PostgresConnection, error,
) {
	// wait for postgres guest user secret to be ready
	guestPostgresSecret := &corev1.Secret{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      postgres.PostgresGuestUserSecretName,
		Namespace: config.GetDefaultNamespace(),
	}, guestPostgresSecret)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf(secretNilErrorMsg, postgres.PostgresGuestUserSecretName)
		}
		return nil, err
	}
	// wait for postgres super user secret to be ready
	superuserPostgresSecret := &corev1.Secret{}
	err = r.Client.Get(ctx, types.NamespacedName{
		Name:      postgres.PostgresSuperUserSecretName,
		Namespace: config.GetDefaultNamespace(),
	}, superuserPostgresSecret)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf(secretNilErrorMsg, postgres.PostgresSuperUserSecretName)
		}
		return nil, err
	}
	// wait for postgres cert secret to be ready
	postgresCertName := &corev1.Secret{}
	err = r.Client.Get(ctx, types.NamespacedName{
		Name:      postgres.PostgresCertName,
		Namespace: config.GetDefaultNamespace(),
	}, postgresCertName)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf(secretNilErrorMsg, postgres.PostgresCertName)
		}
		return nil, err
	}

	return &postgres.PostgresConnection{
		SuperuserDatabaseURI:    string(superuserPostgresSecret.Data["uri"]) + postgres.PostgresURIWithSslmode,
		ReadonlyUserDatabaseURI: string(guestPostgresSecret.Data["uri"]) + postgres.PostgresURIWithSslmode,
		CACert:                  postgresCertName.Data["ca.crt"],
	}, nil
}

// GeneratePGConnectionFromGHStorageSecret returns a postgres connection from the GH storage secret
func (r *MulticlusterGlobalHubReconciler) GeneratePGConnectionFromGHStorageSecret(ctx context.Context) (
	*postgres.PostgresConnection, error,
) {
	pgSecret := &corev1.Secret{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      constants.GHStorageSecretName,
		Namespace: config.GetDefaultNamespace(),
	}, pgSecret)
	if err != nil {
		return nil, err
	}
	return &postgres.PostgresConnection{
		SuperuserDatabaseURI:    string(pgSecret.Data["database_uri"]),
		ReadonlyUserDatabaseURI: string(pgSecret.Data["database_uri_with_readonlyuser"]),
		CACert:                  pgSecret.Data["ca.crt"],
	}, nil
}

func (r *MulticlusterGlobalHubReconciler) InitPostgresByStatefulset(ctx context.Context,
	mgh *globalhubv1alpha4.MulticlusterGlobalHub,
) error {
	// install the postgres statefulset only
	credential, err := getPostgresCredential(ctx, mgh, r)
	if err != nil {
		return err
	}
	imagePullPolicy := corev1.PullAlways
	if mgh.Spec.ImagePullPolicy != "" {
		imagePullPolicy = mgh.Spec.ImagePullPolicy
	}

	// get the postgres objects
	postgresRenderer, postgresDeployer := renderer.NewHoHRenderer(fs), deployer.NewHoHDeployer(r.Client)
	postgresObjects, err := postgresRenderer.Render("manifests/postgres", "",
		func(profile string) (interface{}, error) {
			return struct {
				Namespace                    string
				PostgresImage                string
				StorageSize                  string
				ImagePullSecret              string
				ImagePullPolicy              string
				NodeSelector                 map[string]string
				Tolerations                  []corev1.Toleration
				PostgresAdminUserPassword    string
				PostgresReadonlyUsername     string
				PostgresReadonlyUserPassword string
				StorageClass                 string
			}{
				Namespace:                    mgh.GetNamespace(),
				PostgresImage:                config.GetImage(config.PostgresImageKey),
				ImagePullSecret:              mgh.Spec.ImagePullSecret,
				ImagePullPolicy:              string(imagePullPolicy),
				NodeSelector:                 mgh.Spec.NodeSelector,
				Tolerations:                  mgh.Spec.Tolerations,
				StorageSize:                  config.GetPostgresStorageSize(mgh),
				PostgresAdminUserPassword:    credential.postgresAdminUserPassword,
				PostgresReadonlyUsername:     credential.postgresReadonlyUsername,
				PostgresReadonlyUserPassword: credential.postgresReadonlyUserPassword,
				StorageClass:                 mgh.Spec.DataLayer.StorageClass,
			}, nil
		})
	if err != nil {
		return fmt.Errorf("failed to render postgres manifests: %w", err)
	}

	// create restmapper for deployer to find GVR
	dc, err := discovery.NewDiscoveryClientForConfig(r.Manager.GetConfig())
	if err != nil {
		return err
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	if err = r.manipulateObj(ctx, postgresDeployer, mapper, postgresObjects, mgh, r.Log); err != nil {
		return fmt.Errorf("failed to create/update postgres objects: %w", err)
	}

	ca, err := getPostgresCA(ctx, mgh, r)
	if err != nil {
		return err
	}
	r.MiddlewareConfig.PgConnection = &postgres.PostgresConnection{
		SuperuserDatabaseURI: "postgresql://" + credential.postgresAdminUsername + ":" +
			credential.postgresAdminUserPassword + partialPostgresURI,
		ReadonlyUserDatabaseURI: "postgresql://" + credential.postgresReadonlyUsername + ":" +
			credential.postgresReadonlyUserPassword + partialPostgresURI,
		CACert: []byte(ca),
	}
	return nil
}

func getPostgresCredential(ctx context.Context, mgh *globalhubv1alpha4.MulticlusterGlobalHub,
	r *MulticlusterGlobalHubReconciler,
) (*postgresCredential, error) {
	postgres := &corev1.Secret{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Name:      constants.GHBuiltInStorageSecretName,
		Namespace: mgh.Namespace,
	}, postgres); err != nil && errors.IsNotFound(err) {
		return &postgresCredential{
			postgresAdminUsername:        postgresAdminUsername,
			postgresAdminUserPassword:    generatePassword(16),
			postgresReadonlyUsername:     postgresReadonlyUsername,
			postgresReadonlyUserPassword: generatePassword(16),
		}, nil
	} else if err != nil {
		return nil, err
	}
	return &postgresCredential{
		postgresAdminUsername:        postgresAdminUsername,
		postgresAdminUserPassword:    string(postgres.Data["database-admin-password"]),
		postgresReadonlyUsername:     string(postgres.Data["database-readonly-user"]),
		postgresReadonlyUserPassword: string(postgres.Data["database-readonly-password"]),
	}, nil
}

func getPostgresCA(ctx context.Context,
	mgh *globalhubv1alpha4.MulticlusterGlobalHub, r *MulticlusterGlobalHubReconciler,
) (string, error) {
	ca := &corev1.ConfigMap{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Name:      postgresCA,
		Namespace: mgh.Namespace,
	}, ca); err != nil {
		return "", err
	}
	return ca.Data["service-ca.crt"], nil
}

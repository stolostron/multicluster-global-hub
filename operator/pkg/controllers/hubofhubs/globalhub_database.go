package hubofhubs

import (
	"context"
	"crypto/rand"
	"embed"
	"fmt"
	iofs "io/fs"
	"math/big"
	"net/url"
	"strings"

	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v4"
	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/postgres"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
)

var DatabaseReconcileCounter = 0

//go:embed database
var databaseFS embed.FS

//go:embed database.old
var databaseOldFS embed.FS

type postgresCredential struct {
	postgresAdminUsername        string
	postgresAdminUserPassword    string
	postgresReadonlyUsername     string
	postgresReadonlyUserPassword string
}

const (
	postgresAdminUsername    = "postgres"
	postgresReadonlyUsername = "global-hub-readonly-user" // #nosec G101
	postgresCA               = "multicluster-global-hub-postgres-ca"
)

var partialPostgresURI = "@multicluster-global-hub-postgres." +
	config.GetDefaultNamespace() + ".svc:5432/hoh?sslmode=verify-ca"

func (r *MulticlusterGlobalHubReconciler) ReconcileDatabase(ctx context.Context,
	mgh *globalhubv1alpha4.MulticlusterGlobalHub,
) error {
	log := r.Log.WithName("database")

	if err := r.createPostgres(ctx, mgh, log); err != nil {
		return err
	}

	if condition.ContainConditionStatus(mgh, condition.CONDITION_TYPE_DATABASE_INIT, condition.CONDITION_STATUS_TRUE) {
		log.V(7).Info("database has been initialized, checking the reconcile counter")
		// if the operator is restarted, reconcile the database again
		if DatabaseReconcileCounter > 0 {
			return nil
		}
	}

	conn, err := database.PostgresConnection(ctx, r.MiddlewareConfig.PgConnection.SuperuserDatabaseURI,
		r.MiddlewareConfig.PgConnection.CACert)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() {
		if err := conn.Close(ctx); err != nil {
			log.Error(err, "failed to close connection to database")
		}
	}()

	objURI, err := url.Parse(r.MiddlewareConfig.PgConnection.ReadonlyUserDatabaseURI)
	if err != nil {
		log.Error(err, "failed to parse database_uri_with_readonlyuser")
	}
	readonlyUsername := objURI.User.Username()

	if err := applySQL(ctx, conn, databaseFS, "database", readonlyUsername); err != nil {
		return err
	}

	if r.EnableGlobalResource {
		if err := applySQL(ctx, conn, databaseOldFS, "database.old", readonlyUsername); err != nil {
			return err
		}
	}

	log.V(7).Info("database initialized")
	DatabaseReconcileCounter++
	err = condition.SetConditionDatabaseInit(ctx, r.Client, mgh, condition.CONDITION_STATUS_TRUE)
	if err != nil {
		return condition.FailToSetConditionError(condition.CONDITION_STATUS_TRUE, err)
	}
	return nil
}

func applySQL(ctx context.Context, conn *pgx.Conn, databaseFS embed.FS, rootDir, username string) error {
	err := iofs.WalkDir(databaseFS, rootDir, func(file string, d iofs.DirEntry, beforeError error) error {
		if beforeError != nil {
			return beforeError
		}
		if d.IsDir() {
			return nil
		}
		sqlBytes, err := databaseFS.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", file, err)
		}
		if file == rootDir+"/5.privileges.sql" {
			if username != "" {
				_, err = conn.Exec(ctx, strings.ReplaceAll(string(sqlBytes), "$1", username))
			}
		} else {
			_, err = conn.Exec(ctx, string(sqlBytes))
		}
		if err != nil {
			return fmt.Errorf("failed to create %s: %w", file, err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to exec database sql: %w", err)
	}
	return nil
}

func generatePassword(length int) string {
	chars := "ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz" +
		"0123456789"

	buf := make([]byte, length)
	for i := 0; i < length; i++ {
		nBig, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		buf[i] = chars[nBig.Int64()]
	}
	return string(buf)
}

func (r *MulticlusterGlobalHubReconciler) createPostgres(ctx context.Context,
	mgh *globalhubv1alpha4.MulticlusterGlobalHub, log logr.Logger) error {

	// check if the customer provides the postgres
	var err error
	r.MiddlewareConfig.PgConnection, _ = r.GeneratePGConnectionFromGHStorageSecret(ctx)
	if r.MiddlewareConfig.PgConnection != nil {
		return nil
	}
	// check if install crunchy operator
	if config.GetInstallCrunchyOperator(mgh) {
		return nil
	}

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
				Namespace:                    config.GetDefaultNamespace(),
				PostgresImage:                config.GetImage(config.PostgresImageKey),
				ImagePullSecret:              mgh.Spec.ImagePullSecret,
				ImagePullPolicy:              string(imagePullPolicy),
				NodeSelector:                 mgh.Spec.NodeSelector,
				Tolerations:                  mgh.Spec.Tolerations,
				StorageSize:                  config.GetPostgresStorageSize(mgh),
				PostgresAdminUserPassword:    credential.postgresAdminUserPassword,
				PostgresReadonlyUsername:     credential.postgresReadonlyUsername,
				PostgresReadonlyUserPassword: credential.postgresReadonlyUserPassword,
				StorageClass:                 mgh.Spec.DataLayer.Postgres.StorageClass,
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

	if err = r.manipulateObj(ctx, postgresDeployer, mapper, postgresObjects, mgh, log); err != nil {
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
	r *MulticlusterGlobalHubReconciler) (*postgresCredential, error) {
	postgres := &corev1.Secret{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Name:      "multicluster-global-hub-postgres",
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
	mgh *globalhubv1alpha4.MulticlusterGlobalHub, r *MulticlusterGlobalHubReconciler) (string, error) {
	ca := &corev1.ConfigMap{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Name:      postgresCA,
		Namespace: mgh.Namespace,
	}, ca); err != nil {
		return "", err
	}
	return ca.Data["service-ca.crt"], nil
}

package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	operatorutils "github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var (
	BuiltinPostgresName       = config.COMPONENTS_POSTGRES_NAME // Postgres: sts, service, secrert(credential), ca
	BuiltinPostgresCAName     = fmt.Sprintf("%s-ca", BuiltinPostgresName)
	builtinPostgresCertName   = fmt.Sprintf("%s-cert", BuiltinPostgresName)
	builtinPostgresConfigName = fmt.Sprintf("%s-config", BuiltinPostgresName)
	builtinPostgresInitName   = fmt.Sprintf("%s-init", BuiltinPostgresName)
	builtinPartialPostgresURI = fmt.Sprintf("%s.%s.svc:5432/hoh?sslmode=verify-ca", BuiltinPostgresName,
		utils.GetDefaultNamespace())
)

const (
	BuiltinPostgresAdminUserName             = "postgres"
	BuiltinPostgresCustomizedConfigName      = "multicluster-global-hub-custom-postgresql-config"
	BuiltinPostgresCustomizedUsersName       = "multicluster-global-hub-custom-postgresql-users"
	PostgresCustomizedUserSecretDatabasesKey = "db.names"
	PostgresCustomizedUserSecretUserKey      = "db.user"
	PostgresCustomizedUserSecretHostKey      = "db.host"
	PostgresCustomizedUserSecretPortKey      = "db.port"
	PostgresCustomizedUserSecretCACertKey    = "db.ca_cert"  // #nosec G101
	PostgresCustomizedUserSecretPasswordKey  = "db.password" // #nosec G101
)

type postgresCredential struct {
	postgresAdminUsername        string
	postgresAdminUserPassword    string
	postgresReadonlyUsername     string
	postgresReadonlyUserPassword string
}

func InitPostgresByStatefulset(ctx context.Context, mgh *globalhubv1alpha4.MulticlusterGlobalHub,
	mgr ctrl.Manager,
) (*config.PostgresConnection, error) {
	// install the postgres statefulset only
	credential, err := getPostgresCredential(ctx, mgh, mgr.GetClient())
	if err != nil {
		return nil, err
	}
	imagePullPolicy := corev1.PullAlways
	if mgh.Spec.ImagePullPolicy != "" {
		imagePullPolicy = mgh.Spec.ImagePullPolicy
	}

	// postgres configurable
	customizedConfig, err := getPostgresCustomizedConfig(ctx, mgh, mgr.GetClient())
	if err != nil {
		return nil, err
	}
	log.Infof("the postgres customized config: %s", customizedConfig)

	// get the postgres objects
	postgresRenderer, postgresDeployer := renderer.NewHoHRenderer(stsPostgresFS), deployer.NewHoHDeployer(mgr.GetClient())
	postgresObjects, err := postgresRenderer.Render("manifests.sts", "",
		func(profile string) (interface{}, error) {
			return struct {
				Name                         string
				Namespace                    string
				PostgresImage                string
				PostgresExporterImage        string
				StorageSize                  string
				ImagePullSecret              string
				ImagePullPolicy              string
				NodeSelector                 map[string]string
				Tolerations                  []corev1.Toleration
				PostgresConfigName           string
				PostgresCaName               string
				PostgresCertName             string
				PostgresInitName             string
				PostgresAdminUser            string
				PostgresAdminUserPassword    string
				PostgresReadonlyUsername     string
				PostgresReadonlyUserPassword string
				PostgresURI                  string
				StorageClass                 string
				Resources                    *corev1.ResourceRequirements
				EnableMetrics                bool
				EnablePostgresMetrics        bool
				EnableInventoryAPI           bool
				PostgresCustomizedConfig     []byte
			}{
				Name:                         BuiltinPostgresName,
				Namespace:                    mgh.GetNamespace(),
				PostgresImage:                config.GetImage(config.PostgresImageKey),
				PostgresExporterImage:        config.GetImage(config.PostgresExporterImageKey),
				ImagePullSecret:              mgh.Spec.ImagePullSecret,
				ImagePullPolicy:              string(imagePullPolicy),
				NodeSelector:                 mgh.Spec.NodeSelector,
				Tolerations:                  mgh.Spec.Tolerations,
				StorageSize:                  config.GetPostgresStorageSize(mgh),
				PostgresConfigName:           builtinPostgresConfigName,
				PostgresCaName:               BuiltinPostgresCAName,
				PostgresCertName:             builtinPostgresCertName,
				PostgresInitName:             builtinPostgresInitName,
				PostgresAdminUser:            postgresAdminUsername,
				PostgresAdminUserPassword:    credential.postgresAdminUserPassword,
				PostgresReadonlyUsername:     credential.postgresReadonlyUsername,
				PostgresReadonlyUserPassword: credential.postgresReadonlyUserPassword,
				StorageClass:                 mgh.Spec.DataLayerSpec.StorageClass,
				// Postgres exporter should disable sslmode
				// https://github.com/prometheus-community/postgres_exporter?tab=readme-ov-file#environment-variables
				PostgresURI: strings.ReplaceAll(builtinPartialPostgresURI, "sslmode=verify-ca", "sslmode=disable"),
				Resources: operatorutils.GetResources(operatorconstants.Postgres,
					mgh.Spec.AdvancedSpec),
				EnableMetrics:            mgh.Spec.EnableMetrics,
				EnablePostgresMetrics:    (!config.IsBYOPostgres()) && mgh.Spec.EnableMetrics,
				EnableInventoryAPI:       config.WithInventory(mgh),
				PostgresCustomizedConfig: []byte(customizedConfig),
			}, nil
		})
	if err != nil {
		log.Errorf("failed to render postgres manifests: %w", err)
		return nil, err
	}

	// create restmapper for deployer to find GVR
	dc, err := discovery.NewDiscoveryClientForConfig(mgr.GetConfig())
	if err != nil {
		return nil, err
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	if err = operatorutils.ManipulateGlobalHubObjects(postgresObjects, mgh, postgresDeployer,
		mapper, mgr.GetScheme()); err != nil {
		return nil, fmt.Errorf("failed to create/update postgres objects: %w", err)
	}

	ca, err := getPostgresCA(ctx, mgh, mgr.GetClient())
	if err != nil {
		return nil, err
	}
	return &config.PostgresConnection{
		SuperuserDatabaseURI: "postgresql://" + credential.postgresAdminUsername + ":" +
			credential.postgresAdminUserPassword + "@" + builtinPartialPostgresURI,
		ReadonlyUserDatabaseURI: "postgresql://" + credential.postgresReadonlyUsername + ":" +
			credential.postgresReadonlyUserPassword + "@" + builtinPartialPostgresURI,
		CACert: []byte(ca),
	}, nil
}

func getPostgresCredential(ctx context.Context, mgh *globalhubv1alpha4.MulticlusterGlobalHub,
	c client.Client,
) (*postgresCredential, error) {
	postgres := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{
		Name:      BuiltinPostgresName,
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

func getPostgresCustomizedConfig(ctx context.Context, mgh *globalhubv1alpha4.MulticlusterGlobalHub,
	c client.Client,
) (string, error) {
	cm := &corev1.ConfigMap{}
	err := c.Get(ctx, types.NamespacedName{
		Name:      BuiltinPostgresCustomizedConfigName,
		Namespace: mgh.Namespace,
	}, cm)
	if err != nil && !errors.IsNotFound(err) {
		return "", fmt.Errorf("failed to get the postgres customized config: %v", err)
	}
	customizedConfig := ""
	if !errors.IsNotFound(err) {
		customizedConfig = fmt.Sprintf("\n%s", cm.Data["postgresql.conf"])
	}
	return customizedConfig, nil
}

func getPostgresCA(ctx context.Context, mgh *globalhubv1alpha4.MulticlusterGlobalHub, c client.Client) (string, error) {
	ca := &corev1.ConfigMap{}
	if err := c.Get(ctx, types.NamespacedName{
		Name:      BuiltinPostgresCAName,
		Namespace: mgh.Namespace,
	}, ca); err != nil {
		return "", err
	}
	return ca.Data["service-ca.crt"], nil
}

// postgres user configuration
func applyPostgresUsers(ctx context.Context, conn *pgx.Conn, pgUsers map[string]string,
	mgh *v1alpha4.MulticlusterGlobalHub, c client.Client, s *runtime.Scheme,
) error {
	for userName, dbStr := range pgUsers {

		// parse the databases
		var dbs []string
		err := json.Unmarshal([]byte(dbStr), &dbs)
		if err != nil {
			return fmt.Errorf("failed to parse ConfigMap value: %w", err)
		}

		// create postgres user
		pwd, err := createPostgresDBUser(ctx, conn, userName)
		if err != nil {
			return fmt.Errorf("error creating postgres user %s: %v", userName, err)
		}
		// create database and add permission for the user
		for _, db := range dbs {
			err = createDatabaseIfNotExists(ctx, conn, db)
			if err != nil {
				return fmt.Errorf("error creating database %s: %v", db, err)
			}
			err = grantPermissions(ctx, conn, userName, db)
			if err != nil {
				return fmt.Errorf("failed to grant permissions to user %s on database %s: %v", userName, db, err)
			}
		}
		// create the secret for the postgres user and databases
		if err = createPostgresUserSecret(ctx, userName, pwd, dbStr, mgh, c, s); err != nil {
			return fmt.Errorf("error creating postgres user secret %v", err)
		}
	}
	return nil
}

func createPostgresUserSecret(ctx context.Context, userName string, password string, dbs string,
	mgh *v1alpha4.MulticlusterGlobalHub, c client.Client, s *runtime.Scheme,
) error {
	// convert the userName to a valid secret name
	userSecretName := strings.ReplaceAll(userName, "_", "-")

	userSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(postgresUserNameTemplate, userSecretName),
			Namespace: mgh.Namespace,
		},
	}
	err := c.Get(ctx, client.ObjectKeyFromObject(userSecret), userSecret)
	if err != nil && !errors.IsNotFound(err) {
		return err
	} else if errors.IsNotFound(err) {
		// create secret
		storageConn := config.GetStorageConnection()
		pgConfig, err := pgx.ParseConfig(storageConn.SuperuserDatabaseURI)
		if err != nil {
			return fmt.Errorf("failed the parse the supper user database URI")
		}
		userSecret.Data = map[string][]byte{
			PostgresCustomizedUserSecretHostKey:      []byte(pgConfig.Host),
			PostgresCustomizedUserSecretPortKey:      []byte(fmt.Sprintf("%d", pgConfig.Port)),
			PostgresCustomizedUserSecretUserKey:      []byte(userName),
			PostgresCustomizedUserSecretDatabasesKey: []byte(dbs),
			PostgresCustomizedUserSecretCACertKey:    storageConn.CACert,
		}
		if password != "" {
			userSecret.Data[PostgresCustomizedUserSecretPasswordKey] = []byte(password)
		}
		err = controllerutil.SetControllerReference(mgh, userSecret, s)
		if err != nil {
			return fmt.Errorf("failed to add the owner reference to the user secret: %s", userSecret.Name)
		}
		err = c.Create(ctx, userSecret)
		log.Infof("create the postgresql user secret: %s", userSecret.Name)
		if err != nil {
			return fmt.Errorf("failed to create the postgresql user secret: %s", userSecret.Name)
		}
		return nil
	}

	// update the secret:
	// 1. databases is changed
	// 2. userName is changed, and updated the password if the password is not empty
	log.Infof("the postgresql user secret already exists: %s", userSecret.Name)
	updated := false
	if string(userSecret.Data[PostgresCustomizedUserSecretDatabasesKey]) != dbs {
		updated = true
		userSecret.Data[PostgresCustomizedUserSecretDatabasesKey] = []byte(dbs)
	}
	if string(userSecret.Data[PostgresCustomizedUserSecretUserKey]) != userName {
		updated = true
		userSecret.Data[PostgresCustomizedUserSecretUserKey] = []byte(userName)
		userSecret.Data[PostgresCustomizedUserSecretPasswordKey] = []byte(password)
		log.Warnf("overwrite the postgres user secret %s with db user %s", userSecret.Name, userName)
	}

	if updated {
		err = c.Update(ctx, userSecret)
		if err != nil {
			return fmt.Errorf("failed to updating postgres user secret %s, err %v", userName, err)
		}
		log.Infof("update the postgres user secret: %s", userSecret.Name)
	}

	return nil
}

// GRANT ALL PRIVILEGES ON DATABASE "<dbName>" TO "<user>"; Allows the user to create new schemas and connect
// to the database but does not give permissions to interact with objects within the `public` schema (or any
// other schema that exists).
// However, if the user creates a new schema, they would have full permissions on that schema by default, since
// they have the CREATE privilege on the database and the ability to create schemas.
func grantPermissions(ctx context.Context, conn *pgx.Conn, user, dbName string) error {
	grantDBPermissionSQL := fmt.Sprintf("GRANT ALL PRIVILEGES ON DATABASE \"%s\" TO \"%s\";", dbName, user)
	_, err := conn.Exec(ctx, grantDBPermissionSQL)
	if err != nil {
		return fmt.Errorf("error granting permissions to user %s on database %s: %v", user, dbName, err)
	}

	// grant permission to public schema
	storageConn := config.GetStorageConnection()
	newDBConn, err := database.PostgresDBConn(ctx, storageConn.SuperuserDatabaseURI, storageConn.CACert, dbName)
	if err != nil {
		return fmt.Errorf("failed to establish the connection for db %s: %v", dbName, err)
	}
	defer func() {
		if err = newDBConn.Close(ctx); err != nil {
			log.Errorf("failed to close the connection for db %s: %v", dbName, err)
		}
	}()
	grantPublicSchemaSQL := fmt.Sprintf("GRANT ALL ON SCHEMA public TO \"%s\";", user)
	_, err = newDBConn.Exec(ctx, grantPublicSchemaSQL)
	if err != nil {
		return fmt.Errorf("error granting public schema permissions to user %s on database %s: %v", user, dbName, err)
	}

	log.Infof("granted all privileges to user %s on database %s.\n", user, dbName)
	return nil
}

func createDatabaseIfNotExists(ctx context.Context, conn *pgx.Conn, dbName string) error {
	var exists bool
	err := conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1);", dbName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("error checking if database %s exists: %v", dbName, err)
	}

	if !exists {
		createDBQuery := fmt.Sprintf("CREATE DATABASE \"%s\";", dbName)
		_, err := conn.Exec(ctx, createDBQuery)
		if err != nil {
			return fmt.Errorf("error creating database %s: %v", dbName, err)
		}
		log.Infof("database %s created.", dbName)
	} else {
		log.Infof("database %s already exists.", dbName)
	}

	return nil
}

// createPostgresUser return the password of the created user, if the password is empty if the user is already existing
func createPostgresDBUser(ctx context.Context, conn *pgx.Conn, userName string,
) (string, error) {
	if userName == BuiltinPostgresAdminUserName {
		return "", fmt.Errorf("cannot set the admin user %s", BuiltinPostgresAdminUserName)
	}

	var roleExists bool
	err := conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = $1)",
		userName).Scan(&roleExists)
	if err != nil {
		return "", fmt.Errorf("error checking if role exists: %v", err)
	}

	password := ""
	if roleExists {
		// updatePasswordQuery := fmt.Sprintf("ALTER ROLE \"%s\" WITH PASSWORD '%s';", user.Name, password)
		// _, err = conn.Exec(ctx, updatePasswordQuery)
		// if err != nil {
		// 	return fmt.Errorf("error updating password for role %s: %v", user.Name, err)
		// }
		log.Infof("postgres user '%s' already exists; skipping password generation.", userName)
	} else {
		password = generatePassword(16)
		createRoleQuery := fmt.Sprintf("CREATE ROLE \"%s\" LOGIN PASSWORD '%s';", userName, password)
		_, err = conn.Exec(ctx, createRoleQuery)
		if err != nil {
			return "", fmt.Errorf("error creating role %s: %v", userName, err)
		}
		log.Infof("create postgres user: %s", userName)
	}
	return password, nil
}

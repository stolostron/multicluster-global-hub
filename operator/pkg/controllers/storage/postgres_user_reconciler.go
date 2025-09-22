package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

const (
	BuiltinPostgresAdminUserName = "postgres"

	PostgresCustomizedUsersConfigMapName     = "multicluster-global-hub-custom-postgresql-users"
	PostgresCustomizedUserPasswordSalt       = "f9c67c8e7d2a1b42e5b8fa22417cd696" // #nosec G101
	PostgresCustomizedUserSecretDatabasesKey = "db.names"
	PostgresCustomizedUserSecretUserKey      = "db.user"
	PostgresCustomizedUserSecretHostKey      = "db.host"
	PostgresCustomizedUserSecretPortKey      = "db.port"
	PostgresCustomizedUserSecretCACertKey    = "db.ca_cert"      // #nosec G101
	PostgresCustomizedUserSecretPasswordKey  = "db.password"     // #nosec G101
	PostgresCustomizedUserSecretNamePrefix   = "postgresql-user" // #nosec G101
)

var (
	postgresUserNameTemplate = "postgresql-user-%s"
	configUserReconciler     *PostgresConfigUserReconciler
)

func StartPostgresConfigUserController(initOption config.ControllerOption) (config.ControllerInterface, error) {
	if configUserReconciler != nil {
		return configUserReconciler, nil
	}
	configUserReconciler = &PostgresConfigUserReconciler{initOption.Manager}
	err := configUserReconciler.SetupWithManager(initOption.Manager)
	if err != nil {
		configUserReconciler = nil
		return nil, err
	}
	log.Info("start postgres users controller")
	return configUserReconciler, nil
}

type PostgresConfigUserReconciler struct {
	ctrl.Manager
}

func (r *PostgresConfigUserReconciler) IsResourceRemoved() bool {
	return true
}

func (r *PostgresConfigUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named("postgresUserController").
		For(&corev1.ConfigMap{},
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					return PostgresCustomizedUsersConfigMapName == e.Object.GetName()
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					return PostgresCustomizedUsersConfigMapName == e.ObjectNew.GetName()
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return false
				},
			})).
		Watches(&corev1.Secret{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					return false
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					return e.ObjectNew.GetLabels()[constants.GlobalHubOwnerLabelKey] ==
						PostgresCustomizedUsersConfigMapName
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return e.Object.GetLabels()[constants.GlobalHubOwnerLabelKey] ==
						PostgresCustomizedUsersConfigMapName
				},
			})).
		Complete(r)
}

func (r *PostgresConfigUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.Debug("reconcile config user controller")

	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.GetClient())
	if err != nil || mgh == nil || config.IsPaused(mgh) || config.IsBYOPostgres() {
		return ctrl.Result{}, err
	}
	if mgh.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	// get the user configMap
	pgUsers := &corev1.ConfigMap{}
	err = r.GetClient().Get(ctx, client.ObjectKey{
		Name:      PostgresCustomizedUsersConfigMapName,
		Namespace: mgh.Namespace,
	}, pgUsers)
	if err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("unable to fetch ConfigMap: %w", err)
	}
	if errors.IsNotFound(err) || pgUsers.Data == nil {
		return ctrl.Result{}, nil
	}

	// get db connection
	storageConn := config.GetStorageConnection()
	if storageConn == nil {
		log.Info("storage connection isn't ready")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	conn, err := database.PostgresConnection(ctx, storageConn.SuperuserDatabaseURI, storageConn.CACert)
	defer func() {
		if conn != nil {
			if e := conn.Close(ctx); e != nil {
				log.Error(err, "failed to close connection to database")
			}
		}
	}()
	if err != nil {
		log.Infof("wait database ready, failed to connect database: %v", err)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	if err = r.applyPostgresUsers(ctx, conn, pgUsers.Data, mgh); err != nil {
		return ctrl.Result{}, err
	}
	log.Infof("applied the postgresql users successfully!")
	return ctrl.Result{}, nil
}

// postgres user configuration
func (r *PostgresConfigUserReconciler) applyPostgresUsers(ctx context.Context, conn *pgx.Conn,
	pgUsers map[string]string, mgh *v1alpha4.MulticlusterGlobalHub,
) error {
	for userName, dbStr := range pgUsers {

		// parse the databases
		var dbs []string
		err := json.Unmarshal([]byte(dbStr), &dbs)
		if err != nil {
			return fmt.Errorf("failed to parse ConfigMap value: %w", err)
		}

		// create postgres user
		pwd, err := ensurePgUser(ctx, conn, userName)
		if err != nil {
			return fmt.Errorf("error creating postgres user %s: %v", userName, err)
		}
		// create database and add permission for the user
		for _, db := range dbs {
			err = ensurePgDB(ctx, conn, db)
			if err != nil {
				return fmt.Errorf("error creating database %s: %v", db, err)
			}
			err = grantPermission(ctx, conn, userName, db)
			if err != nil {
				return fmt.Errorf("failed to grant permissions to user %s on database %s: %v", userName, db, err)
			}
		}
		// create the secret for the postgres user and databases
		if err = r.ensureUserSecret(ctx, userName, pwd, dbStr, mgh); err != nil {
			return fmt.Errorf("error creating postgres user secret %v", err)
		}
	}
	return nil
}

func (r *PostgresConfigUserReconciler) ensureUserSecret(ctx context.Context, userName string,
	password string, dbs string, mgh *v1alpha4.MulticlusterGlobalHub,
) error {
	// convert the userName to a valid secret name
	userSecretName := strings.ReplaceAll(userName, "_", "-")

	userSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(postgresUserNameTemplate, userSecretName),
			Namespace: mgh.Namespace,
			Labels: map[string]string{
				constants.GlobalHubOwnerLabelKey: PostgresCustomizedUsersConfigMapName,
			},
		},
	}
	err := r.GetClient().Get(ctx, client.ObjectKeyFromObject(userSecret), userSecret)
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
		err = controllerutil.SetControllerReference(mgh, userSecret, r.GetScheme())
		if err != nil {
			return fmt.Errorf("failed to add the owner reference to the user secret: %s", userSecret.Name)
		}
		err = r.GetClient().Create(ctx, userSecret)
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
		err = r.GetClient().Update(ctx, userSecret)
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
func grantPermission(ctx context.Context, conn *pgx.Conn, user, dbName string) error {
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

func ensurePgDB(ctx context.Context, conn *pgx.Conn, dbName string) error {
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
func ensurePgUser(ctx context.Context, conn *pgx.Conn, userName string,
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

	password := userPassword(userName, PostgresCustomizedUserPasswordSalt)
	if roleExists {
		// updatePasswordQuery := fmt.Sprintf("ALTER ROLE \"%s\" WITH PASSWORD '%s';", user.Name, password)
		// _, err = conn.Exec(ctx, updatePasswordQuery)
		// if err != nil {
		// 	return fmt.Errorf("error updating password for role %s: %v", user.Name, err)
		// }
		log.Infof("postgres user '%s' already exists", userName)
	} else {
		createRoleQuery := fmt.Sprintf("CREATE ROLE \"%s\" LOGIN PASSWORD '%s';", userName, password)
		_, err = conn.Exec(ctx, createRoleQuery)
		if err != nil {
			return "", fmt.Errorf("error creating role %s: %v", userName, err)
		}
		log.Infof("create postgres user: %s", userName)
	}
	return password, nil
}

func userPassword(username string, salt string) string {
	hash := sha256.New()
	hash.Write([]byte(username + salt))
	hashBytes := hash.Sum(nil)
	password := hex.EncodeToString(hashBytes)[:12]
	return password
}

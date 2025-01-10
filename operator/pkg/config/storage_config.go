package config

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var (
	// default names
	PostgresName                = "postgres"
	PostgresGuestUser           = "guest"
	PostgresGuestUserSecretName = PostgresName + "-" + "pguser" + "-" + PostgresGuestUser
	PostgresSuperUser           = "postgres"
	PostgresSuperUserSecretName = PostgresName + "-" + "pguser" + "-" + PostgresSuperUser
	PostgresCertName            = PostgresName + "-cluster-cert"

	// need append "?sslmode=verify-ca" to the end of the uri to access postgres
	PostgresURIWithSslmode = "?sslmode=verify-ca"

	// the storage states
	isBYOPostgres = false
	databaseReady = false
	postgresConn  *PostgresConnection
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

// SetPostgresType assert the current storage is BYO or built-in, and cache the state to memeory
func SetPostgresType(ctx context.Context, runtimeClient client.Client, namespace string) error {
	pgSecret := &corev1.Secret{}
	err := runtimeClient.Get(ctx, types.NamespacedName{
		Name:      constants.GHStorageSecretName,
		Namespace: namespace,
	}, pgSecret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			isBYOPostgres = false
			return nil
		}
		return err
	}
	isBYOPostgres = true
	return nil
}

func IsBYOPostgres() bool {
	return isBYOPostgres
}

func SetBYOPostgres(byo bool) {
	isBYOPostgres = byo
}

func SetDatabaseReady(ready bool) {
	databaseReady = ready
}

func GetDatabaseReady() bool {
	return databaseReady
}

// GeneratePGConnectionFromGHStorageSecret returns a postgres connection from the GH storage secret
func GetPGConnectionFromGHStorageSecret(ctx context.Context, client client.Client) (
	*PostgresConnection, error,
) {
	pgSecret := &corev1.Secret{}
	err := client.Get(ctx, types.NamespacedName{
		Name:      constants.GHStorageSecretName,
		Namespace: utils.GetDefaultNamespace(),
	}, pgSecret)
	if err != nil {
		return nil, err
	}
	return &PostgresConnection{
		SuperuserDatabaseURI:    string(pgSecret.Data["database_uri"]),
		ReadonlyUserDatabaseURI: string(pgSecret.Data["database_uri_with_readonlyuser"]),
		CACert:                  pgSecret.Data["ca.crt"],
	}, nil
}

func GetPGConnectionFromBuildInPostgres(ctx context.Context, client client.Client) (
	*PostgresConnection, error,
) {
	// wait for postgres guest user secret to be ready
	guestPostgresSecret := &corev1.Secret{}
	err := client.Get(ctx, types.NamespacedName{
		Name:      PostgresGuestUserSecretName,
		Namespace: utils.GetDefaultNamespace(),
	}, guestPostgresSecret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("postgres guest user secret %s is nil", PostgresGuestUserSecretName)
		}
		return nil, err
	}
	// wait for postgres super user secret to be ready
	superuserPostgresSecret := &corev1.Secret{}
	err = client.Get(ctx, types.NamespacedName{
		Name:      PostgresSuperUserSecretName,
		Namespace: utils.GetDefaultNamespace(),
	}, superuserPostgresSecret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("postgres super user secret %s is nil", PostgresSuperUserSecretName)
		}
		return nil, err
	}
	// wait for postgres cert secret to be ready
	postgresCertName := &corev1.Secret{}
	err = client.Get(ctx, types.NamespacedName{
		Name:      PostgresCertName,
		Namespace: utils.GetDefaultNamespace(),
	}, postgresCertName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("postgres cert secret %s is nil", PostgresCertName)
		}
		return nil, err
	}

	return &PostgresConnection{
		SuperuserDatabaseURI:    string(superuserPostgresSecret.Data["uri"]) + PostgresURIWithSslmode,
		ReadonlyUserDatabaseURI: string(guestPostgresSecret.Data["uri"]) + PostgresURIWithSslmode,
		CACert:                  postgresCertName.Data["ca.crt"],
	}, nil
}

// SetStorageConnection update the postgres connection
func SetStorageConnection(conn *PostgresConnection) bool {
	log.Debugf("Set Storage Connection: %v", conn == nil)
	if conn == nil {
		postgresConn = nil
		return true
	}
	if !reflect.DeepEqual(conn, postgresConn) {
		postgresConn = conn
		log.Debugf("Update Storage Connection")
		return true
	}
	return false
}

func GetStorageConnection() *PostgresConnection {
	log.Debugf("Get Storage Connection: %v", postgresConn != nil)
	return postgresConn
}

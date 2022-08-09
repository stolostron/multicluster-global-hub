package hubofhubs

import (
	"context"
	"crypto/rand"
	"embed"
	"fmt"
	iofs "io/fs"
	"math/big"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/jackc/pgx/v4"
	hubofhubsv1alpha1 "github.com/stolostron/hub-of-hubs/operator/apis/hubofhubs/v1alpha1"
	"github.com/stolostron/hub-of-hubs/operator/pkg/condition"
)

const (
	PROCESS_URI_KEY    = "process_uri"
	TRANSPORT_URI_KEY  = "transport_uri"
	failedConditionMsg = "failed to set condition(%s): %w"
)

//go:embed database
var databaseFS embed.FS

func (reconciler *ConfigReconciler) reconcileDatabase(ctx context.Context, config *hubofhubsv1alpha1.Config,
	namespacedName types.NamespacedName) error {
	log := ctrllog.FromContext(ctx)
	if condition.ContainConditionStatus(config, condition.CONDITION_TYPE_DATABASE_INIT, condition.CONDITION_STATUS_TRUE) {
		log.Info("Database has initialized")
		return nil
	}

	log.Info("Database initializing")
	postgreSecret := &corev1.Secret{}
	err := reconciler.Client.Get(ctx, namespacedName, postgreSecret)
	if err != nil {
		log.Error(err, "Can't get postgres secret")
		return err
	}
	host := string(postgreSecret.Data["host"])
	port := string(postgreSecret.Data["port"])
	user := string(postgreSecret.Data["user"])
	password := string(postgreSecret.Data["password"])
	database := string(postgreSecret.Data["database"])

	adminDatabaseURI := fmt.Sprintf("postgres://%s:%s@%s:%s/%s", user, password, host, port, database)
	conn, err := pgx.Connect(ctx, adminDatabaseURI)
	if err != nil {
		return fmt.Errorf("failed to connect to postgres: %w", err)
	}
	defer conn.Close(ctx)

	err = iofs.WalkDir(databaseFS, "database", func(file string, d iofs.DirEntry, beforeError error) error {
		if beforeError != nil {
			return beforeError
		}
		if d.IsDir() {
			return nil
		}
		log.Info("Database executing SQL file: " + file)
		sqlBytes, err := databaseFS.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", file, err)
		}
		_, err = conn.Exec(context.Background(), string(sqlBytes))
		if err != nil {
			return fmt.Errorf("failed to create %s: %w", file, err)
		}
		return nil
	})
	if err != nil {
		conditionError := condition.SetConditionDatabaseInit(ctx, reconciler.Client, config, condition.CONDITION_STATUS_FALSE)
		if conditionError != nil {
			return fmt.Errorf(failedConditionMsg, condition.CONDITION_STATUS_FALSE, conditionError)
		}
		return fmt.Errorf("failed to walk database directory: %w", err)
	}

	processPassword := generatePassword(8)
	transportPassword := generatePassword(8)
	_, err = conn.Exec(context.Background(), fmt.Sprintf("ALTER USER process_user with password '%s'", processPassword))
	if err != nil {
		return fmt.Errorf("failed to alter process_user password: %w", err)
	}
	_, err = conn.Exec(context.Background(), fmt.Sprintf("ALTER USER transport_user with password '%s'",
		transportPassword))
	if err != nil {
		return fmt.Errorf("failed to alter transport_user password: %w", err)
	}

	processURI := fmt.Sprintf("postgres://process_user:%s@%s:%s/%s", processPassword, host, port, database)
	transportURI := fmt.Sprintf("postgres://transport_user:%s@%s:%s/%s", transportPassword, host, port, database)

	postgreSecret.Data[PROCESS_URI_KEY] = []byte(processURI)
	postgreSecret.Data[TRANSPORT_URI_KEY] = []byte(transportURI)
	err = reconciler.Client.Update(ctx, postgreSecret)
	if err != nil {
		conditionError := condition.SetConditionDatabaseInit(ctx, reconciler.Client, config, condition.CONDITION_STATUS_FALSE)
		if conditionError != nil {
			return fmt.Errorf(failedConditionMsg, condition.CONDITION_STATUS_FALSE, conditionError)
		}
		return fmt.Errorf("failed to update postgres secret: %w", err)
	}

	log.Info("Database initialized")
	err = condition.SetConditionDatabaseInit(ctx, reconciler.Client, config, condition.CONDITION_STATUS_TRUE)
	if err != nil {
		return fmt.Errorf(failedConditionMsg, condition.CONDITION_STATUS_TRUE, err)
	}
	return nil
}

func generatePassword(len int) string {
	randomStr := make([]byte, len)
	for i := 0; i < len; i++ {
		n, _ := rand.Int(rand.Reader, big.NewInt(25))
		randomStr[i] = byte(65 + n.Int64())
	}
	return string(randomStr)
}

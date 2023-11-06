package hubofhubs

import (
	"context"
	"testing"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/test/pkg/testpostgres"
)

func Test_handleUpgrade(t *testing.T) {
	testPostgres, err := testpostgres.NewTestPostgres()
	if err != nil {
		t.Errorf("Failed to get postgres")
	}
	conn, err := database.PostgresConnection(context.TODO(), testPostgres.URI, nil)
	if err != nil {
		t.Errorf("Failed to get postgres connection: %v", err)
	}

	err = handleUpgrade(context.Background(), conn, ctrl.Log.WithName("test-controller"), upgradeFS, "root")
	if err == nil {
		t.Errorf("Failed to get upgrade :%v", err)
	}
}

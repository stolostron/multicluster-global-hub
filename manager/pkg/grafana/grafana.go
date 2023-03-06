package grafana

import (
	"context"
	"embed"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/go-logr/logr"
	managerconfig "github.com/stolostron/multicluster-global-hub/manager/pkg/config"
)

//go:embed manifests
var fs embed.FS

type GrafanaServer struct {
	log       logr.Logger
	namespace string
}

// Add grafana to the global hub
func AddGrafanaServer(mgr manager.Manager, config *managerconfig.ManagerConfig) error {
	// render := renderer.NewHoHRenderer(fs)
	// grafanaObjects, err := render.Render("manifests", "", func(profile string) (interface{}, error) {
	// 	return
	// })

	if err := mgr.Add(&GrafanaServer{
		log:       ctrl.Log.WithName("grafana-server"),
		namespace: config.ManagerNamespace,
	}); err != nil {
		return fmt.Errorf("failed to add grafana server to the manager: %w", err)
	}
	return nil
}

func (s *GrafanaServer) Start(ctx context.Context) error {
	s.log.Info("starting grafana server")

	go func() {
		<-ctx.Done()
		s.log.Info("stopping grafana server")
	}()
	return nil
}

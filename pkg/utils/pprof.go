package utils

import (
	"net/http"
	_ "net/http/pprof" // #nosec G108
	"time"

	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

func StartDefaultPprofServer() {
	log := logger.DefaultZapLogger()
	server := &http.Server{
		Addr:              ":6060",
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatal("failed to start the pprof sever: ", err)
	}
}

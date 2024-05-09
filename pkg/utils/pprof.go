package utils

import (
	"net/http"
	_ "net/http/pprof" // #nosec G108
	"time"

	"k8s.io/klog"
)

func StartDefaultPprofServer() {
	server := &http.Server{
		Addr:              ":6060",
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		klog.Error(err, "failed to start the pprof server")
	}
}

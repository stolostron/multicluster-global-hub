package utils

import (
	"net/http"
	_ "net/http/pprof"

	"k8s.io/klog"
)

func StartDefaultPprofServer() {
	if err := http.ListenAndServe(":6060", nil); err != nil {
		klog.Error(err, "failed to start the pprof server")
	}
}

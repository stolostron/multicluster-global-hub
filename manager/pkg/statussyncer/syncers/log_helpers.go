package dbsyncer

import (
	"github.com/go-logr/logr"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/helpers"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
)

const (
	startBundleHandlingMessage  = "started handling bundle"
	finishBundleHandlingMessage = "finished handling bundle"
)

func logBundleHandlingMessage(log logr.Logger, bundle status.Bundle, message string) {
	log.V(2).Info(message, "BundleType", helpers.GetBundleType(bundle), "LeafHubName", bundle.GetLeafHubName(),
		"Version", bundle.GetVersion().String())
}

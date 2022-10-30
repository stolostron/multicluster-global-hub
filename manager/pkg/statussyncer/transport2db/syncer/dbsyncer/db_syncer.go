package dbsyncer

import (
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// DBSyncer interface for registering business logic needed for handling bundles.
type DBSyncer interface {
	// RegisterCreateBundleFunctions registers create bundle functions within the transport instance.
	RegisterCreateBundleFunctions(transportInstance transport.Transport)
	// RegisterBundleHandlerFunctions registers bundle handler functions within the conflation manager.
	RegisterBundleHandlerFunctions(conflationManager *conflator.ConflationManager)
}

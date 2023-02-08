package dbsyncer

import (
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/syncer/dispatcher"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
)

// DBSyncer interface for registering business logic needed for handling bundles.
type DBSyncer interface {
	// RegisterCreateBundleFunctions registers create bundle functions within the transport instance.
	RegisterCreateBundleFunctions(transportDispatcher *dispatcher.TransportDispatcher)
	// RegisterBundleHandlerFunctions registers bundle handler functions within the conflation manager.
	RegisterBundleHandlerFunctions(conflationManager *conflator.ConflationManager)
}

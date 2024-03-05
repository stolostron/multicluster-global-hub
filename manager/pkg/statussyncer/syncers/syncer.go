package dbsyncer

import (
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/registration"
)

type BundleRegisterable interface {
	BundleRegister(*registration.BundleRegistration)
}

// Syncer interface for registering business logic needed for handling bundles.
type Syncer interface {
	// RegisterCreateBundleFunctions registers create bundle functions within the transport instance.
	RegisterCreateBundleFunctions(bundleRegisterable BundleRegisterable)
	// RegisterBundleHandlerFunctions registers bundle handler functions within the conflation manager.
	RegisterBundleHandlerFunctions(conflationManager *conflator.ConflationManager)
}

// Handler interface for registering business logic needed for handling bundles.
type Handler interface {
	// RegisterHandler registers event handler functions within the conflation manager.
	RegisterHandler(conflationManager *conflator.ConflationManager)
}

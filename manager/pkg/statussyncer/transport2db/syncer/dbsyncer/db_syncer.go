package dbsyncer

import (
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/registration"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
)

type BundleRegisterable interface {
	BundleRegister(*registration.BundleRegistration)
}

// DBSyncer interface for registering business logic needed for handling bundles.
type DBSyncer interface {
	// RegisterCreateBundleFunctions registers create bundle functions within the transport instance.
	RegisterCreateBundleFunctions(bundleRegisterable BundleRegisterable)
	// RegisterBundleHandlerFunctions registers bundle handler functions within the conflation manager.
	RegisterBundleHandlerFunctions(conflationManager *conflator.ConflationManager)
}

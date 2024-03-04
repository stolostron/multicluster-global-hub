package dbsyncer

import (
	"context"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
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

type Handler interface {
	Type() enum.EventType
	Priority() conflator.ConflationPriority
	Handle(ctx context.Context, evt cloudevents.Event) error
}

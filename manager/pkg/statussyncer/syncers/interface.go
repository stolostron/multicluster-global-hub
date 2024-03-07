package dbsyncer

import (
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/conflator"
)

// Handler interface for registering business logic needed for handling bundles.
type Handler interface {
	// RegisterHandler registers event handler functions within the conflation manager.
	RegisterHandler(conflationManager *conflator.ConflationManager)
}

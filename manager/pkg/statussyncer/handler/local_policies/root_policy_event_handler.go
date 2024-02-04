package localpolicies

import (
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/types"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
)

var _ EventHandler = &localReplicatedPolicyEventHanlder{}

type localReplicatedPolicyEventHanlder struct {
	lastProcessedVersion metadata.BundleVersion
}

func (h *localReplicatedPolicyEventHanlder) ToDatabase(evt cloudevents.Event) error {
	versionStr, err := types.ToString(evt.Extensions()[metadata.ExtVersion])
	if err != nil {
		return err
	}
	version, err := metadata.BundleVersionFrom(versionStr)
	if err != nil {
		return err
	}

	if !version.NewerThan(&h.lastProcessedVersion) {
		return nil
	}

	//TODO:
	// implement the handler
	// add the handler to the transport dispatcher
}

type EventHandler interface {
	ToDatabase(evt cloudevents.Event) error
}

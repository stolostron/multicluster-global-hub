package spec

import (
	"context"

	cloudevents "github.com/cloudevents/sdk-go/v2"
)

type Syncer interface {
	Sync(ctx context.Context, evt *cloudevents.Event) error
}

type Dispatcher interface {
	Start(ctx context.Context) error
	RegisterSyncer(messageID string, syncer Syncer)
}

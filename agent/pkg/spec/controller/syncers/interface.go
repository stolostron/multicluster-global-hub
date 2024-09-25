package syncers

import (
	"context"
)

type Syncer interface {
	Sync(ctx context.Context, payload []byte) error
}

type Dispatcher interface {
	Start(ctx context.Context) error
	RegisterSyncer(messageID string, syncer Syncer)
}

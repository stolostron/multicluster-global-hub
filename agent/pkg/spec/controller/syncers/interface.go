package syncers

import (
	"context"
)

type Syncer interface {
	Sync(payload []byte) error
}

type Dispatcher interface {
	Start(ctx context.Context) error
	RegisterSyncer(messageID string, syncer Syncer)
}

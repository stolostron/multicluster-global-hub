package syncers

import "context"

const GenericMessageKey = "generic"

type Syncer interface {
	Start(ctx context.Context) error
	Channel() chan []byte
}

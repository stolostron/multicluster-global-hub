package syncers

import "context"

type Syncer interface {
	Start(ctx context.Context) error
	Process(bundle interface{}) error
}

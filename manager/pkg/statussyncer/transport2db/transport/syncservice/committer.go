package syncservice

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/open-horizon/edge-sync-service-client/client"

	"github.com/stolostron/multicluster-globalhub/manager/pkg/statussyncer/transport2db/transport"
)

// newCommitter returns a new instance of Committer.
func newCommitter(committerInterval time.Duration, client *client.SyncServiceClient,
	getBundlesMetadataFunc transport.GetBundlesMetadataFunc, log logr.Logger,
) (*committer, error) {
	return &committer{
		log:                           log,
		client:                        client,
		getBundlesMetadataFunc:        getBundlesMetadataFunc,
		committedMetadataToVersionMap: make(map[string]string),
		interval:                      committerInterval,
	}, nil
}

// committer is responsible for committing offsets to transport.
type committer struct {
	log                           logr.Logger
	client                        *client.SyncServiceClient
	getBundlesMetadataFunc        transport.GetBundlesMetadataFunc
	committedMetadataToVersionMap map[string]string
	interval                      time.Duration
}

// start runs the Committer instance.
func (c *committer) start(ctx context.Context) {
	go c.commitMetadata(ctx)
}

func (c *committer) commitMetadata(ctx context.Context) {
	ticker := time.NewTicker(c.interval)

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C: // wait for next time interval
			processedBundleMetadataToCommit := make(map[string]*bundleMetadata)

			metadataArray := c.getBundlesMetadataFunc()
			// sync service objects should be committed only if processed
			for _, transportMetadata := range metadataArray {
				metadata, ok := transportMetadata.(*bundleMetadata)
				if !ok {
					continue // shouldn't happen
				}

				if metadata.Processed() {
					key := fmt.Sprintf("%s.%s", metadata.objectMetadata.ObjectID,
						metadata.objectMetadata.ObjectType)
					processedBundleMetadataToCommit[key] = metadata
				}
			}

			if err := c.commitObjectsMetadata(processedBundleMetadataToCommit); err != nil {
				c.log.Error(err, "committer failed")
			}
		}
	}
}

func (c *committer) commitObjectsMetadata(bundleMetadataMap map[string]*bundleMetadata) error {
	for key, bundleMetadata := range bundleMetadataMap {
		if version, found := c.committedMetadataToVersionMap[key]; found {
			if version == bundleMetadata.objectMetadata.Version {
				continue // already committed
			}
		}

		if err := c.client.MarkObjectConsumed(bundleMetadata.objectMetadata); err != nil {
			return fmt.Errorf("failed to commit object - stopping bulk commit : %w", err)
		}

		c.committedMetadataToVersionMap[key] = bundleMetadata.objectMetadata.Version
	}

	return nil
}

package workerpool

import (
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
)

// NewDBJob creates a new instance of DBJob.
func NewDBJob(bundle status.Bundle, metadata *conflator.BundleMetadata, handlerFunction conflator.BundleHandlerFunc,
	conflationUnitResultReporter conflator.ResultReporter,
) *DBJob {
	return &DBJob{
		bundle:                       bundle,
		bundleMetadata:               metadata,
		handlerFunc:                  handlerFunction,
		conflationUnitResultReporter: conflationUnitResultReporter,
	}
}

// DBJob represents the job to be run by a DBWorker from the pool.
type DBJob struct {
	bundle                       status.Bundle
	bundleMetadata               *conflator.BundleMetadata
	handlerFunc                  conflator.BundleHandlerFunc
	conflationUnitResultReporter conflator.ResultReporter
}

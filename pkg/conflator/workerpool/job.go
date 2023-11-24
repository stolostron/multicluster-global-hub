package workerpool

import (
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
)

// NewDBJob creates a new instance of DBJob.
func NewDBJob(bundle bundle.ManagerBundle, metadata *conflator.ConflationBundleMetadata,
	handlerFunction conflator.BundleHandlerFunc,
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
	bundle                       bundle.ManagerBundle
	bundleMetadata               *conflator.ConflationBundleMetadata
	handlerFunc                  conflator.BundleHandlerFunc
	conflationUnitResultReporter conflator.ResultReporter
}

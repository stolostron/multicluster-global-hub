package statistics

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/helpers"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
)

type StatisticsConfig struct {
	LogInterval time.Duration
}

// NewStatistics creates a new instance of Statistics.
func NewStatistics(statisticsConfig *StatisticsConfig, bundleTypes []string) *Statistics {
	statistics := &Statistics{
		log:           ctrl.Log.WithName("statistics"),
		bundleMetrics: make(map[string]*bundleMetrics),
		logInterval:   statisticsConfig.LogInterval,
	}

	for _, bundleType := range bundleTypes {
		statistics.bundleMetrics[bundleType] = newBundleMetrics()
	}

	return statistics
}

// Statistics aggregates different statistics.
type Statistics struct {
	log                      logr.Logger
	numOfAvailableDBWorkers  int
	conflationReadyQueueSize int
	bundleMetrics            map[string]*bundleMetrics
	logInterval              time.Duration
}

// IncrementNumberOfReceivedBundles increments total number of received bundles of the specific type via transport.
func (s *Statistics) IncrementNumberOfReceivedBundles(bundle status.Bundle) {
	bundleMetrics := s.bundleMetrics[helpers.GetBundleType(bundle)]

	bundleMetrics.totalReceived++
}

// SetNumberOfAvailableDBWorkers sets number of available db workers.
func (s *Statistics) SetNumberOfAvailableDBWorkers(numOf int) {
	s.numOfAvailableDBWorkers = numOf
}

// SetConflationReadyQueueSize sets conflation ready queue size.
func (s *Statistics) SetConflationReadyQueueSize(size int) {
	s.conflationReadyQueueSize = size
}

// StartConflationUnitMetrics starts conflation unit metrics of the specific bundle type.
func (s *Statistics) StartConflationUnitMetrics(bundle status.Bundle) {
	bundleMetrics := s.bundleMetrics[helpers.GetBundleType(bundle)]

	bundleMetrics.conflationUnit.start(bundle.GetLeafHubName())
}

// StopConflationUnitMetrics stops conflation unit metrics of the specific bundle type.
func (s *Statistics) StopConflationUnitMetrics(bundle status.Bundle) {
	bundleMetrics := s.bundleMetrics[helpers.GetBundleType(bundle)]

	bundleMetrics.conflationUnit.stop(bundle.GetLeafHubName())
}

// IncrementNumberOfConflations increments number of conflations of the specific bundle type.
func (s *Statistics) IncrementNumberOfConflations(bundle status.Bundle) {
	bundleMetrics := s.bundleMetrics[helpers.GetBundleType(bundle)]

	bundleMetrics.conflationUnit.incrementNumberOfConflations()
}

// AddDatabaseMetrics adds database metrics of the specific bundle type.
func (s *Statistics) AddDatabaseMetrics(bundle status.Bundle, duration time.Duration, err error) {
	bundleMetrics := s.bundleMetrics[helpers.GetBundleType(bundle)]

	bundleMetrics.database.add(duration, err)
}

// Start starts the statistics.
func (s *Statistics) Start(ctx context.Context) error {
	s.log.Info("starting statistics")

	go s.run(ctx)

	// blocking wait until getting cancel context event
	<-ctx.Done()
	s.log.Info("stopped statistics")

	return nil
}

func (s *Statistics) run(ctx context.Context) {
	if s.logInterval.Seconds() <= 0 {
		return // if log interval is set to 0 or negative value, statistics log is disabled.
	}

	ticker := time.NewTicker(s.logInterval)

	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return

		case <-ticker.C: // dump statistics
			var metrics strings.Builder

			for bundleType, bundleMetrics := range s.bundleMetrics {
				metrics.WriteString(fmt.Sprintf("[%s, (transport {total received=%d}), (cu {%s}), (db process {%s})], ",
					bundleType, bundleMetrics.totalReceived,
					bundleMetrics.conflationUnit.toString(),
					bundleMetrics.database.toString()))
			}

			s.log.Info("statistics:",
				"conflation ready queue size", s.conflationReadyQueueSize,
				"available db workers", s.numOfAvailableDBWorkers,
				"metrics", strings.TrimSuffix(metrics.String(), ", "))
		}
	}
}

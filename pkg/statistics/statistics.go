package statistics

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"go.uber.org/zap"

	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

type StatisticsConfig struct {
	LogInterval string
}

// NewStatistics creates a new instance of Statistics.
func NewStatistics(statisticsConfig *StatisticsConfig) *Statistics {
	return &Statistics{
		log:          logger.DefaultZapLogger(),
		eventMetrics: make(map[string]*eventMetrics),
		logInterval:  statisticsConfig.LogInterval,
	}
}

// Statistics aggregates different statistics.
type Statistics struct {
	log                      *zap.SugaredLogger
	numOfAvailableDBWorkers  int
	conflationReadyQueueSize int
	numOfConflationUnits     int
	eventMetrics             map[string]*eventMetrics
	logInterval              string
	mutex                    sync.Mutex
}

func (s *Statistics) Register(eventType string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.eventMetrics[eventType] = newEventMetrics()
}

func (s *Statistics) ReceivedEvent(evt *cloudevents.Event) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	metrics, ok := s.eventMetrics[evt.Type()]
	if !ok {
		return
	}
	metrics.totalReceived++
}

// SetNumberOfAvailableDBWorkers sets number of available db workers.
func (s *Statistics) SetNumberOfAvailableDBWorkers(numOf int) {
	s.numOfAvailableDBWorkers = numOf
}

// SetConflationReadyQueueSize sets conflation ready queue size.
func (s *Statistics) SetConflationReadyQueueSize(size int) {
	s.conflationReadyQueueSize = size
}

// StartConflationUnitMetrics starts conflation unit metrics of the specific event type.
func (s *Statistics) StartConflationUnitMetrics(evt *cloudevents.Event) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	eventMetrics, ok := s.eventMetrics[evt.Type()]
	if !ok {
		return
	}
	eventMetrics.conflationUnit.start(evt.Source())
}

// StopConflationUnitMetrics stops conflation unit metrics of the specific event type.
func (s *Statistics) StopConflationUnitMetrics(evt *cloudevents.Event, err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	eventMetrics, ok := s.eventMetrics[evt.Type()]
	if !ok {
		return
	}
	eventMetrics.conflationUnit.stop(evt.Source(), err)
}

// IncrementNumberOfConflations increments number of conflations
func (s *Statistics) IncrementNumberOfConflations() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.numOfConflationUnits++
}

// AddDatabaseMetrics adds database metrics of the specific event type.
func (s *Statistics) AddDatabaseMetrics(evt *cloudevents.Event, duration time.Duration, err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	eventMetrics, ok := s.eventMetrics[evt.Type()]
	if !ok {
		return
	}
	eventMetrics.database.add(duration, err)
}

// Start starts the statistics.
func (s *Statistics) Start(ctx context.Context) error {
	s.log.Info("starting statistics")
	duration, err := time.ParseDuration(s.logInterval)
	if err != nil {
		return err
	}

	go s.run(ctx, duration)

	// blocking wait until getting cancel context event
	<-ctx.Done()
	s.log.Info("stopped statistics")

	return nil
}

func (s *Statistics) run(ctx context.Context, duration time.Duration) {
	if duration.Seconds() <= 0 {
		return // if log interval is set to 0 or negative value, statistics log is disabled.
	}

	ticker := time.NewTicker(duration)

	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return

		case <-ticker.C: // dump statistics
			var stringBuilder strings.Builder
			success := int64(0)
			fail := int64(0)
			storageAvg := float64(0)
			conflationAvg := float64(0)

			s.mutex.Lock()
			for eventType, metrics := range s.eventMetrics {
				stringBuilder.WriteString(fmt.Sprintf("[%-42s(%d) | conflation(%-42s) | storage(%-42s)] \n",
					eventType, metrics.totalReceived,
					metrics.conflationUnit.toString(),
					metrics.database.toString()))
				success += metrics.totalReceived
				fail += (metrics.conflationUnit.failures + metrics.database.failures)
				if metrics.conflationUnit.successes > 0 {
					conflationAvg = float64(metrics.conflationUnit.totalDuration / metrics.conflationUnit.successes)
				}
				if metrics.database.successes > 0 {
					storageAvg = float64(metrics.database.totalDuration / metrics.database.successes)
				}
			}
			metricsStr := fmt.Sprintf("{CU=%d, CUQueue=%d, idleDBW=%d, success=%d, fail=%d, CU Avg=%.0f ms, DB Avg=%.0f ms}",
				s.numOfConflationUnits, s.conflationReadyQueueSize, s.numOfAvailableDBWorkers, success, fail,
				conflationAvg, storageAvg)
			s.mutex.Unlock()

			s.log.Debug(fmt.Sprintf("%s\n%s", metricsStr, stringBuilder.String()))
		}
	}
}

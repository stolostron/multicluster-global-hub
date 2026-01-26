package migration

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	migrationStatuses           = make(map[string]*MigrationStatus)
	mu                          sync.RWMutex // optional: for concurrent access
	currentMigrationClusterList = make(map[string][]string)
)

type MigrationStatus struct {
	HubState map[string]*StageState // key: hub-phase
}

type StageState struct {
	started                bool
	finished               bool
	error                  string
	clusterErrors          map[string]string // cluster name -> error message
	lastStartTime          time.Time         // the last start time of the stage
	failedClusters         []string          // clusters that failed to migrate (for rollback)
	failedClustersReported bool              // whether failed clusters have been explicitly reported
}

// AddMigrationStatus init the migration status for the migrationId
func AddMigrationStatus(migrationId string) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := migrationStatuses[migrationId]; ok {
		return
	}
	migrationStatuses[migrationId] = &MigrationStatus{
		HubState: make(map[string]*StageState),
	}
	log.Infof("initialize migration status for migrationId: %s", migrationId)
}

func ResetMigrationStatus(managedHubName string) {
	mu.Lock()
	defer mu.Unlock()
	for migrationId, status := range migrationStatuses {
		for hubPhaseKey, state := range status.HubState {
			lastDashIndex := strings.LastIndex(hubPhaseKey, "-")
			if lastDashIndex == -1 {
				continue // Skip invalid keys without "-"
			}
			hub := hubPhaseKey[:lastDashIndex]
			phase := hubPhaseKey[lastDashIndex+1:]
			if hub != managedHubName {
				continue
			}
			state.started = false
			state.finished = false
			state.error = ""
			state.clusterErrors = nil
			state.failedClusters = nil
			state.failedClustersReported = false
			log.Infof("reset migration status for migrationId: %s, hub: %s, phase: %s", migrationId, hub, phase)
		}
	}
}

// AddMigrationStatus clean the migration status for the migrationId
func RemoveMigrationStatus(migrationId string) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := migrationStatuses[migrationId]; !ok {
		return
	}
	delete(migrationStatuses, migrationId)
	delete(currentMigrationClusterList, migrationId)
	log.Infof("clean up migration status for migrationId: %s", migrationId)
}

func hubPhaseKey(hub, phase string) string {
	return fmt.Sprintf("%s-%s", hub, phase)
}

func getMigrationStatus(migrationId string) *MigrationStatus {
	status, ok := migrationStatuses[migrationId]
	if !ok {
		return nil
	}
	return status
}

func getStageState(migrationId, hub, phase string) *StageState {
	status := getMigrationStatus(migrationId)
	if status == nil {
		log.Warnf("MigrationStatus is nil for migrationId: %s, hub: %s, phase: %s", migrationId, hub, phase)
		return nil
	}
	key := hubPhaseKey(hub, phase)
	if _, exists := status.HubState[key]; !exists {
		status.HubState[key] = &StageState{}
	}
	return status.HubState[key]
}

// ResetStageState resets the stage state for the given hub and phase, allowing re-execution.
// This is used when a rollback retry is requested via annotation.
func ResetStageState(migrationId, hub, phase string) {
	mu.Lock()
	defer mu.Unlock()
	if p := getStageState(migrationId, hub, phase); p != nil {
		p.started = false
		p.finished = false
		p.error = ""
		p.clusterErrors = nil
		p.failedClusters = nil
		p.failedClustersReported = false
		log.Infof("reset stage state for migrationId: %s, hub: %s, phase: %s", migrationId, hub, phase)
	}
}

// SetStarted sets the status of the given stage to started for the hub cluster
func SetStarted(migrationId, hub, phase string) {
	mu.Lock()
	defer mu.Unlock()
	if p := getStageState(migrationId, hub, phase); p != nil {
		p.started = true
		p.lastStartTime = time.Now()
	}
}

// SetFinished sets the status of the given stage to finished for the hub cluster
func SetFinished(migrationId, hub, phase string) {
	mu.Lock()
	defer mu.Unlock()
	if p := getStageState(migrationId, hub, phase); p != nil {
		p.finished = true
	}
}

// SetClusterList sets the managed clusters list for the given migration stage, it invoked by the status handler
func SetClusterList(migrationId string, managedClusters []string) {
	mu.Lock()
	defer mu.Unlock()
	currentMigrationClusterList[migrationId] = managedClusters
}

func SetClusterErrorDetailMap(migrationId string, hub, phase string, clusterErrors map[string]string) {
	mu.Lock()
	defer mu.Unlock()
	if p := getStageState(migrationId, hub, phase); p != nil {
		p.clusterErrors = clusterErrors
	}
}

func SetErrorMessage(migrationId, hub, phase, errMessage string) {
	mu.Lock()
	defer mu.Unlock()
	if p := getStageState(migrationId, hub, phase); p != nil {
		p.error = errMessage
	}
}

// GetStarted returns true if the status of the given stage is started for the hub cluster
func GetStarted(migrationId, hub, phase string) bool {
	mu.RLock()
	defer mu.RUnlock()
	if p := getStageState(migrationId, hub, phase); p != nil {
		return p.started
	}
	return false
}

// GetFinished returns true if the status of the given stage is finished for the hub cluster
func GetFinished(migrationId, hub, phase string) bool {
	mu.RLock()
	defer mu.RUnlock()
	if p := getStageState(migrationId, hub, phase); p != nil {
		return p.finished
	}
	return false
}

func GetErrorMessage(migrationId, hub, phase string) string {
	mu.RLock()
	defer mu.RUnlock()
	if p := getStageState(migrationId, hub, phase); p != nil {
		return p.error
	}
	return ""
}

// GetClusterList returns the managed clusters list for the given migration stage
func GetClusterList(migrationId string) []string {
	mu.RLock()
	defer mu.RUnlock()
	if currentMigrationClusterList == nil {
		return nil
	}
	clusters, ok := currentMigrationClusterList[migrationId]
	if ok {
		return clusters
	}
	return nil
}

func GetClusterErrors(migrationId, hub, phase string) map[string]string {
	mu.RLock()
	defer mu.RUnlock()
	if p := getStageState(migrationId, hub, phase); p != nil {
		return p.clusterErrors
	}
	return nil
}

// SetFailedClusters sets the failed clusters list for the given migration stage
// This is called when the target hub reports failed clusters during rollback
func SetFailedClusters(migrationId, hub, phase string, clusters []string) {
	mu.Lock()
	defer mu.Unlock()
	if p := getStageState(migrationId, hub, phase); p != nil {
		p.failedClusters = clusters
		p.failedClustersReported = true
		log.Infof("set failed clusters for migrationId: %s, hub: %s, phase: %s, clusters: %v",
			migrationId, hub, phase, clusters)
	}
}

// GetFailedClusters returns the failed clusters list for the given migration stage
func GetFailedClusters(migrationId, hub, phase string) []string {
	mu.RLock()
	defer mu.RUnlock()
	if p := getStageState(migrationId, hub, phase); p != nil {
		return p.failedClusters
	}
	return nil
}

// HasFailedClustersReported returns true if failed clusters have been reported for the given stage
func HasFailedClustersReported(migrationId, hub, phase string) bool {
	mu.RLock()
	defer mu.RUnlock()
	if p := getStageState(migrationId, hub, phase); p != nil {
		return p.failedClustersReported
	}
	return false
}

func GetReadyClusters(migrationId, hub, phase string) []string {
	mu.RLock()
	defer mu.RUnlock()

	if currentMigrationClusterList == nil {
		return nil
	}
	allClusters, ok := currentMigrationClusterList[migrationId]
	if !ok {
		return nil
	}

	clusters := []string{}
	if p := getStageState(migrationId, hub, phase); p != nil {
		for _, cluster := range allClusters {
			if _, ok := p.clusterErrors[cluster]; !ok {
				clusters = append(clusters, cluster)
			}
		}
	}
	return clusters
}

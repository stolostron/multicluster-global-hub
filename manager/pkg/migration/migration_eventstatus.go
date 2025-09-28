package migration

import (
	"fmt"
	"strings"
	"sync"
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
	started       bool
	finished      bool
	error         string
	clusterErrors map[string]string // cluster name -> error message
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
		log.Warnf("MigrationStatus is nil for migrationId: %s", migrationId)
		return nil
	}
	return status
}

func getStageState(migrationId, hub, phase string) *StageState {
	status := getMigrationStatus(migrationId)
	if status == nil {
		return nil
	}
	key := hubPhaseKey(hub, phase)
	if _, exists := status.HubState[key]; !exists {
		status.HubState[key] = &StageState{}
	}
	return status.HubState[key]
}

// SetStarted sets the status of the given stage to started for the hub cluster
func SetStarted(migrationId, hub, phase string) {
	mu.Lock()
	defer mu.Unlock()
	if p := getStageState(migrationId, hub, phase); p != nil {
		p.started = true
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

func SetClusterErrorMessage(migrationId string, hub, phase string, clusterErrors map[string]string) {
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

package migration

import (
	"fmt"
	"sync"
)

var (
	migrationStatuses = make(map[string]*MigrationStatus)
	mu                sync.RWMutex // optional: for concurrent access
)

type MigrationStatus struct {
	Progress map[string]*MigrationProgress // key: hub-phase
	Clusters map[string][]string           // key: source hub -> clusters
}

type MigrationProgress struct {
	started  bool
	finished bool
	error    string
}

// AddMigrationStatus init the migration status for the migrationId
func AddMigrationStatus(migrationId string) {
	mu.Lock()
	defer mu.Unlock()
	migrationStatuses[migrationId] = &MigrationStatus{
		Progress: make(map[string]*MigrationProgress),
	}
	log.Infof("initialize migration status for migrationId: %s", migrationId)
}

// AddMigrationStatus clean the migration status for the migrationId
func RemoveMigrationStatus(migrationId string) {
	mu.Lock()
	defer mu.Unlock()
	delete(migrationStatuses, migrationId)
	log.Infof("clean up migration status for migrationId: %s", migrationId)
}

func AddSourceClusters(migrationId string, clusters map[string][]string) {
	mu.RLock()
	defer mu.RUnlock()

	status := getMigrationStatus(migrationId)
	if status == nil {
		return
	}
	status.Clusters = clusters
}

func GetSourceClusters(migrationId string) map[string][]string {
	mu.RLock()
	defer mu.RUnlock()

	status := getMigrationStatus(migrationId)
	if status == nil {
		return nil
	}
	return status.Clusters
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

func getProgress(migrationId, hub, phase string) *MigrationProgress {
	status := getMigrationStatus(migrationId)
	if status == nil {
		return nil
	}
	key := hubPhaseKey(hub, phase)
	if _, exists := status.Progress[key]; !exists {
		status.Progress[key] = &MigrationProgress{}
	}
	return status.Progress[key]
}

// SetStarted sets the status of the given stage to started for the hub cluster
func SetStarted(migrationId, hub, phase string) {
	mu.RLock()
	defer mu.RUnlock()
	if p := getProgress(migrationId, hub, phase); p != nil {
		p.started = true
	}
}

// SetFinished sets the status of the given stage to finished for the hub cluster
func SetFinished(migrationId, hub, phase string) {
	mu.RLock()
	defer mu.RUnlock()
	if p := getProgress(migrationId, hub, phase); p != nil {
		p.finished = true
	}
}

func SetErrorMessage(migrationId, hub, phase, errMessage string) {
	mu.RLock()
	defer mu.RUnlock()
	if p := getProgress(migrationId, hub, phase); p != nil {
		p.error = errMessage
	}
}

// GetStarted returns true if the status of the given stage is started for the hub cluster
func GetStarted(migrationId, hub, phase string) bool {
	mu.RLock()
	defer mu.RUnlock()
	p := getProgress(migrationId, hub, phase)
	return p.started
}

// GetFinished returns true if the status of the given stage is finished for the hub cluster
func GetFinished(migrationId, hub, phase string) bool {
	mu.RLock()
	defer mu.RUnlock()
	p := getProgress(migrationId, hub, phase)
	return p.finished
}

func GetErrorMessage(migrationId, hub, phase string) string {
	mu.RLock()
	defer mu.RUnlock()
	p := getProgress(migrationId, hub, phase)

	return p.error
}

package migration

import (
	"reflect"
)

type MigrationPhaseStatus struct {
	started  bool
	finished bool
	error    string
}

type MigrationPhases struct {
	Validating, Initializing, Deploying, Registering, Cleaning MigrationPhaseStatus
}

type MigrationEventProgress map[string]*MigrationPhases

// Initialize the global variable
var MigrationEventProgressMap = make(map[string]*MigrationEventProgress)

// updatePhaseStatus updates the status (started/finished) in the source or target cluster
func updatePhaseStatus(migrationId, hubCluster, phase string,
	updateFunc func(*MigrationPhaseStatus),
) {
	mep := MigrationEventProgressMap[migrationId]
	if mep == nil {
		log.Warnf("MigrationEventProgress is nil for migrationId: %s", migrationId)
		return
	}
	mp := (*mep)[hubCluster]
	if mp == nil {
		mp = &MigrationPhases{}
		(*mep)[hubCluster] = mp
	}

	v := reflect.ValueOf(mp).Elem()
	field := v.FieldByName(phase)
	if field.IsValid() && field.CanSet() {
		status, ok := field.Interface().(MigrationPhaseStatus)
		if ok {
			updateFunc(&status)
			field.Set(reflect.ValueOf(status))
		}
	}
}

// SetStarted sets the status of the given stage to started for the hub cluster
func SetStarted(migrationId, hubCluster, phase string) {
	updatePhaseStatus(migrationId, hubCluster, phase, func(status *MigrationPhaseStatus) {
		status.started = true
	})
}

// SetFinished sets the status of the given stage to finished for the hub cluster
func SetFinished(migrationId, cluster, phase string) {
	updatePhaseStatus(migrationId, cluster, phase, func(status *MigrationPhaseStatus) {
		status.finished = true
	})
}

func SetErrorMessage(migrationId, hubCluster, phase, errMessage string) {
	updatePhaseStatus(migrationId, hubCluster, phase, func(status *MigrationPhaseStatus) {
		status.error = errMessage
	})
}

// getPhaseStatus extracts the MigrationPhaseStatus for a given phase
func getPhaseStatus(mp *MigrationPhases, phase string) (MigrationPhaseStatus, bool) {
	if mp == nil {
		return MigrationPhaseStatus{}, false
	}

	v := reflect.ValueOf(mp).Elem()
	field := v.FieldByName(phase)
	if !field.IsValid() {
		return MigrationPhaseStatus{}, false
	}

	status, ok := field.Interface().(MigrationPhaseStatus)
	if !ok {
		return MigrationPhaseStatus{}, false
	}

	return status, true
}

// GetStarted returns true if the status of the given stage is started for the hub cluster
func GetStarted(migrationId, hubCluster, phase string) bool {
	mep := MigrationEventProgressMap[migrationId]
	if mep == nil {
		log.Warnf("MigrationEventProgress is nil for migrationId: %s", migrationId)
		return false
	}
	mp := (*mep)[hubCluster]
	status, ok := getPhaseStatus(mp, phase)
	if !ok {
		return false
	}
	return status.started
}

// GetFinished returns true if the status of the given stage is finished for the hub cluster
func GetFinished(migrationId, hubCluster, phase string) bool {
	mep := MigrationEventProgressMap[migrationId]
	mp := (*mep)[hubCluster]
	status, ok := getPhaseStatus(mp, phase)
	if !ok {
		return false
	}
	return status.finished
}

func GetErrorMessage(migrationId, hubCluster, phase string) string {
	mep := MigrationEventProgressMap[migrationId]
	mp := (*mep)[hubCluster]
	status, ok := getPhaseStatus(mp, phase)
	if !ok {
		return ""
	}
	return status.error
}

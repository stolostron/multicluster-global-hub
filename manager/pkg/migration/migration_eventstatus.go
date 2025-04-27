package migration

import (
	"reflect"
)

type MigrationPhaseStatus struct {
	started  bool
	finished bool
	error    string
}

type MigrationPhase struct {
	Validating, Initializing, Deploying, Registering, Cleaning MigrationPhaseStatus
}

type MigrationEventProgress struct {
	sourceStatuses map[string]*MigrationPhase
	targetStatuses map[string]*MigrationPhase
}

// getPhaseStatus extracts the MigrationPhaseStatus for a given phase
func getPhaseStatus(mp *MigrationPhase, phase string) (MigrationPhaseStatus, bool) {
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

// IsSourceStarted returns true if the status of the given stage is started in the source cluster
func IsSourceStarted(mep *MigrationEventProgress, phase, cluster string) bool {
	mp := mep.sourceStatuses[cluster]
	status, ok := getPhaseStatus(mp, phase)
	if !ok {
		return false
	}
	return status.started
}

// IsSourceFinished returns true if the status of the given stage is finished in the source cluster
func IsSourceFinished(mep *MigrationEventProgress, phase, cluster string) bool {
	mp := mep.sourceStatuses[cluster]
	status, ok := getPhaseStatus(mp, phase)
	if !ok {
		return false
	}
	return status.finished
}

// IsTargetStarted returns true if the status of the given stage is started in the target cluster
func IsTargetStarted(mep *MigrationEventProgress, phase, cluster string) bool {
	mp := mep.targetStatuses[cluster]
	status, ok := getPhaseStatus(mp, phase)
	if !ok {
		return false
	}
	return status.started
}

// IsTargetFinished returns true if the status of the given stage is finished in the target cluster
func IsTargetFinished(mep *MigrationEventProgress, phase, cluster string) bool {
	mp := mep.targetStatuses[cluster]
	status, ok := getPhaseStatus(mp, phase)
	if !ok {
		return false
	}
	return status.finished
}

// updatePhaseStatus updates the status (started/finished) in the source or target cluster
func updatePhaseStatus(phases map[string]*MigrationPhase, cluster, phase string,
	updateFunc func(*MigrationPhaseStatus),
) {
	mp := phases[cluster]
	if mp == nil {
		mp = &MigrationPhase{}
		phases[cluster] = mp
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

// SetSourceStarted sets the status of the given stage to started in the source cluster
func SetSourceStarted(mep *MigrationEventProgress, phase, cluster string) {
	updatePhaseStatus(mep.sourceStatuses, cluster, phase, func(status *MigrationPhaseStatus) {
		status.started = true
	})
}

// SetSourceFinished sets the status of the given stage to finished in the source cluster
func SetSourceFinished(mep *MigrationEventProgress, phase, cluster string) {
	updatePhaseStatus(mep.sourceStatuses, cluster, phase, func(status *MigrationPhaseStatus) {
		status.finished = true
	})
}

// SetTargetStarted sets the status of the given stage to started in the target cluster
func SetTargetStarted(mep *MigrationEventProgress, phase, cluster string) {
	updatePhaseStatus(mep.targetStatuses, cluster, phase, func(status *MigrationPhaseStatus) {
		status.started = true
	})
}

// SetTargetFinished sets the status of the given stage to finished in the target cluster
func SetTargetFinished(mep *MigrationEventProgress, phase, cluster string) {
	updatePhaseStatus(mep.targetStatuses, cluster, phase, func(status *MigrationPhaseStatus) {
		status.finished = true
	})
}

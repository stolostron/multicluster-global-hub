package migration

import migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"

type MigrationEventStatus struct {
	Stage      migrationv1alpha1.StageStatus `json:"stage"`
	IsSent     bool                          `json:"isSent"`
	IsReceived bool                          `json:"isReceived"`
	Error      string                        `json:"error,omitempty"` // not used
}

type MigrationEventProgress struct {
	Source_Statuses map[migrationv1alpha1.StageStatus]*MigrationEventStatus `json:"source_statuses"`
	Target_Statuses map[migrationv1alpha1.StageStatus]*MigrationEventStatus `json:"target_statuses"`
	Completed       bool                                                    `json:"completed"`            // not used
	FinalError      string                                                  `json:"finalError,omitempty"` // not used
}

// ReportSentStatus returns true if the status of the given stage is sent
func ReportSentStatus(mep MigrationEventProgress, Stage migrationv1alpha1.StageStatus) bool {
	if mep.Source_Statuses[Stage] == nil {
		return false
	}
	return mep.Source_Statuses[Stage].IsSent
}

// ReportReceivedStatus returns true if the status of the given stage is received
func ReportReceivedStatus(mep MigrationEventProgress, Stage migrationv1alpha1.StageStatus) bool {
	if mep.Target_Statuses[Stage] == nil {
		return false
	}
	return mep.Target_Statuses[Stage].IsReceived
}

// SetSentStatus sets the sent status of the given stage
func SetSentStatus(mep MigrationEventProgress, Stage migrationv1alpha1.StageStatus, isSent bool) {
	if mep.Source_Statuses[Stage] == nil {
		mep.Source_Statuses[Stage] = &MigrationEventStatus{}
	}
	mep.Source_Statuses[Stage].IsSent = isSent
}

// SetReceivedStatus sets the received status of the given stage
func SetReceivedStatus(mep MigrationEventProgress, Stage migrationv1alpha1.StageStatus, isReceived bool) {
	if mep.Target_Statuses[Stage] == nil {
		mep.Target_Statuses[Stage] = &MigrationEventStatus{}
	}
	mep.Target_Statuses[Stage].IsReceived = isReceived
}

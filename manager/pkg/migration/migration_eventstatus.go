package migration

import migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"

type MigrationEventStatus struct {
	Stage  migrationv1alpha1.StageStatus `json:"stage"`
	IsSent bool                          `json:"isSent"`
	Error  string                        `json:"error,omitempty"` // not used
}

type MigrationEventProgress struct {
	ID              string                 `json:"id"` // Migration CR UID
	Source_Statuses []MigrationEventStatus `json:"source_statuses"`
	Target_Statuses []MigrationEventStatus `json:"target_statuses"`
	Completed       bool                   `json:"completed"`            // not used
	FinalError      string                 `json:"finalError,omitempty"` // not used
}

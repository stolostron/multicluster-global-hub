package migration

import (
	"testing"

	"github.com/stretchr/testify/assert"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
)

func TestSetReceivedStatus(t *testing.T) {
	tests := []struct {
		name          string
		initialStatus map[migrationv1alpha1.StageStatus]*MigrationEventStatus
		stage         migrationv1alpha1.StageStatus
		isReceived    bool
		expected      bool
	}{
		{
			name:          "Set received status for a new stage",
			initialStatus: map[migrationv1alpha1.StageStatus]*MigrationEventStatus{},
			stage:         migrationv1alpha1.StageStatus("Stage1"),
			isReceived:    true,
			expected:      true,
		},
		{
			name: "Update received status for an existing stage",
			initialStatus: map[migrationv1alpha1.StageStatus]*MigrationEventStatus{
				migrationv1alpha1.StageStatus("Stage1"): {IsReceived: false},
			},
			stage:      migrationv1alpha1.StageStatus("Stage1"),
			isReceived: true,
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mep := MigrationEventProgress{
				Target_Statuses: tt.initialStatus,
			}

			SetReceivedStatus(mep, tt.stage, tt.isReceived)

			assert.NotNil(t, mep.Target_Statuses[tt.stage])
			assert.Equal(t, tt.expected, mep.Target_Statuses[tt.stage].IsReceived)
		})
	}
}

func TestSetSentStatus(t *testing.T) {
	tests := []struct {
		name          string
		initialStatus map[migrationv1alpha1.StageStatus]*MigrationEventStatus
		stage         migrationv1alpha1.StageStatus
		isSent        bool
		expected      bool
	}{
		{
			name:          "Set sent status for a new stage",
			initialStatus: map[migrationv1alpha1.StageStatus]*MigrationEventStatus{},
			stage:         migrationv1alpha1.StageStatus("Stage1"),
			isSent:        true,
			expected:      true,
		},
		{
			name: "Update sent status for an existing stage",
			initialStatus: map[migrationv1alpha1.StageStatus]*MigrationEventStatus{
				migrationv1alpha1.StageStatus("Stage1"): {IsSent: false},
			},
			stage:    migrationv1alpha1.StageStatus("Stage1"),
			isSent:   true,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mep := MigrationEventProgress{
				Source_Statuses: tt.initialStatus,
			}

			SetSentStatus(mep, tt.stage, tt.isSent)

			assert.NotNil(t, mep.Source_Statuses[tt.stage])
			assert.Equal(t, tt.expected, mep.Source_Statuses[tt.stage].IsSent)
		})
	}
}

func TestReportReceivedStatus(t *testing.T) {
	tests := []struct {
		name          string
		initialStatus map[migrationv1alpha1.StageStatus]*MigrationEventStatus
		stage         migrationv1alpha1.StageStatus
		expected      bool
	}{
		{
			name:          "Report received status for a non-existent stage",
			initialStatus: map[migrationv1alpha1.StageStatus]*MigrationEventStatus{},
			stage:         migrationv1alpha1.StageStatus("Stage1"),
			expected:      false,
		},
		{
			name: "Report received status for an existing stage with IsReceived set to true",
			initialStatus: map[migrationv1alpha1.StageStatus]*MigrationEventStatus{
				migrationv1alpha1.StageStatus("Stage1"): {IsReceived: true},
			},
			stage:    migrationv1alpha1.StageStatus("Stage1"),
			expected: true,
		},
		{
			name: "Report received status for an existing stage with IsReceived set to false",
			initialStatus: map[migrationv1alpha1.StageStatus]*MigrationEventStatus{
				migrationv1alpha1.StageStatus("Stage1"): {IsReceived: false},
			},
			stage:    migrationv1alpha1.StageStatus("Stage1"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mep := MigrationEventProgress{
				Target_Statuses: tt.initialStatus,
			}

			result := ReportReceivedStatus(mep, tt.stage)

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestReportSentStatus(t *testing.T) {
	tests := []struct {
		name          string
		initialStatus map[migrationv1alpha1.StageStatus]*MigrationEventStatus
		stage         migrationv1alpha1.StageStatus
		expected      bool
	}{
		{
			name:          "Report sent status for a non-existent stage",
			initialStatus: map[migrationv1alpha1.StageStatus]*MigrationEventStatus{},
			stage:         migrationv1alpha1.StageStatus("Stage1"),
			expected:      false,
		},
		{
			name: "Report sent status for an existing stage with IsSent set to true",
			initialStatus: map[migrationv1alpha1.StageStatus]*MigrationEventStatus{
				migrationv1alpha1.StageStatus("Stage1"): {IsSent: true},
			},
			stage:    migrationv1alpha1.StageStatus("Stage1"),
			expected: true,
		},
		{
			name: "Report sent status for an existing stage with IsSent set to false",
			initialStatus: map[migrationv1alpha1.StageStatus]*MigrationEventStatus{
				migrationv1alpha1.StageStatus("Stage1"): {IsSent: false},
			},
			stage:    migrationv1alpha1.StageStatus("Stage1"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mep := MigrationEventProgress{
				Source_Statuses: tt.initialStatus,
			}

			result := ReportSentStatus(mep, tt.stage)

			assert.Equal(t, tt.expected, result)
		})
	}
}

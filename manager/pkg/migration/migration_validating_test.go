package migration

import (
	"strings"
	"testing"
)

// go test -run ^TestIsValidResource$ github.com/stolostron/multicluster-global-hub/manager/pkg/migration -v
func TestIsValidResource(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErr  bool
		errorMsg string
	}{
		{"valid configmap", "configmap/default/my-config", false, ""},
		{"valid secret with wildcard", "secret/ns1/*", true, "invalid name"},
		{"invalid format", "configmap/default", true, "invalid format"},
		{"unsupported kind", "pod/ns1/name1", true, "unsupported kind"},
		{"invalid namespace", "configmap/Inv@lid/ns", true, "invalid namespace"},
		{"invalid name", "secret/default/Invalid*", true, "invalid name"},
		{"invalid wildcard placement", "secret/ns1/na*me", true, "invalid name"},
		{"empty name with wildcard char", "secret/ns1/*x", true, "invalid name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsValidResource(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("expected error=%v, got error=%v", tt.wantErr, err)
			}
			if tt.wantErr && err != nil && !contains(err.Error(), tt.errorMsg) {
				t.Errorf("expected error message to contain '%s', got '%s'", tt.errorMsg, err.Error())
			}
		})
	}
}

// helper for substring match
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

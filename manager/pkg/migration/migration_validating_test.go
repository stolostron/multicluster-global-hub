package migration

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// go test -run ^TestIsValidResource$ ./manager/pkg/migration -v
func TestIsValidResource(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		errorMsg string
	}{
		{"valid configmap within dot", "configmap/default/kube-root-ca.crt", ""},
		{"invalid configmap prefix dot", "configmap/default/.kube-root-ca.crt ", "invalid name: .kube-root-ca.crt"},
		{"valid configmap", "configmap/default/my-config", ""},
		{"valid secret with wildcard", "secret/ns1/*", "invalid name: *"},
		{"invalid format", "configmap/default", "invalid format (must be kind/namespace/name): configmap/default"},
		{"unsupported kind", "pod/ns1/name1", "unsupported kind: pod"},
		{"invalid namespace", "configmap/Inv@lid/ns", "invalid namespace: Inv@lid"},
		{"invalid name", "secret/default/Invalid*", "invalid name: Invalid*"},
		{"invalid wildcard placement", "secret/ns1/na*me", "invalid name: na*me"},
		{"empty name with wildcard char", "secret/ns1/*x", "invalid name: *x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsValidResource(tt.input)
			actualErrMessage := ""
			if err != nil {
				actualErrMessage = err.Error()
			}
			require.Equal(t, strings.TrimSpace(tt.errorMsg), strings.TrimSpace(actualErrMessage), tt.name)
		})
	}
}

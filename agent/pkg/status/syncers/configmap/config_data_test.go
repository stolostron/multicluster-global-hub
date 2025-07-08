package configmap

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

func TestGetSyncInterval_Defaults(t *testing.T) {
	assert := assert.New(t)
	// Should return default for unknown type
	unknownType := enum.EventType("unknown")
	assert.Equal(5*time.Second, GetSyncInterval(unknownType))
}

func TestGetSyncInterval_KnownTypes(t *testing.T) {
	assert := assert.New(t)
	assert.Equal(5*time.Second, GetSyncInterval(enum.ManagedClusterType))
	assert.Equal(5*time.Second, GetSyncInterval(enum.LocalPolicySpecType))
	assert.Equal(60*time.Second, GetSyncInterval(enum.HubClusterInfoType))
	assert.Equal(60*time.Second, GetSyncInterval(enum.HubClusterHeartbeatType))
	assert.Equal(5*time.Second, GetSyncInterval(enum.ManagedClusterEventType))
}

func TestSetInterval(t *testing.T) {
	assert := assert.New(t)
	SetInterval(GetSyncKey(enum.ManagedClusterType), 42*time.Second)
	assert.Equal(42*time.Second, GetSyncInterval(enum.ManagedClusterType))
}

func TestGetResyncInterval_Defaults(t *testing.T) {
	assert := assert.New(t)
	unknownType := enum.EventType("unknown")
	assert.Equal(6*time.Hour, GetResyncInterval(unknownType))
}

func TestGetResyncInterval_KnownTypes(t *testing.T) {
	assert := assert.New(t)
	assert.Equal(6*time.Hour, GetResyncInterval(enum.ManagedClusterType))
	assert.Equal(6*time.Hour, GetResyncInterval(enum.LocalPolicySpecType))
}

func TestSetResyncInterval(t *testing.T) {
	assert := assert.New(t)
	SetResyncInterval(GetResyncKey(enum.ManagedClusterType), 2*time.Hour)
	assert.Equal(2*time.Hour, GetResyncInterval(enum.ManagedClusterType))
}

func TestAgentConfigs(t *testing.T) {
	assert := assert.New(t)
	assert.Equal(AggregationFull, GetAggregationLevel())
	assert.Equal(EnableLocalPolicyTrue, GetEnableLocalPolicy())
}

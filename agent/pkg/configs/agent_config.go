package configs

import (
	"sync"
	"time"

	commonobjects "github.com/stolostron/multicluster-global-hub/pkg/objects"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var agentConfigData *AgentConfig

type AgentConfig struct {
	LeafHubName                  string
	PodNamespace                 string
	SpecWorkPoolSize             int
	SpecEnforceHohRbac           bool
	StatusDeltaCountSwitchFactor int
	TransportConfig              *transport.TransportInternalConfig
	TransportConfigSecretName    string
	ElectionConfig               *commonobjects.LeaderElectionConfig
	MetricsAddress               string
	QPS                          float32
	Burst                        int
	EnablePprof                  bool
	DeployMode                   string
	EnableStackroxIntegration    bool
	StackroxPollInterval         time.Duration
	EventMode                    string
	// HubRole indicates the role of this hub in Hub HA setup: "active", "standby", or empty
	// Access must be synchronized using hubRoleMu
	HubRole string
	// StandbyHub is the standby hub name (only populated for active hubs)
	// Access must be synchronized using hubRoleMu
	StandbyHub string
	// hubRoleMu protects concurrent access to HubRole and StandbyHub fields
	hubRoleMu sync.RWMutex
}

func SetAgentConfig(agentConfig *AgentConfig) {
	agentConfigData = agentConfig
}

func GetAgentConfig() *AgentConfig {
	return agentConfigData
}

func GetLeafHubName() string {
	return agentConfigData.LeafHubName
}

// GetHubRole safely retrieves the hub role with proper locking
func (c *AgentConfig) GetHubRole() string {
	if c == nil {
		return ""
	}
	c.hubRoleMu.RLock()
	defer c.hubRoleMu.RUnlock()
	return c.HubRole
}

// GetStandbyHub safely retrieves the standby hub name with proper locking
func (c *AgentConfig) GetStandbyHub() string {
	if c == nil {
		return ""
	}
	c.hubRoleMu.RLock()
	defer c.hubRoleMu.RUnlock()
	return c.StandbyHub
}

// SetHubRole safely sets the hub role with proper locking
func (c *AgentConfig) SetHubRole(role string) {
	if c == nil {
		return
	}
	c.hubRoleMu.Lock()
	defer c.hubRoleMu.Unlock()
	c.HubRole = role
}

// SetStandbyHub safely sets the standby hub name with proper locking
func (c *AgentConfig) SetStandbyHub(hub string) {
	if c == nil {
		return
	}
	c.hubRoleMu.Lock()
	defer c.hubRoleMu.Unlock()
	c.StandbyHub = hub
}

// GetHubRoleAndStandbyHub safely retrieves both values with a single lock to ensure consistency
func (c *AgentConfig) GetHubRoleAndStandbyHub() (role, standbyHub string) {
	if c == nil {
		return "", ""
	}
	c.hubRoleMu.RLock()
	defer c.hubRoleMu.RUnlock()
	return c.HubRole, c.StandbyHub
}

// UpdateHubRoleAndStandbyHub safely updates both values with a single lock and returns previous values
func (c *AgentConfig) UpdateHubRoleAndStandbyHub(role, standbyHub string) (previousRole, previousStandbyHub string) {
	if c == nil {
		return "", ""
	}
	c.hubRoleMu.Lock()
	defer c.hubRoleMu.Unlock()
	previousRole = c.HubRole
	previousStandbyHub = c.StandbyHub
	c.HubRole = role
	c.StandbyHub = standbyHub
	return previousRole, previousStandbyHub
}

var mchVersion string

func GetMCHVersion() string {
	return mchVersion
}

func SetMCHVersion(version string) {
	mchVersion = version
}

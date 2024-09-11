package config

import (
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
	TransportConfig              *transport.TransportConfig
	ElectionConfig               *commonobjects.LeaderElectionConfig
	Terminating                  bool
	MetricsAddress               string
	EnableGlobalResource         bool
	QPS                          float32
	Burst                        int
	EnablePprof                  bool
	Standalone                   bool
}

func SetAgentConfig(agentConfig *AgentConfig) {
	agentConfigData = agentConfig
}

func GetAgentConfig() *AgentConfig {
	return agentConfigData
}

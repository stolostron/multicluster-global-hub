package configs

import (
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

var mchVersion string

func GetMCHVersion() string {
	return mchVersion
}

func SetMCHVersion(version string) {
	mchVersion = version
}

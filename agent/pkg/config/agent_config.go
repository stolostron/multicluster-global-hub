package config

import (
	commonobjects "github.com/stolostron/multicluster-global-hub/pkg/objects"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

type AgentConfig struct {
	LeafHubName                  string
	PodNameSpace                 string
	SpecWorkPoolSize             int
	SpecEnforceHohRbac           bool
	StatusDeltaCountSwitchFactor int
	TransportConfig              *transport.TransportConfig
	ElectionConfig               *commonobjects.LeaderElectionConfig
	Terminating                  bool
	KubeEventExporterConfigPath  string
	MetricsAddress               string
}

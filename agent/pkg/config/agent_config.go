package config

import (
	"time"

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
	MetricsAddress               string
	EnableGlobalResource         bool
	QPS                          float32
	Burst                        int
	EnablePprof                  bool
	EnableStackroxIntegration    bool
	ACSPollInterval              time.Duration
}

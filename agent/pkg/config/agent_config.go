package config

import (
	commonobjects "github.com/stolostron/multicluster-global-hub/pkg/objects"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const (
	// Environment
	KAFKA_BOOTSTRAP_SERVERS = "KAFKA_BOOTSTRAP_SERVERS"
	POD_NAMESPACE           = "POD_NAMESPACE"
	STATUS_TOPIC            = "STATUS_TOPIC"
	EVENT_TOPIC             = "EVENT_TOPIC"
	// Leader Election
	AGETN_ELECTION_ID    = "multicluster-global-hub-agent-lock"
	EXPORTER_ELECTION_ID = "multicluster-global-hub-exporter-lock"
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
	Standalone                   bool
}

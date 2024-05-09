package config

type OperatorConfig struct {
	MetricsAddress        string
	ProbeAddress          string
	PodNamespace          string
	LeaderElection        bool
	GlobalResourceEnabled bool
	EnablePprof           bool
	LogLevel              string
}

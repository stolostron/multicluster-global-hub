package cluster

type HubClusterInfo struct {
	ConsoleURL string `json:"consoleURL"`
	GrafanaURL string `json:"grafanaURL"`
	MchVersion string `json:"mchVersion"`
	ClusterId  string `json:"clusterId"`
}

type HubClusterInfoBundle *HubClusterInfo

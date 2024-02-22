package cluster

type HubClusterInfo struct {
	ConsoleURL string `json:"consoleURL"`
	GrafanaURL string `json:"grafanaURL"`
	ClusterId  string `json:"clusterId"`
}

type HubClusterInfoData *HubClusterInfo

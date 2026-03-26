package haconfig

import corev1 "k8s.io/api/core/v1"

type HAConfigBundle struct {
	ActiveHubName   string         `json:"activeHubName"`
	StandbyHubName  string         `json:"standbyHubName"`
	BootstrapSecret *corev1.Secret `json:"bootstrapSecret"`
}

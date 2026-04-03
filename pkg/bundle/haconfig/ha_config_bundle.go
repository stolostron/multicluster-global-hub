package haconfig

import corev1 "k8s.io/api/core/v1"

type HAConfigBundle struct {
	BootstrapSecret *corev1.Secret `json:"bootstrapSecret"`
}

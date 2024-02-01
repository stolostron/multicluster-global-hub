package event

import (
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	corev1 "k8s.io/api/core/v1"
)

type RootPolicyEventBundle struct {
	Objects       []*corev1.Event         `json:"objects"`
	LeafHubName   string                  `json:"leafHubName"`
	BundleVersion *metadata.BundleVersion `json:"bundleVersion"`
}

type ReplicatedPolicyEventBundle struct {
}

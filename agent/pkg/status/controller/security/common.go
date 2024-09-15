package security

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var centralCRGVK = schema.GroupVersionKind{
	Group:   "platform.stackrox.io",
	Version: "v1alpha1",
	Kind:    "Central",
}

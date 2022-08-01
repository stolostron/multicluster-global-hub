package deployer

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Deployer is the interface for the kubernetes resource deployer
type Deployer interface {
	Deploy(unsObj *unstructured.Unstructured) error
}

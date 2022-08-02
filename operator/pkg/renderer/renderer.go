package renderer

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// GetConfigValuesFunc is function type that returns the configuration values for the given component
type GetConfigValuesFunc func(component string) (interface{}, error)

// GetClusterConfigValuesFunc is function type that returns the configuration values for the given component of given cluster
type GetClusterConfigValuesFunc func(cluster, component string) (interface{}, error)

// Renderer is the interface for the template renderer
type Renderer interface {
	Render(component string, getConfigValuesFunc GetConfigValuesFunc) ([]*unstructured.Unstructured, error)
	RenderWithFilter(component, filter string, getConfigValuesFunc GetConfigValuesFunc) ([]*unstructured.Unstructured, error)
	RenderForCluster(cluster, component string, getClusterConfigValuesFunc GetClusterConfigValuesFunc) ([]*unstructured.Unstructured, error)
	RenderForClusterWithFilter(cluster, component, filter string, getClusterConfigValuesFunc GetClusterConfigValuesFunc) ([]*unstructured.Unstructured, error)
}

package renderer

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// GetConfigValuesFunc is function type that returns the configuration values for the given profile
type GetConfigValuesFunc func(profile string) (interface{}, error)

// Renderer is the interface for the template renderer
type Renderer interface {
	Render(component, profile string, getConfigValuesFunc GetConfigValuesFunc) ([]*unstructured.Unstructured, error)
	RenderWithFilter(component, profile, filterOut string, getConfigValuesFunc GetConfigValuesFunc) (
		[]*unstructured.Unstructured, error)
}

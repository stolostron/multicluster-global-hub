package main

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestSource(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "operator main Suite")
}

var (
	cfg        *rest.Config
	kubeClient kubernetes.Interface
)

var mghJson = `
	{
		"apiVersion": "operator.open-cluster-management.io/v1alpha4",
		"kind":       "MulticlusterGlobalHub",
		"metadata": {
			"name":      "testmgh",
			"namespace": "default"
		},
		"status": {
			"conditions": [
				{
					"lastTransitionTime": "2023-10-18T00:33:39Z",
					"message": "ready",
					"reason": "ready",
					"status": "True",
					"type": "Ready"
				}
			]
		}
	}
`

var resource = schema.GroupVersionResource{
	Group:    "operator.open-cluster-management.io",
	Version:  "v1alpha4",
	Resource: "multiclusterglobalhubs",
}

var _ = BeforeSuite(func() {
	Expect(os.Setenv("POD_NAMESPACE", "default")).NotTo(HaveOccurred())
})

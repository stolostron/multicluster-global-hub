// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package crd

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
)

var (
	cfg           *rest.Config
	ctx           context.Context
	cancel        context.CancelFunc
	kubeClient    *kubernetes.Clientset
	dynamicClient *dynamic.DynamicClient
)

func TestMain(m *testing.M) {
	ctx, cancel = context.WithCancel(context.Background())

	err := os.Setenv("POD_NAMESPACE", "default")
	if err != nil {
		panic(err)
	}
	// start testenv
	testenv := &envtest.Environment{
		ErrorIfCRDPathMissing: true,
	}

	cfg, err = testenv.Start()
	if err != nil {
		panic(err)
	}

	if cfg == nil {
		panic(fmt.Errorf("empty kubeconfig!"))
	}

	kubeClient, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}

	dynamicClient, err = dynamic.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}

	// run testings
	code := m.Run()

	// stop testenv
	if err := testenv.Stop(); err != nil {
		panic(err)
	}
	os.Exit(code)
}

func TestCRDCtr(t *testing.T) {
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Metrics: metricsserver.Options{
			BindAddress: "0", // disable the metrics serving
		}, Scheme: scheme.Scheme,
	})
	assert.Nil(t, err)

	controller, err := AddCRDController(mgr, &config.OperatorConfig{}, nil)
	assert.Nil(t, err)

	go func() {
		err := mgr.Start(ctx)
		assert.Nil(t, err)
	}()
	assert.True(t, mgr.GetCache().WaitForCacheSync(ctx))

	clusterResource := "managedclusters.cluster.open-cluster-management.io"
	clusterResourceFile := "0000_00_cluster.open-cluster-management.io_managedclusters.crd.yaml"

	for _, ok := range controller.resources {
		assert.False(t, ok)
	}

	err = applyYaml(filepath.Join("..", "..", "..", "..", "test", "manifest", "crd", clusterResourceFile))
	assert.Nil(t, err)
	time.Sleep(1 * time.Second)

	for resource, ok := range controller.resources {
		if resource == clusterResource {
			assert.True(t, ok)
		} else {
			assert.False(t, ok)
		}
	}
	cancel()
}

func applyYaml(file string) error {
	b, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}
	decoder := yamlutil.NewYAMLOrJSONDecoder(bytes.NewReader(b), 100)
	var rawObj runtime.RawExtension
	if err = decoder.Decode(&rawObj); err != nil {
		return err
	}
	obj, gvk, err := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme).Decode(rawObj.Raw, nil, nil)
	if err != nil {
		return err
	}
	unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		log.Fatal(err)
	}

	unstructuredObj := &unstructured.Unstructured{Object: unstructuredMap}

	gr, err := restmapper.GetAPIGroupResources(kubeClient.Discovery())
	if err != nil {
		return err
	}

	mapper := restmapper.NewDiscoveryRESTMapper(gr)
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return err
	}

	var dri dynamic.ResourceInterface
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		if unstructuredObj.GetNamespace() == "" {
			unstructuredObj.SetNamespace("default")
		}
		dri = dynamicClient.Resource(mapping.Resource).Namespace(unstructuredObj.GetNamespace())
	} else {
		dri = dynamicClient.Resource(mapping.Resource)
	}

	_, err = dri.Create(ctx, unstructuredObj, metav1.CreateOptions{})
	return err
}

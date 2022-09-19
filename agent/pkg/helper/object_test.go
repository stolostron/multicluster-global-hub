package helper

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	clustersV1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	config        *rest.Config
	runtimeClient client.Client
)

func TestMain(m *testing.M) {
	t := &envtest.Environment{
		ErrorIfCRDPathMissing: true,
		CRDDirectoryPaths:     []string{filepath.Join("..", "controllers", "testdata", "crd")},
	}

	var err error
	config, err = t.Start()
	if err != nil {
		panic(err)
	}

	if err := clustersV1beta1.AddToScheme(scheme.Scheme); err != nil {
		panic(err)
	}
	runtimeClient, err = client.New(config, client.Options{})
	if err != nil {
		panic(err)
	}

	code := m.Run()
	t.Stop()
	os.Exit(code)
}

func TestUpdateObject(t *testing.T) {
	mclSetBindingJson := "{\"apiVersion\":\"cluster.open-cluster-management.io/v1beta1\",\"kind\":\"ManagedClusterSetBinding\",\"metadata\":{\"annotations\":{\"global-hub.open-cluster-management.io/origin-ownerreference-uid\":\"bd957348-d27a-49a4-9218-89d9016338f6\"},\"creationTimestamp\":\"2022-09-19T02:39:42Z\",\"name\":\"default\",\"namespace\":\"default\",\"selfLink\":\"/apis/cluster.open-cluster-management.io/v1beta1/namespaces/default/managedclustersetbindings/default\"},\"spec\":{\"clusterSet\":\"default\"},\"status\":{\"conditions\":null}}\n"

	obj := &unstructured.Unstructured{}
	if err := json.Unmarshal([]byte(mclSetBindingJson), obj); err != nil {
		panic(err)
	}
	if err := UpdateObject(context.Background(), runtimeClient, obj); err != nil {
		panic(err)
	}
}

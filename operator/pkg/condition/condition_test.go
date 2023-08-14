// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package condition

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	operatorv1alpha3 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha3"
)

var (
	cfg           *rest.Config
	runtimeClient client.Client
	ctx           context.Context
)

func TestMain(m *testing.M) {
	ctx = context.TODO()
	// start testenv
	testenv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "pkg", "testdata", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testenv.Start()
	if err != nil {
		panic(err)
	}

	if cfg == nil {
		panic(fmt.Errorf("empty kubeconfig!"))
	}

	scheme := runtime.NewScheme()
	err = operatorv1alpha3.AddToScheme(scheme)
	if err != nil {
		panic(err)
	}

	runtimeClient, err = client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		panic(err)
	}

	// run testings
	code := m.Run()

	// stop testenv
	err = testenv.Stop()
	if err != nil {
		panic(err)
	}
	os.Exit(code)
}

func TestCondition(t *testing.T) {
	mgh := &operatorv1alpha3.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: operatorv1alpha3.MulticlusterGlobalHubSpec{
			DataLayer: &operatorv1alpha3.DataLayerConfig{
				Type:       operatorv1alpha3.LargeScale,
				LargeScale: &operatorv1alpha3.LargeScaleConfig{},
			},
		},
	}

	err := runtimeClient.Create(ctx, mgh)
	assert.NoError(t, err)

	t.Logf("testing condition %s with SetCondition", CONDITION_TYPE_RETENTION_PARSED)
	err = SetCondition(ctx, runtimeClient, mgh, CONDITION_TYPE_RETENTION_PARSED,
		metav1.ConditionStatus(CONDITION_STATUS_TRUE), "reason", "message")
	assert.NoError(t, err)

	err = runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)
	assert.NoError(t, err)
	assert.True(t, ContainConditionMessage(mgh, CONDITION_TYPE_RETENTION_PARSED, "message"))
	assert.True(t, ContainConditionStatusReason(mgh, CONDITION_TYPE_RETENTION_PARSED, "reason", CONDITION_STATUS_TRUE))

	statusTestCases := []struct {
		name          string
		conditionType string
		setFunc       SetConditionFunc
	}{
		{"database initialization", CONDITION_TYPE_DATABASE_INIT, SetConditionDatabaseInit},
		{"transport initialization", CONDITION_TYPE_TRANSPORT_INIT, SetConditionTransportInit},
		{"manager deployment", CONDITION_TYPE_MANAGER_AVAILABLE, SetConditionManagerAvailable},
		{"grafana deployment", CONDITION_TYPE_GRAFANA_AVAILABLE, SetConditionGrafanaAvailable},
	}
	for _, tc := range statusTestCases {
		t.Logf("testing condition %s ", tc.name)

		// set the condition to true
		tc.setFunc(ctx, runtimeClient, mgh, CONDITION_STATUS_TRUE)

		// check the condition status
		err := runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)
		assert.NoError(t, err)
		assert.Equal(t, CONDITION_STATUS_TRUE, string(
			GetConditionStatus(mgh, tc.conditionType)))
	}

	t.Logf("testing condition %s ", CONDITION_TYPE_LEAFHUB_DEPLOY)
	err = SetConditionLeafHubDeployed(ctx, runtimeClient, mgh, "test",
		metav1.ConditionStatus(CONDITION_STATUS_TRUE))
	assert.NoError(t, err)

	err = runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)
	assert.NoError(t, err)
	assert.Equal(t, CONDITION_STATUS_TRUE, string(
		GetConditionStatus(mgh, CONDITION_TYPE_LEAFHUB_DEPLOY)))
}

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

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
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
	err = globalhubv1alpha4.AddToScheme(scheme)
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
	mgh := &globalhubv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{
			DataLayer: globalhubv1alpha4.DataLayerConfig{},
		},
	}

	err := runtimeClient.Create(ctx, mgh)
	assert.NoError(t, err)

	statusTestCases := []struct {
		name          string
		conditionType string
		setFunc       SetConditionFunc
	}{
		{"database initialization", CONDITION_TYPE_DATABASE_INIT, SetConditionDatabaseInit},
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

func TestRetentionCondition(t *testing.T) {
	mgh := &globalhubv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-condition",
			Namespace: "default",
		},
		Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{
			DataLayer: globalhubv1alpha4.DataLayerConfig{},
		},
	}
	err := runtimeClient.Create(ctx, mgh)
	assert.NoError(t, err)

	err = SetConditionDataRetention(ctx, runtimeClient, mgh, CONDITION_STATUS_TRUE, "the rentention is set 18 month")
	assert.NoError(t, err)
	err = runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)
	assert.NoError(t, err)
	assert.True(t, ContainConditionMessage(mgh, CONDITION_TYPE_RETENTION_PARSED, "the rentention is set 18 month"))
	assert.Equal(t, CONDITION_STATUS_TRUE, string(mgh.Status.Conditions[0].Status))

	err = SetConditionDataRetention(ctx, runtimeClient, mgh, CONDITION_STATUS_FALSE, "invalid retention 1s")
	assert.NoError(t, err)
	err = runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)
	assert.NoError(t, err)
	assert.True(t, ContainConditionMessage(mgh, CONDITION_TYPE_RETENTION_PARSED, "invalid retention 1s"))
	assert.Equal(t, CONDITION_STATUS_FALSE, string(mgh.Status.Conditions[0].Status))
}

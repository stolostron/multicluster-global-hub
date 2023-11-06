// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package main

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
)

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

func TestMain(m *testing.M) {
	// start testEnv
	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
	}
	var err error
	err = os.Setenv("POD_NAMESPACE", "default")
	if err != nil {
		panic(err)
	}
	cfg, err = testEnv.Start()
	if err != nil {
		panic(err)
	}

	kubeClient, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}
	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}

	obj := unstructured.Unstructured{}
	err = obj.UnmarshalJSON([]byte(mghJson))
	if err != nil {
		panic(err)
	}
	createdMgh, err := dynamicClient.Resource(resource).Namespace("default").
		Create(context.TODO(), &obj, metav1.CreateOptions{})
	if err != nil {
		panic(err)
	}

	obj.SetResourceVersion(createdMgh.GetResourceVersion())

	_, err = dynamicClient.Resource(resource).Namespace("default").UpdateStatus(context.Background(), &obj, metav1.UpdateOptions{})
	if err != nil {
		panic(err)
	}
	// run testings
	code := m.Run()

	// stop testEnv
	err = testEnv.Stop()
	if err != nil {
		panic(err)
	}

	os.Exit(code)
}

func TestOperator(t *testing.T) {
	// the testing manipuates the os.Args to set them up for the testcases
	// after this testing the initial args will be restored
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	cases := []struct {
		name                 string
		args                 []string
		leaderElectionConfig *corev1.ConfigMap
		expectedExit         int
	}{
		{"flag set with leader-election disabled", []string{
			"--leader-election",
			"false",
			"--metrics-bind-address",
			":18080",
			"--health-probe-bind-address",
			":18081",
		}, nil, 0},
		{"flag set with leader-election enabled", []string{
			"--leader-election",
			"--metrics-bind-address",
			":18080",
			"--health-probe-bind-address",
			":18081",
		}, nil, 0},
		{"flag set with customized leader-election configuration", []string{
			"--leader-election",
			"--metrics-bind-address",
			":18080",
			"--health-probe-bind-address",
			":18081",
		}, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operatorconstants.ControllerConfig,
				Namespace: config.GetDefaultNamespace(),
			},
			Data: map[string]string{"leaseDuration": "138", "renewDeadline": "107", "retryPeriod": "26"},
		}, 0},
	}
	for _, tc := range cases {
		// this call is required because otherwise flags panics, if args are set between flag.Parse call
		flag.CommandLine = flag.NewFlagSet(tc.name, flag.ExitOnError)
		pflag.CommandLine = pflag.NewFlagSet(tc.name, pflag.ExitOnError)
		// we need a value to set Args[0] to cause flag begins parsing at Args[1]
		os.Args = append([]string{tc.name}, tc.args...)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if tc.leaderElectionConfig != nil {
			if _, err := kubeClient.CoreV1().ConfigMaps(
				tc.leaderElectionConfig.GetNamespace()).Create(ctx,
				tc.leaderElectionConfig, metav1.CreateOptions{}); err != nil {
				t.Errorf("failed to create leader election configmap: %v", err)
			}
		}
		actualExit := doMain(ctx, cfg)

		if tc.expectedExit != actualExit {
			t.Errorf("unexpected exit code for args: %v, expected: %v, got: %v",
				tc.args, tc.expectedExit, actualExit)
		}
	}
}

// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package main

import (
	"context"
	"os"
	"testing"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	commonconstants "github.com/stolostron/multicluster-global-hub/pkg/constants"
	commonobjects "github.com/stolostron/multicluster-global-hub/pkg/objects"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var cfg *rest.Config

func TestMain(m *testing.M) {
	// start testEnv
	testEnv := &envtest.Environment{}
	var err error
	cfg, err = testEnv.Start()
	if err != nil {
		panic(err)
	}
	m.Run()
	// stop testEnv
	err = testEnv.Stop()
	if err != nil {
		panic(err)
	}
}

func TestParseFlags(t *testing.T) {
	os.Args = []string{
		"operator",
		"--metrics-bind-address",
		":8081",
		"--health-probe-bind-address",
		":8082",
	}
	managerConfig := parseFlags()

	if managerConfig.MetricsAddress != ":8081" {
		t.Fatalf("expect --metrics-bind-address(%s) == %s", managerConfig.MetricsAddress, ":8081")
	}

	if managerConfig.ProbeAddress != ":8082" {
		t.Fatalf("expect --metrics-bind-address(%s) == %s", managerConfig.ProbeAddress, ":8082")
	}
}

func TestLeaderElectionConfig(t *testing.T) {
	// add election config test
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("get kubeClient error %v", err)
	}
	ocmNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: constants.HOHDefaultNamespace}}
	_, err = kubeClient.CoreV1().Namespaces().Create(context.TODO(), ocmNamespace, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create ocm namespace error %v", err)
	}

	// create configmap
	_, err = kubeClient.CoreV1().ConfigMaps(constants.HOHDefaultNamespace).Create(context.TODO(), &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      commonconstants.ControllerLeaderElectionConfig,
			Namespace: constants.HOHDefaultNamespace,
		},
		Data: map[string]string{"leaseDuration": "138", "renewDeadline": "107", "retryPeriod": "26"},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create configmap %v", err)
	}

	// get LeaderElectionConfig instance
	electionConfig, err := getElectionConfig(kubeClient)
	if err != nil {
		t.Fatalf("get election configmap error %v", err)
	}
	if electionConfig.LeaseDuration != 138 {
		t.Fatalf("Expect LeaseDuration == %d", 138)
	}
}

func TestManager(t *testing.T) {
	operatorConfig := &operatorConfig{
		MetricsAddress: ":8081",
		ProbeAddress:   ":8082",
		PodNamespace:   "default",
	}

	electionConfig := &commonobjects.LeaderElectionConfig{
		LeaseDuration: 137,
		RenewDeadline: 107,
		RetryPeriod:   26,
	}

	newCacheFunc := cache.BuilderWithOptions(cache.Options{
		SelectorsByObject: cache.SelectorsByObject{
			&corev1.ConfigMap{}: {
				Label: labelSelector,
			},
		},
	})
	_, err := getManager(operatorConfig, electionConfig, newCacheFunc, cfg)
	if err != nil {
		t.Fatalf("failed to create manager %v", err)
	}
}

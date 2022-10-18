// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package main_test

import (
	"context"
	"os"
	"testing"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	main "github.com/stolostron/multicluster-global-hub/operator"
)

func TestLeaderElectionConfig(t *testing.T) {
	// specify testEnv configuration
	testEnv := &envtest.Environment{}

	configMapName := "leader-election-config"
	os.Setenv("LEADER_ELECTION_CONFIG", configMapName)

	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("start envtest error %v", err)
	}

	// start testEnv
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("get kubeClient error %v", err)
	}
	ocmNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: constants.HOHDefaultNamespace}}
	_, err = kubeClient.CoreV1().Namespaces().Create(context.TODO(), ocmNamespace, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create ocm namespace error %v", err)
	}

	// create configmap
	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: constants.HOHDefaultNamespace,
		},
		Data: map[string]string{"leaseDuration": "123", "renewDeadline": "345", "retryPeriod": "456"},
	}
	kubeClient.CoreV1().ConfigMaps(constants.HOHDefaultNamespace).Create(context.TODO(), cm, metav1.CreateOptions{})

	electionConfig, err := main.GetElectionConfig(kubeClient)
	if err != nil {
		t.Fatalf("get election configmap error %v", err)
	}

	if electionConfig.LeaseDuration != 123 {
		t.Fatalf("Expect LeaseDuration == %d", 123)
	}

	if electionConfig.RenewDeadline != 345 {
		t.Fatalf("Expect RenewDeadline == %d", 345)
	}

	if electionConfig.RetryPeriod != 456 {
		t.Fatalf("Expect RetryPeriod == %d", 456)
	}

	// stop testEnv
	err = testEnv.Stop()
	if err != nil {
		t.Fatalf("stop envtest error %v", err)
	}
}

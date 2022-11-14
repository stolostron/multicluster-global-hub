// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package incarnation_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/incarnation"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

var (
	cfg        *rest.Config
	kubeClient kubernetes.Interface
	mgr        ctrl.Manager
)

func TestMain(m *testing.M) {
	var err error

	// start testenv
	testenv := &envtest.Environment{}

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

	mgr, err = ctrl.NewManager(cfg, ctrl.Options{
		MetricsBindAddress: "0",
		Scheme:             scheme.Scheme,
	})
	if err != nil {
		panic(err)
	}

	if _, err := kubeClient.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: constants.GHSystemNamespace,
		},
	}, metav1.CreateOptions{}); err != nil {
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

func TestIncarnation(t *testing.T) {
	cases := []struct {
		desc                string
		expectedIncarnation uint64
		expectedErr         error
	}{
		{
			"no incarnation configmap",
			0,
			nil,
		},
		{
			"incarnation configmap exists",
			1,
			nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			gotIncarnation, err := incarnation.GetIncarnation(mgr)
			if err != tc.expectedErr || gotIncarnation != tc.expectedIncarnation {
				t.Errorf("%s:\nexpected incarnation & err:\n%+v\n%v\ngot incarnation & err \n%+v\n%v",
					tc.desc, tc.expectedIncarnation, tc.expectedErr, gotIncarnation, err)
			}
		})
	}
}

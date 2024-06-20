// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package main

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var testEnv *envtest.Environment

var _ = Describe("Start Operator Test", Ordered, func() {
	// Initialize the client
	BeforeEach(func() {
		testEnv = &envtest.Environment{
			CRDDirectoryPaths: []string{
				filepath.Join("config", "crd", "bases"),
				filepath.Join("..", "test", "manifest", "crd"),
			},
			ErrorIfCRDPathMissing: true,
		}

		var err error
		cfg, err = testEnv.Start()
		Expect(err).NotTo(HaveOccurred())

		kubeClient, err = kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())

		dynamicClient, err := dynamic.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())

		obj := unstructured.Unstructured{}
		Expect(obj.UnmarshalJSON([]byte(mghJson))).NotTo(HaveOccurred())
		createdMgh, err := dynamicClient.Resource(resource).Namespace("default").
			Create(context.TODO(), &obj, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		obj.SetResourceVersion(createdMgh.GetResourceVersion())
		_, err = dynamicClient.Resource(resource).Namespace("default").UpdateStatus(context.Background(), &obj, metav1.UpdateOptions{})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(testEnv.Stop()).To(Succeed())
	})

	Context("Test Operator: leader-election disabled", func() {
		It("start operator with leader-election disabled", func() {
			// this call is required because otherwise flags panics, if args are set between flag.Parse call
			flag.CommandLine = flag.NewFlagSet("", flag.ExitOnError)
			pflag.CommandLine = pflag.NewFlagSet("", pflag.ExitOnError)

			os.Args = []string{
				"--leader-election",
				"false",
				"--metrics-bind-address",
				":18080",
				"--health-probe-bind-address",
				":18081",
			}

			operatorConfig := parseFlags()
			Expect(operatorConfig.LeaderElection).To(BeFalse())
		})
	})

	Context("Test Operator: leader-election enabled", func() {
		It("start operator with leader-election", func() {
			// this call is required because otherwise flags panics, if args are set between flag.Parse call
			flag.CommandLine = flag.NewFlagSet("", flag.ExitOnError)
			pflag.CommandLine = pflag.NewFlagSet("", pflag.ExitOnError)

			os.Args = []string{
				"--metrics-bind-address",
				":18080",
				"--health-probe-bind-address",
				":18081",
				"--leader-election",
			}

			operatorConfig := parseFlags()
			Expect(operatorConfig.LeaderElection).To(BeTrue())

			electionConfig, err := config.GetElectionConfig()
			Expect(err).NotTo(HaveOccurred())

			Expect(electionConfig.LeaseDuration).To(Equal(137))
			Expect(electionConfig.RenewDeadline).To(Equal(107))
			Expect(electionConfig.RetryPeriod).To(Equal(26))
		})
	})

	Context("Test Operator: customized leader-election configuration", func() {
		It("start operator with customized leader-election configuration", func() {
			// this call is required because otherwise flags panics, if args are set between flag.Parse call
			flag.CommandLine = flag.NewFlagSet("", flag.ExitOnError)
			pflag.CommandLine = pflag.NewFlagSet("", pflag.ExitOnError)

			os.Args = []string{
				"--metrics-bind-address",
				":18080",
				"--health-probe-bind-address",
				":18081",
				"--leader-election",
			}

			operatorConfig := parseFlags()
			// utils.PrettyPrint(operatorConfig)
			Expect(operatorConfig.LeaderElection).To(BeTrue())

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err := kubeClient.CoreV1().ConfigMaps(
				utils.GetDefaultNamespace()).Create(ctx,
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      operatorconstants.ControllerConfig,
						Namespace: utils.GetDefaultNamespace(),
					},
					Data: map[string]string{"leaseDuration": "10", "renewDeadline": "8", "retryPeriod": "2"},
				}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			err = config.LoadControllerConfig(ctx, kubeClient)
			Expect(err).NotTo(HaveOccurred())

			electionConfig, err := config.GetElectionConfig()
			Expect(err).NotTo(HaveOccurred())

			Expect(electionConfig.LeaseDuration).To(Equal(10))
			Expect(electionConfig.RenewDeadline).To(Equal(8))
			Expect(electionConfig.RetryPeriod).To(Equal(2))
		})
	})
})

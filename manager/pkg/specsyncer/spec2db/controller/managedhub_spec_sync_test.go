// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

var _ = Describe("managed hub controller", Ordered, func() {
	It("create/delete the managed hub in kubernetes", func() {
		managedHub := &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "managedhub-1",
				Namespace: config.GetDefaultNamespace(),
				Labels: map[string]string{
					"vendor": "OpenShift",
				},
			},
			Spec: clusterv1.ManagedClusterSpec{},
		}

		By("create managed hub")
		Expect(kubeClient.Create(ctx, managedHub, &client.CreateOptions{})).Should(Succeed())

		Eventually(func() error {
			err := kubeClient.Get(ctx, client.ObjectKeyFromObject(managedHub), managedHub)
			if err != nil {
				return err
			}
			if controllerutil.ContainsFinalizer(managedHub, constants.GlobalHubCleanupFinalizer) {
				return nil
			} else {
				return fmt.Errorf("the finalizer(%s) isn't added to the cluster", constants.GlobalHubCleanupFinalizer)
			}
		}, 30*time.Second, 1*time.Second).Should(Succeed())

		By("delete managed hub")
		Expect(kubeClient.Delete(ctx, managedHub, &client.DeleteOptions{})).Should(Succeed())

		Eventually(func() error {
			err := kubeClient.Get(ctx, client.ObjectKeyFromObject(managedHub), managedHub)
			if err != nil && errors.IsNotFound(err) {
				return nil
			} else if err != nil {
				return err
			}
			return fmt.Errorf("the managed hub should be deleted")
		}, 30*time.Second, 1*time.Second).Should(Succeed())
	})
})

// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controllers_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

var certSecret = &corev1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Name:      constants.KafkaCertSecretName,
		Namespace: constants.GHAgentNamespace,
	},
	Data: map[string][]byte{
		"service.crt": []byte("testcert"),
	},
}

var _ = Describe("cert controllers", Ordered, func() {
	It("create cert secret when cache is null", func() {
		Eventually(func() error {
			return mgr.GetClient().Create(ctx, certSecret)
		}, 1*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	It("update cert secret", func() {
		Eventually(func() error {
			existingCertSecret := &corev1.Secret{}
			err := mgr.GetClient().Get(ctx, types.NamespacedName{
				Namespace: constants.GHAgentNamespace,
				Name:      constants.KafkaCertSecretName,
			}, existingCertSecret)
			if err != nil {
				return err
			}
			updateCertSecret := existingCertSecret.DeepCopy()
			updateCertSecret.Data = map[string][]byte{
				"service.crt": []byte("updatecert"),
			}
			err = mgr.GetClient().Update(ctx, updateCertSecret)
			return err
		}, 1*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})

package controllers

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers"
	testutils "github.com/stolostron/multicluster-global-hub/test/integration/utils"
)

// go test ./test/integration/operator/hubofhubs -ginkgo.focus "init" -v
var _ = Describe("meta", Ordered, func() {
	var mgh *v1alpha4.MulticlusterGlobalHub
	var namespace string
	BeforeAll(func() {
		namespace = fmt.Sprintf("namespace-%s", rand.String(6))
		mghName := "test-mgh"

		Expect(controllers.NewMetaController(runtimeManager,
			kubeClient, operatorConfig, nil).SetupWithManager(runtimeManager)).To(Succeed())

		// mgh
		Expect(runtimeClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		})).To(Succeed())
		mgh = &v1alpha4.MulticlusterGlobalHub{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mghName,
				Namespace: namespace,
			},
			Spec: v1alpha4.MulticlusterGlobalHubSpec{
				EnableMetrics: true,
				DataLayerSpec: v1alpha4.DataLayerSpec{
					Postgres: v1alpha4.PostgresSpec{
						Retention: "2y",
					},
				},
			},
		}
		Expect(runtimeClient.Create(ctx, mgh)).To(Succeed())
		Expect(runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)).To(Succeed())
	})
	It("mgh init", func() {
		Eventually(func() error {
			currentMgh := &v1alpha4.MulticlusterGlobalHub{}
			Expect(runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), currentMgh)).To(Succeed())
			if len(currentMgh.Finalizers) == 0 {
				return fmt.Errorf("Should add finalizer to mgh, mgh:%v", currentMgh)
			}

			if currentMgh.Status.Phase != v1alpha4.GlobalHubProgressing {
				return fmt.Errorf("mgh phase should be progressing, %v", currentMgh)
			}

			if meta.IsStatusConditionTrue(currentMgh.GetConditions(), config.CONDITION_TYPE_GLOBALHUB_READY) {
				return fmt.Errorf("mgh status should not be ready, %v", currentMgh)
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
	It("mgh deleting", func() {
		Eventually(func() error {
			currentMgh := &v1alpha4.MulticlusterGlobalHub{}
			err := runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), currentMgh)
			if err != nil {
				return err
			}
			currentMgh.Finalizers = []string{
				"fz",
			}
			err = runtimeClient.Update(ctx, currentMgh)
			if err != nil {
				return err
			}
			return runtimeClient.Delete(ctx, currentMgh)
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		Eventually(func() error {
			currentMgh := &v1alpha4.MulticlusterGlobalHub{}
			Expect(runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), currentMgh)).To(Succeed())
			if currentMgh.Status.Phase != v1alpha4.GlobalHubUninstalling {
				return fmt.Errorf("mgh phase should be uninstalling, %v", currentMgh)
			}

			if meta.IsStatusConditionTrue(currentMgh.GetConditions(), config.CONDITION_TYPE_GLOBALHUB_READY) {
				return fmt.Errorf("mgh status should not be ready, %v", currentMgh)
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
	AfterAll(func() {
		Eventually(func() error {
			return testutils.DeleteMgh(ctx, runtimeClient, mgh)
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		err := runtimeClient.Delete(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		})
		Expect(err).To(Succeed())
	})
})

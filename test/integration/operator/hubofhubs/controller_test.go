package hubofhubs

import (
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
	testutils "github.com/stolostron/multicluster-global-hub/test/integration/utils"
)

// go test ./test/integration/operator/hubofhubs -ginkgo.focus "controller" -v
var _ = Describe("controller", Ordered, func() {
	var mgh *v1alpha4.MulticlusterGlobalHub
	var namespace string
	BeforeAll(func() {
		namespace = fmt.Sprintf("namespace-%s", rand.String(6))
		mghName := "test-mgh"

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
				DataLayer: v1alpha4.DataLayerConfig{
					Postgres: v1alpha4.PostgresConfig{
						Retention: "2y",
					},
				},
			},
		}
		Expect(runtimeClient.Create(ctx, mgh)).To(Succeed())
		Expect(runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)).To(Succeed())

		// update the middleware configuration
		// storage
		storageSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.GHStorageSecretName,
				Namespace: mgh.Namespace,
			},
			Data: map[string][]byte{
				"database_uri":                   []byte(testPostgres.URI),
				"database_uri_with_readonlyuser": []byte(testPostgres.URI),
				"ca.crt":                         []byte(""),
			},
			Type: corev1.SecretTypeOpaque,
		}
		Expect(runtimeClient.Create(ctx, storageSecret)).To(Succeed())

		// transport
		err := CreateTestSecretTransport(runtimeClient, mgh.Namespace)
		Expect(err).To(Succeed())
	})

	It("should generate the update the mgh condition", func() {
		reconciler := hubofhubs.NewGlobalHubReconciler(runtimeManager, kubeClient, &config.OperatorConfig{
			LogLevel:              "info",
			EnablePprof:           false,
			GlobalResourceEnabled: true,
		}, nil)

		err := os.Setenv("POD_NAMESPACE", namespace)
		Expect(err).To(Succeed())
		Eventually(func() error {
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(mgh),
			})
			return err
		}, 3*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		err = runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)
		Expect(err).To(Succeed())

		utils.PrettyPrint(mgh)

		Expect(utils.ContainsString(mgh.Finalizers, constants.GlobalHubCleanupFinalizer)).To(BeTrue())
	})

	AfterAll(func() {
		Eventually(func() error {
			return testutils.DeleteMgh(ctx, runtimeClient, mgh)
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		err := runtimeClient.Delete(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.GHTransportSecretName,
				Namespace: mgh.Namespace,
			},
		})
		Expect(err).To(Succeed())

		err = runtimeClient.Delete(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		})
		Expect(err).To(Succeed())
	})
})

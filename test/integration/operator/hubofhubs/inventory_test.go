package hubofhubs

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs/inventory"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// go test ./test/integration/operator/hubofhubs -ginkgo.focus "inventory" -v
var _ = Describe("inventory-api", Ordered, func() {
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
			},
		}
		Expect(runtimeClient.Create(ctx, mgh)).To(Succeed())
		Expect(runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)).To(Succeed())

		// update the middleware configuration
		// storage
		_ = config.SetStorageConnection(&config.PostgresConnection{
			SuperuserDatabaseURI:    "test-url",
			ReadonlyUserDatabaseURI: "test-url",
			CACert:                  []byte("test-crt"),
		})
		config.SetDatabaseReady(true)

		// transport
		err := CreateTestSecretTransport(runtimeClient, mgh.Namespace)
		Expect(err).To(Succeed())
	})

	It("should generate the inventory resources", func() {
		reconciler := inventory.NewInventoryReconciler(runtimeManager, kubeClient)

		err := reconciler.Reconcile(ctx, mgh)
		Expect(err).To(Succeed())

		// deployment
		Eventually(func() error {
			deployment := &appsv1.Deployment{}
			err = runtimeClient.Get(ctx, types.NamespacedName{
				Name:      constants.InventoryDeploymentName,
				Namespace: mgh.Namespace,
			}, deployment)
			if err != nil {
				return err
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	AfterAll(func() {
		err := runtimeClient.Delete(ctx, mgh)
		Expect(err).To(Succeed())

		err = runtimeClient.Delete(ctx, &corev1.Secret{
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

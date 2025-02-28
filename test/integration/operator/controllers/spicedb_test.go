package controllers

import (
	"fmt"
	"time"

	spicedbv1alpha1 "github.com/authzed/spicedb-operator/pkg/apis/authzed/v1alpha1"
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
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/inventory"
	testutils "github.com/stolostron/multicluster-global-hub/test/integration/utils"
)

// go test ./test/integration/operator/controllers -ginkgo.focus "spicedb" -v
var _ = Describe("spicedb", Ordered, func() {
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
				Annotations: map[string]string{
					operatorconstants.AnnotationMGHWithInventory: "",
				},
			},
			Spec: v1alpha4.MulticlusterGlobalHubSpec{
				ImagePullPolicy: corev1.PullIfNotPresent,
				ImagePullSecret: "test-pull-secret",
				NodeSelector:    map[string]string{"test1": "val1"},
				Tolerations: []corev1.Toleration{{
					Key:      "key1",
					Operator: "Equal",
					Value:    "val1",
					Effect:   "NoSchedule",
				}},
			},
		}
		Expect(runtimeClient.Create(ctx, mgh)).To(Succeed())
		Expect(runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)).To(Succeed())

		// storage
		_ = config.SetStorageConnection(&config.PostgresConnection{
			SuperuserDatabaseURI: "postgresql://:@multicluster-global-hub-postgresql.multicluster-global-hub.svc:5432/hoh",
			CACert:               []byte("test-crt"),
		})

		_, err := inventory.StartSpiceDBReconciler(config.ControllerOption{
			Manager: runtimeManager, MulticlusterGlobalHub: mgh,
		})
		Expect(err).To(Succeed())
	})

	It("should generate the spicedb resources", func() {
		// deployment
		Eventually(func() error {
			deployment := &appsv1.Deployment{}
			err := runtimeClient.Get(ctx, types.NamespacedName{
				Name:      "spicedb-operator",
				Namespace: mgh.Namespace,
			}, deployment)
			if err != nil {
				return err
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		// operand
		Eventually(func() error {
			cluster := &spicedbv1alpha1.SpiceDBCluster{}
			err := runtimeClient.Get(ctx, types.NamespacedName{
				Name:      inventory.SpiceDBConfigClusterName,
				Namespace: mgh.Namespace,
			}, cluster)
			if err != nil {
				return err
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	AfterAll(func() {
		Eventually(func() error {
			if err := testutils.DeleteMgh(ctx, runtimeClient, mgh); err != nil {
				return err
			}
			return deleteNamespace(namespace)
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		err := runtimeClient.Delete(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		})
		Expect(err).To(Succeed())
	})
})

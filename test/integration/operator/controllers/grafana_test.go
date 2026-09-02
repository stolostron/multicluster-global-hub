package controllers

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/ini.v1"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/grafana"
	testutils "github.com/stolostron/multicluster-global-hub/test/integration/utils"
)

// go test ./test/integration/operator/hubofhubs -ginkgo.focus "grafana" -v
var _ = Describe("grafana", Ordered, func() {
	var mgh *v1alpha4.MulticlusterGlobalHub
	var namespace string
	BeforeAll(func() {
		namespace = fmt.Sprintf("namespace-%s", rand.String(6))
		mghName := "test-mgh"
		err := runtimeClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		})
		Expect(err).To(Succeed())
		mgh = &v1alpha4.MulticlusterGlobalHub{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mghName,
				Namespace: namespace,
			},
			Spec: v1alpha4.MulticlusterGlobalHubSpec{
				EnableMetrics: true,
			},
		}
		Expect(runtimeClient.Create(ctx, mgh)).To(Succeed(), "test MulticlusterGlobalHub must be created")
		Expect(runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)).To(Succeed(),
			"created MulticlusterGlobalHub must be readable")
		previousConn := config.GetStorageConnection()
		previousReady := config.GetDatabaseReady()
		DeferCleanup(func() {
			_ = config.SetStorageConnection(previousConn)
			config.SetDatabaseReady(previousReady)
		})
		Expect(config.SetStorageConnection(&config.PostgresConnection{
			SuperuserDatabaseURI:    "postgresql://testuser:testpassword@localhost:5432/testdb?sslmode=verify-full",
			ReadonlyUserDatabaseURI: "postgresql://testuser:testpassword@localhost:5432/testdb?sslmode=verify-full",
			CACert:                  []byte("test-crt"),
		})).To(BeTrue(), "set storage configuration for Grafana TLS validation")
		config.SetDatabaseReady(true)
		Expect(grafana.NewGrafanaReconciler(runtimeManager, kubeClient).SetupWithManager(runtimeManager)).
			To(Succeed(), "Grafana reconciler must register with the test manager")
	})

	It("should generate the grafana resources", func() {
		Eventually(func() error {
			deployment := &appsv1.Deployment{}
			if err := runtimeClient.Get(ctx, types.NamespacedName{
				Name:      "multicluster-global-hub-grafana",
				Namespace: mgh.Namespace,
			}, deployment); err != nil {
				return err
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	It("should configure postgres datasource TLS verification", func() {
		Eventually(func() error {
			dsSecret := &corev1.Secret{}
			if err := runtimeClient.Get(ctx, types.NamespacedName{
				Name:      "multicluster-global-hub-grafana-datasources",
				Namespace: mgh.Namespace,
			}, dsSecret); err != nil {
				return err
			}
			var datasources grafana.GrafanaDatasources
			if err := yaml.Unmarshal(dsSecret.Data["datasources.yaml"], &datasources); err != nil {
				return err
			}
			if len(datasources.Datasources) == 0 || datasources.Datasources[0].JSONData == nil {
				return fmt.Errorf("grafana datasource not ready")
			}
			ds := datasources.Datasources[0]
			if ds.JSONData.TLSSkipVerify {
				return fmt.Errorf("expected postgres datasource TLS verification enabled")
			}
			if ds.JSONData.SSLMode != "verify-full" {
				return fmt.Errorf("expected sslmode verify-full, got %q", ds.JSONData.SSLMode)
			}
			if !ds.JSONData.TLSAuth || !ds.JSONData.TLSAuthWithCACert {
				return fmt.Errorf("expected TLS auth flags enabled on postgres datasource")
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
	})

	It("should inject a grafana admin password", func() {
		Eventually(func() error {
			iniSecret := &corev1.Secret{}
			if err := runtimeClient.Get(ctx, types.NamespacedName{
				Name:      "multicluster-global-hub-grafana-config",
				Namespace: mgh.Namespace,
			}, iniSecret); err != nil {
				return err
			}
			cfg, err := ini.Load(iniSecret.Data["grafana.ini"])
			if err != nil {
				return err
			}
			sec, err := cfg.GetSection("security")
			if err != nil {
				return err
			}
			if sec.Key("admin_password").String() == "" {
				return fmt.Errorf("grafana admin_password not set")
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
	})

	AfterAll(func() {
		Eventually(func() error {
			err := testutils.DeleteMgh(ctx, runtimeClient, mgh)
			if err != nil {
				return err
			}
			return deleteNamespace(namespace)
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})

// deleteNamespace removes the test namespace created for Grafana integration coverage.
func deleteNamespace(name string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	return runtimeClient.Delete(ctx, ns)
}

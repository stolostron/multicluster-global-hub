package metrics

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var (
	cfg           *rest.Config
	runtimeClient client.Client
	namespace     = "default"
	ctx           context.Context
	cancel        context.CancelFunc
)

func TestMain(m *testing.M) {
	ctx, cancel = context.WithCancel(context.Background())
	err := os.Setenv("POD_NAMESPACE", namespace)
	if err != nil {
		panic(err)
	}

	testenv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "..", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "..", "..", "pkg", "testdata", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err = testenv.Start()
	if err != nil {
		panic(err)
	}

	runtimeClient, err = client.New(cfg, client.Options{Scheme: config.GetRuntimeScheme()})
	if err != nil {
		panic(err)
	}

	// run testings
	code := m.Run()

	cancel()

	// stop testenv
	if err := testenv.Stop(); err != nil {
		panic(err)
	}
	os.Exit(code)
}

func TestMetricsReconciler(t *testing.T) {
	RegisterTestingT(t)
	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-mgh",
			Namespace:  utils.GetDefaultNamespace(),
			Finalizers: []string{constants.GlobalHubCleanupFinalizer},
		},
		Spec: v1alpha4.MulticlusterGlobalHubSpec{
			DataLayer: v1alpha4.DataLayerConfig{
				Postgres: v1alpha4.PostgresConfig{
					Retention: "2y",
				},
			},
		},
	}

	reconciler := NewMetricsReconciler(runtimeClient)
	err := reconciler.Reconcile(ctx, mgh)
	Expect(err).To(Succeed())

	// add label openshift.io/cluster-monitoring: "true" to the ns, so that the prometheus can detect the ServiceMonitor.
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: utils.GetDefaultNamespace(),
		},
	}
	err = reconciler.Client.Get(ctx, client.ObjectKeyFromObject(ns), ns)
	Expect(err).To(Succeed())

	val, ok := ns.GetLabels()[operatorconstants.ClusterMonitoringLabelKey]
	Expect(ok).To(BeTrue())
	Expect(val).To(Equal(operatorconstants.ClusterMonitoringLabelVal))

	serviceMonitor := &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconstants.GHServiceMonitorName,
			Namespace: utils.GetDefaultNamespace(),
		},
	}
	err = reconciler.Client.Get(ctx, client.ObjectKeyFromObject(serviceMonitor), serviceMonitor)
	Expect(err).To(Succeed())
	utils.PrettyPrint(serviceMonitor)
}

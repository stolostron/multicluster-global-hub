package hubofhubs

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs/metrics"
)

// go test ./test/integration/operator/hubofhubs -ginkgo.focus "metrics" -v
var _ = Describe("metrics", Ordered, func() {
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
	})

	It("should create the metrics resources", func() {
		reconciler := metrics.NewMetricsReconciler(runtimeClient)
		err := reconciler.Reconcile(ctx, mgh)
		Expect(err).To(Succeed())

		// add label openshift.io/cluster-monitoring: "true" to the ns, so that the prometheus can detect the ServiceMonitor.
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: mgh.Namespace,
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
				Namespace: mgh.Namespace,
			},
		}
		err = reconciler.Client.Get(ctx, client.ObjectKeyFromObject(serviceMonitor), serviceMonitor)
		Expect(err).To(Succeed())
	})

	AfterAll(func() {
		err := runtimeClient.Delete(ctx, mgh)
		Expect(err).To(Succeed())

		err = runtimeClient.Delete(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		})
		Expect(err).To(Succeed())
	})
})

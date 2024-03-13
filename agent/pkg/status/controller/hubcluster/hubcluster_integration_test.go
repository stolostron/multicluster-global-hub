package hubcluster

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clustersv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/cluster"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

var _ = Describe("Hub cluster integration test", Ordered, func() {
	It("should receive the heartbeat", func() {
		By("Check the local hearbeat event can be read from cloudevents consumer")
		Eventually(func() error {
			evt := heartbeatTrans.GetEvent()
			fmt.Println(evt)
			if evt.Type() != string(enum.HubClusterHeartbeatType) {
				return fmt.Errorf("want %v, got %v", string(enum.HubClusterHeartbeatType), evt.Type())
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
	})

	It("should get the cluster info", func() {
		By("Create clusterclaim with name <id.k8s.io> in the managed hub cluster")
		clusterClaim := &clustersv1alpha1.ClusterClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: "id.k8s.io",
			},
			Spec: clustersv1alpha1.ClusterClaimSpec{
				Value: "00000000-0000-0000-0000-000000000001",
			},
		}
		Expect(kubeClient.Create(ctx, clusterClaim)).Should(Succeed())

		By("Check the hub cluster info bundle can be read from cloudevents consumer")
		Eventually(func() error {
			evt := mockTrans.GetEvent()
			fmt.Println(evt)
			if evt.Type() != string(enum.HubClusterInfoType) {
				return fmt.Errorf("want %v, got %v", string(enum.HubClusterHeartbeatType), evt.Type())
			}
			return nil
		}, 50*time.Second, 1*time.Second).Should(Succeed())

		By("Create openshift route for hub cluster")
		Expect(kubeClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: constants.OpenShiftConsoleNamespace},
		})).Should(Succeed())

		consoleRoute := &routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.OpenShiftConsoleRouteName,
				Namespace: constants.OpenShiftConsoleNamespace,
			},
			Spec: routev1.RouteSpec{
				Host: "console-openshift-console.apps.test-cluster",
				To: routev1.RouteTargetReference{
					Kind: "Service",
					Name: constants.OpenShiftConsoleRouteName,
				},
			},
		}
		Expect(kubeClient.Create(ctx, consoleRoute)).Should(Succeed())

		By("Create observability grafana route in the managed hub cluster")
		Expect(kubeClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: constants.ObservabilityNamespace},
		})).Should(Succeed())

		obsRoute := &routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.ObservabilityGrafanaRouteName,
				Namespace: constants.ObservabilityNamespace,
			},
			Spec: routev1.RouteSpec{
				Host: "grafana-open-cluster-management-observability.apps.test-cluster",
				To: routev1.RouteTargetReference{
					Kind: "Service",
					Name: constants.ObservabilityGrafanaRouteName,
				},
			},
		}
		Expect(kubeClient.Create(ctx, obsRoute)).Should(Succeed())

		By("Check the hub cluster info bundle can be read from cloudevents consumer")
		Eventually(func() error {
			evt := mockTrans.GetEvent()
			fmt.Println(evt)
			if evt.Type() != string(enum.HubClusterInfoType) {
				return fmt.Errorf("want %v, got %v", string(enum.HubClusterHeartbeatType), evt.Type())
			}

			clusterInfo := &cluster.HubClusterInfo{}
			if err := evt.DataAs(clusterInfo); err != nil {
				return err
			}

			if clusterInfo.ClusterId != clusterClaim.Spec.Value {
				return fmt.Errorf("want %v, got %v", clusterClaim.Spec.Value, clusterInfo.ClusterId)
			}
			if !strings.Contains(clusterInfo.ConsoleURL, consoleRoute.Spec.Host) {
				return fmt.Errorf("want %v, got %v", consoleRoute.Spec.Host, clusterInfo.ConsoleURL)
			}
			if !strings.Contains(clusterInfo.GrafanaURL, obsRoute.Spec.Host) {
				return fmt.Errorf("want %v, got %v", obsRoute.Spec.Host, clusterInfo.GrafanaURL)
			}
			return nil
		}, 50*time.Second, 1*time.Second).Should(Succeed())
	})
})

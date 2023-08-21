package tests

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	mergedAlertName   = "grafana-alerting-acm-global-alerting-policy"
	defaultAlertName  = "grafana-alerting-acm-global-default-alerting"
	alertConfigMapKey = "acm-global-alerting.yaml"

	mergedGrafanaIniName  = "multicluster-global-hub-grafana-config"
	defaultGrafanaIniName = "multicluster-global-hub-default-grafana-config"
	grafanaIniKey         = "grafana.ini"

	alertValueWord        = "{{ $value }}"
	alertValuePlaceHolder = "<ALERT_PLACE_HOLDER>"
)

var _ = Describe("The alert configmap should be created", Ordered, Label("e2e-tests-grafana"), func() {

	It("Merged alert configmap should be same as default configmap", func() {
		ctx := context.Background()
		Eventually(func() error {
			defaultAlertConfigMap, err := testClients.KubeClient().CoreV1().ConfigMaps(Namespace).Get(ctx, defaultAlertName, v1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			mergedAlertConfigMap, err := testClients.KubeClient().CoreV1().ConfigMaps(Namespace).Get(ctx, mergedAlertName, v1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			//replace the placeholder with original value word
			replacedDefaultAlertData := strings.ReplaceAll(defaultAlertConfigMap.Data[alertConfigMapKey], alertValuePlaceHolder, alertValueWord)

			if len(replacedDefaultAlertData) != len(mergedAlertConfigMap.Data[alertConfigMapKey]) {
				klog.Errorf("defaultAlertConfigMap data: %v", replacedDefaultAlertData)
				klog.Errorf("mergedAlertConfigMap data: %v", mergedAlertConfigMap.Data)
				return fmt.Errorf("Default alert configmap is different with merged alert configmap. Default alert : %v, mergedAlert: %v",
					len(replacedDefaultAlertData), len(mergedAlertConfigMap.Data[alertConfigMapKey]))
			}
			return nil
		}, 2*time.Minute, 10*time.Second).ShouldNot(HaveOccurred())

	})

	It("Merged alert configmap should merged default and custom configmap", func() {
		ctx := context.Background()
		customConfig := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: Namespace,
				Name:      operatorconstants.CustomAlertName,
				Labels: map[string]string{
					constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
				},
			},
			Data: map[string]string{
				alertConfigMapKey: `
apiVersion: 1
contactPoints:
- name: alerts-cu-webhook
	orgId: 1
	receivers:
	- disableResolveMessage: false
	type: email
	uid: 4e3bfe25-00cf-4173-b02b-16f077e539da
groups:
- folder: Custom
	name: Suspicious policy change
	orgId: 1
- folder: Custom
	name: Suspicious Cluster Compliance Status Change
	orgId: 1
policies:
- orgId: 1
	receiver: alerts-cu-webhook`,
			},
		}

		// Set MGH instance as the owner and controller
		err := controllerutil.SetControllerReference(mcgh, customConfig, scheme)
		Expect(err).ShouldNot(HaveOccurred())

		_, err = testClients.KubeClient().CoreV1().ConfigMaps(Namespace).Create(ctx, customConfig, v1.CreateOptions{})
		Expect(err).ShouldNot(HaveOccurred())

		Eventually(func() error {
			defaultAlertConfigMap, err := testClients.KubeClient().CoreV1().ConfigMaps(Namespace).Get(ctx, defaultAlertName, v1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			customAlertConfigMap, err := testClients.KubeClient().CoreV1().ConfigMaps(Namespace).Get(ctx, operatorconstants.CustomAlertName, v1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			//replace the placeholder with original value word
			replacedDefaultAlertData := strings.ReplaceAll(defaultAlertConfigMap.Data[alertConfigMapKey], alertValuePlaceHolder, alertValueWord)

			mergedAlertConfigMap, err := testClients.KubeClient().CoreV1().ConfigMaps(Namespace).Get(ctx, mergedAlertName, v1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			if len(mergedAlertConfigMap.Data[alertConfigMapKey]) < (len(customAlertConfigMap.Data[alertConfigMapKey]) + len(replacedDefaultAlertData)) {
				/*	podsList, err := testClients.KubeClient().CoreV1().Pods(Namespace).List(ctx, v1.ListOptions{
						LabelSelector: "name=multicluster-global-hub-operator",
					})
					Expect(err).ShouldNot(HaveOccurred())

						for _, p := range podsList.Items {
							request := testClients.KubeClient().CoreV1().Pods(Namespace).GetLogs(p.Name, &corev1.PodLogOptions{})
							readCloser, err := request.Stream(context.TODO())
							if err != nil {
								klog.Errorf("Failed to read logs %v", err)
								return err
							}
							defer readCloser.Close()
							var stdout io.Writer
							_, err = io.Copy(stdout, readCloser)
							if err != nil {
								klog.Errorf("Failed to copy logs to writer %v", err)
								return err
							}
							var logstr []byte
							n, err := stdout.Write(logstr)
							if err != nil {
								klog.Errorf("Failed to copy logs to writer %v", err)

								klog.Errorf("###:%v, log:%v", n, string(logstr))
								return err
							}
							klog.Errorf("###:%v, log:%v", n, string(logstr))
						}*/
				return fmt.Errorf("Default and custom alert configmap is different with merged alert configmap. Default alert : %v, custom alert: %v, mergedAlert: %v",
					len(replacedDefaultAlertData), len(customAlertConfigMap.Data[alertConfigMapKey]), len(mergedAlertConfigMap.Data[alertConfigMapKey]))
			}
			return nil
		}, 2*time.Minute, 10*time.Second).ShouldNot(HaveOccurred())

		err = testClients.KubeClient().CoreV1().ConfigMaps(Namespace).Delete(ctx, operatorconstants.CustomAlertName, v1.DeleteOptions{})
		Expect(err).ShouldNot(HaveOccurred())

		Eventually(func() error {
			defaultAlertConfigMap, err := testClients.KubeClient().CoreV1().ConfigMaps(Namespace).Get(ctx, defaultAlertName, v1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			mergedAlertConfigMap, err := testClients.KubeClient().CoreV1().ConfigMaps(Namespace).Get(ctx, mergedAlertName, v1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			//replace the placeholder with original value word
			replacedDefaultAlertData := strings.ReplaceAll(defaultAlertConfigMap.Data[alertConfigMapKey], alertValuePlaceHolder, alertValueWord)

			if len(replacedDefaultAlertData) != len(mergedAlertConfigMap.Data[alertConfigMapKey]) {
				return fmt.Errorf("Default alert configmap is different with merged alert configmap. Default alert : %v, mergedAlert: %v",
					len(replacedDefaultAlertData), len(mergedAlertConfigMap.Data[alertConfigMapKey]))
			}
			return nil
		}, 2*time.Minute, 10*time.Second).ShouldNot(HaveOccurred())
	})

})

var _ = Describe("The grafana.ini should be created", Ordered, Label("e2e-tests-grafana"), func() {

	It("Merged grafana.ini should be same as default configmap", func() {
		ctx := context.Background()
		Eventually(func() error {
			defaultGrafanaIniSecret, err := testClients.KubeClient().CoreV1().Secrets(Namespace).Get(ctx, defaultGrafanaIniName, v1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			mergedGrafanaIniSecret, err := testClients.KubeClient().CoreV1().Secrets(Namespace).Get(ctx, mergedGrafanaIniName, v1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			if len(defaultGrafanaIniSecret.Data[grafanaIniKey]) != len(mergedGrafanaIniSecret.Data[grafanaIniKey]) {
				return fmt.Errorf("Default grafana.ini is different with merged grafana.ini. Default grafana.ini : %v, merged grafana.ini: %v",
					len(string(defaultGrafanaIniSecret.Data[grafanaIniKey])), len(string(mergedGrafanaIniSecret.Data[grafanaIniKey])))
			}
			return nil
		}, 2*time.Minute, 10*time.Second).ShouldNot(HaveOccurred())

	})

	It("Merged grafana.ini should merged default and custom grafana.ini", func() {
		ctx := context.Background()
		customSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: Namespace,
				Name:      operatorconstants.CustomGrafanaIniName,
			},
			Data: map[string][]byte{
				grafanaIniKey: []byte("    [smtp]\n    email = example@redhat.com"),
			},
		}
		customSecret, err := testClients.KubeClient().CoreV1().Secrets(Namespace).Create(ctx, customSecret, v1.CreateOptions{})
		Expect(err).ShouldNot(HaveOccurred())

		Eventually(func() error {
			defaultGrafanaIniSecret, err := testClients.KubeClient().CoreV1().Secrets(Namespace).Get(ctx, defaultGrafanaIniName, v1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			mergedGrafanaIniSecret, err := testClients.KubeClient().CoreV1().Secrets(Namespace).Get(ctx, mergedGrafanaIniName, v1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())
			if (len(defaultGrafanaIniSecret.Data[grafanaIniKey]) + len(customSecret.Data[grafanaIniKey]) + 1) != len(mergedGrafanaIniSecret.Data[grafanaIniKey]) {
				return fmt.Errorf("Default and custom grafana.ini secret is different with merged grafana.ini secret. Default : %v, custom: %v, merged: %v",
					len(defaultGrafanaIniSecret.Data[grafanaIniKey]), len(customSecret.Data[grafanaIniKey]), len(mergedGrafanaIniSecret.Data[grafanaIniKey]))
			}
			return nil
		}, 2*time.Minute, 10*time.Second).ShouldNot(HaveOccurred())

		err = testClients.KubeClient().CoreV1().Secrets(Namespace).Delete(ctx, operatorconstants.CustomGrafanaIniName, v1.DeleteOptions{})
		Expect(err).ShouldNot(HaveOccurred())

		Eventually(func() error {
			defaultGrafanaIniSecret, err := testClients.KubeClient().CoreV1().Secrets(Namespace).Get(ctx, defaultGrafanaIniName, v1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			mergedGrafanaIniSecret, err := testClients.KubeClient().CoreV1().Secrets(Namespace).Get(ctx, mergedGrafanaIniName, v1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			if len(defaultGrafanaIniSecret.Data[grafanaIniKey]) != len(mergedGrafanaIniSecret.Data[grafanaIniKey]) {
				return fmt.Errorf("Default grafana.ini is different with merged grafana.ini. Default grafana.ini : %v, merged grafana.ini: %v",
					len(defaultGrafanaIniSecret.Data[grafanaIniKey]), len(mergedGrafanaIniSecret.Data[grafanaIniKey]))
			}
			return nil
		}, 2*time.Minute, 10*time.Second).ShouldNot(HaveOccurred())

	})

})

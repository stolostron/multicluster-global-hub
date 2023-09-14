package tests

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"gopkg.in/ini.v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

const (
	mergedAlertName   = "multicluster-global-hub-alerting"
	defaultAlertName  = "multicluster-global-hub-default-alerting"
	alertConfigMapKey = "alerting.yaml"

	mergedGrafanaIniName  = "multicluster-global-hub-grafana-config"
	defaultGrafanaIniName = "multicluster-global-hub-default-grafana-config"
	grafanaIniKey         = "grafana.ini"
)

var _ = Describe("The alert configmap should be created", Ordered, Label("e2e-tests-grafana"), func() {

	It("Merged alert configmap should be same as default configmap", func() {
		ctx := context.Background()
		Eventually(func() error {
			defaultAlertConfigMap, err := testClients.KubeClient().CoreV1().ConfigMaps(Namespace).Get(ctx, defaultAlertName, metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			mergedAlertConfigMap, err := testClients.KubeClient().CoreV1().ConfigMaps(Namespace).Get(ctx, mergedAlertName, metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			equal, err := utils.IsAlertGPCcountEqual([]byte(defaultAlertConfigMap.Data[alertConfigMapKey]), []byte(mergedAlertConfigMap.Data[alertConfigMapKey]))
			if err != nil {
				return err
			}
			if !equal {
				klog.Errorf("defaultAlertConfigMap data: %v", defaultAlertConfigMap.Data[alertConfigMapKey])
				klog.Errorf("mergedAlertConfigMap data: %v", mergedAlertConfigMap.Data)
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
- folder: Policy
  name: Suspicious Cluster Compliance Status Change
  orgId: 1
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

		_, err := testClients.KubeClient().CoreV1().ConfigMaps(Namespace).Create(ctx, customConfig, metav1.CreateOptions{})
		Expect(err).ShouldNot(HaveOccurred())

		Eventually(func() error {
			defaultAlertConfigMap, err := testClients.KubeClient().CoreV1().ConfigMaps(Namespace).Get(ctx, defaultAlertName, metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			customAlertConfigMap, err := testClients.KubeClient().CoreV1().ConfigMaps(Namespace).Get(ctx, operatorconstants.CustomAlertName, metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			mergedAlertConfigMap, err := testClients.KubeClient().CoreV1().ConfigMaps(Namespace).Get(ctx, mergedAlertName, metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			dg, dp, dc, err := utils.GetAlertGPCcount([]byte(defaultAlertConfigMap.Data[alertConfigMapKey]))
			Expect(err).ShouldNot(HaveOccurred())

			cg, cp, cc, err := utils.GetAlertGPCcount([]byte(customAlertConfigMap.Data[alertConfigMapKey]))
			Expect(err).ShouldNot(HaveOccurred())

			mg, mp, mc, err := utils.GetAlertGPCcount([]byte(mergedAlertConfigMap.Data[alertConfigMapKey]))
			Expect(err).ShouldNot(HaveOccurred())

			if mg == dg+cg && mp == dp+cp && mc == dc+cc {
				return nil
			}
			klog.Errorf("defaultAlertConfigMap data: %v", defaultAlertConfigMap.Data[alertConfigMapKey])
			klog.Errorf("mergedAlertConfigMap data: %v", mergedAlertConfigMap.Data)
			klog.Errorf("customAlertConfigMap data: %v", customAlertConfigMap.Data)

			return fmt.Errorf("mergedAlert is not equal with default and custom. mg:%v,mp:%v,mc:%v", mg, mp, mc)
		}, 2*time.Minute, 10*time.Second).ShouldNot(HaveOccurred())

		err = testClients.KubeClient().CoreV1().ConfigMaps(Namespace).Delete(ctx, operatorconstants.CustomAlertName, metav1.DeleteOptions{})
		Expect(err).ShouldNot(HaveOccurred())

		Eventually(func() error {
			defaultAlertConfigMap, err := testClients.KubeClient().CoreV1().ConfigMaps(Namespace).Get(ctx, defaultAlertName, metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			mergedAlertConfigMap, err := testClients.KubeClient().CoreV1().ConfigMaps(Namespace).Get(ctx, mergedAlertName, metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			equal, err := utils.IsAlertGPCcountEqual([]byte(defaultAlertConfigMap.Data[alertConfigMapKey]), []byte(mergedAlertConfigMap.Data[alertConfigMapKey]))
			if err != nil {
				return err
			}
			if equal {
				return nil
			}

			klog.Errorf("defaultAlertConfigMap data: %v", defaultAlertConfigMap.Data)
			klog.Errorf("mergedAlertConfigMap data: %v", mergedAlertConfigMap.Data)
			return fmt.Errorf("mergedAlert is not equal with default.")
		}, 2*time.Minute, 10*time.Second).ShouldNot(HaveOccurred())
	})

	It("Merged alert configmap should be same as default when custom alert is invalid", func() {
		ctx := context.Background()
		customConfig := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: Namespace,
				Name:      operatorconstants.CustomAlertName,
			},
			Data: map[string]string{
				alertConfigMapKey: `
- name: alerts-cu-webhook
	orgId: 1
  receivers:
  - disableResolveMessage: false
	type: email
	uid: 4e3bfe25-00cf-4173-b02b-16f077e539da`,
			},
		}

		_, err := testClients.KubeClient().CoreV1().ConfigMaps(Namespace).Create(ctx, customConfig, metav1.CreateOptions{})
		Expect(err).ShouldNot(HaveOccurred())

		Eventually(func() error {
			defaultAlertConfigMap, err := testClients.KubeClient().CoreV1().ConfigMaps(Namespace).Get(ctx, defaultAlertName, metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			mergedAlertConfigMap, err := testClients.KubeClient().CoreV1().ConfigMaps(Namespace).Get(ctx, mergedAlertName, metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())
			equal, err := utils.IsAlertGPCcountEqual([]byte(defaultAlertConfigMap.Data[alertConfigMapKey]), []byte(mergedAlertConfigMap.Data[alertConfigMapKey]))
			if err != nil {
				return err
			}

			if equal {
				return nil
			}

			klog.Errorf("defaultAlertConfigMap data: %v", defaultAlertConfigMap.Data[alertConfigMapKey])
			klog.Errorf("mergedAlertConfigMap data: %v", mergedAlertConfigMap.Data)
			return fmt.Errorf("mergedAlert is not equal with default.")

		}, 2*time.Minute, 10*time.Second).ShouldNot(HaveOccurred())

		err = testClients.KubeClient().CoreV1().ConfigMaps(Namespace).Delete(ctx, operatorconstants.CustomAlertName, metav1.DeleteOptions{})
		Expect(err).ShouldNot(HaveOccurred())

	})

})

var _ = Describe("The grafana.ini should be created", Ordered, Label("e2e-tests-grafana"), func() {

	It("Merged grafana.ini should be same as default configmap", func() {
		ctx := context.Background()
		Eventually(func() error {
			defaultGrafanaIniSecret, err := testClients.KubeClient().CoreV1().Secrets(Namespace).Get(ctx, defaultGrafanaIniName, metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			mergedGrafanaIniSecret, err := testClients.KubeClient().CoreV1().Secrets(Namespace).Get(ctx, mergedGrafanaIniName, metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())
			if sectionCount(defaultGrafanaIniSecret.Data[grafanaIniKey]) != sectionCount(mergedGrafanaIniSecret.Data[grafanaIniKey]) {
				return fmt.Errorf("Default grafana.ini is different with merged grafana.ini. Default grafana.ini : %v, merged grafana.ini: %v",
					sectionCount(defaultGrafanaIniSecret.Data[grafanaIniKey]), sectionCount(mergedGrafanaIniSecret.Data[grafanaIniKey]))
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
				grafanaIniKey: []byte(`
    [smtp]
    user = true
    pass = true
    [auth]
    enabled = false
    [dataproxy]
    timeout = 300
    dial_timeout = 30
    keep_alive_seconds = 300
`),
			},
		}
		customSecret, err := testClients.KubeClient().CoreV1().Secrets(Namespace).Create(ctx, customSecret, metav1.CreateOptions{})
		Expect(err).ShouldNot(HaveOccurred())

		Eventually(func() error {
			defaultGrafanaIniSecret, err := testClients.KubeClient().CoreV1().Secrets(Namespace).Get(ctx, defaultGrafanaIniName, metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			mergedGrafanaIniSecret, err := testClients.KubeClient().CoreV1().Secrets(Namespace).Get(ctx, mergedGrafanaIniName, metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())
			if (sectionCount(defaultGrafanaIniSecret.Data[grafanaIniKey]) + sectionCount(customSecret.Data[grafanaIniKey]) - 2) != sectionCount(mergedGrafanaIniSecret.Data[grafanaIniKey]) {
				return fmt.Errorf("Default and custom grafana.ini secret is different with merged grafana.ini secret. Default : %v, custom: %v, merged: %v",
					sectionCount(defaultGrafanaIniSecret.Data[grafanaIniKey]), sectionCount(customSecret.Data[grafanaIniKey]), sectionCount(mergedGrafanaIniSecret.Data[grafanaIniKey]))
			}
			return nil
		}, 2*time.Minute, 10*time.Second).ShouldNot(HaveOccurred())

		err = testClients.KubeClient().CoreV1().Secrets(Namespace).Delete(ctx, operatorconstants.CustomGrafanaIniName, metav1.DeleteOptions{})
		Expect(err).ShouldNot(HaveOccurred())

		Eventually(func() error {
			defaultGrafanaIniSecret, err := testClients.KubeClient().CoreV1().Secrets(Namespace).Get(ctx, defaultGrafanaIniName, metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			mergedGrafanaIniSecret, err := testClients.KubeClient().CoreV1().Secrets(Namespace).Get(ctx, mergedGrafanaIniName, metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			if sectionCount(defaultGrafanaIniSecret.Data[grafanaIniKey]) != sectionCount(mergedGrafanaIniSecret.Data[grafanaIniKey]) {
				return fmt.Errorf("Default grafana.ini is different with merged grafana.ini. Default grafana.ini : %v, merged grafana.ini: %v",
					sectionCount(defaultGrafanaIniSecret.Data[grafanaIniKey]), sectionCount(mergedGrafanaIniSecret.Data[grafanaIniKey]))
			}
			return nil
		}, 2*time.Minute, 10*time.Second).ShouldNot(HaveOccurred())

	})

})

func sectionCount(a []byte) int {
	cfg, err := ini.Load(a)
	if err != nil {
		return -1
	}
	//By Default, There is a DEFAULT section, should not count it
	return len(cfg.Sections()) - 1
}

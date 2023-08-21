package hubofhubs

import (
	"context"
	"testing"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakekube "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
)

func Test_mergeAlertConfigMap(t *testing.T) {
	configNamespace := config.GetDefaultNamespace()

	tests := []struct {
		name                  string
		defaultAlertConfigMap *corev1.ConfigMap
		customAlertConfigMap  *corev1.ConfigMap
		want                  *corev1.ConfigMap
	}{
		{
			name:                  "nil Configmap",
			defaultAlertConfigMap: nil,
			customAlertConfigMap:  nil,
			want:                  nil,
		},
		{
			name: "no custom Configmap",
			defaultAlertConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: configNamespace,
					Name:      "default-alert",
				},
				Data: map[string]string{
					alertConfigMapKey: "apiVersion: 1\ngroups:\n  - orgId: 1\n    name: Suspicious policy change\n    folder: Policy\n  - orgId: 1\n    name: Suspicious Cluster Compliance Status Change\n    folder: Policy",
				},
			}, customAlertConfigMap: nil,
			want: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: configNamespace,
					Name:      mergedAlertName,
				},
				Data: map[string]string{
					alertConfigMapKey: "apiVersion: 1\ngroups:\n  - orgId: 1\n    name: Suspicious policy change\n    folder: Policy\n  - orgId: 1\n    name: Suspicious Cluster Compliance Status Change\n    folder: Policy",
				},
			},
		},
		{
			name: "all configmap are default value",
			defaultAlertConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: configNamespace,
					Name:      "default-alert",
				},
				Data: map[string]string{
					alertConfigMapKey: "apiVersion: 1\ngroups:\n  - orgId: 1\n    name: Suspicious policy change\n    folder: Policy\n  - orgId: 1\n    name: Suspicious Cluster Compliance Status Change\n    folder: Policy",
				},
			},
			customAlertConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: configNamespace,
					Name:      "custom-alert",
				},
				Data: map[string]string{
					alertConfigMapKey: "apiVersion: 1\ngroups:\n  - orgId: 1\n    name: Suspicious policy change\n    folder: Custom\n  - orgId: 1\n    name: Suspicious Cluster Compliance Status Change\n    folder: Custom\ncontactPoints:\n  - orgId: 1\n    name: alerts-cu-webhook\n    receivers:\n      - uid: 4e3bfe25-00cf-4173-b02b-16f077e539da\n        type: email\n        disableResolveMessage: false\npolicies:\n  - orgId: 1\n    receiver: alerts-cu-webhook",
				},
			},
			want: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: configNamespace,
					Name:      mergedAlertName,
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
  name: Suspicious policy change
  orgId: 1
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
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeAlertConfigMap(tt.defaultAlertConfigMap, tt.customAlertConfigMap)
			if got == nil || tt.want == nil {
				if got != tt.want {
					t.Errorf("want:%v, got:%v", tt.want, got)
				}
			} else if len(got.Data[alertConfigMapKey]) != len(tt.want.Data[alertConfigMapKey]) {
				t.Errorf("mergedAlertConfigMap() = %v, want %v", len(got.Data[alertConfigMapKey]), len(tt.want.Data[alertConfigMapKey]))
			}
		})
	}
}

func Test_generateAlertConfigMap(t *testing.T) {
	configNamespace := config.GetDefaultNamespace()

	mgh := &globalhubv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "open-cluster-management",
		},
		Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{
			DataLayer: globalhubv1alpha4.DataLayerConfig{},
		},
	}
	tests := []struct {
		name          string
		initObjects   []runtime.Object
		wantConfigMap *corev1.ConfigMap
		wantErr       bool
		wantChange    bool
	}{
		{
			name: "only has default alert",
			initObjects: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: configNamespace,
						Name:      defaultAlertName,
					},
					Data: map[string]string{
						alertConfigMapKey: "apiVersion: 1\ngroups:\n  - orgId: 1\n    name: Suspicious policy change\n    folder: Policy\n  - orgId: 1\n    name: Suspicious Cluster Compliance Status Change\n    folder: Policy",
					},
				},
			},
			wantConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: configNamespace,
					Name:      mergedAlertName,
				},
				Data: map[string]string{
					alertConfigMapKey: "apiVersion: 1\ngroups:\n  - orgId: 1\n    name: Suspicious policy change\n    folder: Policy\n  - orgId: 1\n    name: Suspicious Cluster Compliance Status Change\n    folder: Policy",
				},
			},
			wantChange: true,
			wantErr:    false,
		},
		{
			name: "only has default alert and no change",
			initObjects: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: configNamespace,
						Name:      defaultAlertName,
					},
					Data: map[string]string{
						alertConfigMapKey: "apiVersion: 1\ngroups:\n  - orgId: 1\n    name: Suspicious policy change\n    folder: Policy\n  - orgId: 1\n    name: Suspicious Cluster Compliance Status Change\n    folder: Policy",
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: configNamespace,
						Name:      mergedAlertName,
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "operator.open-cluster-management.io/v1alpha4",
								Kind:               "MulticlusterGlobalHub",
								Name:               "test",
								BlockOwnerDeletion: pointer.Bool(true),
								Controller:         pointer.Bool(true),
							},
						},
					},
					Data: map[string]string{
						alertConfigMapKey: "apiVersion: 1\ngroups:\n  - orgId: 1\n    name: Suspicious policy change\n    folder: Policy\n  - orgId: 1\n    name: Suspicious Cluster Compliance Status Change\n    folder: Policy",
					},
				},
			},
			wantConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: configNamespace,
					Name:      mergedAlertName,
				},
				Data: map[string]string{
					alertConfigMapKey: "apiVersion: 1\ngroups:\n  - orgId: 1\n    name: Suspicious policy change\n    folder: Policy\n  - orgId: 1\n    name: Suspicious Cluster Compliance Status Change\n    folder: Policy",
				},
			},
			wantChange: false,
			wantErr:    false,
		},
		{
			name: "Has default alert and custom",
			initObjects: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: configNamespace,
						Name:      defaultAlertName,
					},
					Data: map[string]string{
						alertConfigMapKey: "apiVersion: 1\ngroups:\n  - orgId: 1\n    name: Suspicious policy change\n    folder: Policy\n  - orgId: 1\n    name: Suspicious Cluster Compliance Status Change\n    folder: Policy",
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: configNamespace,
						Name:      operatorconstants.CustomAlertName,
					},
					Data: map[string]string{
						alertConfigMapKey: "apiVersion: 1\ngroups:\n  - orgId: 1\n    name: Suspicious policy change\n    folder: Custom\n  - orgId: 1\n    name: Suspicious Cluster Compliance Status Change\n    folder: Custom\ncontactPoints:\n  - orgId: 1\n    name: alerts-cu-webhook\n    receivers:\n      - uid: 4e3bfe25-00cf-4173-b02b-16f077e539da\n        type: email\n        disableResolveMessage: false\npolicies:\n  - orgId: 1\n    receiver: alerts-cu-webhook",
					},
				},
			},
			wantConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: configNamespace,
					Name:      mergedAlertName,
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
  name: Suspicious policy change
  orgId: 1
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
			},
			wantErr:    false,
			wantChange: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := globalhubv1alpha4.AddToScheme(scheme.Scheme)
			if err != nil {
				t.Error("Failed to add scheme")
			}
			kubeClient := fakekube.NewSimpleClientset(tt.initObjects...)
			r := &MulticlusterGlobalHubReconciler{
				KubeClient: kubeClient,
				Scheme:     scheme.Scheme,
			}
			ctx := context.Background()
			changed, err := r.generateAlertConfigMap(ctx, mgh)
			if (err != nil) != tt.wantErr {
				t.Errorf("MulticlusterGlobalHubReconciler.generateAlertConfigMap() error = %v, wantErr %v", err, tt.wantErr)
			}
			if changed != tt.wantChange {
				t.Errorf("Changed:%v, wantChanged:%v", changed, tt.wantChange)
			}
			existConfigMap, err := kubeClient.CoreV1().ConfigMaps(configNamespace).Get(ctx, mergedAlertName, metav1.GetOptions{})
			if err != nil {
				t.Errorf("Failed to get merged configmap. Err:%v", err)
			}
			if len(existConfigMap.Data[alertConfigMapKey]) != len(tt.wantConfigMap.Data[alertConfigMapKey]) {
				t.Errorf("len(existConfigMap.Data[alertConfigMapKey]):%v, len(tt.wantConfigMap.Data[alertConfigMapKey]):%v", len(existConfigMap.Data[alertConfigMapKey]), len(tt.wantConfigMap.Data[alertConfigMapKey]))
			}
		})
	}
}

func TestRestartGrafanaPod(t *testing.T) {
	ctx := context.Background()
	configNamespace := config.GetDefaultNamespace()

	tests := []struct {
		name        string
		initObjects []runtime.Object
		wantErr     bool
	}{
		{
			name:        "no grafana pods",
			initObjects: []runtime.Object{},
			wantErr:     false,
		},
		{
			name: "has grafana pods",
			initObjects: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: configNamespace,
						Name:      grafanaDeploymentName + "xxx",
						Labels: map[string]string{
							"name": grafanaDeploymentName,
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		kubeClient := fakekube.NewSimpleClientset(tt.initObjects...)
		t.Run(tt.name, func(t *testing.T) {
			if err := restartGrafanaPod(ctx, kubeClient); (err != nil) != tt.wantErr {
				t.Errorf("RestartGrafanaPod() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_generateGranafaIni(t *testing.T) {
	configNamespace := config.GetDefaultNamespace()
	mgh := &globalhubv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "open-cluster-management",
		},
		Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{
			DataLayer: globalhubv1alpha4.DataLayerConfig{},
		},
	}
	tests := []struct {
		name        string
		initObjects []runtime.Object
		wantSecret  *corev1.Secret
		wantChange  bool
		wantErr     bool
	}{
		{
			name:       "No custom grafana.ini",
			wantSecret: nil,
			wantChange: false,
			wantErr:    true,
		},
		{
			name: "only has default grafana.ini",
			initObjects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: configNamespace,
						Name:      defaultGrafanaIniName,
						Labels: map[string]string{
							"name": "multicluster-global-hub-grafana",
						},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "operator.open-cluster-management.io/v1alpha4",
								Kind:               "MulticlusterGlobalHub",
								Name:               "test",
								BlockOwnerDeletion: pointer.Bool(true),
								Controller:         pointer.Bool(true),
							},
						},
					},
					Data: map[string][]byte{
						grafanaIniKey: []byte("    [auth]\n    disable_login_form = true\n    disable_signout_menu = true\n    [auth.basic]\n    enabled = false\n    [auth.proxy]\n    auto_sign_up = true\n    enabled = true\n    header_name = X-Forwarded-User\n    [paths]\n    data = /var/lib/grafana\n    logs = /var/lib/grafana/logs\n    plugins = /var/lib/grafana/plugins\n    provisioning = /etc/grafana/provisioning\n    [security]\n    admin_user = WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000\n    cookie_secure = true\n    [server]\n    http_port = 3001\n    #root_url = %(protocol)s://%(domain)s/grafana/\n    #domain = localhost\n    [users]\n    viewers_can_edit = true\n    [alerting]\n    enabled = true\n    execute_alerts = true\n    [dataproxy]\n    timeout = 300\n    dial_timeout = 30\n    keep_alive_seconds = 300\n    [dashboards]\n    default_home_dashboard_path = /grafana-dashboards/0/acm-global-policy-group-compliancy-overview/acm-global-policy-group-compliancy-overview.json"),
					},
				},
			},
			wantSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: configNamespace,
					Name:      mergedGrafanaIniName,
					Labels: map[string]string{
						"name": "multicluster-global-hub-grafana",
					},
				},
				Data: map[string][]byte{
					grafanaIniKey: []byte("    [auth]\n    disable_login_form = true\n    disable_signout_menu = true\n    [auth.basic]\n    enabled = false\n    [auth.proxy]\n    auto_sign_up = true\n    enabled = true\n    header_name = X-Forwarded-User\n    [paths]\n    data = /var/lib/grafana\n    logs = /var/lib/grafana/logs\n    plugins = /var/lib/grafana/plugins\n    provisioning = /etc/grafana/provisioning\n    [security]\n    admin_user = WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000\n    cookie_secure = true\n    [server]\n    http_port = 3001\n    #root_url = %(protocol)s://%(domain)s/grafana/\n    #domain = localhost\n    [users]\n    viewers_can_edit = true\n    [alerting]\n    enabled = true\n    execute_alerts = true\n    [dataproxy]\n    timeout = 300\n    dial_timeout = 30\n    keep_alive_seconds = 300\n    [dashboards]\n    default_home_dashboard_path = /grafana-dashboards/0/acm-global-policy-group-compliancy-overview/acm-global-policy-group-compliancy-overview.json"),
				},
			},
			wantChange: true,
			wantErr:    false,
		},
		{
			name: "has both default and custom grafana.ini",
			initObjects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: configNamespace,
						Name:      defaultGrafanaIniName,
						Labels: map[string]string{
							"name": "multicluster-global-hub-grafana",
						},
					},
					Data: map[string][]byte{
						grafanaIniKey: []byte("    [auth]\n    disable_login_form = true\n    disable_signout_menu = true\n    [auth.basic]\n    enabled = false\n    [auth.proxy]\n    auto_sign_up = true\n    enabled = true\n    header_name = X-Forwarded-User\n    [paths]\n    data = /var/lib/grafana\n    logs = /var/lib/grafana/logs\n    plugins = /var/lib/grafana/plugins\n    provisioning = /etc/grafana/provisioning\n    [security]\n    admin_user = WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000\n    cookie_secure = true\n    [server]\n    http_port = 3001\n    #root_url = %(protocol)s://%(domain)s/grafana/\n    #domain = localhost\n    [users]\n    viewers_can_edit = true\n    [alerting]\n    enabled = true\n    execute_alerts = true\n    [dataproxy]\n    timeout = 300\n    dial_timeout = 30\n    keep_alive_seconds = 300\n    [dashboards]\n    default_home_dashboard_path = /grafana-dashboards/0/acm-global-policy-group-compliancy-overview/acm-global-policy-group-compliancy-overview.json"),
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: configNamespace,
						Name:      operatorconstants.CustomGrafanaIniName,
					},
					Data: map[string][]byte{
						grafanaIniKey: []byte("    [smtp]\n    email = example@redhat.com"),
					},
				},
			},
			wantSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: configNamespace,
					Name:      mergedGrafanaIniName,
					Labels: map[string]string{
						"name": "multicluster-global-hub-grafana",
					},
				},
				Data: map[string][]byte{
					grafanaIniKey: []byte("    [auth]\n    disable_login_form = true\n    disable_signout_menu = true\n    [auth.basic]\n    enabled = false\n    [auth.proxy]\n    auto_sign_up = true\n    enabled = true\n    header_name = X-Forwarded-User\n    [paths]\n    data = /var/lib/grafana\n    logs = /var/lib/grafana/logs\n    plugins = /var/lib/grafana/plugins\n    provisioning = /etc/grafana/provisioning\n    [security]\n    admin_user = WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000\n    cookie_secure = true\n    [server]\n    http_port = 3001\n    #root_url = %(protocol)s://%(domain)s/grafana/\n    #domain = localhost\n    [users]\n    viewers_can_edit = true\n    [alerting]\n    enabled = true\n    execute_alerts = true\n    [dataproxy]\n    timeout = 300\n    dial_timeout = 30\n    keep_alive_seconds = 300\n    [dashboards]\n    default_home_dashboard_path = /grafana-dashboards/0/acm-global-policy-group-compliancy-overview/acm-global-policy-group-compliancy-overview.json\n    [smtp]\n    email = example@redhat.com"),
				},
			},
			wantChange: true,
			wantErr:    false,
		},
		{
			name: "has both default and custom grafana.ini, do not want change",
			initObjects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: configNamespace,
						Name:      defaultGrafanaIniName,
						Labels: map[string]string{
							"name": "multicluster-global-hub-grafana",
						},
					},
					Data: map[string][]byte{
						grafanaIniKey: []byte("    [auth]\n    disable_login_form = true\n    disable_signout_menu = true\n    [auth.basic]\n    enabled = false\n    [auth.proxy]\n    auto_sign_up = true\n    enabled = true\n    header_name = X-Forwarded-User\n    [paths]\n    data = /var/lib/grafana\n    logs = /var/lib/grafana/logs\n    plugins = /var/lib/grafana/plugins\n    provisioning = /etc/grafana/provisioning\n    [security]\n    admin_user = WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000\n    cookie_secure = true\n    [server]\n    http_port = 3001\n    #root_url = %(protocol)s://%(domain)s/grafana/\n    #domain = localhost\n    [users]\n    viewers_can_edit = true\n    [alerting]\n    enabled = true\n    execute_alerts = true\n    [dataproxy]\n    timeout = 300\n    dial_timeout = 30\n    keep_alive_seconds = 300\n    [dashboards]\n    default_home_dashboard_path = /grafana-dashboards/0/acm-global-policy-group-compliancy-overview/acm-global-policy-group-compliancy-overview.json"),
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: configNamespace,
						Name:      operatorconstants.CustomGrafanaIniName,
					},
					Data: map[string][]byte{
						grafanaIniKey: []byte("    [smtp]\n    email = example@redhat.com"),
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: configNamespace,
						Name:      mergedGrafanaIniName,
						Labels: map[string]string{
							"name": "multicluster-global-hub-grafana",
						},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "operator.open-cluster-management.io/v1alpha4",
								Kind:               "MulticlusterGlobalHub",
								Name:               "test",
								BlockOwnerDeletion: pointer.Bool(true),
								Controller:         pointer.Bool(true),
							},
						},
					},
					Data: map[string][]byte{
						grafanaIniKey: []byte("    [auth]\n    disable_login_form = true\n    disable_signout_menu = true\n    [auth.basic]\n    enabled = false\n    [auth.proxy]\n    auto_sign_up = true\n    enabled = true\n    header_name = X-Forwarded-User\n    [paths]\n    data = /var/lib/grafana\n    logs = /var/lib/grafana/logs\n    plugins = /var/lib/grafana/plugins\n    provisioning = /etc/grafana/provisioning\n    [security]\n    admin_user = WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000\n    cookie_secure = true\n    [server]\n    http_port = 3001\n    #root_url = %(protocol)s://%(domain)s/grafana/\n    #domain = localhost\n    [users]\n    viewers_can_edit = true\n    [alerting]\n    enabled = true\n    execute_alerts = true\n    [dataproxy]\n    timeout = 300\n    dial_timeout = 30\n    keep_alive_seconds = 300\n    [dashboards]\n    default_home_dashboard_path = /grafana-dashboards/0/acm-global-policy-group-compliancy-overview/acm-global-policy-group-compliancy-overview.json\n    [smtp]\n    email = example@redhat.com"),
					},
				},
			},
			wantSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: configNamespace,
					Name:      mergedGrafanaIniName,
					Labels: map[string]string{
						"name": "multicluster-global-hub-grafana",
					},
				},
				Data: map[string][]byte{
					grafanaIniKey: []byte("    [auth]\n    disable_login_form = true\n    disable_signout_menu = true\n    [auth.basic]\n    enabled = false\n    [auth.proxy]\n    auto_sign_up = true\n    enabled = true\n    header_name = X-Forwarded-User\n    [paths]\n    data = /var/lib/grafana\n    logs = /var/lib/grafana/logs\n    plugins = /var/lib/grafana/plugins\n    provisioning = /etc/grafana/provisioning\n    [security]\n    admin_user = WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000\n    cookie_secure = true\n    [server]\n    http_port = 3001\n    #root_url = %(protocol)s://%(domain)s/grafana/\n    #domain = localhost\n    [users]\n    viewers_can_edit = true\n    [alerting]\n    enabled = true\n    execute_alerts = true\n    [dataproxy]\n    timeout = 300\n    dial_timeout = 30\n    keep_alive_seconds = 300\n    [dashboards]\n    default_home_dashboard_path = /grafana-dashboards/0/acm-global-policy-group-compliancy-overview/acm-global-policy-group-compliancy-overview.json\n    [smtp]\n    email = example@redhat.com"),
				},
			},
			wantChange: false,
			wantErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := globalhubv1alpha4.AddToScheme(scheme.Scheme)
			if err != nil {
				t.Error("Failed to add scheme")
			}
			kubeClient := fakekube.NewSimpleClientset(tt.initObjects...)
			r := &MulticlusterGlobalHubReconciler{
				KubeClient: kubeClient,
				Scheme:     scheme.Scheme,
			}

			ctx := context.Background()
			got, err := r.generateGrafanaIni(ctx, mgh)

			if (err != nil) != tt.wantErr {
				t.Errorf("generateGranafaIni() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if got != tt.wantChange {
				t.Errorf("generateGranafaIni() got change = %v, wantChange %v", got, tt.wantChange)
				return
			}
			if tt.wantSecret == nil {
				return
			}
			mergedGrafanaIniSecret, err := r.KubeClient.CoreV1().Secrets(configNamespace).Get(ctx, mergedGrafanaIniName, metav1.GetOptions{})
			if err != nil {
				t.Errorf("failed to get merged grafana.ini secret. Namespace:%v, Name:%v, Error: %v", configNamespace, defaultGrafanaIniName, err)
				return
			}
			if string(tt.wantSecret.Data[grafanaIniKey]) != string(mergedGrafanaIniSecret.Data[grafanaIniKey]) {
				t.Errorf("generateGranafaIni want secret = %v, got %v", string(tt.wantSecret.Data[grafanaIniKey]), string(mergedGrafanaIniSecret.Data[grafanaIniKey]))
			}
		})
	}
}

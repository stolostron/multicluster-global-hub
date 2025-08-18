package grafana

import (
	"context"
	"testing"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/stretchr/testify/assert"
	"gopkg.in/ini.v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	fakekube "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	operatorutils "github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func TestMergeAlertConfigMap(t *testing.T) {
	configNamespace := utils.GetDefaultNamespace()

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
					AlertConfigMapKey: "apiVersion: 1\ngroups:\n  - orgId: 1\n    name: Suspicious policy change\n    folder: Policy\n  - orgId: 1\n    name: Suspicious Cluster Compliance Status Change\n    folder: Policy",
				},
			}, customAlertConfigMap: nil,
			want: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: configNamespace,
					Name:      mergedAlertName,
				},
				Data: map[string]string{
					AlertConfigMapKey: "apiVersion: 1\ngroups:\n  - orgId: 1\n    name: Suspicious policy change\n    folder: Policy\n  - orgId: 1\n    name: Suspicious Cluster Compliance Status Change\n    folder: Policy",
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
					AlertConfigMapKey: "apiVersion: 1\ngroups:\n  - orgId: 1\n    name: Suspicious policy change\n    folder: Policy\n  - orgId: 1\n    name: Suspicious Cluster Compliance Status Change\n    folder: Policy",
				},
			},
			customAlertConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: configNamespace,
					Name:      "custom-alert",
				},
				Data: map[string]string{
					AlertConfigMapKey: "apiVersion: 1\ngroups:\n  - orgId: 1\n    name: Suspicious policy change\n    folder: Custom\n  - orgId: 1\n    name: Suspicious Cluster Compliance Status Change\n    folder: Custom\ncontactPoints:\n  - orgId: 1\n    name: alerts-cu-webhook\n    receivers:\n      - uid: 4e3bfe25-00cf-4173-b02b-16f077e539da\n        type: email\n        disableResolveMessage: false\npolicies:\n  - orgId: 1\n    receiver: alerts-cu-webhook",
				},
			},
			want: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: configNamespace,
					Name:      mergedAlertName,
				},
				Data: map[string]string{
					AlertConfigMapKey: `
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
			got, _ := mergeAlertConfigMap(tt.defaultAlertConfigMap, tt.customAlertConfigMap)
			if got == nil || tt.want == nil {
				if got != tt.want {
					t.Errorf("want:%v, got:%v", tt.want, got)
				}
			} else if len(got.Data[AlertConfigMapKey]) != len(tt.want.Data[AlertConfigMapKey]) {
				t.Errorf("mergedAlertConfigMap() = %v, want %v", len(got.Data[AlertConfigMapKey]), len(tt.want.Data[AlertConfigMapKey]))
			}
		})
	}
}

func TestGenerateAlertConfigMap(t *testing.T) {
	configNamespace := utils.GetDefaultNamespace()

	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "multicluster-global-hub",
		},
		Spec: v1alpha4.MulticlusterGlobalHubSpec{
			DataLayerSpec: v1alpha4.DataLayerSpec{},
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
						Name:      DefaultAlertName,
					},
					Data: map[string]string{
						AlertConfigMapKey: "apiVersion: 1\ngroups:\n  - orgId: 1\n    name: Suspicious policy change\n    folder: Policy\n  - orgId: 1\n    name: Suspicious Cluster Compliance Status Change\n    folder: Policy",
					},
				},
			},
			wantConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: configNamespace,
					Name:      mergedAlertName,
				},
				Data: map[string]string{
					AlertConfigMapKey: "apiVersion: 1\ngroups:\n  - orgId: 1\n    name: Suspicious policy change\n    folder: Policy\n  - orgId: 1\n    name: Suspicious Cluster Compliance Status Change\n    folder: Policy",
				},
			},
			wantChange: true,
			wantErr:    false,
		},
		{
			name: "custom alert is invalid",
			initObjects: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: configNamespace,
						Name:      DefaultAlertName,
					},
					Data: map[string]string{
						AlertConfigMapKey: "apiVersion: 1\ngroups:\n  - orgId: 1\n    name: Suspicious policy change\n    folder: Policy\n  - orgId: 1\n    name: Suspicious Cluster Compliance Status Change\n    folder: Policy",
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: configNamespace,
						Name:      constants.CustomAlertName,
					},
					Data: map[string]string{
						AlertConfigMapKey: "- orgId: 1\n	name: Suspicious policy change\n    folder: Custom\n  - orgId: 1\n    name: Suspicious Cluster Compliance Status Change\n    folder: Custom\ncontactPoints:\n  - orgId: 1\n    name: alerts-cu-webhook\n    receivers:\n      - uid: 4e3bfe25-00cf-4173-b02b-16f077e539da\n        type: email\n        disableResolveMessage: false\npolicies:\n  - orgId: 1\n    receiver: alerts-cu-webhook",
					},
				},
			},
			wantConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: configNamespace,
					Name:      mergedAlertName,
				},
				Data: map[string]string{
					AlertConfigMapKey: "apiVersion: 1\ngroups:\n  - orgId: 1\n    name: Suspicious policy change\n    folder: Policy\n  - orgId: 1\n    name: Suspicious Cluster Compliance Status Change\n    folder: Policy",
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
						Name:      DefaultAlertName,
					},
					Data: map[string]string{
						AlertConfigMapKey: "apiVersion: 1\ngroups:\n  - orgId: 1\n    name: Suspicious policy change\n    folder: Policy\n  - orgId: 1\n    name: Suspicious Cluster Compliance Status Change\n    folder: Policy",
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
								BlockOwnerDeletion: ptr.To(true),
								Controller:         ptr.To(true),
							},
						},
					},
					Data: map[string]string{
						AlertConfigMapKey: "apiVersion: 1\ngroups:\n  - orgId: 1\n    name: Suspicious policy change\n    folder: Policy\n  - orgId: 1\n    name: Suspicious Cluster Compliance Status Change\n    folder: Policy",
					},
				},
			},
			wantConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: configNamespace,
					Name:      mergedAlertName,
				},
				Data: map[string]string{
					AlertConfigMapKey: "apiVersion: 1\ngroups:\n  - orgId: 1\n    name: Suspicious policy change\n    folder: Policy\n  - orgId: 1\n    name: Suspicious Cluster Compliance Status Change\n    folder: Policy",
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
						Name:      DefaultAlertName,
					},
					Data: map[string]string{
						AlertConfigMapKey: "apiVersion: 1\ngroups:\n  - orgId: 1\n    name: Suspicious policy change\n    folder: Policy\n  - orgId: 1\n    name: Suspicious Cluster Compliance Status Change\n    folder: Policy",
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: configNamespace,
						Name:      constants.CustomAlertName,
					},
					Data: map[string]string{
						AlertConfigMapKey: "apiVersion: 1\ngroups:\n  - orgId: 1\n    name: Suspicious policy change\n    folder: Custom\n  - orgId: 1\n    name: Suspicious Cluster Compliance Status Change\n    folder: Custom\ncontactPoints:\n  - orgId: 1\n    name: alerts-cu-webhook\n    receivers:\n      - uid: 4e3bfe25-00cf-4173-b02b-16f077e539da\n        type: email\n        disableResolveMessage: false\npolicies:\n  - orgId: 1\n    receiver: alerts-cu-webhook",
					},
				},
			},
			wantConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: configNamespace,
					Name:      mergedAlertName,
				},
				Data: map[string]string{
					AlertConfigMapKey: `
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
			err := v1alpha4.AddToScheme(scheme.Scheme)
			if err != nil {
				t.Error("Failed to add scheme")
			}

			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.initObjects...).Build()
			kubeClient := fakekube.NewSimpleClientset(tt.initObjects...)
			r := &GrafanaReconciler{
				client:     fakeClient,
				kubeClient: kubeClient,
				scheme:     scheme.Scheme,
			}
			ctx := context.Background()
			changed, err := r.generateAlertConfigMap(ctx, mgh)
			if (err != nil) != tt.wantErr {
				t.Errorf("MulticlusterGlobalHubReconciler.generateAlertConfigMap() error = %v, wantErr %v", err, tt.wantErr)
			}
			if changed != tt.wantChange {
				t.Errorf("Changed:%v, wantChanged:%v", changed, tt.wantChange)
			}

			existConfigMap := &corev1.ConfigMap{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Namespace: configNamespace,
				Name:      mergedAlertName,
			}, existConfigMap)
			if err != nil {
				t.Errorf("Failed to get merged configmap. Err:%v", err)
			}
			equal, err := operatorutils.IsAlertGPCcountEqual([]byte(existConfigMap.Data[AlertConfigMapKey]), []byte(tt.wantConfigMap.Data[AlertConfigMapKey]))
			if err != nil || !equal {
				t.Errorf("len(existConfigMap.Data[alertConfigMapKey]):%v, len(tt.wantConfigMap.Data[alertConfigMapKey]):%v", len(existConfigMap.Data[AlertConfigMapKey]), len(tt.wantConfigMap.Data[AlertConfigMapKey]))
			}
		})
	}
}

func TestGenerateGranafaIni(t *testing.T) {
	configNamespace := utils.GetDefaultNamespace()
	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "multicluster-global-hub",
		},
		Spec: v1alpha4.MulticlusterGlobalHubSpec{
			DataLayerSpec: v1alpha4.DataLayerSpec{},
		},
	}
	tests := []struct {
		name        string
		initObjects []runtime.Object
		initRoute   []runtime.Object
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
			initRoute: []runtime.Object{
				&routev1.Route{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: configNamespace,
						Name:      "multicluster-global-hub-grafana",
					},
					Spec: routev1.RouteSpec{
						Host: "grafana.com",
					},
				},
			},
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
								BlockOwnerDeletion: ptr.To(true),
								Controller:         ptr.To(true),
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
						Name:      constants.CustomGrafanaIniName,
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
						Name:      constants.CustomGrafanaIniName,
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
								BlockOwnerDeletion: ptr.To(true),
								Controller:         ptr.To(true),
							},
						},
					},
					Data: map[string][]byte{
						grafanaIniKey: []byte(`
[auth]
disable_login_form   = true
disable_signout_menu = true

[auth.basic]
enabled = false

[auth.proxy]
auto_sign_up = true
enabled      = true
header_name  = X-Forwarded-User

[paths]
data         = /var/lib/grafana
logs         = /var/lib/grafana/logs
plugins      = /var/lib/grafana/plugins
provisioning = /etc/grafana/provisioning

[security]
admin_user    = WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000
cookie_secure = true

[server]
http_port = 3001

# root_url = %(protocol)s://%(domain)s/grafana/
# domain = localhost
[users]
viewers_can_edit = true

[alerting]
enabled        = true
execute_alerts = true

[dataproxy]
timeout            = 300
dial_timeout       = 30
keep_alive_seconds = 300

[dashboards]
default_home_dashboard_path = /grafana-dashboards/0/acm-global-policy-group-compliancy-overview/acm-global-policy-group-compliancy-overview.json

[smtp]
email = example@redhat.com
`),
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Nil(t, v1alpha4.AddToScheme(scheme.Scheme))
			assert.Nil(t, routev1.AddToScheme(scheme.Scheme))

			objs := append(tt.initRoute, tt.initObjects...)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(objs...).Build()

			kubeClient := fakekube.NewSimpleClientset(tt.initObjects...)
			r := &GrafanaReconciler{
				client:     fakeClient,
				kubeClient: kubeClient,
				scheme:     scheme.Scheme,
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
			mergedGrafanaIniSecret := &corev1.Secret{}
			err = r.client.Get(ctx, client.ObjectKeyFromObject(tt.wantSecret), mergedGrafanaIniSecret)
			assert.Nil(t, err)

			if sectionCount(tt.wantSecret.Data[grafanaIniKey]) == -1 || (sectionCount(mergedGrafanaIniSecret.Data[grafanaIniKey]) != sectionCount(tt.wantSecret.Data[grafanaIniKey])) {
				t.Errorf("mergeGrafanaIni() = %v, want %v", sectionCount(mergedGrafanaIniSecret.Data[grafanaIniKey]), sectionCount(tt.wantSecret.Data[grafanaIniKey]))
			}
		})
	}
}

func TestMergeGrafanaIni(t *testing.T) {
	tests := []struct {
		name    string
		a       []byte
		b       []byte
		want    []byte
		wantErr bool
	}{
		{
			name: "only has default",
			a: []byte(`
    [auth]
    disable_login_form = true
    disable_signout_menu = true
    [auth.basic]
    enabled = false
    [auth.proxy]
    auto_sign_up = true
    enabled = true
    header_name = X-Forwarded-User
    [paths]
    data = /var/lib/grafana
    logs = /var/lib/grafana/logs
    plugins = /var/lib/grafana/plugins
    provisioning = /etc/grafana/provisioning
    [security]
    admin_user = WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000
    cookie_secure = true
    [server]
    http_port = 3001
    #root_url = %(protocol)s://%(domain)s/grafana/
    #domain = localhost
    [users]
    viewers_can_edit = true
    [alerting]
    enabled = true
    execute_alerts = true
    [dataproxy]
    timeout = 300
    dial_timeout = 30
    keep_alive_seconds = 300
    [dashboards]
    default_home_dashboard_path = /grafana-dashboards/0/acm-global-policy-group-compliancy-overview/acm-global-policy-group-compliancy-overview.json
`),
			want: []byte(`
[auth]
disable_login_form   = true
disable_signout_menu = true

[auth.basic]
enabled = false

[auth.proxy]
auto_sign_up = true
enabled      = true
header_name  = X-Forwarded-User

[paths]
data         = /var/lib/grafana
logs         = /var/lib/grafana/logs
plugins      = /var/lib/grafana/plugins
provisioning = /etc/grafana/provisioning

[security]
admin_user    = WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000
cookie_secure = true

[server]
http_port = 3001

# root_url = %(protocol)s://%(domain)s/grafana/
# domain = localhost
[users]
viewers_can_edit = true

[alerting]
enabled        = true
execute_alerts = true

[dataproxy]
timeout            = 300
dial_timeout       = 30
keep_alive_seconds = 300

[dashboards]
default_home_dashboard_path = /grafana-dashboards/0/acm-global-policy-group-compliancy-overview/acm-global-policy-group-compliancy-overview.json
`),
			wantErr: false,
		},
		{
			name: "has both default and normal custom value",
			a: []byte(`
    [auth]
    disable_login_form = true
    disable_signout_menu = true
    [auth.basic]
    enabled = false
    [auth.proxy]
    auto_sign_up = true
    enabled = true
    header_name = X-Forwarded-User
    [paths]
    data = /var/lib/grafana
    logs = /var/lib/grafana/logs
    plugins = /var/lib/grafana/plugins
    provisioning = /etc/grafana/provisioning
    [security]
    admin_user = WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000
    cookie_secure = true
    [server]
    http_port = 3001
    #root_url = %(protocol)s://%(domain)s/grafana/
    #domain = localhost
    [users]
    viewers_can_edit = true
    [alerting]
    enabled = true
    execute_alerts = true
    [dataproxy]
    timeout = 300
    dial_timeout = 30
    keep_alive_seconds = 300
    [dashboards]
    default_home_dashboard_path = /grafana-dashboards/0/acm-global-policy-group-compliancy-overview/acm-global-policy-group-compliancy-overview.json
`),

			b: []byte(`
    [smtp]
    user = true
    pass = true
    [slack]
    enabled = false
`),
			want: []byte(`
[auth]
disable_login_form   = true
disable_signout_menu = true

[auth.basic]
enabled = false

[auth.proxy]
auto_sign_up = true
enabled      = true
header_name  = X-Forwarded-User

[paths]
data         = /var/lib/grafana
logs         = /var/lib/grafana/logs
plugins      = /var/lib/grafana/plugins
provisioning = /etc/grafana/provisioning

[security]
admin_user    = WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000
cookie_secure = true

[server]
http_port = 3001

# root_url = %(protocol)s://%(domain)s/grafana/
# domain = localhost
[users]
viewers_can_edit = true

[alerting]
enabled        = true
execute_alerts = true

[dataproxy]
timeout            = 300
dial_timeout       = 30
keep_alive_seconds = 300

[dashboards]
default_home_dashboard_path = /grafana-dashboards/0/acm-global-policy-group-compliancy-overview/acm-global-policy-group-compliancy-overview.json

[smtp]
user = true
pass = true

[slack]
enabled = false
`),
			wantErr: false,
		},
		{
			name: "has both default and custom value, custom has some section in default",
			a: []byte(`
    [auth]
    disable_login_form = true
    disable_signout_menu = true
    [auth.basic]
    enabled = false
    [auth.proxy]
    auto_sign_up = true
    enabled = true
    header_name = X-Forwarded-User
    [paths]
    data = /var/lib/grafana
    logs = /var/lib/grafana/logs
    plugins = /var/lib/grafana/plugins
    provisioning = /etc/grafana/provisioning
    [security]
    admin_user = WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000
    cookie_secure = true
    [server]
    http_port = 3001
    #root_url = %(protocol)s://%(domain)s/grafana/
    #domain = localhost
    [users]
    viewers_can_edit = true
    [alerting]
    enabled = true
    execute_alerts = true
    [dataproxy]
    timeout = 300
    dial_timeout = 30
    keep_alive_seconds = 300
    [dashboards]
    default_home_dashboard_path = /grafana-dashboards/0/acm-global-policy-group-compliancy-overview/acm-global-policy-group-compliancy-overview.json
`),

			b: []byte(`
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
			want: []byte(`
[auth]
disable_login_form   = true
disable_signout_menu = true

[auth.basic]
enabled = false

[auth.proxy]
auto_sign_up = true
enabled      = true
header_name  = X-Forwarded-User

[paths]
data         = /var/lib/grafana
logs         = /var/lib/grafana/logs
plugins      = /var/lib/grafana/plugins
provisioning = /etc/grafana/provisioning

[security]
admin_user    = WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000
cookie_secure = true

[server]
http_port = 3001

# root_url = %(protocol)s://%(domain)s/grafana/
# domain = localhost
[users]
viewers_can_edit = true

[alerting]
enabled        = true
execute_alerts = true

[dataproxy]
timeout            = 300
dial_timeout       = 30
keep_alive_seconds = 300

[dashboards]
default_home_dashboard_path = /grafana-dashboards/0/acm-global-policy-group-compliancy-overview/acm-global-policy-group-compliancy-overview.json

[smtp]
user = true
pass = true
`),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mergeGrafanaIni(tt.a, tt.b)
			if (err != nil) != tt.wantErr {
				t.Errorf("mergeGrafanaIni() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if sectionCount(got) == -1 || (sectionCount(got) != sectionCount(tt.want)) {
				t.Errorf("mergeGrafanaIni() = %v, want %v", sectionCount(got), sectionCount(tt.want))
			}
		})
	}
}

func sectionCount(a []byte) int {
	cfg, err := ini.Load(a)
	if err != nil {
		return -1
	}
	// By Default, There is a DEFAULT section, should not count it
	return len(cfg.Sections()) - 1
}

package grafana

import (
	"context"
	"fmt"
	"testing"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/ini.v1"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	fakekube "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

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

			// F003: generateGrafanaIni must inject a random admin password into grafana.ini.
			iniCfg, err := ini.Load(mergedGrafanaIniSecret.Data[grafanaIniKey])
			require.NoError(t, err, "merged grafana.ini must remain valid INI after reconciliation")
			sec, err := iniCfg.GetSection("security")
			require.NoError(t, err, "merged grafana.ini must retain a security section")
			assert.NotEmpty(t, sec.Key("admin_password").String(),
				"Grafana admin_password must be injected instead of default admin/admin")
		})
	}
}

func defaultGrafanaIniSecretForTest(namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      defaultGrafanaIniName,
			Labels: map[string]string{
				"name": "multicluster-global-hub-grafana",
			},
		},
		Data: map[string][]byte{
			grafanaIniKey: []byte("[security]\nadmin_user = admin\n[server]\nhttp_port = 3001\n"),
		},
	}
}

// TestGenerateGrafanaIniPersistedSecretErrors verifies secret lookup failures do not mint a new password.
func TestGenerateGrafanaIniPersistedSecretErrors(t *testing.T) {
	configNamespace := utils.GetDefaultNamespace()
	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: configNamespace,
		},
		Spec: v1alpha4.MulticlusterGlobalHubSpec{},
	}

	t.Run("merged secret lookup failure", func(t *testing.T) {
		require.NoError(t, v1alpha4.AddToScheme(scheme.Scheme),
			"operator scheme registration must succeed before fake client setup")
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme.Scheme).
			WithRuntimeObjects(defaultGrafanaIniSecretForTest(configNamespace)).
			WithInterceptorFuncs(interceptor.Funcs{
				Get: func(
					ctx context.Context,
					c client.WithWatch,
					key client.ObjectKey,
					obj client.Object,
					opts ...client.GetOption,
				) error {
					if key.Name == mergedGrafanaIniName {
						return fmt.Errorf("simulated merged grafana.ini lookup failure")
					}
					return c.Get(ctx, key, obj, opts...)
				},
			}).
			Build()

		r := &GrafanaReconciler{
			client: fakeClient,
			scheme: scheme.Scheme,
		}

		_, err := r.generateGrafanaIni(context.Background(), mgh)
		require.Error(t, err, "merged grafana.ini lookup failures must abort reconciliation")
		assert.Contains(t, err.Error(), "failed to get merged grafana.ini secret",
			"lookup failures must identify the persisted Grafana secret")
	})

	t.Run("invalid persisted grafana.ini", func(t *testing.T) {
		require.NoError(t, v1alpha4.AddToScheme(scheme.Scheme),
			"operator scheme registration must succeed before fake client setup")
		existingMerged := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: configNamespace,
				Name:      mergedGrafanaIniName,
			},
			Data: map[string][]byte{
				grafanaIniKey: []byte("[security]\nadmin_password = leaked-secret\n[[[broken"),
			},
		}
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme.Scheme).
			WithRuntimeObjects(
				defaultGrafanaIniSecretForTest(configNamespace),
				existingMerged,
			).
			Build()

		r := &GrafanaReconciler{
			client: fakeClient,
			scheme: scheme.Scheme,
		}

		_, err := r.generateGrafanaIni(context.Background(), mgh)
		require.Error(t, err, "invalid persisted grafana.ini must abort reconciliation")
		assert.Contains(t, err.Error(), "failed to parse persisted grafana.ini",
			"persisted INI parse failures must not rotate the Grafana admin password")
		assert.NotContains(t, err.Error(), "leaked-secret",
			"INI parse errors must not include grafana.ini contents")
		assert.NotContains(t, err.Error(), "[[[broken",
			"INI parse errors must not include parser details from grafana.ini")

		unchanged := &corev1.Secret{}
		require.NoError(t, fakeClient.Get(
			context.Background(),
			client.ObjectKeyFromObject(existingMerged),
			unchanged,
		), "failed reconciliation must still leave the persisted grafana.ini secret readable")
		assert.Equal(t, existingMerged.Data[grafanaIniKey], unchanged.Data[grafanaIniKey],
			"failed reconciliation must not overwrite the persisted grafana.ini secret")
	})
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

// TestAdminPasswordFromGrafanaIni verifies persisted password restore and INI error redaction.
func TestAdminPasswordFromGrafanaIni(t *testing.T) {
	t.Run("reads persisted password", func(t *testing.T) {
		password, err := adminPasswordFromGrafanaIni([]byte("[security]\nadmin_password = cached\n"))
		require.NoError(t, err, "valid persisted grafana.ini must parse")
		assert.Equal(t, "cached", password, "persisted Grafana admin password must be restored")
	})

	t.Run("invalid ini", func(t *testing.T) {
		_, err := adminPasswordFromGrafanaIni([]byte("[security]\nadmin_password = leaked-secret\n[[[broken"))
		require.Error(t, err, "invalid persisted grafana.ini must be rejected")
		assert.Equal(t, "failed to load grafana.ini", err.Error(),
			"INI parse failures must use a fixed message without parser details")
		assert.NotContains(t, err.Error(), "leaked-secret",
			"INI parse errors must not include grafana.ini contents")
	})
}

// TestInjectGrafanaAdminPassword verifies admin_password injection and INI error redaction.
func TestInjectGrafanaAdminPassword(t *testing.T) {
	t.Run("sets password in existing security section", func(t *testing.T) {
		merged, err := injectGrafanaAdminPassword([]byte("[security]\nadmin_user = admin\n"))
		require.NoError(t, err, "valid grafana.ini must accept admin password injection")

		cfg, err := ini.Load(merged)
		require.NoError(t, err, "injected grafana.ini must remain valid INI")

		sec, err := cfg.GetSection("security")
		require.NoError(t, err, "injected grafana.ini must keep a security section")
		assert.NotEmpty(t, sec.Key("admin_password").String(),
			"Grafana admin_password must be injected into an existing security section")
	})

	t.Run("creates security section when missing", func(t *testing.T) {
		merged, err := injectGrafanaAdminPassword([]byte("[server]\nhttp_port = 3001\n"))
		require.NoError(t, err, "grafana.ini without security must accept admin password injection")

		cfg, err := ini.Load(merged)
		require.NoError(t, err, "injected grafana.ini must remain valid INI")

		sec, err := cfg.GetSection("security")
		require.NoError(t, err, "injection must create a security section")
		assert.NotEmpty(t, sec.Key("admin_password").String(),
			"Grafana admin_password must be injected when the security section is missing")
	})

	t.Run("invalid ini does not leak contents", func(t *testing.T) {
		_, err := injectGrafanaAdminPassword([]byte("[security]\nadmin_password = leaked-secret\n[[[broken"))
		require.Error(t, err, "invalid grafana.ini must be rejected before password injection")
		assert.Equal(t, "failed to load grafana.ini", err.Error(),
			"INI parse failures must use a fixed message without parser details")
		assert.NotContains(t, err.Error(), "leaked-secret",
			"INI parse errors must not include grafana.ini contents")
	})
}

// F004: postgres connection parsing must not echo credentials in errors.
func TestParsePostgresConnection(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		want    postgresConnectionParams
		wantErr bool
	}{
		{
			name: "valid uri",
			uri:  "postgresql://grafana:secret@pg.example:5432/mydb?sslmode=verify-full",
			want: postgresConnectionParams{
				host: "pg.example:5432", user: "grafana", password: "secret",
				database: "mydb", sslMode: "verify-full",
			},
		},
		{
			name: "default database",
			uri:  "postgresql://grafana:secret@pg.example:5432",
			want: postgresConnectionParams{
				host: "pg.example:5432", user: "grafana", password: "secret", database: "hoh",
			},
		},
		{
			name: "trailing slash keeps default database",
			uri:  "postgresql://grafana:secret@pg.example:5432/",
			want: postgresConnectionParams{
				host: "pg.example:5432", user: "grafana", password: "secret", database: "hoh",
			},
		},
		{
			name:    "missing password",
			uri:     "postgresql://grafana@pg.example:5432/mydb",
			wantErr: true,
		},
		{
			name:    "invalid uri",
			uri:     "://bad-uri",
			wantErr: true,
		},
		{
			name:    "malformed uri with password",
			uri:     "postgresql://grafana:secret-password%zz@db.example:5432/hoh",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePostgresConnection(tt.uri)
			if tt.wantErr {
				require.Error(t, err, "invalid postgres URI %q must be rejected", tt.name)
				assert.NotContains(t, err.Error(), "postgresql://",
					"parse errors must not expose the raw PostgreSQL URI")
				assert.NotContains(t, err.Error(), "secret-password",
					"parse errors must not expose the PostgreSQL password")
				return
			}
			require.NoError(t, err, "valid postgres URI %q must parse", tt.name)
			assert.Equal(t, tt.want, got, "parsed postgres connection fields must match")
		})
	}
}

// F002: Grafana Postgres datasource must verify TLS when a CA cert is configured.
func TestGrafanaDataSource(t *testing.T) {
	uri := "postgresql://grafana:secret@pg.example:5432/mydb?sslmode=verify-full"

	t.Run("without cert", func(t *testing.T) {
		raw, err := GrafanaDataSource(uri, nil, "")
		require.NoError(t, err, "datasource generation without CA must succeed")

		var datasources GrafanaDatasources
		require.NoError(t, yaml.Unmarshal(raw, &datasources), "datasource YAML must unmarshal")
		require.Len(t, datasources.Datasources, 1, "postgres datasource must be present")
		assert.Equal(t, "Global-Hub-DataSource", datasources.Datasources[0].Name,
			"postgres datasource name must match")
		require.NotNil(t, datasources.Datasources[0].JSONData, "postgres JSONData must be set")
		assert.Equal(t, "mydb", datasources.Datasources[0].JSONData.Database,
			"postgres database name must come from the URI")
	})

	t.Run("with cert", func(t *testing.T) {
		raw, err := GrafanaDataSource(uri, []byte("ca-cert"), "")
		require.NoError(t, err, "datasource generation with CA must succeed")

		var datasources GrafanaDatasources
		require.NoError(t, yaml.Unmarshal(raw, &datasources), "datasource YAML must unmarshal")
		require.Len(t, datasources.Datasources, 1, "postgres datasource must be present")

		ds := datasources.Datasources[0]
		require.NotNil(t, ds.JSONData, "postgres JSONData must be set")
		assert.Equal(t, "verify-full", ds.JSONData.SSLMode,
			"CA-backed datasource must keep a verifying sslmode")
		assert.True(t, ds.JSONData.TLSAuth, "CA-backed datasource must enable TLS auth")
		assert.True(t, ds.JSONData.TLSAuthWithCACert, "CA-backed datasource must attach the CA cert")
		assert.False(t, ds.JSONData.TLSSkipVerify, "CA-backed datasource must not skip TLS verify")
		assert.NotEmpty(t, ds.SecureJSONData.TLSCACert, "CA-backed datasource must store the CA cert")
	})

	t.Run("with cert rejects require sslmode", func(t *testing.T) {
		uriRequire := "postgresql://grafana:secret@pg.example:5432/mydb?sslmode=require"
		_, err := GrafanaDataSource(uriRequire, []byte("ca-cert"), "")
		require.Error(t, err, "sslmode=require must be rejected when a CA certificate is configured")
		assert.Contains(t, err.Error(), "verify-ca or verify-full",
			"the error must require a certificate-verifying sslmode")
	})

	t.Run("with cert rejects disable sslmode", func(t *testing.T) {
		uriDisable := "postgresql://grafana:secret@pg.example:5432/mydb?sslmode=disable"
		_, err := GrafanaDataSource(uriDisable, []byte("ca-cert"), "")
		require.Error(t, err, "sslmode=disable must be rejected when a CA certificate is configured")
		assert.Contains(t, err.Error(), "verify-ca or verify-full",
			"the error must require a certificate-verifying sslmode")
	})

	t.Run("with service account token", func(t *testing.T) {
		raw, err := GrafanaDataSource(uri, nil, "token")
		require.NoError(t, err, "datasource generation with a service account token must succeed")

		var datasources GrafanaDatasources
		require.NoError(t, yaml.Unmarshal(raw, &datasources), "datasource YAML must unmarshal")
		require.Len(t, datasources.Datasources, 2, "prometheus datasource must be appended")
		assert.Equal(t, "Prometheus", datasources.Datasources[1].Name,
			"second datasource must be Prometheus")
		assert.Equal(t, "Bearer token", datasources.Datasources[1].SecureJSONData.HttpHeaderValue1,
			"prometheus datasource must use the bearer token header")
	})
}

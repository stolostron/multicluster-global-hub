package status

import (
	"context"
	"reflect"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
)

var (
	name              = "mgh"
	namespace         = "default"
	ready             = "Ready"
	falseStatus       = "False"
	trueStatus        = "True"
	replica     int32 = 2
	now               = metav1.Now()
)

func Test_needUpdatePhase(t *testing.T) {
	tests := []struct {
		name                    string
		mgh                     *v1alpha4.MulticlusterGlobalHub
		desiredComponentsStatus map[string]v1alpha4.StatusCondition
		want                    bool
		want1                   v1alpha4.GlobalHubPhaseType
	}{
		{
			name: "no mgh status, components not ready",
			mgh: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
			},
			desiredComponentsStatus: map[string]v1alpha4.StatusCondition{
				config.COMPONENTS_MANAGER_NAME: {
					Kind:               "Deployment",
					Name:               config.COMPONENTS_MANAGER_NAME,
					Type:               config.COMPONENTS_AVAILABLE,
					Status:             config.CONDITION_STATUS_FALSE,
					Reason:             config.MINIMUM_REPLICAS_UNAVAILABLE,
					Message:            config.MESSAGE_WAIT_CREATED,
					LastTransitionTime: metav1.Time{Time: time.Now()},
				},
				config.COMPONENTS_GRAFANA_NAME: {
					Kind:               "Deployment",
					Name:               config.COMPONENTS_GRAFANA_NAME,
					Type:               config.COMPONENTS_AVAILABLE,
					Status:             config.CONDITION_STATUS_FALSE,
					Reason:             config.COMPONENTS_CREATING,
					Message:            config.MESSAGE_WAIT_CREATED,
					LastTransitionTime: metav1.Time{Time: time.Now()},
				},
			},
			want:  true,
			want1: v1alpha4.GlobalHubProgressing,
		},
		{
			name: "no mgh status, components ready",
			mgh: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Status: v1alpha4.MulticlusterGlobalHubStatus{
					Phase: v1alpha4.GlobalHubProgressing,
				},
			},
			desiredComponentsStatus: map[string]v1alpha4.StatusCondition{
				config.COMPONENTS_MANAGER_NAME: {
					Kind:               "Deployment",
					Name:               config.COMPONENTS_MANAGER_NAME,
					Type:               config.COMPONENTS_AVAILABLE,
					Status:             config.CONDITION_STATUS_TRUE,
					LastTransitionTime: metav1.Time{Time: time.Now()},
				},
				config.COMPONENTS_GRAFANA_NAME: {
					Kind:               "Deployment",
					Name:               config.COMPONENTS_GRAFANA_NAME,
					Type:               config.COMPONENTS_AVAILABLE,
					Status:             config.CONDITION_STATUS_TRUE,
					LastTransitionTime: metav1.Time{Time: time.Now()},
				},
			},
			want:  true,
			want1: v1alpha4.GlobalHubRunning,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desiredComponents = sets.NewString(
				config.COMPONENTS_MANAGER_NAME,
				config.COMPONENTS_GRAFANA_NAME,
			)
			got, got1 := needUpdatePhase(tt.mgh, tt.desiredComponentsStatus)
			if got != tt.want {
				t.Errorf("name:%v, needUpdatePhase() got = %v, want %v", tt.name, got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("name:%v, needUpdatePhase() got1 = %v, want %v", tt.name, got1, tt.want1)
			}
		})
	}
}

func Test_needUpdateComponentsStatus(t *testing.T) {
	tests := []struct {
		name                    string
		currentComponentsStatus map[string]v1alpha4.StatusCondition
		desiredComponentsStatus map[string]v1alpha4.StatusCondition
		want                    bool
		want1                   map[string]v1alpha4.StatusCondition
	}{
		{
			name: "do not exist in current components",
			currentComponentsStatus: map[string]v1alpha4.StatusCondition{
				config.COMPONENTS_MANAGER_NAME: {
					Kind:    "Deployment",
					Name:    config.COMPONENTS_MANAGER_NAME,
					Type:    config.COMPONENTS_AVAILABLE,
					Status:  config.CONDITION_STATUS_FALSE,
					Reason:  config.MINIMUM_REPLICAS_UNAVAILABLE,
					Message: config.MESSAGE_WAIT_CREATED,
				},
			},
			desiredComponentsStatus: map[string]v1alpha4.StatusCondition{
				config.COMPONENTS_MANAGER_NAME: {
					Kind:   "Deployment",
					Name:   config.COMPONENTS_MANAGER_NAME,
					Type:   config.COMPONENTS_AVAILABLE,
					Status: config.CONDITION_STATUS_TRUE,
				},
				config.COMPONENTS_GRAFANA_NAME: {
					Kind:    "Deployment",
					Name:    config.COMPONENTS_GRAFANA_NAME,
					Type:    config.COMPONENTS_AVAILABLE,
					Status:  config.CONDITION_STATUS_FALSE,
					Reason:  config.COMPONENTS_CREATING,
					Message: config.MESSAGE_WAIT_CREATED,
				},
			},
			want: true,
			want1: map[string]v1alpha4.StatusCondition{
				config.COMPONENTS_MANAGER_NAME: {
					Kind:   "Deployment",
					Name:   config.COMPONENTS_MANAGER_NAME,
					Type:   config.COMPONENTS_AVAILABLE,
					Status: config.CONDITION_STATUS_TRUE,
				},
				config.COMPONENTS_GRAFANA_NAME: {
					Kind:    "Deployment",
					Name:    config.COMPONENTS_GRAFANA_NAME,
					Type:    config.COMPONENTS_AVAILABLE,
					Status:  config.CONDITION_STATUS_FALSE,
					Reason:  config.COMPONENTS_CREATING,
					Message: config.MESSAGE_WAIT_CREATED,
				},
			},
		},
		{
			name: "current components equal with desired components",
			currentComponentsStatus: map[string]v1alpha4.StatusCondition{
				config.COMPONENTS_MANAGER_NAME: {
					Kind:    "Deployment",
					Name:    config.COMPONENTS_MANAGER_NAME,
					Type:    config.COMPONENTS_AVAILABLE,
					Status:  config.CONDITION_STATUS_FALSE,
					Reason:  config.MINIMUM_REPLICAS_UNAVAILABLE,
					Message: config.MESSAGE_WAIT_CREATED,
				},
				config.COMPONENTS_GRAFANA_NAME: {
					Kind:    "Deployment",
					Name:    config.COMPONENTS_GRAFANA_NAME,
					Type:    config.COMPONENTS_AVAILABLE,
					Status:  config.CONDITION_STATUS_FALSE,
					Reason:  config.COMPONENTS_CREATING,
					Message: config.MESSAGE_WAIT_CREATED,
				},
			},
			desiredComponentsStatus: map[string]v1alpha4.StatusCondition{
				config.COMPONENTS_MANAGER_NAME: {
					Kind:    "Deployment",
					Name:    config.COMPONENTS_MANAGER_NAME,
					Type:    config.COMPONENTS_AVAILABLE,
					Status:  config.CONDITION_STATUS_FALSE,
					Reason:  config.MINIMUM_REPLICAS_UNAVAILABLE,
					Message: config.MESSAGE_WAIT_CREATED,
				},
				config.COMPONENTS_GRAFANA_NAME: {
					Kind:    "Deployment",
					Name:    config.COMPONENTS_GRAFANA_NAME,
					Type:    config.COMPONENTS_AVAILABLE,
					Status:  config.CONDITION_STATUS_FALSE,
					Reason:  config.COMPONENTS_CREATING,
					Message: config.MESSAGE_WAIT_CREATED,
				},
			},
			want: false,
			want1: map[string]v1alpha4.StatusCondition{
				config.COMPONENTS_MANAGER_NAME: {
					Kind:    "Deployment",
					Name:    config.COMPONENTS_MANAGER_NAME,
					Type:    config.COMPONENTS_AVAILABLE,
					Status:  config.CONDITION_STATUS_FALSE,
					Reason:  config.MINIMUM_REPLICAS_UNAVAILABLE,
					Message: config.MESSAGE_WAIT_CREATED,
				},
				config.COMPONENTS_GRAFANA_NAME: {
					Kind:    "Deployment",
					Name:    config.COMPONENTS_GRAFANA_NAME,
					Type:    config.COMPONENTS_AVAILABLE,
					Status:  config.CONDITION_STATUS_FALSE,
					Reason:  config.COMPONENTS_CREATING,
					Message: config.MESSAGE_WAIT_CREATED,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := needUpdateComponentsStatus(tt.currentComponentsStatus, tt.desiredComponentsStatus)
			if got != tt.want {
				t.Errorf("needUpdateComponentsStatus() got = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("needUpdateComponentsStatus() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func Test_updateStatefulsetComponents(t *testing.T) {
	tests := []struct {
		name             string
		componentsStatus map[string]v1alpha4.StatusCondition
		initObj          []runtime.Object
		wantErr          bool
	}{
		{
			name:    "no statefulset",
			wantErr: false,
		},
		{
			name: "no postgres statefulset",
			initObj: []runtime.Object{
				&appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ss",
						Namespace: namespace,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "have postgres statefulset, but not ready",
			initObj: []runtime.Object{
				&appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      config.COMPONENTS_POSTGRES_NAME,
						Namespace: namespace,
					},
					Spec: appsv1.StatefulSetSpec{
						Replicas: &replica,
					},
				},
				&appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ss",
						Namespace: namespace,
					},
				},
			},
			componentsStatus: map[string]v1alpha4.StatusCondition{
				config.COMPONENTS_POSTGRES_NAME: {
					Kind:   "StatefulSet",
					Name:   config.COMPONENTS_POSTGRES_NAME,
					Type:   config.COMPONENTS_AVAILABLE,
					Status: config.CONDITION_STATUS_FALSE,
				},
			},
			wantErr: false,
		},
		{
			name: "have postgres statefulset, and ready",
			initObj: []runtime.Object{
				&appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      config.COMPONENTS_POSTGRES_NAME,
						Namespace: namespace,
					},
					Spec: appsv1.StatefulSetSpec{
						Replicas: &replica,
					},
					Status: appsv1.StatefulSetStatus{
						AvailableReplicas: 2,
					},
				},
				&appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ss",
						Namespace: namespace,
					},
				},
			},
			componentsStatus: map[string]v1alpha4.StatusCondition{
				config.COMPONENTS_POSTGRES_NAME: {
					Kind:   "StatefulSet",
					Name:   config.COMPONENTS_POSTGRES_NAME,
					Type:   config.COMPONENTS_AVAILABLE,
					Status: config.CONDITION_STATUS_TRUE,
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initComponentsStatus := map[string]v1alpha4.StatusCondition{
				config.COMPONENTS_POSTGRES_NAME: {
					Kind:   "StatefulSet",
					Name:   config.COMPONENTS_POSTGRES_NAME,
					Type:   config.COMPONENTS_AVAILABLE,
					Status: config.CONDITION_STATUS_TRUE,
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.initObj...).Build()
			ctx := context.Background()
			if err := updateStatefulsetComponents(ctx, fakeClient, namespace, initComponentsStatus); (err != nil) != tt.wantErr {
				t.Errorf("updateStatefulsetComponents() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.componentsStatus != nil &&
				tt.componentsStatus[config.COMPONENTS_POSTGRES_NAME].Status != initComponentsStatus[config.COMPONENTS_POSTGRES_NAME].Status {
				t.Errorf("name: %v, updateKafkaComponents() = %v, want %v",
					tt.name, tt.componentsStatus[config.COMPONENTS_POSTGRES_NAME],
					initComponentsStatus[config.COMPONENTS_POSTGRES_NAME])
			}
		})
	}
}

func Test_updateDeploymentComponents(t *testing.T) {
	tests := []struct {
		name             string
		componentsStatus map[string]v1alpha4.StatusCondition
		initObj          []runtime.Object
		wantErr          bool
	}{
		{
			name:    "no deployment",
			wantErr: false,
		},
		{
			name: "no manager deployment",
			initObj: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ss",
						Namespace: namespace,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "have manager deployment, but not ready",
			initObj: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      config.COMPONENTS_MANAGER_NAME,
						Namespace: namespace,
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: &replica,
					},
				},
				&appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ss",
						Namespace: namespace,
					},
				},
			},
			componentsStatus: map[string]v1alpha4.StatusCondition{
				config.COMPONENTS_MANAGER_NAME: {
					Kind:   "Deployment",
					Name:   config.COMPONENTS_MANAGER_NAME,
					Type:   config.COMPONENTS_AVAILABLE,
					Status: config.CONDITION_STATUS_FALSE,
				},
			},
			wantErr: false,
		},
		{
			name: "have manager deployment, and ready",
			initObj: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      config.COMPONENTS_MANAGER_NAME,
						Namespace: namespace,
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: &replica,
					},
					Status: appsv1.DeploymentStatus{
						AvailableReplicas: 2,
					},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ss",
						Namespace: namespace,
					},
				},
			},
			componentsStatus: map[string]v1alpha4.StatusCondition{
				config.COMPONENTS_MANAGER_NAME: {
					Kind:   "Deployment",
					Name:   config.COMPONENTS_MANAGER_NAME,
					Type:   config.COMPONENTS_AVAILABLE,
					Status: config.CONDITION_STATUS_TRUE,
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initComponentsStatus := map[string]v1alpha4.StatusCondition{
				config.COMPONENTS_MANAGER_NAME: {
					Kind:   "Deployment",
					Name:   config.COMPONENTS_MANAGER_NAME,
					Type:   config.COMPONENTS_AVAILABLE,
					Status: config.CONDITION_STATUS_TRUE,
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.initObj...).Build()
			ctx := context.Background()
			if err := updateDeploymentComponents(ctx, fakeClient, namespace, initComponentsStatus); (err != nil) != tt.wantErr {
				t.Errorf("updateDeploymentComponents() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.componentsStatus != nil &&
				tt.componentsStatus[config.COMPONENTS_MANAGER_NAME].Status != initComponentsStatus[config.COMPONENTS_MANAGER_NAME].Status {
				t.Errorf("name: %v, updateDeploymentComponents() = %v, want %v",
					tt.name, tt.componentsStatus[config.COMPONENTS_MANAGER_NAME],
					initComponentsStatus[config.COMPONENTS_MANAGER_NAME])
			}
		})
	}
}

func Test_updateRetentionConditions(t *testing.T) {
	tests := []struct {
		name          string
		mgh           *v1alpha4.MulticlusterGlobalHub
		want          bool
		wantCondition []metav1.Condition
	}{
		{
			name: "do not set retention",
			mgh: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
			},
			want: true,
			wantCondition: []metav1.Condition{
				{
					Type:   config.CONDITION_TYPE_DATABASE,
					Status: config.CONDITION_STATUS_FALSE,
				},
			},
		},
		{
			name: "set retention",
			mgh: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: v1alpha4.MulticlusterGlobalHubSpec{
					DataLayerSpec: v1alpha4.DataLayerSpec{
						Postgres: v1alpha4.PostgresSpec{
							Retention: "10m",
						},
					},
				},
			},
			want: true,
			wantCondition: []metav1.Condition{
				{
					Type:   config.CONDITION_TYPE_DATABASE,
					Status: config.CONDITION_STATUS_TRUE,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := updateRetentionConditions(tt.mgh)
			if got != tt.want {
				t.Errorf("updateRetentionConditions() got = %v, want %v", got, tt.want)
			}
			if got1[0].Status != tt.wantCondition[0].Status {
				t.Errorf("updateRetentionConditions() got1 = %v, want %v", got1, tt.wantCondition)
			}
		})
	}
}

func TestStatusReconciler_Reconcile(t *testing.T) {
	tests := []struct {
		name        string
		initObj     []runtime.Object
		want        ctrl.Result
		wantErr     bool
		expectPhase v1alpha4.GlobalHubPhaseType
	}{
		{
			name:    "no mgh",
			want:    ctrl.Result{},
			wantErr: false,
		},
		{
			name: "have init mgh", // requeue, wait kafka crd created
			initObj: []runtime.Object{
				&v1alpha4.MulticlusterGlobalHub{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
					},
				},
			},
			want:        ctrl.Result{},
			wantErr:     false,
			expectPhase: v1alpha4.GlobalHubProgressing,
		},
		{
			name: "delete mgh", // requeue, wait kafka crd created
			initObj: []runtime.Object{
				&v1alpha4.MulticlusterGlobalHub{
					ObjectMeta: metav1.ObjectMeta{
						Name:              name,
						Namespace:         namespace,
						DeletionTimestamp: &now,
						Finalizers: []string{
							"pendingdelete",
						},
					},
					Status: v1alpha4.MulticlusterGlobalHubStatus{
						Phase: v1alpha4.GlobalHubProgressing,
					},
				},
			},
			expectPhase: v1alpha4.GlobalHubUninstalling,
			want:        ctrl.Result{},
			wantErr:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v1alpha4.AddToScheme(scheme.Scheme)
			apiextensionsv1.AddToScheme(scheme.Scheme)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.initObj...).WithStatusSubresource(&v1alpha4.MulticlusterGlobalHub{}).Build()
			ctx := context.Background()
			r := &StatusReconciler{
				Client: fakeClient,
			}
			got, err := r.Reconcile(ctx, ctrl.Request{})
			if (err != nil) != tt.wantErr {
				t.Errorf("Name:%v, StatusReconciler.Reconcile() error = %v, wantErr %v", tt.name, err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Name:%v, StatusReconciler.Reconcile() = %v, want %v", tt.name, got, tt.want)
			}
			if len(tt.initObj) == 0 {
				return
			}
			returnedMgh := &v1alpha4.MulticlusterGlobalHub{}
			err = fakeClient.Get(ctx, r.namespacedName, returnedMgh)
			if err != nil {
				t.Errorf("Failed to get mgh, err:%v", err)
			}
			if returnedMgh.Status.Phase != tt.expectPhase {
				t.Errorf("name: %v, returned phase:%v, expect phase: %v", tt.name, returnedMgh.Status.Phase, tt.expectPhase)
			}
		})
	}
}

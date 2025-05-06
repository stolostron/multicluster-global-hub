package policy

import (
	"context"
	"testing"

	http "github.com/go-kratos/kratos/v2/transport/http"
	kessel "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/resources"
	"github.com/project-kessel/inventory-client-go/v1beta1"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func Test_generateCreateUpdateDeletePolicy(t *testing.T) {
	type args struct {
		data                       []policiesv1.Policy
		leafHubName                string
		policyIdToVersionMapFromDB map[string]models.ResourceVersion
	}
	tests := []struct {
		name  string
		args  args
		want  []policiesv1.Policy
		want1 []policiesv1.Policy
		want2 []models.ResourceVersion
	}{
		{
			name: "empty policy list",
			args: args{
				data:                       []policiesv1.Policy{},
				leafHubName:                "hub1",
				policyIdToVersionMapFromDB: map[string]models.ResourceVersion{},
			},
			want:  []policiesv1.Policy{},
			want1: []policiesv1.Policy{},
			want2: []models.ResourceVersion{},
		},
		{
			name: "new policy to create",
			args: args{
				data: []policiesv1.Policy{
					{
						ObjectMeta: metav1.ObjectMeta{
							UID:             "policy1",
							ResourceVersion: "1",
						},
					},
				},
				leafHubName:                "hub1",
				policyIdToVersionMapFromDB: map[string]models.ResourceVersion{},
			},
			want: []policiesv1.Policy{
				{
					ObjectMeta: metav1.ObjectMeta{
						UID:             "policy1",
						ResourceVersion: "1",
					},
				},
			},
			want1: []policiesv1.Policy{},
			want2: []models.ResourceVersion{},
		},
		{
			name: "policy to update",
			args: args{
				data: []policiesv1.Policy{
					{
						ObjectMeta: metav1.ObjectMeta{
							UID:             "policy1",
							ResourceVersion: "2",
						},
					},
				},
				leafHubName: "hub1",
				policyIdToVersionMapFromDB: map[string]models.ResourceVersion{
					"policy1": {
						Key:             "policy1",
						ResourceVersion: "1",
					},
				},
			},
			want: []policiesv1.Policy{},
			want1: []policiesv1.Policy{
				{
					ObjectMeta: metav1.ObjectMeta{
						UID:             "policy1",
						ResourceVersion: "2",
					},
				},
			},
			want2: []models.ResourceVersion{},
		},
		{
			name: "policy to delete",
			args: args{
				data:        []policiesv1.Policy{},
				leafHubName: "hub1",
				policyIdToVersionMapFromDB: map[string]models.ResourceVersion{
					"policy1": {
						Key:             "policy1",
						ResourceVersion: "1",
					},
				},
			},
			want:  []policiesv1.Policy{},
			want1: []policiesv1.Policy{},
			want2: []models.ResourceVersion{
				{
					Key:             "policy1",
					ResourceVersion: "1",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, got2 := generateCreateUpdateDeletePolicy(logger.ZapLogger("test"), tt.args.data, tt.args.leafHubName, tt.args.policyIdToVersionMapFromDB)
			if !equality.Semantic.DeepEqual(got, tt.want) {
				t.Errorf("generateCreateUpdateDeletePolicy() got = %v, want %v", got, tt.want)
			}
			if !equality.Semantic.DeepEqual(got1, tt.want1) {
				t.Errorf("generateCreateUpdateDeletePolicy() got1 = %v, want %v", got1, tt.want1)
			}
			if !equality.Semantic.DeepEqual(got2, tt.want2) {
				t.Errorf("generateCreateUpdateDeletePolicy() got2 = %v, want %v", got2, tt.want2)
			}
		})
	}
}

func Test_postPolicyToInventoryApi(t *testing.T) {
	type args struct {
		ctx          context.Context
		createPolicy []policiesv1.Policy
		updatePolicy []policiesv1.Policy
		deletePolicy []models.ResourceVersion
		leafHubName  string
		mchVersion   string
	}
	tests := []struct {
		name  string
		args  args
		want  []policiesv1.Policy
		want1 []policiesv1.Policy
		want2 []models.ResourceVersion
	}{
		{
			name: "empty policies",
			args: args{
				ctx:          context.TODO(),
				createPolicy: []policiesv1.Policy{},
				updatePolicy: []policiesv1.Policy{},
				deletePolicy: []models.ResourceVersion{},
				leafHubName:  "hub1",
				mchVersion:   "2.7",
			},
			want:  []policiesv1.Policy{},
			want1: []policiesv1.Policy{},
			want2: []models.ResourceVersion{},
		},
		{
			name: "create policy success",
			args: args{
				ctx: context.TODO(),
				createPolicy: []policiesv1.Policy{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "policy1",
							UID:  "uid1",
							Labels: map[string]string{
								"policy.open-cluster-management.io/cluster-name": "hub1",
							},
							Annotations: map[string]string{
								"policy.open-cluster-management.io/cluster-name": "hub1",
							},
						},
					},
				},
				updatePolicy: []policiesv1.Policy{},
				deletePolicy: []models.ResourceVersion{},
				leafHubName:  "hub1",
				mchVersion:   "2.7",
			},
			want: []policiesv1.Policy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "policy1",
						UID:  "uid1",
						Labels: map[string]string{
							"policy.open-cluster-management.io/cluster-name": "hub1",
						},
						Annotations: map[string]string{
							"policy.open-cluster-management.io/cluster-name": "hub1",
						},
					},
				},
			},
			want1: []policiesv1.Policy{},
			want2: []models.ResourceVersion{},
		},
		{
			name: "update policy success",
			args: args{
				ctx:          context.TODO(),
				createPolicy: []policiesv1.Policy{},
				updatePolicy: []policiesv1.Policy{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "policy1",
							UID:  "uid1",
						},
					},
				},
				deletePolicy: []models.ResourceVersion{},
				leafHubName:  "hub1",
				mchVersion:   "2.7",
			},
			want: []policiesv1.Policy{},
			want1: []policiesv1.Policy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "policy1",
						UID:  "uid1",
					},
				},
			},
			want2: []models.ResourceVersion{},
		},
		{
			name: "update delete policy success",
			args: args{
				ctx:          context.TODO(),
				createPolicy: []policiesv1.Policy{},
				updatePolicy: []policiesv1.Policy{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "policy1",
							UID:  "uid1",
						},
					},
				},
				deletePolicy: []models.ResourceVersion{
					{
						Name: "default/policy1",
					},
				},
				leafHubName: "hub1",
				mchVersion:  "2.7",
			},
			want: []policiesv1.Policy{},
			want1: []policiesv1.Policy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "policy1",
						UID:  "uid1",
					},
				},
			},
			want2: []models.ResourceVersion{
				{
					Name: "default/policy1",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeRequester := &FakeRequester{
				HttpClient: &v1beta1.InventoryHttpClient{
					PolicyServiceClient: &FakeKesselK8SPolicyServiceHTTPClientImpl{},
				},
			}
			h := &localPolicySpecHandler{
				log:       logger.ZapLogger("test"),
				requester: fakeRequester,
			}
			h.postPolicyToInventoryApi(tt.args.ctx, tt.args.createPolicy, tt.args.updatePolicy, tt.args.deletePolicy, tt.args.leafHubName, tt.args.mchVersion)
		})
	}
}

// FakeRequester is a mock implementation of the Requester interface.
type FakeRequester struct {
	HttpClient *v1beta1.InventoryHttpClient
}

// RefreshClient is a mock implementation that simulates refreshing the client.
func (f *FakeRequester) RefreshClient(ctx context.Context, restConfig *transport.RestfulConfig) error {
	// Simulate a successful refresh operation
	return nil
}

// GetHttpClient returns a mock InventoryHttpClient.
func (f *FakeRequester) GetHttpClient() *v1beta1.InventoryHttpClient {
	// Return the fake HTTP client
	return f.HttpClient
}

type FakeKesselK8SPolicyServiceHTTPClientImpl struct{}

func (c *FakeKesselK8SPolicyServiceHTTPClientImpl) CreateK8SPolicy(ctx context.Context, in *kessel.CreateK8SPolicyRequest, opts ...http.CallOption) (*kessel.CreateK8SPolicyResponse, error) {
	return nil, nil
}

func (c *FakeKesselK8SPolicyServiceHTTPClientImpl) DeleteK8SPolicy(ctx context.Context, in *kessel.DeleteK8SPolicyRequest, opts ...http.CallOption) (*kessel.DeleteK8SPolicyResponse, error) {
	return nil, nil
}

func (c *FakeKesselK8SPolicyServiceHTTPClientImpl) UpdateK8SPolicy(ctx context.Context, in *kessel.UpdateK8SPolicyRequest, opts ...http.CallOption) (*kessel.UpdateK8SPolicyResponse, error) {
	return nil, nil
}

func Test_localPolicySpecHandler_syncInventory(t *testing.T) {
	type fields struct {
		log           *zap.SugaredLogger
		eventType     string
		eventSyncMode enum.EventSyncMode
		eventPriority conflator.ConflationPriority
		requester     transport.Requester
	}
	type args struct {
		ctx                        context.Context
		data                       []policiesv1.Policy
		leafHubName                string
		policyIdToVersionMapFromDB map[string]models.ResourceVersion
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "empty policies - no sync needed",
			fields: fields{
				log:       logger.ZapLogger("test"),
				requester: &FakeRequester{},
			},
			args: args{
				ctx:                        context.TODO(),
				data:                       []policiesv1.Policy{},
				leafHubName:                "hub1",
				policyIdToVersionMapFromDB: make(map[string]models.ResourceVersion),
			},
			wantErr: false,
		},
		{
			name: "nil requester returns error",
			fields: fields{
				log:       logger.ZapLogger("test"),
				requester: nil,
			},
			args: args{
				ctx: context.TODO(),
				data: []policiesv1.Policy{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "policy1",
							UID:  "uid1",
						},
					},
				},
				leafHubName:                "hub1",
				policyIdToVersionMapFromDB: make(map[string]models.ResourceVersion),
			},
			wantErr: true,
		},
		{
			name: "successful sync with policies",
			fields: fields{
				log: logger.ZapLogger("test"),
				requester: &FakeRequester{
					HttpClient: &v1beta1.InventoryHttpClient{
						PolicyServiceClient: &FakeKesselK8SPolicyServiceHTTPClientImpl{},
					},
				},
			},
			args: args{
				ctx: context.TODO(),
				data: []policiesv1.Policy{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "policy1",
							UID:  "uid1",
						},
					},
				},
				leafHubName: "hub1",
				policyIdToVersionMapFromDB: map[string]models.ResourceVersion{
					"uid2": {
						Key:  "uid2",
						Name: "policy2",
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &localPolicySpecHandler{
				log:           tt.fields.log,
				eventType:     tt.fields.eventType,
				eventSyncMode: tt.fields.eventSyncMode,
				eventPriority: tt.fields.eventPriority,
				requester:     tt.fields.requester,
			}
			err := h.syncInventory(tt.args.ctx, tt.args.data, tt.args.leafHubName, tt.args.policyIdToVersionMapFromDB)
			if (err != nil) != tt.wantErr {
				t.Errorf("localPolicySpecHandler.syncInventory() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

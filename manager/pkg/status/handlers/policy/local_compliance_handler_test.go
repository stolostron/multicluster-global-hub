package policy

import (
	"context"
	"testing"

	set "github.com/deckarep/golang-set"
	http "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/relationships"
	"github.com/project-kessel/inventory-client-go/v1beta1"
	"go.uber.org/zap"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

type FakeKesselK8SPolicyIsPropagatedToK8SClusterServiceHTTPClientImpl struct{}

func (c *FakeKesselK8SPolicyIsPropagatedToK8SClusterServiceHTTPClientImpl) CreateK8SPolicyIsPropagatedToK8SCluster(ctx context.Context, in *relationships.CreateK8SPolicyIsPropagatedToK8SClusterRequest, opts ...http.CallOption) (*relationships.CreateK8SPolicyIsPropagatedToK8SClusterResponse, error) {
	return nil, nil
}

func (c *FakeKesselK8SPolicyIsPropagatedToK8SClusterServiceHTTPClientImpl) DeleteK8SPolicyIsPropagatedToK8SCluster(ctx context.Context, in *relationships.DeleteK8SPolicyIsPropagatedToK8SClusterRequest, opts ...http.CallOption) (*relationships.DeleteK8SPolicyIsPropagatedToK8SClusterResponse, error) {
	return nil, nil
}

func (c *FakeKesselK8SPolicyIsPropagatedToK8SClusterServiceHTTPClientImpl) UpdateK8SPolicyIsPropagatedToK8SCluster(ctx context.Context, in *relationships.UpdateK8SPolicyIsPropagatedToK8SClusterRequest, opts ...http.CallOption) (*relationships.UpdateK8SPolicyIsPropagatedToK8SClusterResponse, error) {
	return nil, nil
}

func TestPostCompliancesToInventoryApi(t *testing.T) {
	tests := []struct {
		name              string
		createCompliances []models.LocalStatusCompliance
		updateCompliances []models.LocalStatusCompliance
		deleteCompliances []models.LocalStatusCompliance
		wantErr           bool
	}{
		{
			name: "Test successful create/update/delete compliances",
			createCompliances: []models.LocalStatusCompliance{
				{
					LeafHubName: "leaf1",
					PolicyID:    "policy1",
					ClusterName: "cluster1",
					Compliance:  database.Compliant,
				},
			},
			updateCompliances: []models.LocalStatusCompliance{
				{
					LeafHubName: "leaf1",
					PolicyID:    "policy2",
					ClusterName: "cluster2",
					Compliance:  database.NonCompliant,
				},
			},
			deleteCompliances: []models.LocalStatusCompliance{
				{
					LeafHubName: "leaf1",
					PolicyID:    "policy3",
					ClusterName: "cluster3",
				},
			},
			wantErr: false,
		},
		{
			name:              "Test empty compliances",
			createCompliances: []models.LocalStatusCompliance{},
			updateCompliances: []models.LocalStatusCompliance{},
			deleteCompliances: []models.LocalStatusCompliance{},
			wantErr:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRequester := &FakeRequester{
				HttpClient: &v1beta1.InventoryHttpClient{
					K8SPolicyIsPropagatedToK8SClusterServiceHTTPClient: &FakeKesselK8SPolicyIsPropagatedToK8SClusterServiceHTTPClientImpl{},
				},
			}

			err := postCompliancesToInventoryApi(
				"default/policy-name",
				mockRequester,
				"test-hub",
				tt.createCompliances,
				tt.updateCompliances,
				tt.deleteCompliances,
				"2.8.0",
			)

			if (err != nil) != tt.wantErr {
				t.Errorf("postCompliancesToInventoryApi() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestUpdateK8SPolicyIsPropagatedToK8SCluster(t *testing.T) {
	tests := []struct {
		name               string
		subjectId          string
		objectId           string
		status             string
		reporterInstanceId string
		mchVersion         string
	}{
		{
			name:               "Test non_compliant status",
			subjectId:          "subject1",
			objectId:           "object1",
			status:             "non_compliant",
			reporterInstanceId: "reporter1",
			mchVersion:         "2.8.0",
		},
		{
			name:               "Test compliant status",
			subjectId:          "subject2",
			objectId:           "object2",
			status:             "compliant",
			reporterInstanceId: "reporter1",
			mchVersion:         "2.8.0",
		},
		{
			name:               "Test unknown status",
			subjectId:          "subject3",
			objectId:           "object3",
			status:             "unknown",
			reporterInstanceId: "reporter1",
			mchVersion:         "2.8.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := updateK8SPolicyIsPropagatedToK8SCluster(
				tt.subjectId,
				tt.objectId,
				tt.status,
				tt.reporterInstanceId,
				tt.mchVersion,
			)

			if got.K8SpolicyIspropagatedtoK8Scluster.Metadata.RelationshipType != "k8spolicy_ispropagatedto_k8scluster" {
				t.Errorf("updateK8SPolicyIsPropagatedToK8SCluster() RelationshipType = %v, want %v",
					got.K8SpolicyIspropagatedtoK8Scluster.Metadata.RelationshipType, "k8spolicy_ispropagatedto_k8scluster")
			}

			if got.K8SpolicyIspropagatedtoK8Scluster.ReporterData.ReporterInstanceId != tt.reporterInstanceId {
				t.Errorf("updateK8SPolicyIsPropagatedToK8SCluster() ReporterInstanceId = %v, want %v",
					got.K8SpolicyIspropagatedtoK8Scluster.ReporterData.ReporterInstanceId, tt.reporterInstanceId)
			}

			if got.K8SpolicyIspropagatedtoK8Scluster.ReporterData.ReporterVersion != tt.mchVersion {
				t.Errorf("updateK8SPolicyIsPropagatedToK8SCluster() ReporterVersion = %v, want %v",
					got.K8SpolicyIspropagatedtoK8Scluster.ReporterData.ReporterVersion, tt.mchVersion)
			}

			if got.K8SpolicyIspropagatedtoK8Scluster.ReporterData.SubjectLocalResourceId != tt.subjectId {
				t.Errorf("updateK8SPolicyIsPropagatedToK8SCluster() SubjectLocalResourceId = %v, want %v",
					got.K8SpolicyIspropagatedtoK8Scluster.ReporterData.SubjectLocalResourceId, tt.subjectId)
			}

			if got.K8SpolicyIspropagatedtoK8Scluster.ReporterData.ObjectLocalResourceId != tt.objectId {
				t.Errorf("updateK8SPolicyIsPropagatedToK8SCluster() ObjectLocalResourceId = %v, want %v",
					got.K8SpolicyIspropagatedtoK8Scluster.ReporterData.ObjectLocalResourceId, tt.objectId)
			}
		})
	}
}

func TestDeleteK8SPolicyIsPropagatedToK8SCluster(t *testing.T) {
	tests := []struct {
		name               string
		subjectId          string
		objectId           string
		reporterInstanceId string
		mchVersion         string
	}{
		{
			name:               "Test delete request with all fields populated",
			subjectId:          "subject1",
			objectId:           "object1",
			reporterInstanceId: "reporter1",
			mchVersion:         "2.8.0",
		},
		{
			name:               "Test delete request with empty subject",
			subjectId:          "",
			objectId:           "object2",
			reporterInstanceId: "reporter1",
			mchVersion:         "2.8.0",
		},
		{
			name:               "Test delete request with empty object",
			subjectId:          "subject3",
			objectId:           "",
			reporterInstanceId: "reporter1",
			mchVersion:         "2.8.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deleteK8SPolicyIsPropagatedToK8SCluster(
				tt.subjectId,
				tt.objectId,
				tt.reporterInstanceId,
				tt.mchVersion,
			)

			if got.ReporterData.ReporterType != relationships.ReporterData_ACM {
				t.Errorf("deleteK8SPolicyIsPropagatedToK8SCluster() ReporterType = %v, want %v",
					got.ReporterData.ReporterType, relationships.ReporterData_ACM)
			}

			if got.ReporterData.ReporterInstanceId != tt.reporterInstanceId {
				t.Errorf("deleteK8SPolicyIsPropagatedToK8SCluster() ReporterInstanceId = %v, want %v",
					got.ReporterData.ReporterInstanceId, tt.reporterInstanceId)
			}

			if got.ReporterData.ReporterVersion != tt.mchVersion {
				t.Errorf("deleteK8SPolicyIsPropagatedToK8SCluster() ReporterVersion = %v, want %v",
					got.ReporterData.ReporterVersion, tt.mchVersion)
			}

			if got.ReporterData.SubjectLocalResourceId != tt.subjectId {
				t.Errorf("deleteK8SPolicyIsPropagatedToK8SCluster() SubjectLocalResourceId = %v, want %v",
					got.ReporterData.SubjectLocalResourceId, tt.subjectId)
			}

			if got.ReporterData.ObjectLocalResourceId != tt.objectId {
				t.Errorf("deleteK8SPolicyIsPropagatedToK8SCluster() ObjectLocalResourceId = %v, want %v",
					got.ReporterData.ObjectLocalResourceId, tt.objectId)
			}
		})
	}
}

func TestGenerateCreateUpdateDeleteCompliances(t *testing.T) {
	tests := []struct {
		name                     string
		compliancesFromEvents    []models.LocalStatusCompliance
		complianceFromDb         map[database.ComplianceStatus]set.Set
		existClustersOnDB        set.Set
		leafHub                  string
		policyID                 string
		wantCreateComplianceLen  int
		wantUpdateComplianceLen  int
		wantDeleteComplianceLen  int
		wantCreateComplianceName string
		wantUpdateComplianceName string
		wantDeleteComplianceName string
	}{
		{
			name: "Test new compliance creation",
			compliancesFromEvents: []models.LocalStatusCompliance{
				{
					LeafHubName: "hub1",
					PolicyID:    "policy1",
					ClusterName: "cluster1",
					Compliance:  database.Compliant,
				},
			},
			complianceFromDb: map[database.ComplianceStatus]set.Set{},
			existClustersOnDB: set.NewSetFromSlice([]interface{}{
				"cluster2",
			}),
			leafHub:                  "hub1",
			policyID:                 "policy1",
			wantCreateComplianceLen:  1,
			wantUpdateComplianceLen:  0,
			wantDeleteComplianceLen:  1,
			wantCreateComplianceName: "cluster1",
			wantDeleteComplianceName: "cluster2",
		},
		{
			name: "Test compliance update",
			compliancesFromEvents: []models.LocalStatusCompliance{
				{
					LeafHubName: "hub1",
					PolicyID:    "policy1",
					ClusterName: "cluster1",
					Compliance:  database.NonCompliant,
				},
			},
			complianceFromDb: map[database.ComplianceStatus]set.Set{
				database.Compliant: set.NewSetFromSlice([]interface{}{
					"cluster1",
				}),
			},
			existClustersOnDB:        nil,
			leafHub:                  "hub1",
			policyID:                 "policy1",
			wantCreateComplianceLen:  0,
			wantUpdateComplianceLen:  1,
			wantDeleteComplianceLen:  0,
			wantUpdateComplianceName: "cluster1",
		},
		{
			name:                    "Test empty inputs",
			compliancesFromEvents:   []models.LocalStatusCompliance{},
			complianceFromDb:        map[database.ComplianceStatus]set.Set{},
			existClustersOnDB:       set.NewSet(),
			leafHub:                 "hub1",
			policyID:                "policy1",
			wantCreateComplianceLen: 0,
			wantUpdateComplianceLen: 0,
			wantDeleteComplianceLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creates, updates, deletes := generateCreateUpdateDeleteCompliances(
				tt.compliancesFromEvents,
				tt.complianceFromDb,
				tt.existClustersOnDB,
				tt.leafHub,
				tt.policyID,
			)

			if len(creates) != tt.wantCreateComplianceLen {
				t.Errorf("createCompliances length = %v, want %v", len(creates), tt.wantCreateComplianceLen)
			}
			if len(updates) != tt.wantUpdateComplianceLen {
				t.Errorf("updateCompliances length = %v, want %v", len(updates), tt.wantUpdateComplianceLen)
			}
			if len(deletes) != tt.wantDeleteComplianceLen {
				t.Errorf("deleteCompliances length = %v, want %v", len(deletes), tt.wantDeleteComplianceLen)
			}

			if tt.wantCreateComplianceName != "" && len(creates) > 0 {
				if creates[0].ClusterName != tt.wantCreateComplianceName {
					t.Errorf("createCompliance cluster name = %v, want %v", creates[0].ClusterName, tt.wantCreateComplianceName)
				}
			}
			if tt.wantUpdateComplianceName != "" && len(updates) > 0 {
				if updates[0].ClusterName != tt.wantUpdateComplianceName {
					t.Errorf("updateCompliance cluster name = %v, want %v", updates[0].ClusterName, tt.wantUpdateComplianceName)
				}
			}
			if tt.wantDeleteComplianceName != "" && len(deletes) > 0 {
				if deletes[0].ClusterName != tt.wantDeleteComplianceName {
					t.Errorf("deleteCompliance cluster name = %v, want %v", deletes[0].ClusterName, tt.wantDeleteComplianceName)
				}
			}
		})
	}
}

func TestSyncInventory(t *testing.T) {
	tests := []struct {
		name                       string
		log                        *zap.SugaredLogger
		requester                  transport.Requester
		leafHubName                string
		policyID                   string
		localCompliancesFromEvents []models.LocalStatusCompliance
		complianceFromDb           map[database.ComplianceStatus]set.Set
		allClustersOnDB            set.Set
		wantErr                    bool
	}{
		{
			name: "Test successful sync",
			log:  logger.ZapLogger("test"),
			requester: &FakeRequester{
				HttpClient: &v1beta1.InventoryHttpClient{
					K8SPolicyIsPropagatedToK8SClusterServiceHTTPClient: &FakeKesselK8SPolicyIsPropagatedToK8SClusterServiceHTTPClientImpl{},
				},
			},
			leafHubName: "leaf1",
			policyID:    "policy1",
			localCompliancesFromEvents: []models.LocalStatusCompliance{
				{
					LeafHubName: "leaf1",
					PolicyID:    "policy1",
					ClusterName: "cluster1",
					Compliance:  database.Compliant,
				},
			},
			complianceFromDb: map[database.ComplianceStatus]set.Set{
				database.NonCompliant: set.NewSetFromSlice([]interface{}{"cluster2"}),
			},
			allClustersOnDB: set.NewSetFromSlice([]interface{}{"cluster2"}),
			wantErr:         false,
		},
		{
			name:        "Test requester is nil",
			log:         logger.ZapLogger("test"),
			requester:   nil,
			leafHubName: "leaf1",
			policyID:    "policy1",
			localCompliancesFromEvents: []models.LocalStatusCompliance{
				{
					LeafHubName: "leaf1",
					PolicyID:    "policy1",
					ClusterName: "cluster1",
					Compliance:  database.Compliant,
				},
			},
			complianceFromDb: map[database.ComplianceStatus]set.Set{},
			allClustersOnDB:  set.NewSet(),
			wantErr:          true,
		},
		{
			name: "Test empty inputs",
			log:  logger.ZapLogger("test"),
			requester: &FakeRequester{
				HttpClient: &v1beta1.InventoryHttpClient{
					K8SPolicyIsPropagatedToK8SClusterServiceHTTPClient: &FakeKesselK8SPolicyIsPropagatedToK8SClusterServiceHTTPClientImpl{},
				},
			},
			leafHubName:                "leaf1",
			policyID:                   "policy1",
			localCompliancesFromEvents: []models.LocalStatusCompliance{},
			complianceFromDb:           map[database.ComplianceStatus]set.Set{},
			allClustersOnDB:            set.NewSet(),
			wantErr:                    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := syncInventory(
				tt.log,
				tt.requester,
				tt.leafHubName,
				models.ResourceVersion{
					Key:  tt.policyID,
					Name: "policy-name",
				},
				tt.localCompliancesFromEvents,
				tt.complianceFromDb,
				tt.allClustersOnDB,
			)

			if (err != nil) != tt.wantErr {
				t.Errorf("syncInventory() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

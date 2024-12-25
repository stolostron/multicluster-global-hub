package kessel

import "time"

// https://github.com/project-kessel/inventory-api/issues/243
type ResourceData struct {
	Metadata     ResourceMetadata       `json:"metadata"`
	ReporterData ResourceReporter       `json:"reporter_data"`
	ResourceData map[string]interface{} `json:"resource_data,omitempty"`
}

type ResourceMetadata struct {
	Id           string          `json:"id"`
	ResourceType string          `json:"resource_type"`
	OrgId        string          `json:"org_id"`
	CreatedAt    *time.Time      `json:"created_at,omitempty"`
	UpdatedAt    *time.Time      `json:"updated_at,omitempty"`
	DeletedAt    *time.Time      `json:"deleted_at,omitempty"`
	WorkspaceId  string          `json:"workspace_id"`
	Labels       []ResourceLabel `json:"labels,omitempty"`
}

type ResourceLabel struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type ResourceReporter struct {
	ReporterInstanceId string `json:"reporter_instance_id"`
	ReporterType       string `json:"reporter_type"`
	ConsoleHref        string `json:"console_href"`
	ApiHref            string `json:"api_href"`
	LocalResourceId    string `json:"local_resource_id"`
	ReporterVersion    string `json:"reporter_version"`
}

type RelationshipData struct {
	Metadata     RelationshipMetadata `json:"metadata"`
	ReporterData RelationshipReporter `json:"reporter_data"`
	Relationship Relationship         `json:"relationship_data,omitempty"`
}

type RelationshipMetadata struct {
	Id               int64      `protobuf:"varint,3355,opt,name=id,proto3" json:"id,omitempty"`
	RelationshipType string     `json:"relationship_type"`
	CreatedAt        *time.Time `json:"created_at,omitempty"`
	UpdatedAt        *time.Time `json:"updated_at,omitempty"`
	DeletedAt        *time.Time `json:"deleted_at,omitempty"`
}

type RelationshipReporter struct {
	ReporterType           string `json:"reporter_type"`
	SubjectLocalResourceId string `json:"subject_local_resource_id"`
	ObjectLocalResourceId  string `json:"object_local_resource_id"`
	ReporterVersion        string `json:"reporter_version"`
	ReporterInstanceId     string `json:"reporter_instance_id"`
}

// TODO: Not defined relationship_data in eventing api
// https://github.com/project-kessel/inventory-api/blob/main/internal/eventing/api/event.go#L32
type Relationship struct {
	Status    string `json:"status"`
	PolicyId  int64  `json:"k8s_policy_id"`
	ClusterId int64  `json:"k8s_cluster_id"`
}

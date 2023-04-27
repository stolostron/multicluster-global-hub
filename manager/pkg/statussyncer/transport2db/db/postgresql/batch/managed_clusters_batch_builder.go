package batch

import (
	"fmt"
	"strings"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

const (
	managedClustersJsonbColumnIndex = 3
	managedClustersUUIDColumnIndex  = 1
	managedClustersDeleteRowKey     = "payload->'metadata'->>'name'"
	managedClustersTableColumns     = "cluster_id, leaf_hub_name, payload, error"
)

// NewManagedClustersBatchBuilder creates a new instance of PostgreSQL ManagedClustersBatchBuilder.
func NewManagedClustersBatchBuilder(schema string, tableName string, leafHubName string) *ManagedClustersBatchBuilder {
	tableSpecialColumns := make(map[int]string)
	tableSpecialColumns[managedClustersUUIDColumnIndex] = database.UUID
	tableSpecialColumns[managedClustersJsonbColumnIndex] = database.Jsonb

	builder := &ManagedClustersBatchBuilder{
		baseBatchBuilder: newBaseBatchBuilder(schema, tableName, managedClustersTableColumns,
			tableSpecialColumns, leafHubName, managedClustersDeleteRowKey),
	}

	builder.setUpdateStatementFunc(builder.generateUpdateStatement)

	return builder
}

// ManagedClustersBatchBuilder is the PostgreSQL implementation of the ManagedClustersBatchBuilder interface.
type ManagedClustersBatchBuilder struct {
	*baseBatchBuilder
}

// Insert adds the given (cluster payload, error string) to the batch to be inserted to the db.
func (builder *ManagedClustersBatchBuilder) Insert(clusterID string, payload interface{}, errorString string) {
	builder.insert(clusterID, builder.leafHubName, payload, errorString)
}

// Update adds the given arguments to the batch to update clusterName with the given payload in db.
func (builder *ManagedClustersBatchBuilder) Update(clusterID string, clusterName string, payload interface{}) {
	builder.update(clusterID, builder.leafHubName, payload, clusterName)
}

// Delete adds delete statement to the batch to delete the given cluster from db.
func (builder *ManagedClustersBatchBuilder) Delete(clusterName string) {
	builder.delete(clusterName)
}

// Build builds the batch object.
func (builder *ManagedClustersBatchBuilder) Build() interface{} {
	return builder.build()
}

func (builder *ManagedClustersBatchBuilder) generateUpdateStatement() string {
	var stringBuilder strings.Builder

	stringBuilder.WriteString(fmt.Sprintf("UPDATE %s.%s AS old SET payload=new.payload FROM (values ",
		builder.schema, builder.tableName))

	numberOfColumns := len(builder.updateArgs) / builder.updateRowsCount // total num of args divided by num of rows
	stringBuilder.WriteString(builder.generateInsertOrUpdateArgs(builder.updateRowsCount, numberOfColumns,
		builder.tableSpecialColumns))

	stringBuilder.WriteString(") AS new(leaf_hub_name,payload,cluster_name) ")
	stringBuilder.WriteString("WHERE old.leaf_hub_name=new.leaf_hub_name ")
	stringBuilder.WriteString("AND old.payload->'metadata'->>'name'=new.cluster_name")

	return stringBuilder.String()
}

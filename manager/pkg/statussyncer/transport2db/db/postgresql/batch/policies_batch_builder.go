package batch

import (
	"fmt"
	"strings"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

const (
	policyUUIDColumnIndex                  = 1
	policyDeleteRowKey                     = "id"
	deleteClusterCompliancePrefixArgsCount = 2
	updatePolicyComplianceTypeColumnIndex  = 3
	updateClusterComplianceTypeColumnIndex = 4
	clusterComplianceUpdateArgsCount       = 4
	policyComplianceTableColumns           = "id, cluster_name, leaf_hub_name, error, compliance"
)

// NewPoliciesBatchBuilder creates a new instance of PostgreSQL PoliciesBatchBuilder.
func NewPoliciesBatchBuilder(schema string, tableName string, leafHubName string) *PoliciesBatchBuilder {
	tableSpecialColumns := make(map[int]string)
	tableSpecialColumns[policyUUIDColumnIndex] = database.UUID

	builder := &PoliciesBatchBuilder{
		baseBatchBuilder: newBaseBatchBuilder(schema, tableName, policyComplianceTableColumns,
			tableSpecialColumns, leafHubName, policyDeleteRowKey),
		updateClusterComplianceArgs:      make([]interface{}, 0),
		updateClusterComplianceRowsCount: 0,
		deleteClusterComplianceArgs:      make(map[string][]interface{}),
	}

	builder.setUpdateStatementFunc(builder.generateUpdatePolicyComplianceStatement)

	return builder
}

// PoliciesBatchBuilder is the PostgreSQL implementation of the PoliciesBatchBuilder interface.
type PoliciesBatchBuilder struct {
	*baseBatchBuilder
	updateClusterComplianceArgs      []interface{}
	updateClusterComplianceRowsCount int
	deleteClusterComplianceArgs      map[string][]interface{} // map from policyID to clusters
}

// Insert adds the given (policyID, clusterName, errorString, compliance) to the batch to be inserted to the database.
func (builder *PoliciesBatchBuilder) Insert(policyID string, clusterName string, errorString string,
	compliance database.ComplianceStatus,
) {
	builder.insert(policyID, clusterName, builder.leafHubName, errorString, compliance)
}

// UpdatePolicyCompliance adds the given row args to be updated in the batch.
func (builder *PoliciesBatchBuilder) UpdatePolicyCompliance(policyID string, compliance database.ComplianceStatus) {
	// use the builder base update args to implement the policy updates. for specific clusters rows use different vars.
	builder.update(policyID, builder.leafHubName, compliance)
}

// UpdateClusterCompliance adds the given row args to be updated in the batch.
func (builder *PoliciesBatchBuilder) UpdateClusterCompliance(policyID string, clusterName string,
	compliance database.ComplianceStatus,
) {
	// if adding args will exceeded max args limit, create update statement from current args and zero the count/args.
	if len(builder.updateClusterComplianceArgs)+clusterComplianceUpdateArgsCount >= maxColumnsUpdateInStatement {
		builder.updateBatchItems = append(builder.updateBatchItems, &batchItem{
			query:     builder.generateUpdateClusterComplianceStatement(),
			arguments: builder.updateClusterComplianceArgs,
		})

		builder.updateClusterComplianceArgs = make([]interface{}, 0)
		builder.updateClusterComplianceRowsCount = 0
	}

	builder.updateClusterComplianceArgs = append(builder.updateClusterComplianceArgs, policyID, clusterName,
		builder.leafHubName, compliance)
	builder.updateClusterComplianceRowsCount++
}

// DeletePolicy adds delete statement to the batch to delete the given policyId from database.
func (builder *PoliciesBatchBuilder) DeletePolicy(policyID string) {
	// use the builder base delete args to implement the policy delete. for specific clusters rows use different vars.
	builder.delete(policyID)
}

// DeleteClusterStatus adds delete statement to the batch to delete the given (policyId,clusterName) from database.
func (builder *PoliciesBatchBuilder) DeleteClusterStatus(policyID string, clusterName string) {
	_, found := builder.deleteClusterComplianceArgs[policyID]
	if !found {
		// first args of the delete statement are policyID and leafHubName
		builder.deleteClusterComplianceArgs[policyID] =
			append(make([]interface{}, 0), policyID, builder.leafHubName)
	}

	builder.deleteClusterComplianceArgs[policyID] = append(
		builder.deleteClusterComplianceArgs[policyID], clusterName)
}

// Build builds the batch object.
func (builder *PoliciesBatchBuilder) Build() interface{} {
	batch := builder.build()

	if len(builder.deleteClusterComplianceArgs) > 0 {
		for policyID := range builder.deleteClusterComplianceArgs {
			batch.Queue(builder.generateDeleteClusterComplianceStatement(policyID),
				builder.deleteClusterComplianceArgs[policyID]...)
		}
	}

	if builder.updateClusterComplianceRowsCount > 0 {
		batch.Queue(builder.generateUpdateClusterComplianceStatement(), builder.updateClusterComplianceArgs...)
	}

	return batch
}

func (builder *PoliciesBatchBuilder) generateUpdatePolicyComplianceStatement() string {
	var stringBuilder strings.Builder

	stringBuilder.WriteString(fmt.Sprintf("UPDATE %s.%s AS old SET compliance=new.compliance FROM (values ",
		builder.schema, builder.tableName))

	specialColumns := make(map[int]string)
	specialColumns[policyUUIDColumnIndex] = database.UUID
	specialColumns[updatePolicyComplianceTypeColumnIndex] = fmt.Sprintf("%s.%s", builder.schema,
		database.ComplianceType) // compliance column index

	numberOfColumns := len(builder.updateArgs) / builder.updateRowsCount // total num of args divided by num of rows
	stringBuilder.WriteString(builder.generateInsertOrUpdateArgs(builder.updateRowsCount, numberOfColumns,
		specialColumns))

	stringBuilder.WriteString(") AS new(id,leaf_hub_name,compliance) ")
	stringBuilder.WriteString("WHERE old.id=new.id AND old.leaf_hub_name=new.leaf_hub_name")

	return stringBuilder.String()
}

func (builder *PoliciesBatchBuilder) generateUpdateClusterComplianceStatement() string {
	var stringBuilder strings.Builder

	stringBuilder.WriteString(fmt.Sprintf("UPDATE %s.%s AS old SET compliance=new.compliance FROM (values ",
		builder.schema, builder.tableName))

	specialColumns := make(map[int]string)
	specialColumns[policyUUIDColumnIndex] = database.UUID
	specialColumns[updateClusterComplianceTypeColumnIndex] = fmt.Sprintf("%s.%s", builder.schema,
		database.ComplianceType) // compliance column index

	// num of columns is total num of args divided by num of rows
	columnCount := len(builder.updateClusterComplianceArgs) /
		builder.updateClusterComplianceRowsCount
	stringBuilder.WriteString(builder.generateInsertOrUpdateArgs(
		builder.updateClusterComplianceRowsCount, columnCount,
		specialColumns))

	stringBuilder.WriteString(") AS new(id,cluster_name,leaf_hub_name,compliance) ")
	stringBuilder.WriteString("WHERE old.id=new.id AND old.leaf_hub_name=new.leaf_hub_name ")
	stringBuilder.WriteString("AND old.cluster_name=new.cluster_name")

	return stringBuilder.String()
}

func (builder *PoliciesBatchBuilder) generateDeleteClusterComplianceStatement(policyID string) string {
	deletedClustersCount := len(builder.deleteClusterComplianceArgs[policyID]) - deleteClusterCompliancePrefixArgsCount

	return fmt.Sprintf("DELETE from %s.%s WHERE id=$1 AND leaf_hub_name=$2 AND cluster_name IN (%s)",
		builder.schema, builder.tableName, builder.generateArgsList(deletedClustersCount,
			deleteClusterCompliancePrefixArgsCount+1, make(map[int]string)))
}

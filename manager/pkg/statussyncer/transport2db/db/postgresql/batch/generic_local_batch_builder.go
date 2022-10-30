package batch

import (
	"fmt"
	"strings"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

const (
	genericLocalJsonbColumnIndex = 2
	genericLocalDeleteRowKey     = "payload->'metadata'->>'uid'"
)

// NewGenericLocalBatchBuilder creates a new instance of PostgreSQL GenericLocalBatchBuilder.
func NewGenericLocalBatchBuilder(schema string, tableName string, leafHubName string) *GenericLocalBatchBuilder {
	tableSpecialColumns := make(map[int]string)
	tableSpecialColumns[genericLocalJsonbColumnIndex] = database.Jsonb
	builder := &GenericLocalBatchBuilder{
		baseBatchBuilder: newBaseBatchBuilder(schema, tableName, tableSpecialColumns, leafHubName,
			genericLocalDeleteRowKey),
	}

	builder.setUpdateStatementFunc(builder.generateUpdateStatement)

	return builder
}

// GenericLocalBatchBuilder is the PostgreSQL implementation of the GenericLocalBatchBuilder interface.
type GenericLocalBatchBuilder struct {
	*baseBatchBuilder
}

// Insert adds the given payload to the batch to be inserted to the db.
func (builder *GenericLocalBatchBuilder) Insert(payload interface{}) {
	builder.insert(builder.leafHubName, payload)
}

// Update adds the given payload to the batch to be updated in the db.
func (builder *GenericLocalBatchBuilder) Update(payload interface{}) {
	builder.update(builder.leafHubName, payload)
}

// Delete adds the given id to the batch to be deleted from db.
func (builder *GenericLocalBatchBuilder) Delete(id string) {
	builder.delete(id)
}

// Build builds the batch object.
func (builder *GenericLocalBatchBuilder) Build() interface{} {
	return builder.build()
}

func (builder *GenericLocalBatchBuilder) generateUpdateStatement() string {
	var stringBuilder strings.Builder

	stringBuilder.WriteString(fmt.Sprintf("UPDATE %s.%s AS old SET payload=new.payload FROM (values ",
		builder.schema, builder.tableName))

	numberOfColumns := len(builder.updateArgs) / builder.updateRowsCount // total num of args divided by num of rows
	stringBuilder.WriteString(builder.generateInsertOrUpdateArgs(builder.updateRowsCount, numberOfColumns,
		builder.tableSpecialColumns))

	stringBuilder.WriteString(") AS new(leaf_hub_name,payload) ")
	stringBuilder.WriteString("WHERE old.leaf_hub_name=new.leaf_hub_name ")
	stringBuilder.WriteString("AND old.payload->'metadata'->>'uid'=new.payload->'metadata'->>'uid'")

	return stringBuilder.String()
}

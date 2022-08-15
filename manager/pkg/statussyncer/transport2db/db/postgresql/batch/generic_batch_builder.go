package batch

import (
	"fmt"
	"strings"

	"github.com/stolostron/multicluster-globalhub/manager/pkg/statussyncer/transport2db/db"
)

const (
	genericUUIDColumnIndex  = 1
	genericJsonbColumnIndex = 3
	genericDeleteRowKey     = "id"
)

// NewGenericBatchBuilder creates a new instance of PostgreSQL GenericBatchBuilder.
func NewGenericBatchBuilder(schema string, tableName string, leafHubName string) *GenericBatchBuilder {
	tableSpecialColumns := make(map[int]string)
	tableSpecialColumns[genericUUIDColumnIndex] = db.UUID
	tableSpecialColumns[genericJsonbColumnIndex] = db.Jsonb
	builder := &GenericBatchBuilder{
		baseBatchBuilder: newBaseBatchBuilder(schema, tableName, tableSpecialColumns, leafHubName,
			genericDeleteRowKey),
	}

	builder.setUpdateStatementFunc(builder.generateUpdateStatement)

	return builder
}

// GenericBatchBuilder is the PostgreSQL implementation of the GenericBatchBuilder interface.
type GenericBatchBuilder struct {
	*baseBatchBuilder
}

// Insert adds the given (id, payload) to the batch to be inserted to the db.
func (builder *GenericBatchBuilder) Insert(id string, payload interface{}) {
	builder.insert(id, builder.leafHubName, payload)
}

// Update adds the given (id, payload) to the batch to be updated in the db.
func (builder *GenericBatchBuilder) Update(id string, payload interface{}) {
	builder.update(id, builder.leafHubName, payload)
}

// Delete adds the given id to the batch to be deleted from db.
func (builder *GenericBatchBuilder) Delete(id string) {
	builder.delete(id)
}

// Build builds the batch object.
func (builder *GenericBatchBuilder) Build() interface{} {
	return builder.build()
}

func (builder *GenericBatchBuilder) generateUpdateStatement() string {
	var stringBuilder strings.Builder

	stringBuilder.WriteString(fmt.Sprintf("UPDATE %s.%s AS old SET payload=new.payload FROM (values ",
		builder.schema, builder.tableName))

	numberOfColumns := len(builder.updateArgs) / builder.updateRowsCount // total num of args divided by num of rows
	stringBuilder.WriteString(builder.generateInsertOrUpdateArgs(builder.updateRowsCount, numberOfColumns,
		builder.tableSpecialColumns))

	stringBuilder.WriteString(") AS new(id,leaf_hub_name,payload) ")
	stringBuilder.WriteString("WHERE old.leaf_hub_name=new.leaf_hub_name ")
	stringBuilder.WriteString("AND old.id=new.id")

	return stringBuilder.String()
}

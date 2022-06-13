package batch

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v4"
)

type generateStatementFunc func() string

const (
	// according to postgresql docs, client can update at most 2^16 columns in a single query.
	maxColumnsUpdateInStatement = 65536
	deleteStartingIndex         = 2
)

type batchItem struct {
	query     string
	arguments []interface{}
}

func newBaseBatchBuilder(schema string, tableName string, tableSpecialColumns map[int]string, leafHubName string,
	deleteRowKey string,
) *baseBatchBuilder {
	return &baseBatchBuilder{
		deleteBatchItems:    make([]*batchItem, 0),
		insertBatchItems:    make([]*batchItem, 0),
		updateBatchItems:    make([]*batchItem, 0),
		schema:              schema,
		tableName:           tableName,
		tableSpecialColumns: tableSpecialColumns,
		leafHubName:         leafHubName,
		insertArgs:          make([]interface{}, 0),
		insertRowsCount:     0,
		updateArgs:          make([]interface{}, 0),
		updateRowsCount:     0,
		deleteArgs:          append(make([]interface{}, 0), leafHubName), // leafHubName is first arg in delete query
		deleteRowsCount:     0,
		deleteRowKey:        deleteRowKey,
	}
}

type baseBatchBuilder struct {
	deleteBatchItems        []*batchItem
	insertBatchItems        []*batchItem
	updateBatchItems        []*batchItem
	schema                  string
	tableName               string
	tableSpecialColumns     map[int]string
	leafHubName             string
	insertArgs              []interface{}
	insertRowsCount         int
	updateArgs              []interface{}
	updateRowsCount         int
	generateUpdateStatement generateStatementFunc
	deleteArgs              []interface{}
	deleteRowsCount         int
	deleteRowKey            string
}

func (builder *baseBatchBuilder) insert(insertArgs ...interface{}) {
	// if adding args will exceeded max args limit, create insert statement from current args and zero the count/args.
	if len(builder.insertArgs)+len(insertArgs) >= maxColumnsUpdateInStatement {
		builder.insertBatchItems = append(builder.insertBatchItems, &batchItem{
			query:     builder.generateInsertStatement(),
			arguments: builder.insertArgs,
		})

		builder.insertArgs = make([]interface{}, 0)
		builder.insertRowsCount = 0
	}

	builder.insertArgs = append(builder.insertArgs, insertArgs...)
	builder.insertRowsCount++
}

func (builder *baseBatchBuilder) update(updateArgs ...interface{}) {
	// if adding args will exceeded max args limit, create update statement from current args and zero the count/args.
	if len(builder.updateArgs)+len(updateArgs) >= maxColumnsUpdateInStatement {
		builder.updateBatchItems = append(builder.updateBatchItems, &batchItem{
			query:     builder.generateUpdateStatement(),
			arguments: builder.updateArgs,
		})

		builder.updateArgs = make([]interface{}, 0)
		builder.updateRowsCount = 0
	}

	builder.updateArgs = append(builder.updateArgs, updateArgs...)
	builder.updateRowsCount++
}

func (builder *baseBatchBuilder) delete(deleteArgs ...interface{}) {
	// if adding args will exceeded max args limit, create delete statement from current args and zero the count/args.
	if len(builder.deleteArgs)+len(deleteArgs) >= maxColumnsUpdateInStatement {
		builder.deleteBatchItems = append(builder.deleteBatchItems, &batchItem{
			query:     builder.generateDeleteStatement(),
			arguments: builder.deleteArgs,
		})

		builder.deleteArgs = append(make([]interface{}, 0), builder.leafHubName) // leafHubName is first arg in delete
		builder.deleteRowsCount = 0
	}

	builder.deleteArgs = append(builder.deleteArgs, deleteArgs...)
	builder.deleteRowsCount++
}

func (builder *baseBatchBuilder) build() *pgx.Batch {
	batch := &pgx.Batch{}

	// append deletes first
	for _, batchItem := range builder.deleteBatchItems {
		batch.Queue(batchItem.query, batchItem.arguments)
	}

	if builder.deleteRowsCount > 0 { // generate DELETE statement for multiple rows into the batch
		batch.Queue(builder.generateDeleteStatement(), builder.deleteArgs...)
	}
	// then inserts
	for _, batchItem := range builder.insertBatchItems {
		batch.Queue(batchItem.query, batchItem.arguments)
	}

	if builder.insertRowsCount > 0 { // generate INSERT statement for multiple rows into the batch
		batch.Queue(builder.generateInsertStatement(), builder.insertArgs...)
	}
	// finally, updates
	for _, batchItem := range builder.updateBatchItems {
		batch.Queue(batchItem.query, batchItem.arguments)
	}

	if builder.updateRowsCount > 0 { // generate UPDATE statement for multiple rows into the batch
		batch.Queue(builder.generateUpdateStatement(), builder.updateArgs...)
	}

	return batch
}

func (builder *baseBatchBuilder) setUpdateStatementFunc(generateUpdateStatementFunc generateStatementFunc) {
	builder.generateUpdateStatement = generateUpdateStatementFunc
}

func (builder *baseBatchBuilder) generateInsertStatement() string {
	var stringBuilder strings.Builder

	stringBuilder.WriteString(fmt.Sprintf("INSERT into %s.%s values ", builder.schema, builder.tableName))

	columnsCount := len(builder.insertArgs) / builder.insertRowsCount // total num of args divided by num of rows
	stringBuilder.WriteString(builder.generateInsertOrUpdateArgs(builder.insertRowsCount, columnsCount,
		builder.tableSpecialColumns))

	return stringBuilder.String()
}

func (builder *baseBatchBuilder) generateDeleteStatement() string {
	return fmt.Sprintf("DELETE from %s.%s WHERE leaf_hub_name=$1 AND %s IN (%s)", builder.schema,
		builder.tableName, builder.deleteRowKey, builder.generateArgsList(builder.deleteRowsCount, deleteStartingIndex,
			make(map[int]string)))
}

// generateInsertOrUpdateArgs is a generic function used to auto generation batch statements.
// in case some args should be marked as special types, specialColumns map should be used to specify column index
// mapped to a special column type, e.g. map(2) = jsonb.
func (builder *baseBatchBuilder) generateInsertOrUpdateArgs(rowsCount int, columnsCount int,
	specialColumns map[int]string,
) string {
	var stringBuilder strings.Builder

	for i := 0; i < rowsCount; i++ {
		stringBuilder.WriteString(fmt.Sprintf("(%s)", builder.generateArgsList(columnsCount,
			i*columnsCount+1, specialColumns)))

		if i < rowsCount-1 {
			stringBuilder.WriteString(", ")
		}
	}

	return stringBuilder.String()
}

func (builder *baseBatchBuilder) generateArgsList(argsCount int, startingIndex int, specialArgs map[int]string) string {
	var stringBuilder strings.Builder

	for index := 0; index < argsCount; index++ {
		stringBuilder.WriteString("$")
		stringBuilder.WriteString(strconv.Itoa(index + startingIndex))
		// if a column contains definition for special column (e.g. jsonb type, uuid type)
		if columnSpecialType, found := specialArgs[index+1]; found {
			stringBuilder.WriteString(fmt.Sprintf("::%s", columnSpecialType))
		}

		if index < argsCount-1 {
			stringBuilder.WriteString(", ")
		}
	}

	return stringBuilder.String()
}

package bigquery

import (
	"context"
	"errors"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
)

// --- Mock implementations ---

// mockBigQueryClient implements BigqueryAPI for testing.
type mockBigQueryClient struct {
	datasetIDs  []string
	datasetMeta map[string]*bigquery.DatasetMetadata
	datasetErr  error
	tables      map[string][]string // datasetID -> tableIDs
	tableMeta   map[string]*bigquery.TableMetadata
	queryRows   []map[string]bigquery.Value
	querySchema bigquery.Schema
	queryErr    error
}

func (m *mockBigQueryClient) Datasets(_ context.Context) DatasetIterator {
	return &mockDatasetIterator{datasetIDs: m.datasetIDs, err: m.datasetErr}
}

func (m *mockBigQueryClient) Dataset(datasetID string) DatasetHandle {
	return &mockDatasetHandle{
		datasetID: datasetID,
		meta:      m.datasetMeta[datasetID],
		tableIDs:  m.tables[datasetID],
		tableMeta: m.tableMeta,
	}
}

func (m *mockBigQueryClient) Query(_ string) QueryHandle {
	return &mockQueryHandle{rows: m.queryRows, schema: m.querySchema, err: m.queryErr}
}

func (m *mockBigQueryClient) Close() error { return nil }

type mockDatasetIterator struct {
	datasetIDs []string
	err        error
	pos        int
}

func (it *mockDatasetIterator) Next() (*bigquery.Dataset, error) {
	if it.err != nil {
		return nil, it.err
	}
	if it.pos >= len(it.datasetIDs) {
		return nil, iterator.Done
	}
	ds := &bigquery.Dataset{DatasetID: it.datasetIDs[it.pos]}
	it.pos++
	return ds, nil
}

type mockDatasetHandle struct {
	datasetID string
	meta      *bigquery.DatasetMetadata
	tableIDs  []string
	tableMeta map[string]*bigquery.TableMetadata
}

func (d *mockDatasetHandle) Metadata(_ context.Context) (*bigquery.DatasetMetadata, error) {
	if d.meta == nil {
		return nil, errors.New("dataset not found: " + d.datasetID)
	}
	return d.meta, nil
}

func (d *mockDatasetHandle) Tables(_ context.Context) TableIterator {
	return &mockTableIterator{tableIDs: d.tableIDs, datasetID: d.datasetID}
}

func (d *mockDatasetHandle) Table(tableID string) TableHandle {
	return &mockTableHandle{meta: d.tableMeta[d.datasetID+"."+tableID]}
}

type mockTableIterator struct {
	tableIDs  []string
	datasetID string
	pos       int
}

func (it *mockTableIterator) Next() (*bigquery.Table, error) {
	if it.pos >= len(it.tableIDs) {
		return nil, iterator.Done
	}
	t := &bigquery.Table{
		DatasetID: it.datasetID,
		TableID:   it.tableIDs[it.pos],
	}
	it.pos++
	return t, nil
}

type mockTableHandle struct {
	meta *bigquery.TableMetadata
}

func (t *mockTableHandle) Metadata(_ context.Context) (*bigquery.TableMetadata, error) {
	if t.meta == nil {
		return nil, errors.New("table metadata not found")
	}
	return t.meta, nil
}

type mockQueryHandle struct {
	rows   []map[string]bigquery.Value
	schema bigquery.Schema
	err    error
}

func (q *mockQueryHandle) Read(_ context.Context) (RowIterator, error) {
	if q.err != nil {
		return nil, q.err
	}
	return &mockRowIterator{rows: q.rows, schema: q.schema}, nil
}

type mockRowIterator struct {
	rows   []map[string]bigquery.Value
	schema bigquery.Schema
	pos    int
}

func (r *mockRowIterator) Next(dst interface{}) error {
	if r.pos >= len(r.rows) {
		return iterator.Done
	}
	if m, ok := dst.(*map[string]bigquery.Value); ok {
		*m = r.rows[r.pos]
	}
	r.pos++
	return nil
}

func (r *mockRowIterator) Schema() bigquery.Schema {
	return r.schema
}

// --- Tests ---

func TestListDatasets(t *testing.T) {
	now := time.Now()
	client := &mockBigQueryClient{
		datasetIDs: []string{"dataset_a", "dataset_b"},
		datasetMeta: map[string]*bigquery.DatasetMetadata{
			"dataset_a": {
				Location:         "US",
				CreationTime:     now,
				LastModifiedTime: now,
				Description:      "Test dataset A",
			},
			"dataset_b": {
				Location:         "EU",
				CreationTime:     now,
				LastModifiedTime: now,
				Description:      "",
			},
		},
	}

	results, err := ListDatasets(context.Background(), client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 datasets, got %d", len(results))
	}
	if results[0].DatasetID != "dataset_a" {
		t.Errorf("expected DatasetID 'dataset_a', got '%s'", results[0].DatasetID)
	}
	if results[0].Location != "US" {
		t.Errorf("expected Location 'US', got '%s'", results[0].Location)
	}
	if results[1].DatasetID != "dataset_b" {
		t.Errorf("expected DatasetID 'dataset_b', got '%s'", results[1].DatasetID)
	}
}

func TestListDatasets_Empty(t *testing.T) {
	client := &mockBigQueryClient{}

	results, err := ListDatasets(context.Background(), client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 datasets, got %d", len(results))
	}
}

func TestListDatasets_IterError(t *testing.T) {
	client := &mockBigQueryClient{
		datasetErr: errors.New("api error"),
	}

	_, err := ListDatasets(context.Background(), client)
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestListTables(t *testing.T) {
	now := time.Now()
	client := &mockBigQueryClient{
		tables: map[string][]string{
			"my_dataset": {"table_a", "table_b"},
		},
		tableMeta: map[string]*bigquery.TableMetadata{
			"my_dataset.table_a": {
				Type:             bigquery.RegularTable,
				CreationTime:     now,
				LastModifiedTime: now,
				NumRows:          100,
				NumBytes:         1024,
			},
			"my_dataset.table_b": {
				Type:             bigquery.ViewTable,
				CreationTime:     now,
				LastModifiedTime: now,
				NumRows:          50,
				NumBytes:         512,
			},
		},
	}

	tables, err := ListTables(context.Background(), client, "my_dataset")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(tables))
	}
	if tables[0].TableID != "table_a" {
		t.Errorf("expected TableID 'table_a', got '%s'", tables[0].TableID)
	}
	if tables[0].Type != "TABLE" {
		t.Errorf("expected Type 'TABLE', got '%s'", tables[0].Type)
	}
	if tables[0].NumRows != "100" {
		t.Errorf("expected NumRows '100', got '%s'", tables[0].NumRows)
	}
	if tables[1].Type != "VIEW" {
		t.Errorf("expected Type 'VIEW', got '%s'", tables[1].Type)
	}
}

func TestListTables_Error(t *testing.T) {
	client := &mockBigQueryClient{
		tables: map[string][]string{"ds": {"t"}},
		// tableMeta intentionally nil → Metadata() returns error
	}

	_, err := ListTables(context.Background(), client, "ds")
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestGetTableSchema(t *testing.T) {
	tests := []struct {
		name      string
		datasetID string
		tableID   string
		meta      *bigquery.TableMetadata
		wantLen   int
		wantErr   bool
	}{
		{
			name:      "simple schema",
			datasetID: "ds",
			tableID:   "t",
			meta: &bigquery.TableMetadata{
				Schema: bigquery.Schema{
					{Name: "id", Type: bigquery.IntegerFieldType, Required: true, Description: "Primary key"},
					{Name: "name", Type: bigquery.StringFieldType, Description: "User name"},
					{Name: "active", Type: bigquery.BooleanFieldType},
				},
			},
			wantLen: 3,
		},
		{
			name:      "nested record schema",
			datasetID: "ds",
			tableID:   "nested",
			meta: &bigquery.TableMetadata{
				Schema: bigquery.Schema{
					{Name: "user", Type: bigquery.RecordFieldType, Schema: bigquery.Schema{
						{Name: "id", Type: bigquery.IntegerFieldType},
						{Name: "email", Type: bigquery.StringFieldType},
					}},
				},
			},
			wantLen: 3, // user + user.id + user.email
		},
		{
			name:      "missing metadata",
			datasetID: "ds",
			tableID:   "missing",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tableMeta := map[string]*bigquery.TableMetadata{}
			if tt.meta != nil {
				tableMeta[tt.datasetID+"."+tt.tableID] = tt.meta
			}
			client := &mockBigQueryClient{tableMeta: tableMeta}

			fields, err := GetTableSchema(context.Background(), client, tt.datasetID, tt.tableID)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(fields) != tt.wantLen {
				t.Errorf("expected %d fields, got %d", tt.wantLen, len(fields))
			}
		})
	}
}

func TestGetTableSchema_FieldDetails(t *testing.T) {
	client := &mockBigQueryClient{
		tableMeta: map[string]*bigquery.TableMetadata{
			"ds.t": {
				Schema: bigquery.Schema{
					{Name: "id", Type: bigquery.IntegerFieldType, Required: true, Description: "PK"},
					{Name: "tags", Type: bigquery.StringFieldType, Repeated: true},
					{Name: "score", Type: bigquery.FloatFieldType},
				},
			},
		},
	}

	fields, err := GetTableSchema(context.Background(), client, "ds", "t")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(fields))
	}

	if fields[0].FieldName != "id" || fields[0].Mode != "REQUIRED" || fields[0].Description != "PK" {
		t.Errorf("unexpected field[0]: %+v", fields[0])
	}
	if fields[1].Mode != "REPEATED" {
		t.Errorf("expected mode 'REPEATED', got '%s'", fields[1].Mode)
	}
	if fields[2].Mode != "NULLABLE" {
		t.Errorf("expected mode 'NULLABLE', got '%s'", fields[2].Mode)
	}
}

func TestExecuteQuery(t *testing.T) {
	schema := bigquery.Schema{
		{Name: "name", Type: bigquery.StringFieldType},
		{Name: "age", Type: bigquery.IntegerFieldType},
	}
	rows := []map[string]bigquery.Value{
		{"name": "Alice", "age": int64(30)},
		{"name": "Bob", "age": int64(25)},
	}
	client := &mockBigQueryClient{querySchema: schema, queryRows: rows}

	colNames, resultRows, err := ExecuteQuery(context.Background(), client, "SELECT name, age FROM t")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(colNames) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(colNames))
	}
	if colNames[0] != "name" || colNames[1] != "age" {
		t.Errorf("unexpected column names: %v", colNames)
	}
	if len(resultRows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(resultRows))
	}
	if resultRows[0][0] != "Alice" {
		t.Errorf("expected 'Alice', got '%s'", resultRows[0][0])
	}
	if resultRows[1][1] != "25" {
		t.Errorf("expected '25', got '%s'", resultRows[1][1])
	}
}

func TestExecuteQuery_Empty(t *testing.T) {
	schema := bigquery.Schema{{Name: "col", Type: bigquery.StringFieldType}}
	client := &mockBigQueryClient{querySchema: schema}

	colNames, resultRows, err := ExecuteQuery(context.Background(), client, "SELECT col FROM t WHERE 1=0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(colNames) != 1 {
		t.Errorf("expected 1 column, got %d", len(colNames))
	}
	if len(resultRows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(resultRows))
	}
}

func TestExecuteQuery_Error(t *testing.T) {
	client := &mockBigQueryClient{queryErr: errors.New("query failed")}

	_, _, err := ExecuteQuery(context.Background(), client, "INVALID SQL")
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestFieldMode(t *testing.T) {
	tests := []struct {
		name     string
		field    *bigquery.FieldSchema
		expected string
	}{
		{"repeated", &bigquery.FieldSchema{Repeated: true}, "REPEATED"},
		{"required", &bigquery.FieldSchema{Required: true}, "REQUIRED"},
		{"nullable", &bigquery.FieldSchema{}, "NULLABLE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fieldMode(tt.field)
			if got != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, got)
			}
		})
	}
}

func TestDatasetInfo_ToRow(t *testing.T) {
	d := DatasetInfo{
		DatasetID:        "my_dataset",
		Location:         "US",
		CreationTime:     "2024-01-01T00:00:00Z",
		LastModifiedTime: "2024-06-01T00:00:00Z",
		Description:      "Test",
	}
	row := d.ToRow()
	if len(row) != 5 {
		t.Fatalf("expected 5 fields, got %d", len(row))
	}
	if row[0] != "my_dataset" {
		t.Errorf("expected 'my_dataset', got '%s'", row[0])
	}
	if row[1] != "US" {
		t.Errorf("expected 'US', got '%s'", row[1])
	}
}

func TestTableInfo_ToRow(t *testing.T) {
	ti := TableInfo{
		TableID:          "my_table",
		Type:             "TABLE",
		CreationTime:     "2024-01-01T00:00:00Z",
		LastModifiedTime: "2024-06-01T00:00:00Z",
		NumRows:          "100",
		NumBytes:         "1024",
	}
	row := ti.ToRow()
	if len(row) != 6 {
		t.Fatalf("expected 6 fields, got %d", len(row))
	}
	if row[4] != "100" {
		t.Errorf("expected '100', got '%s'", row[4])
	}
	if row[5] != "1024" {
		t.Errorf("expected '1024', got '%s'", row[5])
	}
}

func TestFieldInfo_ToRow(t *testing.T) {
	f := FieldInfo{
		FieldName:   "user_id",
		FieldType:   "INTEGER",
		Mode:        "REQUIRED",
		Description: "Primary key",
	}
	row := f.ToRow()
	if len(row) != 4 {
		t.Fatalf("expected 4 fields, got %d", len(row))
	}
	if row[0] != "user_id" {
		t.Errorf("expected 'user_id', got '%s'", row[0])
	}
	if row[2] != "REQUIRED" {
		t.Errorf("expected 'REQUIRED', got '%s'", row[2])
	}
}

// Package bigquery provides a client wrapper and data types for interacting
// with Google BigQuery. It uses Application Default Credentials (ADC) for
// authentication, defaulting to ~/.config/gcloud/application_default_credentials.json.
package bigquery

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
)

// BigqueryAPI defines the interface for BigQuery operations.
// This enables mock-based unit testing without connecting to real BigQuery.
type BigqueryAPI interface {
	Datasets(ctx context.Context) DatasetIterator
	Dataset(datasetID string) DatasetHandle
	Query(query string) QueryHandle
	Close() error
}

// DatasetIterator abstracts iteration over datasets.
type DatasetIterator interface {
	Next() (*bigquery.Dataset, error)
}

// DatasetHandle abstracts operations on a single dataset.
type DatasetHandle interface {
	Metadata(ctx context.Context) (*bigquery.DatasetMetadata, error)
	Tables(ctx context.Context) TableIterator
	Table(tableID string) TableHandle
}

// TableIterator abstracts iteration over tables.
type TableIterator interface {
	Next() (*bigquery.Table, error)
}

// TableHandle abstracts operations on a single table.
type TableHandle interface {
	Metadata(ctx context.Context) (*bigquery.TableMetadata, error)
}

// QueryHandle abstracts a BigQuery query.
type QueryHandle interface {
	Read(ctx context.Context) (RowIterator, error)
}

// RowIterator abstracts iteration over query result rows.
type RowIterator interface {
	Next(dst interface{}) error
	Schema() bigquery.Schema
}

// --- Real SDK wrappers ---

type clientWrapper struct {
	client *bigquery.Client
}

// NewBigQueryClient creates a new BigQuery client using Application Default Credentials.
// ADC discovers credentials from GOOGLE_APPLICATION_CREDENTIALS env var or
// ~/.config/gcloud/application_default_credentials.json.
func NewBigQueryClient(ctx context.Context, projectID string) (BigqueryAPI, error) {
	c, err := bigquery.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create bigquery client: %w", err)
	}
	return &clientWrapper{client: c}, nil
}

func (w *clientWrapper) Datasets(ctx context.Context) DatasetIterator {
	return w.client.Datasets(ctx)
}

func (w *clientWrapper) Dataset(datasetID string) DatasetHandle {
	return &datasetHandleWrapper{ds: w.client.Dataset(datasetID)}
}

func (w *clientWrapper) Query(query string) QueryHandle {
	return &queryHandleWrapper{q: w.client.Query(query)}
}

func (w *clientWrapper) Close() error {
	return w.client.Close()
}

type datasetHandleWrapper struct {
	ds *bigquery.Dataset
}

func (d *datasetHandleWrapper) Metadata(ctx context.Context) (*bigquery.DatasetMetadata, error) {
	return d.ds.Metadata(ctx)
}

func (d *datasetHandleWrapper) Tables(ctx context.Context) TableIterator {
	return d.ds.Tables(ctx)
}

func (d *datasetHandleWrapper) Table(tableID string) TableHandle {
	return &tableHandleWrapper{t: d.ds.Table(tableID)}
}

type tableHandleWrapper struct {
	t *bigquery.Table
}

func (t *tableHandleWrapper) Metadata(ctx context.Context) (*bigquery.TableMetadata, error) {
	return t.t.Metadata(ctx)
}

type queryHandleWrapper struct {
	q *bigquery.Query
}

func (qh *queryHandleWrapper) Read(ctx context.Context) (RowIterator, error) {
	it, err := qh.q.Read(ctx)
	if err != nil {
		return nil, err
	}
	return &rowIteratorWrapper{it: it}, nil
}

type rowIteratorWrapper struct {
	it *bigquery.RowIterator
}

func (r *rowIteratorWrapper) Next(dst interface{}) error {
	return r.it.Next(dst)
}

func (r *rowIteratorWrapper) Schema() bigquery.Schema {
	return r.it.Schema
}

// --- Data Structs ---

// DatasetInfo holds display fields for a BigQuery dataset.
type DatasetInfo struct {
	DatasetID        string
	Location         string
	CreationTime     string
	LastModifiedTime string
	Description      string
}

// ToRow converts DatasetInfo to a string slice for table output.
func (d DatasetInfo) ToRow() []string {
	return []string{d.DatasetID, d.Location, d.CreationTime, d.LastModifiedTime, d.Description}
}

// TableInfo holds display fields for a BigQuery table.
type TableInfo struct {
	TableID          string
	Type             string
	CreationTime     string
	LastModifiedTime string
	NumRows          string
	NumBytes         string
}

// ToRow converts TableInfo to a string slice for table output.
func (t TableInfo) ToRow() []string {
	return []string{t.TableID, t.Type, t.CreationTime, t.LastModifiedTime, t.NumRows, t.NumBytes}
}

// FieldInfo holds display fields for a BigQuery table schema field.
type FieldInfo struct {
	FieldName   string
	FieldType   string
	Mode        string
	Description string
}

// ToRow converts FieldInfo to a string slice for table output.
func (f FieldInfo) ToRow() []string {
	return []string{f.FieldName, f.FieldType, f.Mode, f.Description}
}

// --- Fetch Functions ---

// ListDatasets returns metadata for all datasets in the project.
func ListDatasets(ctx context.Context, client BigqueryAPI) ([]DatasetInfo, error) {
	var datasets []DatasetInfo
	it := client.Datasets(ctx)
	for {
		ds, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("iterate datasets: %w", err)
		}
		meta, err := client.Dataset(ds.DatasetID).Metadata(ctx)
		if err != nil {
			return nil, fmt.Errorf("get dataset metadata for %s: %w", ds.DatasetID, err)
		}
		datasets = append(datasets, DatasetInfo{
			DatasetID:        ds.DatasetID,
			Location:         meta.Location,
			CreationTime:     meta.CreationTime.Format(time.RFC3339),
			LastModifiedTime: meta.LastModifiedTime.Format(time.RFC3339),
			Description:      meta.Description,
		})
	}
	return datasets, nil
}

// ListTables returns metadata for all tables in the specified dataset.
func ListTables(ctx context.Context, client BigqueryAPI, datasetID string) ([]TableInfo, error) {
	var tables []TableInfo
	it := client.Dataset(datasetID).Tables(ctx)
	for {
		t, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("iterate tables in %s: %w", datasetID, err)
		}
		meta, err := client.Dataset(datasetID).Table(t.TableID).Metadata(ctx)
		if err != nil {
			return nil, fmt.Errorf("get table metadata for %s.%s: %w", datasetID, t.TableID, err)
		}
		tables = append(tables, TableInfo{
			TableID:          t.TableID,
			Type:             string(meta.Type),
			CreationTime:     meta.CreationTime.Format(time.RFC3339),
			LastModifiedTime: meta.LastModifiedTime.Format(time.RFC3339),
			NumRows:          fmt.Sprintf("%d", meta.NumRows),
			NumBytes:         fmt.Sprintf("%d", meta.NumBytes),
		})
	}
	return tables, nil
}

// GetTableSchema returns schema field information for the specified table.
func GetTableSchema(ctx context.Context, client BigqueryAPI, datasetID, tableID string) ([]FieldInfo, error) {
	meta, err := client.Dataset(datasetID).Table(tableID).Metadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("get table metadata for %s.%s: %w", datasetID, tableID, err)
	}
	return schemaToFieldInfos(meta.Schema), nil
}

// schemaToFieldInfos converts a bigquery.Schema to a slice of FieldInfo,
// flattening nested RECORD fields with dot-notation names.
func schemaToFieldInfos(schema bigquery.Schema) []FieldInfo {
	var fields []FieldInfo
	for _, f := range schema {
		fields = append(fields, FieldInfo{
			FieldName:   f.Name,
			FieldType:   string(f.Type),
			Mode:        fieldMode(f),
			Description: f.Description,
		})
		// Flatten nested RECORD fields with dot-notation
		if f.Type == bigquery.RecordFieldType {
			for _, nested := range schemaToFieldInfos(f.Schema) {
				nested.FieldName = f.Name + "." + nested.FieldName
				fields = append(fields, nested)
			}
		}
	}
	return fields
}

// fieldMode returns the mode string for a schema field.
func fieldMode(f *bigquery.FieldSchema) string {
	if f.Repeated {
		return "REPEATED"
	}
	if f.Required {
		return "REQUIRED"
	}
	return "NULLABLE"
}

// ExecuteQuery runs a SQL query and returns dynamic column names and rows.
// Rows are returned as [][]string where each value is formatted with fmt.Sprintf("%v").
func ExecuteQuery(ctx context.Context, client BigqueryAPI, sql string) ([]string, [][]string, error) {
	it, err := client.Query(sql).Read(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("execute query: %w", err)
	}

	schema := it.Schema()
	colNames := make([]string, len(schema))
	for i, f := range schema {
		colNames[i] = f.Name
	}

	var rows [][]string
	for {
		var rowMap map[string]bigquery.Value
		err := it.Next(&rowMap)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("iterate query results: %w", err)
		}
		row := make([]string, len(colNames))
		for i, col := range colNames {
			if v, ok := rowMap[col]; ok && v != nil {
				row[i] = fmt.Sprintf("%v", v)
			}
		}
		rows = append(rows, row)
	}

	return colNames, rows, nil
}

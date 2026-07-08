// Package bigquery provides a BigQuery client wrapper using Application Default Credentials.
// ADC resolves credentials from GOOGLE_APPLICATION_CREDENTIALS env var or
// ~/.config/gcloud/application_default_credentials.json.
package bigquery

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
)

// Client wraps the BigQuery SDK client.
type Client struct {
	bq        *bigquery.Client
	projectID string
}

// NewClient creates a BigQuery Client using Application Default Credentials.
func NewClient(ctx context.Context, projectID string) (*Client, error) {
	c, err := bigquery.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create bigquery client: %w", err)
	}
	return &Client{bq: c, projectID: projectID}, nil
}

// Close releases the underlying client.
func (c *Client) Close() error { return c.bq.Close() }

// DatasetInfo holds display fields for a BigQuery dataset.
type DatasetInfo struct {
	DatasetID        string `json:"dataset_id"`
	Location         string `json:"location"`
	CreationTime     string `json:"creation_time"`
	LastModifiedTime string `json:"last_modified_time"`
	Description      string `json:"description"`
}

// TableInfo holds display fields for a BigQuery table.
type TableInfo struct {
	TableID          string `json:"table_id"`
	Type             string `json:"type"`
	CreationTime     string `json:"creation_time"`
	LastModifiedTime string `json:"last_modified_time"`
	NumRows          uint64 `json:"num_rows"`
	NumBytes         int64  `json:"num_bytes"`
}

// FieldInfo holds display fields for a BigQuery table schema field.
type FieldInfo struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Mode        string `json:"mode"`
	Description string `json:"description"`
}

// ListDatasets returns metadata for all datasets in the project.
func (c *Client) ListDatasets(ctx context.Context) ([]DatasetInfo, error) {
	var datasets []DatasetInfo
	it := c.bq.Datasets(ctx)
	for {
		ds, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("iterate datasets: %w", err)
		}
		meta, err := c.bq.Dataset(ds.DatasetID).Metadata(ctx)
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

// ListTables returns metadata for all tables in the given dataset.
func (c *Client) ListTables(ctx context.Context, datasetID string) ([]TableInfo, error) {
	var tables []TableInfo
	it := c.bq.Dataset(datasetID).Tables(ctx)
	for {
		t, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("iterate tables in %s: %w", datasetID, err)
		}
		meta, err := c.bq.Dataset(datasetID).Table(t.TableID).Metadata(ctx)
		if err != nil {
			return nil, fmt.Errorf("get table metadata for %s.%s: %w", datasetID, t.TableID, err)
		}
		tables = append(tables, TableInfo{
			TableID:          t.TableID,
			Type:             string(meta.Type),
			CreationTime:     meta.CreationTime.Format(time.RFC3339),
			LastModifiedTime: meta.LastModifiedTime.Format(time.RFC3339),
			NumRows:          meta.NumRows,
			NumBytes:         meta.NumBytes,
		})
	}
	return tables, nil
}

// GetTableSchema returns schema field information for the given table.
func (c *Client) GetTableSchema(ctx context.Context, datasetID, tableID string) ([]FieldInfo, error) {
	meta, err := c.bq.Dataset(datasetID).Table(tableID).Metadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("get table metadata for %s.%s: %w", datasetID, tableID, err)
	}
	return schemaToFieldInfos(meta.Schema), nil
}

func schemaToFieldInfos(schema bigquery.Schema) []FieldInfo {
	var fields []FieldInfo
	for _, f := range schema {
		fields = append(fields, FieldInfo{
			Name:        f.Name,
			Type:        string(f.Type),
			Mode:        fieldMode(f),
			Description: f.Description,
		})
		if f.Type == bigquery.RecordFieldType {
			for _, nested := range schemaToFieldInfos(f.Schema) {
				nested.Name = f.Name + "." + nested.Name
				fields = append(fields, nested)
			}
		}
	}
	return fields
}

func fieldMode(f *bigquery.FieldSchema) string {
	if f.Repeated {
		return "REPEATED"
	}
	if f.Required {
		return "REQUIRED"
	}
	return "NULLABLE"
}

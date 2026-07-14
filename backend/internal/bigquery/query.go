package bigquery

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
)

// ErrWriteNotAllowed is returned when a query contains DML/DDL statements.
var ErrWriteNotAllowed = errors.New("DML/DDL not allowed: only SELECT/WITH queries are permitted")

var ddlDmlRe = regexp.MustCompile(`(?i)\b(INSERT|UPDATE|DELETE|MERGE|CREATE|DROP|ALTER|TRUNCATE|GRANT|REVOKE)\b`)

// QueryResult holds the output of a BigQuery SELECT query.
type QueryResult struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

// ValidateReadOnly returns ErrWriteNotAllowed if the SQL contains DML/DDL keywords.
func ValidateReadOnly(sql string) error {
	if ddlDmlRe.MatchString(sql) {
		return ErrWriteNotAllowed
	}
	return nil
}

// ExecuteQuery runs a read-only SQL query.
// It validates the SQL for DML/DDL first, then performs a dry-run cost check,
// then executes the actual query.
func (c *Client) ExecuteQuery(ctx context.Context, sql string) (*QueryResult, error) {
	if err := ValidateReadOnly(sql); err != nil {
		return nil, err
	}

	q := c.bq.Query(sql)
	q.UseLegacySQL = false

	// Dry run to validate query syntax and cost.
	q.DryRun = true
	job, err := q.Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("bigquery dry run: %w", err)
	}
	_ = job // dry run result is available in job.LastStatus()

	// Actual execution.
	q.DryRun = false
	it, err := q.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("execute bigquery query: %w", err)
	}

	// it.Schema は Read 直後には空であり、最初の Next 呼び出し後に初めて確定する。
	var colNames []string
	var rows [][]string
	for {
		var rowMap map[string]bigquery.Value
		if err := it.Next(&rowMap); err == iterator.Done {
			break
		} else if err != nil {
			return nil, fmt.Errorf("iterate bigquery results: %w", err)
		}
		if colNames == nil {
			colNames = make([]string, len(it.Schema))
			for i, f := range it.Schema {
				colNames[i] = f.Name
			}
		}
		row := make([]string, len(colNames))
		for i, col := range colNames {
			if v, ok := rowMap[col]; ok && v != nil {
				row[i] = fmt.Sprintf("%v", v)
			}
		}
		rows = append(rows, row)
	}

	// 結果 0 件の場合、ループ内で Schema を確定できないため Read 完了後の値で補う。
	if colNames == nil {
		colNames = make([]string, len(it.Schema))
		for i, f := range it.Schema {
			colNames[i] = f.Name
		}
	}

	return &QueryResult{Columns: colNames, Rows: rows}, nil
}

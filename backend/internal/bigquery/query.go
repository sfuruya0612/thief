package bigquery

import (
	"context"
	"fmt"

	"cloud.google.com/go/bigquery"
	"github.com/sfuruya0612/thief/backend/internal/sqlguard"
	"google.golang.org/api/iterator"
)

// ErrWriteNotAllowed is returned when a query contains DML/DDL statements.
// 実体は sqlguard.ErrWriteNotAllowed で、既存の呼び出し側互換のため再公開する。
var ErrWriteNotAllowed = sqlguard.ErrWriteNotAllowed

// QueryResult holds the output of a BigQuery SELECT query.
type QueryResult struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

// ValidateReadOnly returns ErrWriteNotAllowed if the SQL contains DML/DDL keywords.
func ValidateReadOnly(sql string) error {
	return sqlguard.ValidateReadOnly(sql)
}

// ExecuteQueryUnrestricted runs any SQL query without the read-only validation
// and without a dry-run check. レガシー CLI (thief bq query) 互換の実行経路で、
// ローカルユーザー自身の認証情報で任意の SQL (DML/DDL を含む) を実行する。
// API サーバからは呼ばないこと (サーバ経路は StartQuery の read-only 検証を維持する)。
func (c *Client) ExecuteQueryUnrestricted(ctx context.Context, sql string) (*QueryResult, error) {
	q := c.bq.Query(sql)
	q.UseLegacySQL = false
	return c.readQuery(ctx, q)
}

// readQuery はクエリを実行し、結果を文字列テーブルへ変換して返す。
func (c *Client) readQuery(ctx context.Context, q *bigquery.Query) (*QueryResult, error) {
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

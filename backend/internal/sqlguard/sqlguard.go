// Package sqlguard は API サーバ経由で実行される SQL の読み取り専用検証を提供する。
// BigQuery / Athena のクエリ実行エンドポイントが共通で利用する。
package sqlguard

import (
	"errors"
	"regexp"
)

// ErrWriteNotAllowed is returned when a query contains DML/DDL statements.
var ErrWriteNotAllowed = errors.New("DML/DDL not allowed: only SELECT/WITH queries are permitted")

// ddlDmlRe は書き込み系ステートメントのキーワードを検出する。UNLOAD は Athena の
// S3 書き出しステートメントで、読み取り専用経路では許可しない。
var ddlDmlRe = regexp.MustCompile(`(?i)\b(INSERT|UPDATE|DELETE|MERGE|CREATE|DROP|ALTER|TRUNCATE|GRANT|REVOKE|UNLOAD)\b`)

// ValidateReadOnly returns ErrWriteNotAllowed if the SQL contains DML/DDL keywords.
func ValidateReadOnly(sql string) error {
	if ddlDmlRe.MatchString(sql) {
		return ErrWriteNotAllowed
	}
	return nil
}

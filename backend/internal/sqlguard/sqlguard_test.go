package sqlguard

import (
	"errors"
	"testing"
)

func TestValidateReadOnly(t *testing.T) {
	tests := []struct {
		name    string
		sql     string
		wantErr error
	}{
		{name: "select", sql: "SELECT * FROM t"},
		{name: "with", sql: "WITH x AS (SELECT 1) SELECT * FROM x"},
		{name: "lowercase select", sql: "select count(*) from t"},
		{name: "insert", sql: "INSERT INTO t VALUES (1)", wantErr: ErrWriteNotAllowed},
		{name: "lowercase update", sql: "update t set a = 1", wantErr: ErrWriteNotAllowed},
		{name: "delete", sql: "DELETE FROM t", wantErr: ErrWriteNotAllowed},
		{name: "merge", sql: "MERGE INTO t USING s ON t.id = s.id", wantErr: ErrWriteNotAllowed},
		{name: "create", sql: "CREATE TABLE t (id INT)", wantErr: ErrWriteNotAllowed},
		{name: "drop", sql: "DROP TABLE t", wantErr: ErrWriteNotAllowed},
		{name: "alter", sql: "ALTER TABLE t ADD COLUMN c INT", wantErr: ErrWriteNotAllowed},
		{name: "truncate", sql: "TRUNCATE TABLE t", wantErr: ErrWriteNotAllowed},
		{name: "unload", sql: "UNLOAD (SELECT * FROM t) TO 's3://b/p'", wantErr: ErrWriteNotAllowed},
		{name: "keyword as column name substring", sql: "SELECT created_at, updated_at FROM t"},
		{name: "keyword inside identifier", sql: "SELECT * FROM deleted_users_view"},
		{name: "keyword as exact word in where", sql: "SELECT * FROM t WHERE action = 'DELETE'", wantErr: ErrWriteNotAllowed},
		{name: "empty", sql: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateReadOnly(tt.sql)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateReadOnly(%q) = %v, want %v", tt.sql, err, tt.wantErr)
			}
		})
	}
}

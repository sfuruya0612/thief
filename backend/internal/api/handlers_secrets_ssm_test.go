package api

import (
	"errors"
	"strings"
	"testing"
)

func TestParseValueUpdate(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantName  string
		wantValue string
		wantErrIs error  // 期待するセンチネルエラー (errors.Is で照合)
		wantErrIn string // wantErrIs が nil のとき、エラーメッセージに含まれるべき部分文字列
	}{
		{
			name:      "valid name and value",
			body:      `{"name":"/app/db","value":"secret"}`,
			wantName:  "/app/db",
			wantValue: "secret",
		},
		{
			name:      "empty value is allowed",
			body:      `{"name":"/app/db","value":""}`,
			wantName:  "/app/db",
			wantValue: "",
		},
		{
			name:      "missing value key defaults to empty",
			body:      `{"name":"/app/db"}`,
			wantName:  "/app/db",
			wantValue: "",
		},
		{
			name:      "missing name",
			body:      `{"value":"secret"}`,
			wantErrIs: errValueUpdateNameRequired,
		},
		{
			name:      "empty name",
			body:      `{"name":"","value":"secret"}`,
			wantErrIs: errValueUpdateNameRequired,
		},
		{
			name:      "invalid json",
			body:      `{not valid json`,
			wantErrIn: "invalid request body",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseValueUpdate(strings.NewReader(tt.body))

			if tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("err = %v, want errors.Is %v", err, tt.wantErrIs)
				}
				return
			}
			if tt.wantErrIn != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrIn) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErrIn)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantName)
			}
			if got.Value != tt.wantValue {
				t.Errorf("Value = %q, want %q", got.Value, tt.wantValue)
			}
		})
	}
}

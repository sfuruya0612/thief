package datadog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	ddapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
)

func TestIsForbidden(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "plain error", err: errors.New("403 Forbidden"), want: false},
		{
			name: "403 api error",
			err:  ddapi.GenericOpenAPIError{ErrorMessage: "403 Forbidden"},
			want: true,
		},
		{
			// GetHistoricalCost / GetEstimatedCost は %w でラップして返す。
			name: "wrapped 403 api error",
			err:  fmt.Errorf("get datadog historical cost: %w", ddapi.GenericOpenAPIError{ErrorMessage: "403 Forbidden"}),
			want: true,
		},
		{
			name: "401 api error",
			err:  ddapi.GenericOpenAPIError{ErrorMessage: "401 Unauthorized"},
			want: false,
		},
		{
			// 接続に失敗した場合 ErrorMessage は状態行ではなく素のエラー文になる。
			name: "transport api error",
			err:  ddapi.GenericOpenAPIError{ErrorMessage: "dial tcp: connection refused"},
			want: false,
		},
		{
			// 状態コードは先頭のトークンで判定するので、本文に 403 を含むだけの
			// 別状態のエラーを取り違えない。
			name: "500 api error mentioning 403",
			err:  ddapi.GenericOpenAPIError{ErrorMessage: "500 Internal Server Error", ErrorBody: []byte("403")},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsForbidden(tt.err); got != tt.want {
				t.Errorf("IsForbidden(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestErrorBody(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		maxLen int
		want   string
	}{
		{name: "not an api error", err: errors.New("boom"), maxLen: 10, want: ""},
		{
			name:   "body is trimmed",
			err:    ddapi.GenericOpenAPIError{ErrorBody: []byte("  {\"errors\":[\"insufficient_scope\"]}\n")},
			maxLen: 100,
			want:   `{"errors":["insufficient_scope"]}`,
		},
		{
			name:   "body is truncated",
			err:    ddapi.GenericOpenAPIError{ErrorBody: []byte(strings.Repeat("a", 20))},
			maxLen: 5,
			want:   "aaaaa",
		},
		{name: "no body", err: ddapi.GenericOpenAPIError{ErrorMessage: "403 Forbidden"}, maxLen: 10, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ErrorBody(tt.err, tt.maxLen); got != tt.want {
				t.Errorf("ErrorBody() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestNewOAuthContext は OAuth のコンテキストが Bearer トークンだけを運び、静的キーの
// コンテキストと混ざらないことを確認する。両方が同じリクエストに載ると、どちらの資格情報で
// 呼んだのかが判別できなくなる。
func TestNewOAuthContext(t *testing.T) {
	base := context.Background()

	oauthCtx := NewOAuthContext(base, "access-token")
	if got, _ := oauthCtx.Value(ddapi.ContextAccessToken).(string); got != "access-token" {
		t.Errorf("access token in context = %q, want %q", got, "access-token")
	}
	if oauthCtx.Value(ddapi.ContextAPIKeys) != nil {
		t.Error("oauth context carries static API keys")
	}
	if !HasOAuthToken(oauthCtx) {
		t.Error("HasOAuthToken(oauth context) = false, want true")
	}

	staticCtx := NewContext(base, "api-key", "app-key")
	if HasOAuthToken(staticCtx) {
		t.Error("HasOAuthToken(static context) = true, want false")
	}
	if HasOAuthToken(base) {
		t.Error("HasOAuthToken(background) = true, want false")
	}
	if HasOAuthToken(NewOAuthContext(base, "")) {
		t.Error("HasOAuthToken(empty token) = true, want false")
	}
}

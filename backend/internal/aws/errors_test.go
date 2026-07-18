package aws

import (
	"errors"
	"fmt"
	"testing"

	smithy "github.com/aws/smithy-go"
)

func TestIsSSOTokenExpired(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "sentinel", err: ErrSSOTokenExpired, want: true},
		{name: "wrapped sentinel", err: fmt.Errorf("describe ec2 instances: %w", ErrSSOTokenExpired), want: true},
		{name: "unrelated error", err: errors.New("connection refused"), want: false},
		{
			name: "sso session expired",
			err:  errors.New("get identity: get credentials: failed to refresh cached credentials, the SSO session has expired or is invalid"),
			want: true,
		},
		{
			name: "token has expired",
			err:  errors.New("the security token has expired"),
			want: true,
		},
		{
			name: "forbidden exception",
			err:  errors.New("ForbiddenException: no access"),
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSSOTokenExpired(tt.err)
			if got != tt.want {
				t.Errorf("IsSSOTokenExpired(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsAccessDenied(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{
			name: "AccessDeniedException",
			err: &smithy.GenericAPIError{
				Code:    "AccessDeniedException",
				Message: "User: arn:aws:iam::123456789012:user/foo is not authorized to perform: pricing:GetProducts",
			},
			want: true,
		},
		{
			name: "AccessDenied",
			err:  &smithy.GenericAPIError{Code: "AccessDenied", Message: "Access Denied"},
			want: true,
		},
		{
			name: "wrapped AccessDeniedException",
			err:  fmt.Errorf("get products: %w", &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "denied"}),
			want: true,
		},
		{
			name: "unrelated smithy error",
			err:  &smithy.GenericAPIError{Code: "ValidationException", Message: "bad request"},
			want: false,
		},
		{name: "plain error", err: errors.New("not authorized to do this"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAccessDenied(tt.err); got != tt.want {
				t.Errorf("IsAccessDenied(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsAccessDeniedTakesPriorityOverSSOTokenExpired(t *testing.T) {
	// AccessDeniedException のメッセージには "is not authorized to perform" が
	// 含まれることが多く、IsSSOTokenExpired の部分一致 ("not authorized") にも
	// マッチしてしまう。呼び出し側は IsAccessDenied を先に判定する契約なので、
	// 少なくとも両者が同じ入力に true を返し得ることを確認しておく (呼び出し順で
	// 正しく IAM 権限不足として分類できることの前提)。
	err := &smithy.GenericAPIError{
		Code:    "AccessDeniedException",
		Message: "User: arn:aws:iam::123456789012:user/foo is not authorized to perform: pricing:GetProducts",
	}
	if !IsAccessDenied(err) {
		t.Fatal("IsAccessDenied() = false, want true")
	}
	if !IsSSOTokenExpired(err) {
		t.Fatal("IsSSOTokenExpired() = false, want true (confirms the ambiguity IsAccessDenied must be checked first to resolve)")
	}
}

func TestIsThrottled(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "ThrottlingException", err: &smithy.GenericAPIError{Code: "ThrottlingException"}, want: true},
		{name: "TooManyRequestsException", err: &smithy.GenericAPIError{Code: "TooManyRequestsException"}, want: true},
		{name: "unrelated", err: &smithy.GenericAPIError{Code: "ValidationException"}, want: false},
		{name: "plain error", err: errors.New("rate limited"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsThrottled(tt.err); got != tt.want {
				t.Errorf("IsThrottled(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

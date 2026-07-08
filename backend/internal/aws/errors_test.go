package aws

import (
	"errors"
	"fmt"
	"testing"
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

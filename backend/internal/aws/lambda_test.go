package aws

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
)

func TestLambdaFromFunction(t *testing.T) {
	tests := []struct {
		name  string
		state lambdatypes.State
		want  string
	}{
		{name: "empty defaults to active", state: "", want: "active"},
		{name: "pending", state: lambdatypes.StatePending, want: "pending"},
		{name: "active", state: lambdatypes.StateActive, want: "active"},
		{name: "inactive", state: lambdatypes.StateInactive, want: "inactive"},
		{name: "failed", state: lambdatypes.StateFailed, want: "failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn := lambdatypes.FunctionConfiguration{
				FunctionArn:  aws.String("arn:aws:lambda:ap-northeast-1:123:function:foo"),
				FunctionName: aws.String("foo"),
				State:        tt.state,
			}
			got := lambdaFromFunction(fn)
			if got.State != tt.want {
				t.Errorf("state = %q, want %q", got.State, tt.want)
			}
		})
	}
}

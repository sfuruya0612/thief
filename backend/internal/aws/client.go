package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// NewClient creates a typed AWS service client using the given profile and region.
// factory receives the aws.Config and should return the concrete SDK client.
func NewClient[T any](ctx context.Context, profile, region string, factory func(aws.Config) T) (T, error) {
	var zero T
	cfg, err := GetSession(ctx, profile, region)
	if err != nil {
		return zero, fmt.Errorf("new client: %w", err)
	}
	return factory(cfg), nil
}

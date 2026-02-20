// Package aws provides AWS service clients and utilities for interacting with AWS services.
package aws

import (
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
)

// NewClient creates an AWS service client from profile and region using the provided factory function.
// It is a generic helper that eliminates the repeated GetSession → NewFromConfig boilerplate
// common to all New*Client functions.
func NewClient[T any](profile, region string, factory func(awssdk.Config) T) (T, error) {
	cfg, err := GetSession(profile, region)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("create client: %w", err)
	}
	return factory(cfg), nil
}

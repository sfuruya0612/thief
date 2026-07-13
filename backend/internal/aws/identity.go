package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// CallerIdentity is the result of STS GetCallerIdentity.
type CallerIdentity struct {
	AccountID string
	ARN       string
	UserID    string
}

// GetCallerIdentity calls STS GetCallerIdentity for the given profile and
// returns the resolved account ID, ARN, and user ID. Unlike ListProfiles
// (which only reads ~/.aws/config statically), this makes a live AWS call
// and therefore requires valid credentials for profile.
func GetCallerIdentity(ctx context.Context, profile string) (*CallerIdentity, error) {
	client, err := NewClient(ctx, profile, "", func(cfg aws.Config) *sts.Client {
		return sts.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	out, err := client.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, fmt.Errorf("get caller identity for profile %s: %w", profile, err)
	}

	return &CallerIdentity{
		AccountID: ptrStr(out.Account),
		ARN:       ptrStr(out.Arn),
		UserID:    ptrStr(out.UserId),
	}, nil
}

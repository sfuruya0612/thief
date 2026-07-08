package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// StartSessionResult は SSM Session Manager / ECS Exec のデータチャネル接続に必要な情報を保持する。
// TokenValue は使い捨ての短命トークンだが、機密情報としてログには出力しないこと。
type StartSessionResult struct {
	SessionID  string
	StreamURL  string
	TokenValue string
}

// StartSSMSession starts an SSM Session Manager session against the given managed node target
// (typically an EC2 instance ID) and returns the data channel connection info.
func StartSSMSession(ctx context.Context, profile, region, target string) (*StartSessionResult, error) {
	client, err := NewClient(ctx, profile, region, func(cfg aws.Config) *ssm.Client {
		return ssm.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	out, err := client.StartSession(ctx, &ssm.StartSessionInput{
		Target: aws.String(target),
	})
	if err != nil {
		return nil, fmt.Errorf("start ssm session for target %s: %w", target, err)
	}

	return &StartSessionResult{
		SessionID:  ptrStr(out.SessionId),
		StreamURL:  ptrStr(out.StreamUrl),
		TokenValue: ptrStr(out.TokenValue),
	}, nil
}

// TerminateSSMSession terminates the given SSM Session Manager session.
// Callers should invoke this with a short-lived context (e.g. detached from the
// original request context) since it runs as bridge cleanup.
func TerminateSSMSession(ctx context.Context, profile, region, sessionID string) error {
	client, err := NewClient(ctx, profile, region, func(cfg aws.Config) *ssm.Client {
		return ssm.NewFromConfig(cfg)
	})
	if err != nil {
		return err
	}

	if _, err := client.TerminateSession(ctx, &ssm.TerminateSessionInput{
		SessionId: aws.String(sessionID),
	}); err != nil {
		return fmt.Errorf("terminate ssm session %s: %w", sessionID, err)
	}
	return nil
}

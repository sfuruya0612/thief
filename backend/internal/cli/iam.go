package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

func newIAMCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "iam",
		Short: "List IAM users",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd,
				[]util.Column{{Header: "Name"}, {Header: "ARN"}, {Header: "MFA"}, {Header: "LastActive"}, {Header: "Groups"}, {Header: "Policies"}},
				func(ctx context.Context, profile, region string) ([]awsinternal.IAMResource, error) {
					return awsinternal.ListIAMResources(ctx, profile, region)
				},
			)
		},
	}
}

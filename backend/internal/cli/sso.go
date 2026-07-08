package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

func newSSOCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sso",
		Short: "List SSO accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd,
				[]util.Column{{Header: "ID"}, {Header: "Name"}, {Header: "Email"}, {Header: "Roles"}},
				func(ctx context.Context, profile, region string) ([]awsinternal.SSOAccountResource, error) {
					return awsinternal.ListSSOAccounts(ctx, profile, region)
				},
			)
		},
	}
}

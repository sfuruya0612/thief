package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

var iamUserColumns = []util.Column{
	{Header: "UserName"},
	{Header: "UserID"},
	{Header: "Groups"},
	{Header: "Policies"},
	{Header: "CreateDate"},
}

func newIAMCmd() *cobra.Command {
	iamCmd := &cobra.Command{
		Use:   "iam",
		Short: "Manage IAM resources",
		Long:  `Provides commands to list and inspect AWS IAM users.`,
	}

	lsCmd := &cobra.Command{
		Use:   "ls",
		Short: "List IAM users",
		Long:  `Retrieves and displays all IAM users with their attached groups and managed policies.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, ListConfig[awsinternal.IAMUserInfo]{
				Columns:  iamUserColumns,
				EmptyMsg: "No IAM users found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.IAMUserInfo, error) {
					return awsinternal.ListIAMUserInfos(ctx, cfg.Profile)
				},
			})
		},
	}

	iamCmd.AddCommand(lsCmd)
	return iamCmd
}

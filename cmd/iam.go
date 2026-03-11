package cmd

import (
	"github.com/spf13/cobra"

	"github.com/sfuruya0612/thief/internal/aws"
	"github.com/sfuruya0612/thief/internal/config"
	"github.com/sfuruya0612/thief/internal/util"
)

// iamCmd represents the base command for IAM operations.
var iamCmd = &cobra.Command{
	Use:   "iam",
	Short: "Manage IAM resources",
	Long:  `Provides commands to list and inspect AWS IAM users.`,
}

// iamListCmd lists all IAM users with their groups and attached policies.
var iamListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List IAM users",
	Long:  `Retrieves and displays all IAM users with their attached groups and managed policies.`,
	RunE:  listIAMUsers,
}

var iamUserColumns = []util.Column{
	{Header: "UserName"},
	{Header: "UserID"},
	{Header: "Groups"},
	{Header: "Policies"},
	{Header: "CreateDate"},
}

// listIAMUsers retrieves and displays all IAM users.
func listIAMUsers(cmd *cobra.Command, args []string) error {
	return runList[aws.IAMUserInfo](cmd, ListConfig[aws.IAMUserInfo]{
		Columns:  iamUserColumns,
		EmptyMsg: "No IAM users found",
		Fetch: func(cfg *config.Config) ([]aws.IAMUserInfo, error) {
			client, err := aws.NewIAMClient(cfg.Profile, cfg.Region)
			if err != nil {
				return nil, err
			}
			return aws.ListIAMUsers(client)
		},
	})
}

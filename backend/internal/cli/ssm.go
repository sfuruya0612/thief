package cli

import (
	"context"
	"fmt"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

func newSSMCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ssm",
		Short: "Manage SSM Parameter Store",
	}

	listCmd := &cobra.Command{
		Use:   "ls",
		Short: "List SSM parameters",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd,
				[]util.Column{{Header: "Name"}, {Header: "Type"}, {Header: "Tier"}, {Header: "Version"}, {Header: "LastModified"}},
				func(ctx context.Context, profile, region string) ([]awsinternal.SSMParameterResource, error) {
					return awsinternal.ListSSMParameters(ctx, profile, region)
				},
			)
		},
	}

	getCmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get SSM parameter value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			decrypt, _ := cmd.Flags().GetBool("decrypt")
			value, err := awsinternal.GetSSMParameter(context.Background(), cfg.Profile, cfg.Region, args[0], decrypt)
			if err != nil {
				return err
			}
			fmt.Println(value)
			return nil
		},
	}
	getCmd.Flags().Bool("decrypt", false, "Decrypt SecureString value")

	cmd.AddCommand(listCmd, getCmd)
	return cmd
}

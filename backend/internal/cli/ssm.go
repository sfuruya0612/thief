package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

var ssmParamListColumns = []util.Column{
	{Header: "Name"},
	{Header: "Type"},
	{Header: "LastModifiedDate"},
	{Header: "Version"},
	{Header: "DataType"},
}

var ssmParamGetColumns = []util.Column{
	{Header: "Name"},
	{Header: "Type"},
	{Header: "Value"},
	{Header: "Version"},
	{Header: "ARN"},
}

func newSSMCmd() *cobra.Command {
	ssmCmd := &cobra.Command{
		Use:   "ssm",
		Short: "SSM commands",
	}

	paramCmd := &cobra.Command{
		Use:   "param",
		Short: "Parameter Store commands",
	}

	lsCmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List SSM parameters",
		Long:    "Retrieves and displays a list of SSM Parameter Store parameters.",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := cmd.Flags().GetString("path")
			return runList(cmd, ListConfig[awsinternal.SSMParameterInfo]{
				Columns:  ssmParamListColumns,
				EmptyMsg: "No SSM parameters found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.SSMParameterInfo, error) {
					return awsinternal.ListSSMParameterInfos(ctx, cfg.Profile, cfg.Region, path)
				},
			})
		},
	}
	lsCmd.Flags().StringP("path", "", "", "Filter parameters by path prefix (e.g. /myapp/)")

	getCmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get SSM parameter value",
		Long:  "Retrieves the value of a single SSM Parameter Store parameter.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			withDecryption, _ := cmd.Flags().GetBool("with-decryption")

			param, err := awsinternal.GetSSMParameterDetail(context.Background(), cfg.Profile, cfg.Region, args[0], withDecryption)
			if err != nil {
				return err
			}

			return printRowsOrGroupBy(cfg, ssmParamGetColumns, [][]string{param.ToRow()})
		},
	}
	getCmd.Flags().BoolP("with-decryption", "", false, "Decrypt SecureString parameter values")

	putCmd := &cobra.Command{
		Use:   "put <name>",
		Short: "Update an SSM parameter value",
		Long: "Overwrites the value of an existing SSM Parameter Store parameter (Overwrite=true). " +
			"The parameter type and KMS key are retained. " +
			"Provide the new value with --value, or omit it to read the value from stdin.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			value, err := readUpdateValue(cmd, cmd.InOrStdin())
			if err != nil {
				return err
			}
			if err := awsinternal.PutSSMParameter(context.Background(), cfg.Profile, cfg.Region, args[0], value); err != nil {
				return err
			}
			cmd.Printf("Updated SSM parameter %s\n", args[0])
			return nil
		},
	}
	putCmd.Flags().StringP("value", "", "", "New parameter value (if omitted, read from stdin)")

	paramCmd.AddCommand(lsCmd, getCmd, putCmd)
	ssmCmd.AddCommand(paramCmd)
	return ssmCmd
}

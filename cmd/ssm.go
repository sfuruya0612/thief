package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sfuruya0612/thief/internal/aws"
	"github.com/sfuruya0612/thief/internal/config"
	"github.com/sfuruya0612/thief/internal/util"
)

func init() {
	rootCmd.AddCommand(ssmCmd)

	ssmCmd.AddCommand(ssmParamCmd)
	ssmParamCmd.AddCommand(ssmParamListCmd, ssmParamGetCmd)

	ssmParamListCmd.Flags().StringP("path", "", "", "Filter parameters by path prefix (e.g. /myapp/)")
	ssmParamGetCmd.Flags().BoolP("with-decryption", "", false, "Decrypt SecureString parameter values")
}

var ssmCmd = &cobra.Command{
	Use:   "ssm",
	Short: "SSM commands",
}

var ssmParamCmd = &cobra.Command{
	Use:   "param",
	Short: "Parameter Store commands",
}

var ssmParamListCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list"},
	Short:   "List SSM parameters",
	Long:    "Retrieves and displays a list of SSM Parameter Store parameters.",
	RunE:    displaySSMParameters,
}

var ssmParamGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Get SSM parameter value",
	Long:  "Retrieves the value of a single SSM Parameter Store parameter.",
	Args:  cobra.ExactArgs(1),
	RunE:  displaySSMParameterValue,
}

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

func displaySSMParameters(cmd *cobra.Command, args []string) error {
	path, _ := cmd.Flags().GetString("path")

	return runList(cmd, ListConfig[aws.SSMParameterInfo]{
		Columns:  ssmParamListColumns,
		EmptyMsg: "No SSM parameters found",
		Fetch: func(cfg *config.Config) ([]aws.SSMParameterInfo, error) {
			client, err := aws.NewSsmClient(cfg.Profile, cfg.Region)
			if err != nil {
				return nil, fmt.Errorf("create SSM client: %w", err)
			}
			return aws.DescribeParameters(client, aws.GenerateDescribeParametersInput(path))
		},
	})
}

func displaySSMParameterValue(cmd *cobra.Command, args []string) error {
	cfg := config.FromContext(cmd.Context())
	withDecryption, _ := cmd.Flags().GetBool("with-decryption")

	client, err := aws.NewSsmClient(cfg.Profile, cfg.Region)
	if err != nil {
		return fmt.Errorf("create SSM client: %w", err)
	}

	param, err := aws.GetParameter(client, args[0], withDecryption)
	if err != nil {
		return err
	}

	return printRowsOrGroupBy(cfg, ssmParamGetColumns, [][]string{param.ToRow()})
}

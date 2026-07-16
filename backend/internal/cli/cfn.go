package cli

import (
	"context"
	"fmt"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

var cfnStackColumns = []util.Column{
	{Header: "StackName"},
	{Header: "Status"},
	{Header: "DriftStatus"},
	{Header: "CreatedTime"},
	{Header: "UpdatedTime"},
	{Header: "Description"},
}

var cfnParameterColumns = []util.Column{
	{Header: "Key"},
	{Header: "Value"},
	{Header: "ResolvedValue"},
}

var cfnOutputColumns = []util.Column{
	{Header: "Key"},
	{Header: "Value"},
	{Header: "ExportName"},
	{Header: "Description"},
}

var cfnTagColumns = []util.Column{
	{Header: "Key"},
	{Header: "Value"},
}

var cfnChangeColumns = []util.Column{
	{Header: "Action"},
	{Header: "LogicalID"},
	{Header: "ResourceType"},
	{Header: "Replacement"},
}

func newCFNCmd() *cobra.Command {
	cfnCmd := &cobra.Command{
		Use:   "cfn",
		Short: "Manage CloudFormation stacks",
		Long:  `Provides commands to list and inspect AWS CloudFormation stacks and change sets.`,
	}

	lsCmd := &cobra.Command{
		Use:   "ls",
		Short: "List CloudFormation stacks",
		Long:  `Retrieves and displays all CloudFormation stacks (excluding DELETE_COMPLETE), including their drift detection status.`,
		RunE:  listCfnStacks,
	}

	describeCmd := &cobra.Command{
		Use:   "describe <stack-name>",
		Short: "Describe a CloudFormation stack",
		Long:  `Displays detailed information for the specified CloudFormation stack, including its parameters, outputs, and tags.`,
		Args:  cobra.ExactArgs(1),
		RunE:  describeCfnStack,
	}

	changesetCmd := &cobra.Command{
		Use:   "changeset <stack-name> <changeset-name>",
		Short: "Show a CloudFormation Change Set",
		Long:  `Displays the resource changes contained in the specified Change Set.`,
		Args:  cobra.ExactArgs(2),
		RunE:  describeCfnChangeset,
	}

	cfnCmd.AddCommand(lsCmd, describeCmd, changesetCmd)
	return cfnCmd
}

// listCfnStacks retrieves and displays all CloudFormation stacks.
func listCfnStacks(cmd *cobra.Command, args []string) error {
	return runList(cmd, ListConfig[awsinternal.CfnStackSummary]{
		Columns:  cfnStackColumns,
		EmptyMsg: "No CloudFormation stacks found",
		Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.CfnStackSummary, error) {
			return awsinternal.ListCfnStackSummaries(ctx, cfg.Profile, cfg.Region)
		},
	})
}

// describeCfnStack retrieves and displays detailed information for a single CloudFormation stack.
func describeCfnStack(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	stackName := args[0]

	detail, err := awsinternal.DescribeCfnStack(context.Background(), cfg.Profile, cfg.Region, stackName)
	if err != nil {
		return fmt.Errorf("describe stack: %w", err)
	}

	// スタックの基本情報を key-value 形式で出力する。
	infoRows := [][]string{
		{"StackName", detail.StackName},
		{"Status", detail.Status},
		{"DriftStatus", detail.DriftStatus},
		{"CreatedTime", detail.CreatedTime},
		{"UpdatedTime", detail.UpdatedTime},
		{"Description", detail.Description},
	}
	infoColumns := []util.Column{{Header: "Field"}, {Header: "Value"}}
	infoFormatter := util.NewTableFormatter(infoColumns, cfg.Output)
	if !cfg.NoHeader {
		infoFormatter.PrintHeader()
	}
	infoFormatter.PrintRows(infoRows)

	if len(detail.Parameters) > 0 {
		cmd.Println("\nParameters:")
		if err := printRowsOrGroupBy(cfg, cfnParameterColumns, toRows(detail.Parameters)); err != nil {
			return err
		}
	}

	if len(detail.Outputs) > 0 {
		cmd.Println("\nOutputs:")
		if err := printRowsOrGroupBy(cfg, cfnOutputColumns, toRows(detail.Outputs)); err != nil {
			return err
		}
	}

	if len(detail.Tags) > 0 {
		cmd.Println("\nTags:")
		if err := printRowsOrGroupBy(cfg, cfnTagColumns, toRows(detail.Tags)); err != nil {
			return err
		}
	}

	return nil
}

// describeCfnChangeset retrieves and displays the changes within a CloudFormation Change Set.
func describeCfnChangeset(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	stackName := args[0]
	changeSetName := args[1]

	changes, err := awsinternal.DescribeCfnChangeSet(context.Background(), cfg.Profile, cfg.Region, stackName, changeSetName)
	if err != nil {
		return fmt.Errorf("describe change set: %w", err)
	}

	if len(changes) == 0 {
		cmd.Println("No changes found in change set")
		return nil
	}

	return printRowsOrGroupBy(cfg, cfnChangeColumns, toRows(changes))
}

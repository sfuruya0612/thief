package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sfuruya0612/thief/internal/aws"
	"github.com/sfuruya0612/thief/internal/config"
	"github.com/sfuruya0612/thief/internal/util"
)

// cfnCmd represents the base command for CloudFormation operations.
var cfnCmd = &cobra.Command{
	Use:   "cfn",
	Short: "Manage CloudFormation stacks",
	Long:  `Provides commands to list and inspect AWS CloudFormation stacks and change sets.`,
}

// cfnListCmd lists all CloudFormation stacks with drift status.
var cfnListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List CloudFormation stacks",
	Long:  `Retrieves and displays all CloudFormation stacks (excluding DELETE_COMPLETE), including their drift detection status.`,
	RunE:  listCfnStacks,
}

// cfnDescribeCmd shows detailed information for a single stack.
var cfnDescribeCmd = &cobra.Command{
	Use:   "describe <stack-name>",
	Short: "Describe a CloudFormation stack",
	Long:  `Displays detailed information for the specified CloudFormation stack, including its parameters, outputs, and tags.`,
	Args:  cobra.ExactArgs(1),
	RunE:  describeCfnStack,
}

// cfnChangesetCmd shows the changes within a Change Set.
var cfnChangesetCmd = &cobra.Command{
	Use:   "changeset <stack-name> <changeset-name>",
	Short: "Show a CloudFormation Change Set",
	Long:  `Displays the resource changes contained in the specified Change Set.`,
	Args:  cobra.ExactArgs(2),
	RunE:  describeCfnChangeset,
}

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

// listCfnStacks retrieves and displays all CloudFormation stacks.
func listCfnStacks(cmd *cobra.Command, args []string) error {
	return runList[aws.CfnStackSummary](cmd, ListConfig[aws.CfnStackSummary]{
		Columns:  cfnStackColumns,
		EmptyMsg: "No CloudFormation stacks found",
		Fetch: func(cfg *config.Config) ([]aws.CfnStackSummary, error) {
			client, err := aws.NewCfnClient(cfg.Profile, cfg.Region)
			if err != nil {
				return nil, fmt.Errorf("create CloudFormation client: %w", err)
			}
			return aws.ListCfnStacks(client)
		},
	})
}

// describeCfnStack retrieves and displays detailed information for a single CloudFormation stack.
func describeCfnStack(cmd *cobra.Command, args []string) error {
	cfg := config.FromContext(cmd.Context())
	stackName := args[0]

	client, err := aws.NewCfnClient(cfg.Profile, cfg.Region)
	if err != nil {
		return fmt.Errorf("create CloudFormation client: %w", err)
	}

	detail, err := aws.DescribeCfnStack(client, stackName)
	if err != nil {
		return fmt.Errorf("describe stack: %w", err)
	}

	// Print basic stack information as key-value pairs.
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

	// Print Parameters table.
	if len(detail.Parameters) > 0 {
		cmd.Println("\nParameters:")
		if err := printRowsOrGroupBy(cfg, cfnParameterColumns, toRows(detail.Parameters)); err != nil {
			return err
		}
	}

	// Print Outputs table.
	if len(detail.Outputs) > 0 {
		cmd.Println("\nOutputs:")
		if err := printRowsOrGroupBy(cfg, cfnOutputColumns, toRows(detail.Outputs)); err != nil {
			return err
		}
	}

	// Print Tags table.
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
	cfg := config.FromContext(cmd.Context())
	stackName := args[0]
	changeSetName := args[1]

	client, err := aws.NewCfnClient(cfg.Profile, cfg.Region)
	if err != nil {
		return fmt.Errorf("create CloudFormation client: %w", err)
	}

	changes, err := aws.DescribeCfnChangeSet(client, stackName, changeSetName)
	if err != nil {
		return fmt.Errorf("describe change set: %w", err)
	}

	if len(changes) == 0 {
		cmd.Println("No changes found in change set")
		return nil
	}

	return printRowsOrGroupBy(cfg, cfnChangeColumns, toRows(changes))
}

// Package cmd implements the command line interface for thief.
// It uses the Cobra library to define commands and flags.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sfuruya0612/thief/internal/config"
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "thief",
	Short: "CLI for AWS, Datadog, and TiDB services.",
	Long: `Thief is a command-line interface tool designed to interact with
and manage resources across various cloud platforms and services,
including AWS, Datadog, and TiDB.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cmd)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		cmd.SetContext(config.ToContext(cmd.Context(), cfg))
		return nil
	},
}

func init() {
	// Persistent flags available to all commands.
	rootCmd.PersistentFlags().StringP("profile", "p", "", "AWS profile (default uses environment or config file)")
	rootCmd.PersistentFlags().StringP("region", "r", "", "AWS region (default ap-northeast-1)")
	rootCmd.PersistentFlags().StringP("output", "o", "", "Output format (tab/csv)")
	rootCmd.PersistentFlags().BoolP("no-header", "", false, "Hide the header in output")
	rootCmd.PersistentFlags().StringP("group-by", "g", "", "Group output by column name(s) and show count (comma-separated for multiple)")

	rootCmd.AddCommand(
		ssoCmd,
		ec2Cmd,
		ecsCmd,
		rdsCmd,
		elasticacheCmd,
		costexplorerCmd,
	)

	// SSO
	ssoCmd.AddCommand(
		ssoLoginCmd,
		ssoLogoutCmd,
		ssoGenerateConfigCmd,
	)

	ssoLoginCmd.Flags().StringP("url", "", "", "AWS access portal URL")
	ssoGenerateConfigCmd.Flags().StringP("url", "", "", "AWS access portal URL")

	// EC2
	ec2Cmd.AddCommand(
		ec2ListCmd,
		ec2StartSessionCmd,
	)

	ec2ListCmd.Flags().BoolP("running", "", false, "Show only running instances")
	ec2ListCmd.Flags().BoolP("global", "", false, "Show instances in all regions")
	ec2StartSessionCmd.Flags().StringP("instance-id", "i", "", "Instance ID")

	// ECS
	ecsCmd.AddCommand(
		ecsClustersCmd,
		ecsServicesCmd,
		ecsTasksCmd,
		ecsExecCmd,
	)

	ecsTasksCmd.Flags().StringP("cluster", "", "", "Cluster name")
	ecsTasksCmd.Flags().BoolP("running", "", false, "Show only running tasks")

	ecsExecCmd.Flags().StringP("cluster", "", "", "Cluster name")
	ecsExecCmd.Flags().StringP("task", "", "", "Task name")
	ecsExecCmd.Flags().StringP("container", "", "", "Container name")
	ecsExecCmd.Flags().StringP("command", "", "/bin/sh", "Command")

	// RDS
	rdsCmd.AddCommand(
		rdsInstanceCmd,
		rdsClusterCmd,
	)

	// Elasticache
	elasticacheCmd.AddCommand(elasticacheListCmd)

	// Datadog
	rootCmd.AddCommand(datadogCmd)

	datadogCmd.AddCommand(datadogHistoricalCostCmd, datadogEstimatedCostCmd)

	datadogCmd.PersistentFlags().StringP("site", "", "datadoghq.com", "Datadog Site")
	datadogCmd.PersistentFlags().StringP("api-key", "", "", "Datadog API Key")
	datadogCmd.PersistentFlags().StringP("app-key", "", "", "Datadog APP Key")
	datadogCmd.PersistentFlags().StringP("view", "", "summary", "String to specify whether cost is broken down at a parent-org level or at the sub-org level. Available views are summary and sub-org")
	datadogCmd.PersistentFlags().StringP("start-month", "", "", "[YYYY-MM] for cost beginning this month")
	datadogCmd.PersistentFlags().StringP("end-month", "", "", "[YYYY-MM] for cost ending this month")

	// TiDB
	rootCmd.AddCommand(tidbCmd)

	tidbCmd.AddCommand(tidbProjectCmd, tidbClusterCmd, tidbCostCmd)

	tidbCmd.PersistentFlags().StringP("public-key", "", "", "Public Key")
	tidbCmd.PersistentFlags().StringP("private-key", "", "", "Private Key")

	tidbCostCmd.Flags().StringP("billed-month", "", "", "The month of this bill happens for the specified organization. The format is YYYY-MM, for example '2024-05'")

	// Cost Explorer
	costexplorerCmd.AddCommand(costByServiceCmd, costByAccountCmd, costByUsageTypeCmd, costOverviewCmd)

	costexplorerCmd.PersistentFlags().StringP("start-date", "", "", "Start date (YYYY-MM-DD)")
	costexplorerCmd.PersistentFlags().StringP("end-date", "", "", "End date (YYYY-MM-DD)")
	costexplorerCmd.PersistentFlags().StringVarP(&costMetric, "metric", "m", "UnblendedCost", "Cost metric (UnblendedCost, BlendedCost, NetUnblendedCost, NetAmortizedCost, AmortizedCost, UsageQuantity, NormalizedUsageAmount)")
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// GetRootCmd returns the root command for documentation generation.
func GetRootCmd() *cobra.Command {
	return rootCmd
}

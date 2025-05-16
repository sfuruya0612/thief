// Package cmd implements the command line interface for thief.
// It uses the Cobra library to define commands and flags.
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "thief",
	Short: "CLI for AWS, Datadog, and TiDB services.",
	Long: `Thief is a command-line interface tool designed to interact with
and manage resources across various cloud platforms and services,
including AWS, Datadog, and TiDB.`,
}

// initConfig reads in config file and ENV variables if set
func initConfig() {
	// Set the base name of the config file, without extension
	viper.SetConfigName("config")

	// Set the config type
	viper.SetConfigType("yaml")

	// Add config file paths in order of preference
	// First check current directory
	viper.AddConfigPath(".")

	// Then try XDG config path
	if configHome := os.Getenv("XDG_CONFIG_HOME"); configHome != "" {
		viper.AddConfigPath(filepath.Join(configHome, "thief"))
	} else {
		// Fallback to ~/.config/thief
		home, err := os.UserHomeDir()
		if err == nil {
			viper.AddConfigPath(filepath.Join(home, ".config", "thief"))
		}
	}

	// Also look in the user's home directory
	home, err := os.UserHomeDir()
	if err == nil {
		viper.AddConfigPath(filepath.Join(home, ".thief"))
	}

	// Set environment variable prefix to avoid collisions
	viper.SetEnvPrefix("THIEF")

	// Replace . and - in env vars with _
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	// Read in environment variables that match
	viper.AutomaticEnv()

	// Set default values
	viper.SetDefault("region", "ap-northeast-1")
	viper.SetDefault("output", "tab")
	viper.SetDefault("no-header", false)

	// If a config file is found, read it in
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}

func init() {
	// Initialize Viper before command execution
	cobra.OnInitialize(initConfig)

	// Define persistent flags and their default values
	rootCmd.PersistentFlags().StringP("profile", "p", "", "AWS profile (default uses environment or config file)")
	rootCmd.PersistentFlags().StringP("region", "r", "", "AWS region (default ap-northeast-1)")
	rootCmd.PersistentFlags().StringP("output", "o", "", "Output format (tab/csv)")
	rootCmd.PersistentFlags().BoolP("no-header", "", false, "Hide the header in output")

	// Bind flags to viper configuration keys
	if err := viper.BindPFlag("profile", rootCmd.PersistentFlags().Lookup("profile")); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding profile flag: %v\n", err)
	}
	if err := viper.BindPFlag("region", rootCmd.PersistentFlags().Lookup("region")); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding region flag: %v\n", err)
	}
	if err := viper.BindPFlag("output", rootCmd.PersistentFlags().Lookup("output")); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding output flag: %v\n", err)
	}
	if err := viper.BindPFlag("no-header", rootCmd.PersistentFlags().Lookup("no-header")); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding no-header flag: %v\n", err)
	}

	// Also respect AWS_PROFILE environment variable (non-prefixed)
	if err := viper.BindEnv("profile", "AWS_PROFILE"); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding AWS_PROFILE environment variable: %v\n", err)
	}

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
	)

	ssoLoginCmd.Flags().StringP("url", "", "", "AWS access portal URL")

	// EC2
	ec2Cmd.AddCommand(
		ec2ListCmd,
		ec2StartSessionCmd,
	)

	ec2ListCmd.Flags().BoolP("running", "", false, "Show only running instances")
	ec2ListCmd.Flags().BoolP("global", "", false, "Show instances in all regions")

	ec2StartSessionCmd.Flags().StringP("instance-id", "i", "", "Instance ID")

	// Bind EC2 flags to viper
	if err := viper.BindPFlag("running", ec2ListCmd.Flags().Lookup("running")); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding running flag: %v\n", err)
	}
	if err := viper.BindPFlag("global", ec2ListCmd.Flags().Lookup("global")); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding global flag: %v\n", err)
	}
	if err := viper.BindPFlag("instance-id", ec2StartSessionCmd.Flags().Lookup("instance-id")); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding instance-id flag: %v\n", err)
	}

	// Set defaults for EC2 flags
	viper.SetDefault("running", false)
	viper.SetDefault("global", false)
	viper.SetDefault("instance-id", "")

	// ECS
	ecsCmd.AddCommand(
		ecsClustersCmd,
		ecsServicesCmd,
		ecsTasksCmd,
		ecsExecCmd,
	)

	ecsTasksCmd.Flags().StringP("cluster", "", "", "Cluster name")

	ecsExecCmd.Flags().StringP("cluster", "", "", "Cluster name")
	ecsExecCmd.Flags().StringP("task", "", "", "Task name")
	ecsExecCmd.Flags().StringP("container", "", "", "Container name")
	ecsExecCmd.Flags().StringP("command", "", "/bin/sh", "Command")
	// If specified false, the command return error.
	// InvalidParameterException: Interactive is the only mode supported currently.
	// ecsExecCmd.Flags().BoolP("interactive", "", true, "Interactive mode")

	// ECR

	// RDS
	rdsCmd.AddCommand(
		rdsInstanceCmd,
		rdsClusterCmd,
	)

	// Elasticache
	elasticacheCmd.AddCommand(elasticacheListCmd)

	// Lambda

	// Kinesis

	// CloudFormation

	// Route53

	// Datadog
	rootCmd.AddCommand(datadogCmd)

	datadogCmd.AddCommand(datadogHistoricalCostCmd, datadogEstimatedCostCmd)

	datadogCmd.PersistentFlags().StringP("site", "", "datadoghq.com", "Datadog Site")
	datadogCmd.PersistentFlags().StringP("api-key", "", "", "Datadog API Key")
	datadogCmd.PersistentFlags().StringP("app-key", "", "", "Datadog APP Key")
	datadogCmd.PersistentFlags().StringP("view", "", "summary", "String to specify whether cost is broken down at a parent-org level or at the sub-org level. Available views are summary and sub-org")
	datadogCmd.PersistentFlags().StringP("start-month", "", "", "[YYYY-MM] for cost beginning this month")
	datadogCmd.PersistentFlags().StringP("end-month", "", "", "[YYYY-MM] for cost ending this month")

	// Bind Datadog flags to viper
	if err := viper.BindPFlag("datadog.site", datadogCmd.PersistentFlags().Lookup("site")); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding datadog.site flag: %v\n", err)
	}
	if err := viper.BindPFlag("datadog.api-key", datadogCmd.PersistentFlags().Lookup("api-key")); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding datadog.api-key flag: %v\n", err)
	}
	if err := viper.BindPFlag("datadog.app-key", datadogCmd.PersistentFlags().Lookup("app-key")); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding datadog.app-key flag: %v\n", err)
	}
	if err := viper.BindPFlag("datadog.view", datadogCmd.PersistentFlags().Lookup("view")); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding datadog.view flag: %v\n", err)
	}
	if err := viper.BindPFlag("datadog.start-month", datadogCmd.PersistentFlags().Lookup("start-month")); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding datadog.start-month flag: %v\n", err)
	}
	if err := viper.BindPFlag("datadog.end-month", datadogCmd.PersistentFlags().Lookup("end-month")); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding datadog.end-month flag: %v\n", err)
	}

	// Set defaults
	viper.SetDefault("datadog.site", "datadoghq.com")
	viper.SetDefault("datadog.view", "summary")

	// Bind to environment variables with proper prefixing
	if err := viper.BindEnv("datadog.api-key", "DATADOG_API_KEY"); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding DATADOG_API_KEY environment variable: %v\n", err)
	}
	if err := viper.BindEnv("datadog.app-key", "DATADOG_APP_KEY"); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding DATADOG_APP_KEY environment variable: %v\n", err)
	}

	// TiDB
	rootCmd.AddCommand(tidbCmd)

	tidbCmd.AddCommand(tidbProjectCmd, tidbClusterCmd, tidbCostCmd)

	tidbCmd.PersistentFlags().StringP("public-key", "", "", "Public Key")
	tidbCmd.PersistentFlags().StringP("private-key", "", "", "Private Key")

	tidbCostCmd.Flags().StringP("billed-month", "", "", "The month of this bill happens for the specified organization. The format is YYYY-MM, for example '2024-05'")

	// Bind TiDB flags to viper
	if err := viper.BindPFlag("tidb.public-key", tidbCmd.PersistentFlags().Lookup("public-key")); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding tidb.public-key flag: %v\n", err)
	}
	if err := viper.BindPFlag("tidb.private-key", tidbCmd.PersistentFlags().Lookup("private-key")); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding tidb.private-key flag: %v\n", err)
	}
	if err := viper.BindPFlag("tidb.billed-month", tidbCostCmd.Flags().Lookup("billed-month")); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding tidb.billed-month flag: %v\n", err)
	}

	// Bind to environment variables with proper prefixing
	if err := viper.BindEnv("tidb.public-key", "TIDB_PUBLIC_KEY"); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding TIDB_PUBLIC_KEY environment variable: %v\n", err)
	}
	if err := viper.BindEnv("tidb.private-key", "TIDB_PRIVATE_KEY"); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding TIDB_PRIVATE_KEY environment variable: %v\n", err)
	}
	// tidbCmd.PersistentFlags().StringP("region", "", "", "Region")

	// Cost Explorer
	costexplorerCmd.AddCommand(costByServiceCmd, costByAccountCmd, costOverviewCmd)

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

// for generate documentation
func GetRootCmd() *cobra.Command {
	return rootCmd
}

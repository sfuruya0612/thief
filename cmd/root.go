package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// TODO: Description
var rootCmd = &cobra.Command{
	Use:   "thief",
	Short: "",
	Long:  ``,
}

func init() {
	rootCmd.PersistentFlags().StringP("profile", "p", viper.GetString("AWS_PROFILE"), "AWS profile (default $AWS_PROFILE)")
	rootCmd.PersistentFlags().StringP("region", "r", "ap-northeast-1", "AWS region")
	rootCmd.PersistentFlags().StringP("output", "o", "tab", "Output format (tab/csv)")
	// TODO: ヘッダーの出し分け
	// rootCmd.PersistentFlags().BoolP("header", "h", false, "Show header (true/false)")

	rootCmd.AddCommand(ssoCmd, ec2Cmd, rdsCmd, elasticacheCmd)

	// SSO
	ssoCmd.AddCommand(ssoLoginCmd, ssoLogoutCmd)

	ssoLoginCmd.Flags().StringP("url", "", "", "AWS access portal URL")

	// EC2
	ec2Cmd.AddCommand(ec2ListCmd, ec2StartSessionCmd)

	ec2ListCmd.Flags().BoolP("running", "", false, "Show only running instances")
	ec2ListCmd.Flags().BoolP("global", "", false, "Show instances in all regions")

	ec2StartSessionCmd.Flags().StringP("instance-id", "i", "", "Instance ID")

	// ECS

	// ECR

	// RDS
	rdsCmd.AddCommand(rdsInstanceCmd, rdsClusterCmd)

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
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// for generate documentation
func GetRootCmd() *cobra.Command {
	return rootCmd
}

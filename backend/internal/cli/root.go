package cli

import (
	"github.com/spf13/cobra"
)

// NewRootCmd returns the root cobra command with all subcommands registered.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "thief",
		Short:        "Cloud resource viewer — AWS, BigQuery, Datadog, TiDB",
		SilenceUsage: true,
	}

	// Persistent flags available to all subcommands.
	root.PersistentFlags().StringP("profile", "p", "", "AWS profile name (overrides AWS_PROFILE)")
	root.PersistentFlags().StringP("region", "r", "ap-northeast-1", "AWS region")
	root.PersistentFlags().StringP("output", "o", "tab", "Output format: tab|csv")
	root.PersistentFlags().Bool("no-header", false, "Suppress header row")

	root.AddCommand(
		newEC2Cmd(),
		newRDSCmd(),
		newElastiCacheCmd(),
		newLambdaCmd(),
		newECSCmd(),
		newECRCmd(),
		newS3Cmd(),
		newIAMCmd(),
		newSSOCmd(),
		newSSMCmd(),
		newCFNCmd(),
		newKinesisCmd(),
		newCloudFrontCmd(),
		newELBCmd(),
		newCostCmd(),
		newBQCmd(),
		newDatadogCmd(),
		newTiDBCmd(),
		newServerCmd(),
	)
	return root
}

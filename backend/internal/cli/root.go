package cli

import (
	"github.com/spf13/cobra"
)

// NewRootCmd returns the root cobra command with all subcommands registered.
// コマンドツリーとフラグはレガシー CLI (リポジトリルート cmd/) のインターフェースに合わせる。
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "thief",
		Short: "CLI for AWS, BigQuery, Datadog, and TiDB services.",
		Long: `Thief is a command-line interface tool designed to interact with
and manage resources across various cloud platforms and services,
including AWS, BigQuery, Datadog, and TiDB.`,

		// エラーと usage の表示は Run に集約する。Cobra にも表示させると同じ文言が
		// 2 回並ぶ。中断時に usage 全文が出るのも防ぐ。使い方の誤りに対する help への
		// 誘導は Run が出し直す。
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	// Persistent flags available to all subcommands.
	root.PersistentFlags().StringP("profile", "p", "", "AWS profile (default uses environment or config file)")
	root.PersistentFlags().StringP("region", "r", "", "AWS region (default ap-northeast-1)")
	root.PersistentFlags().StringP("output", "o", "", "Output format (tab/csv)")
	root.PersistentFlags().BoolP("no-header", "", false, "Hide the header in output")
	root.PersistentFlags().StringP("group-by", "g", "", "Group output by column name(s) and show count (comma-separated for multiple)")

	root.AddCommand(
		// レガシー CLI と共通のコマンド群
		newSSOCmd(),
		newEC2Cmd(),
		newECSCmd(),
		newRDSCmd(),
		newElastiCacheCmd(),
		newCostCmd(),
		newCFNCmd(),
		newIAMCmd(),
		newECRCmd(),
		newDatadogCmd(),
		newTiDBCmd(),
		newBQCmd(),
		newS3Cmd(),
		newSSMCmd(),
		newSecretsManagerCmd(),
		// backend 専用のコマンド群
		newLambdaCmd(),
		newKinesisCmd(),
		newCloudFrontCmd(),
		newELBCmd(),
		newLogsCmd(),
		newGCPCmd(),
		newServerCmd(),
	)
	return root
}

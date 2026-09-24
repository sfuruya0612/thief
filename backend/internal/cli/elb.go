package cli

import (
	"context"
	"fmt"
	"strings"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

var elbColumns = []util.Column{
	{Header: "Name"},
	{Header: "Type"},
	{Header: "State"},
	{Header: "Scheme"},
	{Header: "DNS"},
	{Header: "VPC"},
	{Header: "AZs"},
}

var elbListenerColumns = []util.Column{
	{Header: "Port"},
	{Header: "Protocol"},
	{Header: "DefaultAction"},
	{Header: "TargetGroup"},
	{Header: "ARN"},
}

var elbRuleColumns = []util.Column{
	{Header: "Priority"},
	{Header: "Conditions"},
	{Header: "Action"},
	{Header: "TargetGroup"},
	{Header: "ARN"},
}

var elbTargetGroupColumns = []util.Column{
	{Header: "Name"},
	{Header: "Protocol"},
	{Header: "Port"},
	{Header: "TargetType"},
	{Header: "VPC"},
	{Header: "HealthCheckPath"},
	{Header: "ARN"},
}

var elbTargetHealthColumns = []util.Column{
	{Header: "Target"},
	{Header: "Port"},
	{Header: "AZ"},
	{Header: "State"},
	{Header: "Reason"},
	{Header: "Description"},
}

// elbLister は --lb に渡された名前を ARN へ解決するときに使うロードバランサ一覧の取得。
// テストで AWS への接続を差し替えられるよう関数型で受け取る。
type elbLister func(ctx context.Context, profile, region string) ([]awsinternal.ELBResource, error)

// elbOps は elb のサブコマンドが呼ぶ取得関数の集合。テストで AWS への接続を差し替え、
// フラグから取得関数の引数と列定義までの結線を検証できるようにする (athenaQueryOps と同じ形)。
type elbOps struct {
	listLoadBalancers    elbLister
	listListeners        func(ctx context.Context, profile, region, lbArn string) ([]awsinternal.ELBListenerResource, error)
	listRules            func(ctx context.Context, profile, region, listenerArn string) ([]awsinternal.ELBRuleResource, error)
	listTargetGroups     func(ctx context.Context, profile, region, lbArn string) ([]awsinternal.ELBTargetGroupResource, error)
	describeTargetHealth func(ctx context.Context, profile, region, tgArn string) ([]awsinternal.ELBTargetHealthResource, error)
}

// defaultELBOps は internal/aws の実装を束ねた本番用の取得関数の集合を返す。
func defaultELBOps() elbOps {
	return elbOps{
		listLoadBalancers:    awsinternal.ListELBResources,
		listListeners:        awsinternal.ListELBListeners,
		listRules:            awsinternal.ListELBRules,
		listTargetGroups:     awsinternal.ListELBTargetGroups,
		describeTargetHealth: awsinternal.DescribeELBTargetHealth,
	}
}

// resolveELBLoadBalancerArn はロードバランサを名前または ARN で指定できるようにする。
// ARN は API を呼ばずにそのまま使い、名前は一覧の Name と突き合わせて ID (ARN) を返す。
func resolveELBLoadBalancerArn(ctx context.Context, list elbLister, profile, region, value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("load balancer name or ARN is required")
	}
	if strings.HasPrefix(value, "arn:") {
		return value, nil
	}

	resources, err := list(ctx, profile, region)
	if err != nil {
		return "", fmt.Errorf("list load balancers: %w", err)
	}
	for _, lb := range resources {
		if lb.Name == value {
			return lb.ID, nil
		}
	}
	return "", fmt.Errorf("load balancer not found: %s", value)
}

func newELBCmd() *cobra.Command {
	return newELBCmdWithOps(defaultELBOps())
}

// newELBCmdWithOps は取得関数を差し替えられる形で elb コマンドを組み立てる。
func newELBCmdWithOps(ops elbOps) *cobra.Command {
	// 親の elb は従来どおりロードバランサの一覧を出す。サブコマンドを持つようになったため、
	// 綴りを誤ったサブコマンド名が一覧の出力に化けないよう位置引数は受け付けない。
	elbCmd := &cobra.Command{
		Use:   "elb",
		Short: "List Elastic Load Balancers (ALB/NLB/CLB)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, ListConfig[awsinternal.ELBResource]{
				Columns:  elbColumns,
				EmptyMsg: "No load balancers found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.ELBResource, error) {
					return ops.listLoadBalancers(ctx, cfg.Profile, cfg.Region)
				},
			})
		},
	}

	// listeners と target-groups は --lb の名前を ARN へ解決してから取得する。
	listenersCmd := &cobra.Command{
		Use:   "listeners",
		Short: "List listeners of a load balancer",
		Long:  "Retrieves and displays the listeners of the load balancer specified by --lb (name or ARN).",
		RunE: func(cmd *cobra.Command, args []string) error {
			lb, _ := cmd.Flags().GetString("lb")
			return runList(cmd, ListConfig[awsinternal.ELBListenerResource]{
				Columns:  elbListenerColumns,
				EmptyMsg: "No ELB listeners found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.ELBListenerResource, error) {
					lbArn, err := resolveELBLoadBalancerArn(ctx, ops.listLoadBalancers, cfg.Profile, cfg.Region, lb)
					if err != nil {
						return nil, err
					}
					return ops.listListeners(ctx, cfg.Profile, cfg.Region, lbArn)
				},
			})
		},
	}
	listenersCmd.Flags().String("lb", "", "Load balancer name or ARN (required)")
	_ = listenersCmd.MarkFlagRequired("lb")

	rulesCmd := &cobra.Command{
		Use:   "rules",
		Short: "List rules of a listener",
		Long:  "Retrieves and displays the rules of the listener specified by --listener (listener ARN).",
		RunE: func(cmd *cobra.Command, args []string) error {
			listener, _ := cmd.Flags().GetString("listener")
			return runList(cmd, ListConfig[awsinternal.ELBRuleResource]{
				Columns:  elbRuleColumns,
				EmptyMsg: "No ELB rules found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.ELBRuleResource, error) {
					return ops.listRules(ctx, cfg.Profile, cfg.Region, listener)
				},
			})
		},
	}
	rulesCmd.Flags().String("listener", "", "Listener ARN (required)")
	_ = rulesCmd.MarkFlagRequired("listener")

	targetGroupsCmd := &cobra.Command{
		Use:   "target-groups",
		Short: "List target groups of a load balancer",
		Long:  "Retrieves and displays the target groups of the load balancer specified by --lb (name or ARN).",
		RunE: func(cmd *cobra.Command, args []string) error {
			lb, _ := cmd.Flags().GetString("lb")
			return runList(cmd, ListConfig[awsinternal.ELBTargetGroupResource]{
				Columns:  elbTargetGroupColumns,
				EmptyMsg: "No ELB target groups found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.ELBTargetGroupResource, error) {
					lbArn, err := resolveELBLoadBalancerArn(ctx, ops.listLoadBalancers, cfg.Profile, cfg.Region, lb)
					if err != nil {
						return nil, err
					}
					return ops.listTargetGroups(ctx, cfg.Profile, cfg.Region, lbArn)
				},
			})
		},
	}
	targetGroupsCmd.Flags().String("lb", "", "Load balancer name or ARN (required)")
	_ = targetGroupsCmd.MarkFlagRequired("lb")

	targetHealthCmd := &cobra.Command{
		Use:   "target-health",
		Short: "List the health of the targets in a target group",
		Long:  "Retrieves and displays the health of every target registered with the target group specified by --target-group (target group ARN).",
		RunE: func(cmd *cobra.Command, args []string) error {
			targetGroup, _ := cmd.Flags().GetString("target-group")
			return runList(cmd, ListConfig[awsinternal.ELBTargetHealthResource]{
				Columns:  elbTargetHealthColumns,
				EmptyMsg: "No ELB targets found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.ELBTargetHealthResource, error) {
					return ops.describeTargetHealth(ctx, cfg.Profile, cfg.Region, targetGroup)
				},
			})
		},
	}
	targetHealthCmd.Flags().String("target-group", "", "Target group ARN (required)")
	_ = targetHealthCmd.MarkFlagRequired("target-group")

	elbCmd.AddCommand(listenersCmd, rulesCmd, targetGroupsCmd, targetHealthCmd)
	return elbCmd
}

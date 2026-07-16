package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

func newELBCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "elb",
		Short: "List Elastic Load Balancers (ALB/NLB/CLB)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, ListConfig[awsinternal.ELBResource]{
				Columns:  []util.Column{{Header: "Name"}, {Header: "Type"}, {Header: "State"}, {Header: "Scheme"}, {Header: "DNS"}, {Header: "VPC"}, {Header: "AZs"}},
				EmptyMsg: "No load balancers found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.ELBResource, error) {
					return awsinternal.ListELBResources(ctx, cfg.Profile, cfg.Region)
				},
			})
		},
	}
}

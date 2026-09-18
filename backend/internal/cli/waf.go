package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

var wafColumns = []util.Column{
	{Header: "WebACL"},
	{Header: "Description"},
	{Header: "State"},
	{Header: "Scope"},
	{Header: "Rules"},
	{Header: "Associated"},
}

func newWAFCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "waf",
		Short: "List WAFv2 Web ACLs (REGIONAL and CLOUDFRONT)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, ListConfig[awsinternal.WAFResource]{
				Columns:  wafColumns,
				EmptyMsg: "No WAF Web ACLs found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.WAFResource, error) {
					return awsinternal.ListWAFResources(ctx, cfg.Profile, cfg.Region)
				},
			})
		},
	}
}

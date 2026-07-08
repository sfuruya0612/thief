package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

func newRDSCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rds",
		Short: "List RDS instances",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd,
				[]util.Column{{Header: "ID"}, {Header: "State"}, {Header: "Engine"}, {Header: "Version"}, {Header: "Class"}, {Header: "MultiAZ"}, {Header: "Endpoint"}},
				func(ctx context.Context, profile, region string) ([]awsinternal.RDSResource, error) {
					return awsinternal.ListRDSResources(ctx, profile, region)
				},
			)
		},
	}
}

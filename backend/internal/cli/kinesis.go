package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

func newKinesisCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "kinesis",
		Short: "List Kinesis Data Streams",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd,
				[]util.Column{{Header: "Name"}, {Header: "State"}, {Header: "Shards"}, {Header: "Retention(h)"}, {Header: "Encryption"}},
				func(ctx context.Context, profile, region string) ([]awsinternal.KinesisResource, error) {
					return awsinternal.ListKinesisResources(ctx, profile, region)
				},
			)
		},
	}
}

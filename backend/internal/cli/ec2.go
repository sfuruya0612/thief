package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

func newEC2Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ec2",
		Short: "List EC2 instances",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd,
				[]util.Column{{Header: "ID"}, {Header: "Name"}, {Header: "State"}, {Header: "Type"}, {Header: "AZ"}, {Header: "PrivateIP"}, {Header: "PublicIP"}, {Header: "VPC"}, {Header: "Tags"}},
				func(ctx context.Context, profile, region string) ([]awsinternal.EC2Resource, error) {
					return awsinternal.ListEC2Resources(ctx, profile, region)
				},
			)
		},
	}
}

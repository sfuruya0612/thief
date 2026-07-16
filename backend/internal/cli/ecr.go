package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

var ecrRepoColumns = []util.Column{
	{Header: "RepositoryName"},
	{Header: "RepositoryUri"},
	{Header: "CreatedAt"},
}

var ecrImageColumns = []util.Column{
	{Header: "RepositoryName"},
	{Header: "ImageTag"},
	{Header: "ImageDigest"},
	{Header: "PushedAt"},
	{Header: "LastPulledAt"},
	{Header: "ImageSizeBytes"},
}

func newECRCmd() *cobra.Command {
	ecrCmd := &cobra.Command{
		Use:   "ecr",
		Short: "Manage ECR resources",
		Long:  `Provides commands to list ECR repositories and their images.`,
	}

	lsCmd := &cobra.Command{
		Use:   "ls",
		Short: "List ECR repositories",
		Long:  `Retrieves and displays all ECR repositories.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, ListConfig[awsinternal.ECRRepoInfo]{
				Columns:  ecrRepoColumns,
				EmptyMsg: "No ECR repositories found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.ECRRepoInfo, error) {
					return awsinternal.ListECRRepoInfos(ctx, cfg.Profile, cfg.Region)
				},
			})
		},
	}

	imagesCmd := &cobra.Command{
		Use:   "images",
		Short: "List images in an ECR repository",
		Long:  `Retrieves and displays all images and their tags in the specified ECR repository.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := cmd.Flags().GetString("repo")
			if err != nil {
				return err
			}
			all, err := cmd.Flags().GetBool("all")
			if err != nil {
				return err
			}
			return runList(cmd, ListConfig[awsinternal.ECRImageInfo]{
				Columns:  ecrImageColumns,
				EmptyMsg: "No images found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.ECRImageInfo, error) {
					return awsinternal.ListECRImageInfos(ctx, cfg.Profile, cfg.Region, repo, all)
				},
			})
		},
	}
	imagesCmd.Flags().StringP("repo", "", "", "Repository name")
	imagesCmd.Flags().BoolP("all", "", false, "Fetch all images across all pages")

	ecrCmd.AddCommand(lsCmd, imagesCmd)
	return ecrCmd
}

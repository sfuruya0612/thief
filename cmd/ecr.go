package cmd

import (
	"github.com/spf13/cobra"

	"github.com/sfuruya0612/thief/internal/aws"
	"github.com/sfuruya0612/thief/internal/config"
	"github.com/sfuruya0612/thief/internal/util"
)

// ecrCmd represents the base command for ECR operations.
var ecrCmd = &cobra.Command{
	Use:   "ecr",
	Short: "Manage ECR resources",
	Long:  `Provides commands to list ECR repositories and their images.`,
}

// ecrListCmd lists all ECR repositories.
var ecrListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List ECR repositories",
	Long:  `Retrieves and displays all ECR repositories.`,
	RunE:  listECRRepositories,
}

// ecrImagesCmd lists images in a specified ECR repository.
var ecrImagesCmd = &cobra.Command{
	Use:   "images",
	Short: "List images in an ECR repository",
	Long:  `Retrieves and displays all images and their tags in the specified ECR repository.`,
	RunE:  listECRImages,
}

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

// listECRRepositories retrieves and displays all ECR repositories.
func listECRRepositories(cmd *cobra.Command, args []string) error {
	return runList[aws.ECRRepoInfo](cmd, ListConfig[aws.ECRRepoInfo]{
		Columns:  ecrRepoColumns,
		EmptyMsg: "No ECR repositories found",
		Fetch: func(cfg *config.Config) ([]aws.ECRRepoInfo, error) {
			client, err := aws.NewECRClient(cfg.Profile, cfg.Region)
			if err != nil {
				return nil, err
			}
			return aws.ListECRRepositories(client)
		},
	})
}

// listECRImages retrieves and displays images in the specified ECR repository.
func listECRImages(cmd *cobra.Command, args []string) error {
	return runList[aws.ECRImageInfo](cmd, ListConfig[aws.ECRImageInfo]{
		Columns:  ecrImageColumns,
		EmptyMsg: "No images found",
		Fetch: func(cfg *config.Config) ([]aws.ECRImageInfo, error) {
			repo, err := cmd.Flags().GetString("repo")
			if err != nil {
				return nil, err
			}
			all, err := cmd.Flags().GetBool("all")
			if err != nil {
				return nil, err
			}
			client, err := aws.NewECRClient(cfg.Profile, cfg.Region)
			if err != nil {
				return nil, err
			}
			return aws.ListECRImages(client, repo, all)
		},
	})
}

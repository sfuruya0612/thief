package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
)

// ECRRepoResource represents an ECR repository.
type ECRRepoResource struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	State              string `json:"state"`
	URI                string `json:"uri"`
	CreatedAt          string `json:"created_at"`
	ImageTagMutability string `json:"image_tag_mutability"`
	ScanOnPush         bool   `json:"scan_on_push"`
}

func (r ECRRepoResource) ResourceID() string    { return r.ID }
func (r ECRRepoResource) ResourceName() string  { return r.Name }
func (r ECRRepoResource) ResourceState() string { return "active" }
func (r ECRRepoResource) ServiceName() string   { return "ecr" }

// ECRImageResource represents a single image in an ECR repository.
type ECRImageResource struct {
	RepositoryName string `json:"repository_name"`
	ImageTag       string `json:"image_tag"`
	ImageDigest    string `json:"image_digest"`
	PushedAt       string `json:"pushed_at"`
	ImageSizeBytes int64  `json:"image_size_bytes"`
}

// ListECRResources returns all ECR repositories for the given profile/region.
func ListECRResources(ctx context.Context, profile, region string) ([]ECRRepoResource, error) {
	client, err := NewClient(ctx, profile, region, func(cfg aws.Config) *ecr.Client {
		return ecr.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	var resources []ECRRepoResource
	paginator := ecr.NewDescribeRepositoriesPaginator(client, &ecr.DescribeRepositoriesInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe ecr repositories: %w", err)
		}
		for _, repo := range page.Repositories {
			resources = append(resources, ecrRepoFromRepo(repo))
		}
	}
	return resources, nil
}

// ListECRImages returns all images in the given ECR repository.
func ListECRImages(ctx context.Context, profile, region, repoName string) ([]ECRImageResource, error) {
	client, err := NewClient(ctx, profile, region, func(cfg aws.Config) *ecr.Client {
		return ecr.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	var images []ECRImageResource
	paginator := ecr.NewDescribeImagesPaginator(client, &ecr.DescribeImagesInput{
		RepositoryName: aws.String(repoName),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe ecr images for %s: %w", repoName, err)
		}
		for _, img := range page.ImageDetails {
			images = append(images, ecrImageFromDetail(repoName, img))
		}
	}
	return images, nil
}

func ecrRepoFromRepo(repo ecrtypes.Repository) ECRRepoResource {
	createdAt := ""
	if repo.CreatedAt != nil {
		createdAt = repo.CreatedAt.Format(time.RFC3339)
	}
	scanOnPush := false
	if repo.ImageScanningConfiguration != nil {
		scanOnPush = repo.ImageScanningConfiguration.ScanOnPush
	}
	return ECRRepoResource{
		ID:                 ptrStr(repo.RepositoryArn),
		Name:               ptrStr(repo.RepositoryName),
		URI:                ptrStr(repo.RepositoryUri),
		CreatedAt:          createdAt,
		ImageTagMutability: string(repo.ImageTagMutability),
		ScanOnPush:         scanOnPush,
	}
}

func ecrImageFromDetail(repoName string, img ecrtypes.ImageDetail) ECRImageResource {
	tag := ""
	if len(img.ImageTags) > 0 {
		tag = img.ImageTags[0]
	}
	pushedAt := ""
	if img.ImagePushedAt != nil {
		pushedAt = img.ImagePushedAt.Format(time.RFC3339)
	}
	return ECRImageResource{
		RepositoryName: repoName,
		ImageTag:       tag,
		ImageDigest:    ptrStr(img.ImageDigest),
		PushedAt:       pushedAt,
		ImageSizeBytes: ptrInt64(img.ImageSizeInBytes),
	}
}

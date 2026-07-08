// Package aws provides AWS service clients and utilities for interacting with AWS services.
package aws

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecr/types"
)

// ecrApi defines the interface for ECR API operations used by this package.
type ecrApi interface {
	DescribeRepositories(ctx context.Context, input *ecr.DescribeRepositoriesInput, opts ...func(*ecr.Options)) (*ecr.DescribeRepositoriesOutput, error)
	DescribeImages(ctx context.Context, input *ecr.DescribeImagesInput, opts ...func(*ecr.Options)) (*ecr.DescribeImagesOutput, error)
}

// NewECRClient creates a new ECR client using the specified AWS profile and region.
func NewECRClient(profile, region string) (ecrApi, error) {
	return NewClient(profile, region, func(cfg aws.Config) *ecr.Client {
		return ecr.NewFromConfig(cfg)
	})
}

// ECRRepoInfo holds the display fields for an ECR repository.
type ECRRepoInfo struct {
	RepositoryName string
	RepositoryUri  string
	CreatedAt      string
}

// ToRow converts ECRRepoInfo to a string slice suitable for table formatting.
func (r ECRRepoInfo) ToRow() []string {
	return []string{r.RepositoryName, r.RepositoryUri, r.CreatedAt}
}

// ECRImageInfo holds the display fields for an ECR image.
type ECRImageInfo struct {
	RepositoryName   string
	ImageTag         string
	ImageDigest      string
	PushedAt         string
	LastPulledAt     string
	ImageSizeBytes   string
}

// ToRow converts ECRImageInfo to a string slice suitable for table formatting.
func (i ECRImageInfo) ToRow() []string {
	return []string{i.RepositoryName, i.ImageTag, i.ImageDigest, i.PushedAt, i.LastPulledAt, i.ImageSizeBytes}
}

// ListECRRepositories retrieves all ECR repositories with pagination.
func ListECRRepositories(api ecrApi) ([]ECRRepoInfo, error) {
	var repos []ECRRepoInfo
	var nextToken *string

	for {
		o, err := api.DescribeRepositories(context.Background(), &ecr.DescribeRepositoriesInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, err
		}

		for _, r := range o.Repositories {
			createdAt := ""
			if r.CreatedAt != nil {
				createdAt = r.CreatedAt.String()
			}
			repos = append(repos, ECRRepoInfo{
				RepositoryName: aws.ToString(r.RepositoryName),
				RepositoryUri:  aws.ToString(r.RepositoryUri),
				CreatedAt:      createdAt,
			})
		}

		if o.NextToken == nil {
			break
		}
		nextToken = o.NextToken
	}

	return repos, nil
}

// ListECRImages retrieves images in the specified ECR repository.
// By default it fetches only the first page (up to 1000 results).
// If all is true, it fetches all pages via pagination.
func ListECRImages(api ecrApi, repoName string, all bool) ([]ECRImageInfo, error) {
	var images []ECRImageInfo
	var nextToken *string

	for {
		input := &ecr.DescribeImagesInput{
			RepositoryName: aws.String(repoName),
			NextToken:      nextToken,
			MaxResults:     aws.Int32(map[bool]int32{true: 1000, false: 30}[all]),
		}
		if !all {
			input.Filter = &types.DescribeImagesFilter{TagStatus: types.TagStatusTagged}
		}
		o, err := api.DescribeImages(context.Background(), input)
		if err != nil {
			return nil, fmt.Errorf("describe images for %s: %w", repoName, err)
		}

		for _, img := range o.ImageDetails {
			tags := strings.Join(img.ImageTags, ",")
			pushedAt := ""
			if img.ImagePushedAt != nil {
				pushedAt = img.ImagePushedAt.Format("2006-01-02 15:04:05")
			}
			lastPulledAt := ""
			if img.LastRecordedPullTime != nil {
				lastPulledAt = img.LastRecordedPullTime.Format("2006-01-02 15:04:05")
			}
			sizeBytes := ""
			if img.ImageSizeInBytes != nil {
				sizeBytes = fmt.Sprintf("%d", *img.ImageSizeInBytes)
			}
			images = append(images, ECRImageInfo{
				RepositoryName: repoName,
				ImageTag:       tags,
				ImageDigest:    aws.ToString(img.ImageDigest),
				PushedAt:       pushedAt,
				LastPulledAt:   lastPulledAt,
				ImageSizeBytes: sizeBytes,
			})
		}

		if !all || o.NextToken == nil {
			break
		}
		nextToken = o.NextToken
	}

	sort.Slice(images, func(i, j int) bool {
		return images[i].PushedAt > images[j].PushedAt
	})

	return images, nil
}

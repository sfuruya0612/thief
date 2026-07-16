package aws

import (
	"context"
	"fmt"
	"sort"
	"strings"
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
	LastPulledAt   string `json:"last_pulled_at"`
	ImageSizeBytes int64  `json:"image_size_bytes"`
}

// ListECRResources returns all ECR repositories for the given profile/region.
func ListECRResources(ctx context.Context, profile, region string) ([]ECRRepoResource, error) {
	client, err := newECRClient(ctx, profile, region)
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
	client, err := newECRClient(ctx, profile, region)
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
	lastPulledAt := ""
	if img.LastRecordedPullTime != nil {
		lastPulledAt = img.LastRecordedPullTime.Format(time.RFC3339)
	}
	return ECRImageResource{
		RepositoryName: repoName,
		ImageTag:       tag,
		ImageDigest:    ptrStr(img.ImageDigest),
		PushedAt:       pushedAt,
		LastPulledAt:   lastPulledAt,
		ImageSizeBytes: ptrInt64(img.ImageSizeInBytes),
	}
}

// ecrImagesPageSizeAll / ecrImagesPageSizeTagged は DescribeImages の 1 ページあたり取得件数。
// 全件取得時は最大値、既定 (タグ付きのみ) は直近の把握に十分な件数に絞る。
const (
	ecrImagesPageSizeAll    = 1000
	ecrImagesPageSizeTagged = 30
)

// ECRRepoInfo はレガシー CLI 互換の ECR リポジトリ表示用フィールドを保持する。
type ECRRepoInfo struct {
	RepositoryName string
	RepositoryUri  string
	CreatedAt      string
}

// ToRow converts ECRRepoInfo to a string slice suitable for table formatting.
func (r ECRRepoInfo) ToRow() []string {
	return []string{r.RepositoryName, r.RepositoryUri, r.CreatedAt}
}

// ECRImageInfo はレガシー CLI 互換の ECR イメージ表示用フィールドを保持する。
type ECRImageInfo struct {
	RepositoryName string
	ImageTag       string
	ImageDigest    string
	PushedAt       string
	LastPulledAt   string
	ImageSizeBytes string
}

// ToRow converts ECRImageInfo to a string slice suitable for table formatting.
func (i ECRImageInfo) ToRow() []string {
	return []string{i.RepositoryName, i.ImageTag, i.ImageDigest, i.PushedAt, i.LastPulledAt, i.ImageSizeBytes}
}

// ListECRRepoInfos は全 ECR リポジトリをレガシー CLI 互換フィールドで返す。
func ListECRRepoInfos(ctx context.Context, profile, region string) ([]ECRRepoInfo, error) {
	client, err := newECRClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	var repos []ECRRepoInfo
	paginator := ecr.NewDescribeRepositoriesPaginator(client, &ecr.DescribeRepositoriesInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe ecr repositories: %w", err)
		}
		for _, repo := range page.Repositories {
			createdAt := ""
			if repo.CreatedAt != nil {
				createdAt = repo.CreatedAt.String()
			}
			repos = append(repos, ECRRepoInfo{
				RepositoryName: ptrStr(repo.RepositoryName),
				RepositoryUri:  ptrStr(repo.RepositoryUri),
				CreatedAt:      createdAt,
			})
		}
	}
	return repos, nil
}

// ListECRImageInfos は指定リポジトリのイメージ一覧を PushedAt 降順で返す。
// all が false のときはタグ付きイメージの先頭ページのみ、true のときは全ページを取得する。
func ListECRImageInfos(ctx context.Context, profile, region, repoName string, all bool) ([]ECRImageInfo, error) {
	client, err := newECRClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	var images []ECRImageInfo
	var nextToken *string
	for {
		input := &ecr.DescribeImagesInput{
			RepositoryName: aws.String(repoName),
			NextToken:      nextToken,
		}
		if all {
			input.MaxResults = aws.Int32(ecrImagesPageSizeAll)
		} else {
			input.MaxResults = aws.Int32(ecrImagesPageSizeTagged)
			input.Filter = &ecrtypes.DescribeImagesFilter{TagStatus: ecrtypes.TagStatusTagged}
		}

		o, err := client.DescribeImages(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("describe images for %s: %w", repoName, err)
		}

		for _, img := range o.ImageDetails {
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
				ImageTag:       strings.Join(img.ImageTags, ","),
				ImageDigest:    ptrStr(img.ImageDigest),
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

// newECRClient は ECR API クライアントを生成する。
func newECRClient(ctx context.Context, profile, region string) (*ecr.Client, error) {
	return NewClient(ctx, profile, region, func(cfg aws.Config) *ecr.Client {
		return ecr.NewFromConfig(cfg)
	})
}

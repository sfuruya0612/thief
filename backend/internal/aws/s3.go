package aws

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"golang.org/x/sync/errgroup"
)

// S3Resource represents a single S3 bucket.
type S3Resource struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	State       string  `json:"state"`
	Region      string  `json:"region"`
	CreatedAt   string  `json:"created_at"`
	Public      bool    `json:"public"`
	Encryption  string  `json:"encryption"`
	CostMonthly float64 `json:"cost_monthly"`
}

func (r S3Resource) ResourceID() string    { return r.ID }
func (r S3Resource) ResourceName() string  { return r.Name }
func (r S3Resource) ResourceState() string { return "active" }
func (r S3Resource) ServiceName() string   { return "s3" }

// s3BucketConcurrency はバケットごとの属性解決 (GetBucketLocation / GetBucketEncryption /
// GetPublicAccessBlock) を同時実行する上限数。無制限にすると S3 のリクエストレート上限に
// 抵触しうるため上限を設ける (issue 0043 の Cloud Run ListJobs 並列化と同型)。
const s3BucketConcurrency = 30

// ListS3Resources returns all S3 buckets accessible via the given profile.
// ListBuckets is called against us-east-1; per-bucket region is resolved with GetBucketLocation.
func ListS3Resources(ctx context.Context, profile, _ string) ([]S3Resource, error) {
	// S3 ListBuckets is a global operation; us-east-1 is the canonical endpoint.
	client, err := newS3Client(ctx, profile, "us-east-1")
	if err != nil {
		return nil, err
	}

	out, err := client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, fmt.Errorf("list s3 buckets: %w", err)
	}

	// バケットごとの属性解決 (region / encryption / public) は 1 バケットあたり 3 本の
	// 直列 API 呼び出しを伴い、バケット間では互いに独立している。バケット間で並列実行し、
	// 各 goroutine は自分の index にのみ書き込むため結果スライスへの書き込みはロック不要で
	// 競合しない (データオーナーシップを goroutine ごとに分離)。
	resources := make([]S3Resource, len(out.Buckets))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(s3BucketConcurrency)
	for i, b := range out.Buckets {
		g.Go(func() error {
			resources[i] = s3FromBucket(gctx, client, b)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return resources, nil
}

func s3FromBucket(ctx context.Context, client *s3.Client, b s3types.Bucket) S3Resource {
	name := ptrStr(b.Name)
	createdAt := ""
	if b.CreationDate != nil {
		createdAt = b.CreationDate.Format(time.RFC3339)
	}

	region := resolveS3Region(ctx, client, name)
	encryption := resolveS3Encryption(ctx, client, name)
	public := resolveS3Public(ctx, client, name)

	return S3Resource{
		ID:         name,
		Name:       name,
		CreatedAt:  createdAt,
		Region:     region,
		Public:     public,
		Encryption: encryption,
	}
}

func resolveS3Region(ctx context.Context, client *s3.Client, bucket string) string {
	out, err := client.GetBucketLocation(ctx, &s3.GetBucketLocationInput{Bucket: aws.String(bucket)})
	if err != nil {
		return "unknown"
	}
	if out.LocationConstraint == "" {
		return "us-east-1"
	}
	return string(out.LocationConstraint)
}

func resolveS3Encryption(ctx context.Context, client *s3.Client, bucket string) string {
	out, err := client.GetBucketEncryption(ctx, &s3.GetBucketEncryptionInput{Bucket: aws.String(bucket)})
	if err != nil {
		return "none"
	}
	if out.ServerSideEncryptionConfiguration != nil && len(out.ServerSideEncryptionConfiguration.Rules) > 0 {
		rule := out.ServerSideEncryptionConfiguration.Rules[0]
		if rule.ApplyServerSideEncryptionByDefault != nil {
			return string(rule.ApplyServerSideEncryptionByDefault.SSEAlgorithm)
		}
	}
	return "none"
}

func resolveS3Public(ctx context.Context, client *s3.Client, bucket string) bool {
	out, err := client.GetPublicAccessBlock(ctx, &s3.GetPublicAccessBlockInput{Bucket: aws.String(bucket)})
	if err != nil {
		return false
	}
	if out.PublicAccessBlockConfiguration == nil {
		return true
	}
	cfg := out.PublicAccessBlockConfiguration
	// If all block settings are enabled, the bucket is not public.
	return !(ptrBool(cfg.BlockPublicAcls) && ptrBool(cfg.BlockPublicPolicy) &&
		ptrBool(cfg.IgnorePublicAcls) && ptrBool(cfg.RestrictPublicBuckets))
}

// S3BucketInfo はレガシー CLI 互換の S3 バケット表示用フィールドを保持する。
type S3BucketInfo struct {
	Name         string
	CreationDate string
}

// ToRow converts S3BucketInfo to a string slice suitable for table formatting.
func (b S3BucketInfo) ToRow() []string {
	return []string{b.Name, b.CreationDate}
}

// ListS3BucketInfos は S3 バケット一覧を返す。
// ListS3Resources と異なりバケットごとのリージョン・暗号化・公開設定の解決を行わない (軽量)。
func ListS3BucketInfos(ctx context.Context, profile string) ([]S3BucketInfo, error) {
	client, err := newS3Client(ctx, profile, "us-east-1")
	if err != nil {
		return nil, err
	}

	out, err := client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, fmt.Errorf("list buckets: %w", err)
	}

	var buckets []S3BucketInfo
	for _, b := range out.Buckets {
		creationDate := ""
		if b.CreationDate != nil {
			creationDate = b.CreationDate.Format("2006-01-02 15:04:05")
		}
		buckets = append(buckets, S3BucketInfo{
			Name:         ptrStr(b.Name),
			CreationDate: creationDate,
		})
	}
	return buckets, nil
}

// newS3Client は S3 API クライアントを生成する。
//
// path-style アクセス (floci 等の S3 互換エミュレータ向け opt-in) は THIEF_S3_PATH_STYLE
// 環境変数を internal/aws パッケージ内で直接参照して解決する。internal/config.Config
// 経由で bool を渡す設計も検討したが、S3 クライアント生成は newS3Client の 1 箇所に
// 集約されている一方、config.Config はこの関数の呼び出し元 (handler 層) までしか
// 到達しておらず、bool を橋渡しするには NewClient や呼び出し元シグネチャすべてに
// 変更が波及する。影響範囲を S3 のみに閉じるため、この関数内で環境変数を直接読む
// 局所的な opt-in とした (AGENTS.md: 抽象化は実需が出てから入れる)。
func newS3Client(ctx context.Context, profile, region string) (*s3.Client, error) {
	pathStyle := s3PathStyleEnabled()
	return NewClient(ctx, profile, region, func(cfg aws.Config) *s3.Client {
		return s3.NewFromConfig(cfg, s3PathStyleOption(pathStyle))
	})
}

// s3PathStyleEnabled は THIEF_S3_PATH_STYLE 環境変数を bool として解釈する。
// 未設定または不正な値の場合は false (virtual-hosted style) を返す。
func s3PathStyleEnabled() bool {
	v, err := strconv.ParseBool(os.Getenv("THIEF_S3_PATH_STYLE"))
	if err != nil {
		return false
	}
	return v
}

// s3PathStyleOption は s3.Options.UsePathStyle を pathStyle に設定する s3.NewFromConfig 用の
// オプション関数を返す。
func s3PathStyleOption(pathStyle bool) func(*s3.Options) {
	return func(o *s3.Options) {
		o.UsePathStyle = pathStyle
	}
}

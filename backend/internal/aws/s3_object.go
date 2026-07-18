package aws

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3ObjectResource は S3 バケット内の 1 オブジェクトを表す。
type S3ObjectResource struct {
	Key          string `json:"key"`
	Size         int64  `json:"size"`
	LastModified string `json:"last_modified"`
	StorageClass string `json:"storage_class"`
	ETag         string `json:"etag"`
}

func (r S3ObjectResource) ResourceID() string    { return r.Key }
func (r S3ObjectResource) ResourceName() string  { return r.Key }
func (r S3ObjectResource) ResourceState() string { return "" }
func (r S3ObjectResource) ServiceName() string   { return "s3-objects" }

// maxS3ListObjects は ListS3Objects が返すオブジェクト件数の上限。オブジェクトが膨大な
// バケットで取得に時間がかかりレスポンスが肥大化するのを避けるため、蓄積件数がこれに
// 達した時点でページネーションを打ち切る。ページサイズ (ListObjectsV2 の既定 1000 件/ページ)
// に依存せず、オブジェクト単位でカウントする。
const maxS3ListObjects = 1000

// ListS3Objects は指定バケット (と prefix) のオブジェクト一覧を返す。
// バケットのリージョンを GetBucketLocation で解決してから ListObjectsV2 を呼ぶ。
// 蓄積件数が maxS3ListObjects に達した時点で打ち切り、truncated に true を返す。
func ListS3Objects(ctx context.Context, profile, region, bucket, prefix string) (resources []S3ObjectResource, truncated bool, err error) {
	client, err := newS3ClientForBucket(ctx, profile, region, bucket)
	if err != nil {
		return nil, false, err
	}

	input := &s3.ListObjectsV2Input{Bucket: aws.String(bucket)}
	if prefix != "" {
		input.Prefix = aws.String(prefix)
	}

	paginator := s3.NewListObjectsV2Paginator(client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, false, fmt.Errorf("list s3 objects in %s: %w", bucket, err)
		}
		var pageTruncated bool
		resources, pageTruncated = appendS3ObjectsUpToLimit(resources, page.Contents, maxS3ListObjects)
		if pageTruncated {
			return resources, true, nil
		}
	}
	return resources, false, nil
}

// appendS3ObjectsUpToLimit は objs を resources に変換しながら追加し、蓄積件数が max に
// 達した時点で追加を止める。ページの途中でも打ち切るため、ListObjectsV2 の既定ページサイズに
// 依存せずオブジェクト単位で上限を判定できる。
func appendS3ObjectsUpToLimit(
	resources []S3ObjectResource,
	objs []s3types.Object,
	max int,
) (updated []S3ObjectResource, truncated bool) {
	for _, obj := range objs {
		if len(resources) >= max {
			return resources, true
		}
		resources = append(resources, s3ObjectFromSDK(obj))
	}
	return resources, false
}

// GetS3Object は指定オブジェクトを取得する。Body はストリーミングで返す。
// 呼び出し側で out.Body.Close() を必ず行うこと。
func GetS3Object(ctx context.Context, profile, region, bucket, key string) (*s3.GetObjectOutput, error) {
	client, err := newS3ClientForBucket(ctx, profile, region, bucket)
	if err != nil {
		return nil, err
	}
	out, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("get s3 object %s/%s: %w", bucket, key, err)
	}
	return out, nil
}

// PutS3Object は body を PutObject の Body にそのまま渡してオブジェクトを書き込む。
// S3 の PutObject は Content-Length が必須のため、呼び出し側は body の
// 全長が確定した contentLength を渡すこと。
func PutS3Object(ctx context.Context, profile, region, bucket, key string, body io.Reader, contentLength int64, contentType string) error {
	client, err := newS3ClientForBucket(ctx, profile, region, bucket)
	if err != nil {
		return err
	}
	input := &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentLength: aws.Int64(contentLength),
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}
	if _, err := client.PutObject(ctx, input); err != nil {
		return fmt.Errorf("put s3 object %s/%s: %w", bucket, key, err)
	}
	return nil
}

// newS3ClientForBucket はバケットの実リージョンを解決してその上に S3 クライアントを作る。
// S3 は署名 (SigV4) のためリージョン一致が必要で、us-east-1 固定では別リージョンのバケットに
// 対する GetObject が 301 でリダイレクトする。
func newS3ClientForBucket(ctx context.Context, profile, region, bucket string) (*s3.Client, error) {
	base, err := newS3Client(ctx, profile, "us-east-1")
	if err != nil {
		return nil, err
	}
	resolved := resolveS3Region(ctx, base, bucket)
	if resolved == "unknown" || resolved == "" {
		if region != "" {
			resolved = region
		} else {
			resolved = "us-east-1"
		}
	}
	client, err := newS3Client(ctx, profile, resolved)
	if err != nil {
		return nil, err
	}
	return client, nil
}

// s3ObjectFromSDK は SDK の Object を UI 用リソースに変換する。
func s3ObjectFromSDK(o s3types.Object) S3ObjectResource {
	lastMod := ""
	if o.LastModified != nil {
		lastMod = o.LastModified.UTC().Format(time.RFC3339)
	}
	var size int64
	if o.Size != nil {
		size = *o.Size
	}
	return S3ObjectResource{
		Key:          ptrStr(o.Key),
		Size:         size,
		LastModified: lastMod,
		StorageClass: string(o.StorageClass),
		ETag:         ptrStr(o.ETag),
	}
}

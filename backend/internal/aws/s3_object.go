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

// maxS3ListObjects は ListS3Objects が蓄積するオブジェクトとフォルダ (CommonPrefixes) を
// 合わせた件数の上限。オブジェクトが膨大なバケットで取得に時間がかかりレスポンスが肥大化
// するのを避けるため、蓄積件数がこれに達した後にまだエントリが残るときだけページネーションを
// 打ち切る (ちょうど上限で終わるときは打ち切らない)。ページサイズ (ListObjectsV2 の既定
// 1000 件/ページ) に依存せず、エントリ単位でカウントする。
const maxS3ListObjects = 1000

// ListS3Objects は指定バケット (と prefix) のオブジェクト一覧を返す。
// バケットのリージョンを GetBucketLocation で解決してから ListObjectsV2 を呼ぶ。
//
// delimiter が "/" の階層モードでは、prefix から次の区切り文字までを CommonPrefixes に
// 畳み込ませ、その完全な prefix (末尾 "/" 付き) を prefixes として返す。畳み込まれたキーは
// objects には含まれない。空のフラットモードでは従来どおり平らな一覧を返す。
// オブジェクトとフォルダを合わせた蓄積件数が maxS3ListObjects に達した後にまだ追加する
// エントリがあるときだけ打ち切り、truncated に true を返す。
func ListS3Objects(ctx context.Context, profile, region, bucket, prefix, delimiter string) (objects []S3ObjectResource, prefixes []string, truncated bool, err error) {
	client, err := newS3ClientForBucket(ctx, profile, region, bucket)
	if err != nil {
		return nil, nil, false, err
	}

	input := &s3.ListObjectsV2Input{Bucket: aws.String(bucket)}
	if prefix != "" {
		input.Prefix = aws.String(prefix)
	}
	if delimiter != "" {
		input.Delimiter = aws.String(delimiter)
	}

	paginator := s3.NewListObjectsV2Paginator(client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, nil, false, fmt.Errorf("list s3 objects in %s: %w", bucket, err)
		}
		var pageTruncated bool
		objects, prefixes, pageTruncated = appendS3ListEntriesUpToLimit(
			objects, prefixes, page.Contents, page.CommonPrefixes, maxS3ListObjects,
		)
		if pageTruncated {
			return objects, prefixes, true, nil
		}
	}
	return objects, prefixes, false, nil
}

// appendS3ListEntriesUpToLimit はオブジェクトとフォルダ (CommonPrefixes) をまとめて
// 蓄積し、オブジェクトとフォルダを合わせた蓄積件数が max に達した後にまだ追加する
// エントリがあるときだけ truncated を true にして追加を止める。上限ちょうどで終わるときは
// false のままで、S3 の従来の境界 (appendS3ObjectsUpToLimit) と同じである。
//
// 1 ページの応答では Contents と CommonPrefixes が別の配列で返り、両者を混ぜたキー順は
// 復元できないため、フォルダ (辿るための入口) を先に数えて残す。
func appendS3ListEntriesUpToLimit(
	objects []S3ObjectResource,
	prefixes []string,
	objs []s3types.Object,
	commonPrefixes []s3types.CommonPrefix,
	max int,
) (outObjects []S3ObjectResource, outPrefixes []string, truncated bool) {
	outObjects, outPrefixes = objects, prefixes
	for _, cp := range commonPrefixes {
		if len(outObjects)+len(outPrefixes) >= max {
			return outObjects, outPrefixes, true
		}
		outPrefixes = append(outPrefixes, ptrStr(cp.Prefix))
	}
	for _, obj := range objs {
		if len(outObjects)+len(outPrefixes) >= max {
			return outObjects, outPrefixes, true
		}
		outObjects = append(outObjects, s3ObjectFromSDK(obj))
	}
	return outObjects, outPrefixes, false
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

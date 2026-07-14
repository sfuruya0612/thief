package gcp

import (
	"context"
	"fmt"
	"io"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

// BucketInfo は Cloud Storage バケットの表示用メタデータ。
type BucketInfo struct {
	Name         string `json:"name"`
	Location     string `json:"location"`
	StorageClass string `json:"storage_class"`
	CreateTime   string `json:"create_time"`
	UpdateTime   string `json:"update_time"`
}

// ObjectInfo は Cloud Storage オブジェクトの表示用メタデータ。
type ObjectInfo struct {
	Name         string `json:"name"`
	Bucket       string `json:"bucket"`
	Size         int64  `json:"size"`
	ContentType  string `json:"content_type"`
	StorageClass string `json:"storage_class"`
	Updated      string `json:"updated"`
}

// ListBuckets は指定プロジェクトの Cloud Storage バケット一覧を返す。
func ListBuckets(ctx context.Context, projectID string) ([]BucketInfo, error) {
	// storage.NewClient は内部で htransport.NewClient が生成した *http.Client を
	// option.WithHTTPClient として自身の opts に追加してから raw.NewService を呼ぶため、
	// 呼び出し側が option.WithQuotaProject を渡すと "WithHTTPClient is incompatible with
	// QuotaProject" で失敗する (cloud.google.com/go/storage v1.57 以降で入った制約)。
	// ListBuckets の課金/権限判定は API 呼び出し自体に渡す projectID で行われるため、
	// クライアント生成時に quota project を指定する必要はない。
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create storage client: %w", err)
	}
	defer client.Close()

	var buckets []BucketInfo
	it := client.Buckets(ctx, projectID)
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("iterate buckets: %w", err)
		}
		buckets = append(buckets, bucketFromAttrs(attrs))
	}
	return buckets, nil
}

// ListObjects は指定バケット内のオブジェクトを prefix 絞り込みで列挙する。
func ListObjects(ctx context.Context, projectID, bucket, prefix string) ([]ObjectInfo, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create storage client: %w", err)
	}
	defer client.Close()

	var objects []ObjectInfo
	it := client.Bucket(bucket).UserProject(projectID).Objects(ctx, &storage.Query{Prefix: prefix})
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("iterate objects in %s: %w", bucket, err)
		}
		objects = append(objects, objectFromAttrs(attrs))
	}
	return objects, nil
}

// ObjectReader は GCS オブジェクトのダウンロード用リーダーとメタデータを保持する。
// 呼び出し側は読み終えたら Close すること (内部の storage.Client も併せて解放する)。
type ObjectReader struct {
	io.Reader
	ContentType string
	Size        int64
	client      *storage.Client
	reader      *storage.Reader
}

// Close はオブジェクト読み取り用の Reader と、その生成元の Client を両方解放する。
func (r *ObjectReader) Close() error {
	err := r.reader.Close()
	if cerr := r.client.Close(); cerr != nil && err == nil {
		err = cerr
	}
	return err
}

// GetObject は指定バケット・オブジェクトのダウンロード用ストリームを開く。
func GetObject(ctx context.Context, projectID, bucket, key string) (*ObjectReader, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create storage client: %w", err)
	}

	reader, err := client.Bucket(bucket).UserProject(projectID).Object(key).NewReader(ctx)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("get object %s/%s: %w", bucket, key, err)
	}

	return &ObjectReader{
		Reader:      reader,
		ContentType: reader.Attrs.ContentType,
		Size:        reader.Attrs.Size,
		client:      client,
		reader:      reader,
	}, nil
}

// PutObject は body を Cloud Storage オブジェクトとして書き込む。
func PutObject(ctx context.Context, projectID, bucket, key string, body io.Reader, contentType string) error {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("create storage client: %w", err)
	}
	defer client.Close()

	writer := client.Bucket(bucket).UserProject(projectID).Object(key).NewWriter(ctx)
	if contentType != "" {
		writer.ContentType = contentType
	}
	if _, err := io.Copy(writer, body); err != nil {
		writer.Close()
		return fmt.Errorf("put object %s/%s: %w", bucket, key, err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("put object %s/%s: %w", bucket, key, err)
	}
	return nil
}

func bucketFromAttrs(attrs *storage.BucketAttrs) BucketInfo {
	if attrs == nil {
		return BucketInfo{}
	}
	return BucketInfo{
		Name:         attrs.Name,
		Location:     attrs.Location,
		StorageClass: attrs.StorageClass,
		CreateTime:   formatTimestamp(attrs.Created, !attrs.Created.IsZero()),
		UpdateTime:   formatTimestamp(attrs.Updated, !attrs.Updated.IsZero()),
	}
}

func objectFromAttrs(attrs *storage.ObjectAttrs) ObjectInfo {
	if attrs == nil {
		return ObjectInfo{}
	}
	return ObjectInfo{
		Name:         attrs.Name,
		Bucket:       attrs.Bucket,
		Size:         attrs.Size,
		ContentType:  attrs.ContentType,
		StorageClass: attrs.StorageClass,
		Updated:      formatTimestamp(attrs.Updated, !attrs.Updated.IsZero()),
	}
}

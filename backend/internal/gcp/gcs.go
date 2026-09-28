package gcp

import (
	"context"
	"errors"
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

// maxGCSListObjects は ListObjects が蓄積するオブジェクトとフォルダを合わせた件数の上限。
// S3 の maxS3ListObjects と同じ考え方で、蓄積件数がこれに達した後にまだエントリが返る
// ときだけ列挙を打ち切る (ちょうど上限で終わるときは打ち切らない)。
const maxGCSListObjects = 1000

// ListObjects は指定バケット内のオブジェクトを prefix 絞り込みで列挙する。
// delimiter が "/" の階層モードでは、次の区切り文字までを畳み込んだ合成ディレクトリ
// エントリ (ObjectAttrs.Prefix のみを持つ) を prefixes として返し、objects には含めない。
// 空のフラットモードでは従来どおり平らな一覧を返す。
// オブジェクトとフォルダを合わせた蓄積件数が maxGCSListObjects に達した後にまだ追加する
// エントリがあるときだけ打ち切り、truncated に true を返す。
func ListObjects(ctx context.Context, projectID, bucket, prefix, delimiter string) (objects []ObjectInfo, prefixes []string, truncated bool, err error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, nil, false, fmt.Errorf("create storage client: %w", err)
	}
	defer client.Close()

	query := &storage.Query{Prefix: prefix}
	if delimiter != "" {
		query.Delimiter = delimiter
	}
	it := client.Bucket(bucket).UserProject(projectID).Objects(ctx, query)
	objects, prefixes, truncated, err = collectGCSObjects(it.Next, maxGCSListObjects)
	if err != nil {
		return nil, nil, false, fmt.Errorf("iterate objects in %s: %w", bucket, err)
	}
	return objects, prefixes, truncated, nil
}

// errNilObjectAttrs は iterator.Next が Done でもエラーでもないのに nil を返したときのエラー。
var errNilObjectAttrs = errors.New("object iterator returned nil attrs")

// collectGCSObjects は it.Next に相当する next から 1 件ずつ取り出し、オブジェクトと
// フォルダ (ObjectAttrs.Prefix が空でない合成ディレクトリエントリ) を蓄積する。
// オブジェクトとフォルダを合わせた蓄積件数が max に達した後にまだエントリが返るときだけ
// truncated を true にする。ちょうど max 件で終わるときは false である
// (S3 の appendS3ListEntriesUpToLimit と同じ境界)。
//
// storage.Client を必要としない純関数に切り出すことで、境界とフォルダの振り分けを
// storage.Client 無しでテストできるようにしている。
func collectGCSObjects(
	next func() (*storage.ObjectAttrs, error),
	max int,
) (objects []ObjectInfo, prefixes []string, truncated bool, err error) {
	for {
		attrs, err := next()
		if errors.Is(err, iterator.Done) {
			return objects, prefixes, false, nil
		}
		if err != nil {
			return nil, nil, false, err
		}
		if attrs == nil {
			// iterator.Next は Done でもエラーでもないときに nil を返さない。返ったときは
			// 契約違反としてエラーにする (読み飛ばすと上限の数え上げが進まず、nil が
			// 返り続けたときに終了しない)。上限の判定より先に見て、上限の位置で返った
			// nil を打ち切りとして扱わない。
			return nil, nil, false, errNilObjectAttrs
		}
		if len(objects)+len(prefixes) >= max {
			return objects, prefixes, true, nil
		}
		if attrs.Prefix != "" {
			prefixes = append(prefixes, attrs.Prefix)
			continue
		}
		objects = append(objects, objectFromAttrs(attrs))
	}
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

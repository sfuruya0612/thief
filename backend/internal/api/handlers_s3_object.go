package api

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
)

// handleS3Objects は指定バケット (と prefix) のオブジェクト一覧を返す。
// キャッシュキーには prefix も含める (prefix ごとに独立キャッシュ)。
func (s *Server) handleS3Objects(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	bucket := r.PathValue("bucket")
	if bucket == "" {
		writeBadRequest(w, "bucket is required")
		return
	}
	prefix := r.URL.Query().Get("prefix")

	s.serveCached(w, r, cacheKey("s3-objects", profile, region, bucket, prefix), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListS3Objects(r.Context(), profile, region, bucket, prefix)
	})
}

// handleS3ObjectDownload は S3 オブジェクトをストリーミングでダウンロードする。
// レスポンスボディは []byte 化せず io.Copy で直接ライトする。
func (s *Server) handleS3ObjectDownload(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	bucket := r.PathValue("bucket")
	if bucket == "" {
		writeBadRequest(w, "bucket is required")
		return
	}
	objectKey := r.URL.Query().Get("key")
	if objectKey == "" {
		writeBadRequest(w, "key is required")
		return
	}

	out, err := awsinternal.GetS3Object(r.Context(), profile, region, bucket, objectKey)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	defer out.Body.Close()

	filename := sanitizeContentDispositionFilename(objectKey)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	if out.ContentType != nil && *out.ContentType != "" {
		w.Header().Set("Content-Type", *out.ContentType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	if out.ContentLength != nil && *out.ContentLength > 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", *out.ContentLength))
	}
	if _, err := io.Copy(w, out.Body); err != nil {
		// ヘッダは送信済みなのでエラー応答は書けない。ログのみ。
		slog.Warn("s3 download stream copy failed", "bucket", bucket, "key", objectKey, "err", err.Error())
	}
}

// maxS3UploadSize は handleS3ObjectUpload が受け付けるアップロードの上限サイズ。
// S3 PutObject は Content-Length が必須で、multipart.Part は io.Seeker を実装しないため
// アップロード前にメモリへ読み込んでサイズを確定する。無制限に読み込むとメモリを圧迫するため
// 上限を設ける。
const maxS3UploadSize = 100 << 20 // 100MiB

// errS3UploadTooLarge は読み込んだ body が maxS3UploadSize を超えたことを示す。
var errS3UploadTooLarge = fmt.Errorf("file exceeds max upload size of %d bytes", maxS3UploadSize)

// readS3UploadBody は r から最大 maxS3UploadSize+1 バイトを読み込み、上限超過を検出する。
// 上限を超えた場合は errS3UploadTooLarge を返す。
func readS3UploadBody(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxS3UploadSize+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxS3UploadSize {
		return nil, errS3UploadTooLarge
	}
	return body, nil
}

// handleS3ObjectUpload は multipart/form-data の file パートを読み込んで PutObject する。
func (s *Server) handleS3ObjectUpload(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	bucket := r.PathValue("bucket")
	if bucket == "" {
		writeBadRequest(w, "bucket is required")
		return
	}
	objectKey := r.URL.Query().Get("key")
	if objectKey == "" {
		writeBadRequest(w, "key is required")
		return
	}

	reader, err := r.MultipartReader()
	if err != nil {
		writeBadRequest(w, "invalid multipart body: "+err.Error())
		return
	}

	var uploaded bool
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			writeBadRequest(w, "read multipart part: "+err.Error())
			return
		}
		if part.FormName() != "file" {
			part.Close()
			continue
		}
		contentType := part.Header.Get("Content-Type")
		body, err := readS3UploadBody(part)
		part.Close()
		if err != nil {
			writeBadRequest(w, "read file part: "+err.Error())
			return
		}
		if err := awsinternal.PutS3Object(r.Context(), profile, region, bucket, objectKey, bytes.NewReader(body), int64(len(body)), contentType); err != nil {
			writeAWSError(w, err)
			return
		}
		uploaded = true
		break
	}

	if !uploaded {
		writeBadRequest(w, `multipart form must contain a "file" part`)
		return
	}

	// アップロード成功後、対象バケット配下のオブジェクト一覧キャッシュを無効化する。
	// prefix ごとにキーが分かれるため、バケット単位のキー前方一致で一括無効化する。
	s.resourceCache.InvalidatePrefix(cacheKey("s3-objects", profile, region, bucket, ""))
	writeJSON(w, map[string]string{"status": "ok", "key": objectKey})
}

// sanitizeContentDispositionFilename は Content-Disposition の filename に埋め込む前に
// 改行・二重引用符・バックスラッシュを除去し、ヘッダインジェクション/破壊を防ぐ。
func sanitizeContentDispositionFilename(key string) string {
	// key はパス区切りを含む可能性があるので最終セグメントのみを使う。
	if idx := strings.LastIndex(key, "/"); idx >= 0 {
		key = key[idx+1:]
	}
	replacer := strings.NewReplacer(
		"\r", "",
		"\n", "",
		"\"", "",
		"\\", "",
	)
	sanitized := replacer.Replace(key)
	if sanitized == "" {
		return "download"
	}
	return sanitized
}

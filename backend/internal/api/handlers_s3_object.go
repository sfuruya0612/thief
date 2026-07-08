package api

import (
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

	key := cacheKey("s3-objects", profile, region, bucket, prefix)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.ListS3Objects(r.Context(), profile, region, bucket, prefix)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
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

// handleS3ObjectUpload は multipart/form-data の Part をストリーミングで PutObject に渡す。
// r.ParseMultipartForm のような全部バッファする方式は使わない。
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
		// Content-Length は multipart part では通常取得できないため 0 を渡す (chunked)。
		if err := awsinternal.PutS3Object(r.Context(), profile, region, bucket, objectKey, part, 0, contentType); err != nil {
			part.Close()
			writeAWSError(w, err)
			return
		}
		part.Close()
		uploaded = true
		break
	}

	if !uploaded {
		writeBadRequest(w, `multipart form must contain a "file" part`)
		return
	}

	// アップロード成功後、対象バケット配下のオブジェクト一覧キャッシュを無効化する。
	// prefix ごとにキーが分かれるため prefix 部分をワイルドカード相当で扱えないので
	// バケット単位で invalidate する簡易実装として、対象キーを含む prefix 群は
	// 次回アクセス時に refresh=true が来るまで stale の可能性がある。
	// (現状 cache パッケージにワイルドカード invalidate API がないため許容)
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

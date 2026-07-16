package api

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/gcp"
)

// gcpProjectIDFromQuery はクエリパラメータ ?project_id= を優先し、無ければ
// config の BigQuery.ProjectID (GOOGLE_CLOUD_PROJECT) にフォールバックする。
// どちらも空の場合は 503 GCP_NOT_CONFIGURED を書き込み false を返す。
func (s *Server) gcpProjectIDFromQuery(w http.ResponseWriter, r *http.Request) (string, bool) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		projectID = s.cfg.BigQuery.ProjectID
	}
	if projectID == "" {
		writeGCPNotConfigured(w)
		return "", false
	}
	return projectID, true
}

// handleGCPProjects は GCP プロジェクト一覧を返す。project_id 指定は不要 (全プロジェクト列挙のため)。
//
// プロジェクトの作成/削除は頻繁ではないため、定期的な自動更新は行わない。ローカルディスク
// (~/.config/thief/gcp-projects.json) に保存された一覧をそのまま返し、Cloud Resource Manager
// への API 呼び出しは「ディスクにキャッシュが存在しない初回起動時」または「?refresh=true が
// 明示された手動更新時」のみ行う。
func (s *Server) handleGCPProjects(w http.ResponseWriter, r *http.Request) {
	dir, err := config.Dir()
	if err != nil {
		writeInternalError(w, err.Error())
		return
	}

	refresh := s.refresh(r)
	s.serveCached(w, r, cacheKey("gcp-projects"), regionsCacheTTL, writeInternalFromError, func() (any, error) {
		if !refresh {
			if projects, _, ok, err := gcp.LoadProjectsFromDisk(dir); err != nil {
				return nil, err
			} else if ok {
				return projects, nil
			}
		}
		return gcp.RefreshProjectsOnDisk(r.Context(), dir)
	})
}

// handleGCPCloudRun は指定プロジェクトの Cloud Run サービス / ジョブを返す。
func (s *Server) handleGCPCloudRun(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.gcpProjectIDFromQuery(w, r)
	if !ok {
		return
	}
	s.serveCached(w, r, cacheKey("gcp-cloudrun", projectID), cacheTTL, writeInternalFromError, func() (any, error) {
		return gcp.ListCloudRun(r.Context(), projectID)
	})
}

// handleGCPGCS は指定プロジェクトの Cloud Storage バケット一覧を返す。
func (s *Server) handleGCPGCS(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.gcpProjectIDFromQuery(w, r)
	if !ok {
		return
	}
	s.serveCached(w, r, cacheKey("gcp-gcs", projectID), cacheTTL, writeInternalFromError, func() (any, error) {
		return gcp.ListBuckets(r.Context(), projectID)
	})
}

// handleGCPGCSObjects は指定バケット配下のオブジェクトを prefix 絞り込みで返す。
func (s *Server) handleGCPGCSObjects(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.gcpProjectIDFromQuery(w, r)
	if !ok {
		return
	}
	bucket := r.PathValue("bucket")
	prefix := r.URL.Query().Get("prefix")
	s.serveCached(w, r, cacheKey("gcp-gcs-objects", projectID, bucket, prefix), cacheTTL, writeInternalFromError, func() (any, error) {
		return gcp.ListObjects(r.Context(), projectID, bucket, prefix)
	})
}

// handleGCPGCSObjectDownload は GCS オブジェクトをストリーミングでダウンロードする。
// レスポンスボディは []byte 化せず io.Copy で直接ライトする。
func (s *Server) handleGCPGCSObjectDownload(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.gcpProjectIDFromQuery(w, r)
	if !ok {
		return
	}
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

	obj, err := gcp.GetObject(r.Context(), projectID, bucket, objectKey)
	if err != nil {
		writeInternalError(w, err.Error())
		return
	}
	defer obj.Close()

	filename := sanitizeContentDispositionFilename(objectKey)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	if obj.ContentType != "" {
		w.Header().Set("Content-Type", obj.ContentType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	if obj.Size > 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", obj.Size))
	}
	if _, err := io.Copy(w, obj); err != nil {
		// ヘッダは送信済みなのでエラー応答は書けない。ログのみ。
		slog.Warn("gcs download stream copy failed", "bucket", bucket, "key", objectKey, "err", err.Error())
	}
}

// handleGCPIAM は指定プロジェクトの IAM ポリシーをメンバー単位に展開して返す。
func (s *Server) handleGCPIAM(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.gcpProjectIDFromQuery(w, r)
	if !ok {
		return
	}
	s.serveCached(w, r, cacheKey("gcp-iam", projectID), cacheTTL, writeInternalFromError, func() (any, error) {
		return gcp.ListIAMBindings(r.Context(), projectID)
	})
}

// handleGCPServiceAccounts は指定プロジェクトの Service Account 一覧を返す。
func (s *Server) handleGCPServiceAccounts(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.gcpProjectIDFromQuery(w, r)
	if !ok {
		return
	}
	s.serveCached(w, r, cacheKey("gcp-serviceaccounts", projectID), cacheTTL, writeInternalFromError, func() (any, error) {
		return gcp.ListServiceAccounts(r.Context(), projectID)
	})
}

// handleGCPGCSObjectUpload は multipart/form-data の file パートを読み込んで GCS に書き込む。
// アップロード上限サイズは S3 と同じ maxS3UploadSize / readS3UploadBody を再利用する。
func (s *Server) handleGCPGCSObjectUpload(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.gcpProjectIDFromQuery(w, r)
	if !ok {
		return
	}
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
		if err := gcp.PutObject(r.Context(), projectID, bucket, objectKey, bytes.NewReader(body), contentType); err != nil {
			writeInternalError(w, err.Error())
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
	s.resourceCache.InvalidatePrefix(cacheKey("gcp-gcs-objects", projectID, bucket, ""))
	writeJSON(w, map[string]string{"status": "ok", "key": objectKey})
}

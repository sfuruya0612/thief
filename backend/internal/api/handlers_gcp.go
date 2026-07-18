package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/coder/websocket"
	"golang.org/x/sync/errgroup"

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

// handleGCPGCSObjectPreview は GCS オブジェクトの中身をプレビュー用 JSON エンベロープで返す。
// 既知のバイナリ拡張子を除く 5 MB 未満のオブジェクトを対象とし、中身がテキストでなければ
// buildPreviewResponse が弾く。
func (s *Server) handleGCPGCSObjectPreview(w http.ResponseWriter, r *http.Request) {
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
	if !previewExtensionAllowed(objectKey) {
		writePreviewUnsupportedType(w)
		return
	}

	obj, err := gcp.GetObject(r.Context(), projectID, bucket, objectKey)
	if err != nil {
		writeInternalError(w, err.Error())
		return
	}
	defer obj.Close()

	if !previewSizeAllowed(obj.Size) {
		writePreviewTooLarge(w)
		return
	}

	resp, err := buildPreviewResponse(obj, obj.ContentType)
	if err != nil {
		writePreviewError(w, err)
		return
	}
	writeJSON(w, resp)
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

// handleGCPLoggingEntries は指定プロジェクトのログエントリを期間・フィルター指定で 1 ページ返す。
// 実行のたびに結果が変わりうる読み取りのため、BigQuery のクエリ実行系 (handlers_bigquery.go:54)
// と同じ方針でキャッシュ (serveCached) を通さない。
func (s *Server) handleGCPLoggingEntries(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.gcpProjectIDFromQuery(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	pageSize, _ := strconv.Atoi(q.Get("page_size"))

	page, err := gcp.ListLogEntries(r.Context(), projectID, q.Get("filter"), q.Get("start"), q.Get("end"), q.Get("page_token"), pageSize)
	if err != nil {
		writeInternalError(w, err.Error())
		return
	}
	writeJSON(w, page)
}

// gcpLoggingTailReadLimit はブラウザ側 WebSocket からの 1 メッセージあたりの読み取り上限バイト数。
// ブラウザから backend へは切断検知のためのメッセージしか届かない想定のため小さくてよい。
const gcpLoggingTailReadLimit = 1 << 12 // 4KiB

// gcpLoggingTailControlMessage は Live Tail 終了時にブラウザへ送る制御メッセージ
// (session/bridge.go の exit 通知に倣った形)。
type gcpLoggingTailControlMessage struct {
	Type   string `json:"type"`
	Reason string `json:"reason,omitempty"`
}

// handleGCPLoggingTail は Cloud Logging の Live Tail を WebSocket 経由でブラウザへ中継する。
// フレーム規約: ログエントリは TEXT フレームの JSON (gcp.LogEntryInfo) で 1 件ずつ push する。
// 終了時は {"type":"end","reason":"..."} を送ってからクローズする。
func (s *Server) handleGCPLoggingTail(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.gcpProjectIDFromQuery(w, r)
	if !ok {
		return
	}
	filter := r.URL.Query().Get("filter")

	// OriginPatterns は cfg.WebOrigins に従う。DNS rebinding 対策のためここにのみ渡し、
	// InsecureSkipVerify は使わない (handlers_session.go:84 と同じ規約)。
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: s.cfg.WebOrigins,
	})
	if err != nil {
		slog.Warn("failed to accept gcp logging tail websocket", "err", err)
		return
	}
	conn.SetReadLimit(gcpLoggingTailReadLimit)

	// errgroup + 明示的な cancel で、tail ストリームの終了とブラウザ切断のどちらが先に
	// 起きても確実にもう片方を止める (session/bridge.go の Bridge.Run と同じ構図を
	// 片方向 push 用に簡略化したもの)。
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		defer cancel()
		return gcp.TailLogEntries(ctx, projectID, filter, func(entry gcp.LogEntryInfo) error {
			payload, err := json.Marshal(entry)
			if err != nil {
				return fmt.Errorf("marshal tail log entry: %w", err)
			}
			return conn.Write(ctx, websocket.MessageText, payload)
		})
	})
	g.Go(func() error {
		defer cancel()
		return discardGCPLoggingBrowserMessages(ctx, conn)
	})

	err = g.Wait()

	reason := "stream ended"
	switch {
	case err == nil:
		// discardGCPLoggingBrowserMessages がブラウザの正常切断で nil を返したケース。
	case errors.Is(err, context.Canceled):
		// もう片方の goroutine 終了に伴う cancel。ブラウザへは汎用メッセージのみ通知する。
	default:
		reason = err.Error()
		slog.Warn("gcp logging tail ended with error", "project_id", projectID, "err", err)
	}
	notifyGCPLoggingTailEnd(conn, reason)
	if err := conn.Close(websocket.StatusNormalClosure, ""); err != nil {
		slog.Warn("failed to close gcp logging tail websocket", "err", err)
	}
}

// discardGCPLoggingBrowserMessages はブラウザ→backend 方向を切断検知のためだけに読み捨てる。
// ブラウザの正常切断 (タブ/Drawer を閉じる等) は nil を返し、それ以外はエラーを返す。
func discardGCPLoggingBrowserMessages(ctx context.Context, conn *websocket.Conn) error {
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			switch websocket.CloseStatus(err) {
			case websocket.StatusNormalClosure, websocket.StatusGoingAway:
				return nil
			}
			return fmt.Errorf("read from browser: %w", err)
		}
	}
}

// notifyGCPLoggingTailEnd はセッション終了をブラウザへ通知する。呼び出し時点で r.Context() は
// すでにキャンセルされている可能性があるため、専用の短命 context を使う
// (session/bridge.go の cleanup と同じ規約)。送信エラーはログに残すのみで処理は継続する。
func notifyGCPLoggingTailEnd(conn *websocket.Conn, reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), sessionTerminateTimeout)
	defer cancel()

	payload, err := json.Marshal(gcpLoggingTailControlMessage{Type: "end", Reason: reason})
	if err != nil {
		slog.Warn("failed to marshal gcp logging tail end message", "err", err)
		return
	}
	if err := conn.Write(ctx, websocket.MessageText, payload); err != nil {
		slog.Warn("failed to notify browser of gcp logging tail end", "err", err)
	}
}

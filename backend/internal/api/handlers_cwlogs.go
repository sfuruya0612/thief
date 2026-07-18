package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
)

// handleCWLogGroups は指定 profile/region の CloudWatch Logs ロググループ一覧を返す。
// 変化が緩やかなためキャッシュを通す。
func (s *Server) handleCWLogGroups(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	s.serveCached(w, r, cacheKey("cwlogs-groups", profile, region), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListLogGroups(r.Context(), profile, region)
	})
}

// handleCWLogEvents は選択ロググループ群を横断してログイベントを検索し 1 ページ返す。
// クエリパラメータ: group (複数可、ロググループ ARN) / filter / start / end / page_token / limit。
// 実行のたびに結果が変わりうる読み取りのためキャッシュは通さない (GCP logging と同方針)。
func (s *Server) handleCWLogEvents(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))

	page, err := awsinternal.FilterLogEvents(
		r.Context(), profile, region,
		q["group"], q.Get("filter"), q.Get("start"), q.Get("end"), q.Get("page_token"), limit,
	)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeJSON(w, page)
}

// handleCWLogTail は CloudWatch Logs の Live Tail を WebSocket 経由でブラウザへ中継する。
// WebSocket の中継・終了処理は serveLogTail (logtail.go) に集約し、ここでは StartLiveTail
// からのイベント取得だけを担う。
func (s *Server) handleCWLogTail(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	q := r.URL.Query()
	groups := q["group"]
	filter := q.Get("filter")

	s.serveLogTail(w, r, func(ctx context.Context, send func(payload []byte) error) error {
		return awsinternal.StartLiveTail(ctx, profile, region, groups, filter, func(e awsinternal.LogEventInfo) error {
			payload, err := json.Marshal(e)
			if err != nil {
				return fmt.Errorf("marshal tail log event: %w", err)
			}
			return send(payload)
		})
	})
}

package api

import (
	"net/http"
	"strings"
)

// cacheInvalidateViews は POST /api/cache/invalidate が受け付ける view の値。
// frontend の AppView (aws / gcp / datadog / tidb) と対応する。
var cacheInvalidateViews = map[string]bool{
	"aws":     true,
	"gcp":     true,
	"datadog": true,
	"tidb":    true,
}

// cacheInvalidateExcluded は Refresh 起因の一括破棄から除外するキャッシュキーの
// 第 1 セグメント。cost / cost-forecast は Cost Explorer のリクエスト課金を避けるため、
// regions / gcp-projects は 24 時間 TTL で保持する明示的な設計のため、dynamo-items は
// Query / Scan の再実行による RCU 消費を避けるため (再検索はクエリパネルから明示的に行える)。
var cacheInvalidateExcluded = map[string]bool{
	"cost":          true,
	"cost-forecast": true,
	"regions":       true,
	"gcp-projects":  true,
	"dynamo-items":  true,
}

// viewOwnsCacheKey は view の Refresh がキャッシュキー key を破棄対象とするかを返す。
// キーの第 1 セグメント (先頭から最初の ":" まで) で判定する。除外集合は完全一致、
// provider プレフィックス (gcp- / bq- / dd- / tidb-) は前方一致、aws は「どのプレフィックス
// にも該当しない残り全部」とする。プレフィックスの列挙表ではなく振り分け規則にすることで、
// 新しい AWS サービスのキーは追記なしで自動的に aws の破棄対象になる。
// 新 provider を追加する場合はここに分岐を足す (足すまでそのキーは aws に分類される)。
func viewOwnsCacheKey(view, key string) bool {
	seg, _, _ := strings.Cut(key, ":")
	if cacheInvalidateExcluded[seg] {
		return false
	}
	isGCP := strings.HasPrefix(seg, "gcp-") || strings.HasPrefix(seg, "bq-")
	isDatadog := strings.HasPrefix(seg, "dd-")
	isTiDB := strings.HasPrefix(seg, "tidb-")
	switch view {
	case "gcp":
		return isGCP
	case "datadog":
		return isDatadog
	case "tidb":
		return isTiDB
	case "aws":
		return !isGCP && !isDatadog && !isTiDB
	default:
		return false
	}
}

// handleCacheInvalidate は view に対応するリソースキャッシュのエントリをまとめて破棄する。
// TopBar の Refresh から呼ばれ、破棄後の再取得が backend のキャッシュ HIT で
// 古いデータを返し続けるのを防ぐ。AWS API は呼ばないため SSO の状態には依存しない。
func (s *Server) handleCacheInvalidate(w http.ResponseWriter, r *http.Request) {
	view, ok := requireQueryParam(w, r, "view")
	if !ok {
		return
	}
	if !cacheInvalidateViews[view] {
		writeBadRequest(w, "view must be one of aws, gcp, datadog, tidb")
		return
	}
	s.resourceCache.InvalidateFunc(func(key string) bool {
		return viewOwnsCacheKey(view, key)
	})
	w.WriteHeader(http.StatusNoContent)
}

package api

import "net/http"

// ClientConfigResponse は GET /api/config のレスポンス。frontend が起動後に参照する
// 実行時設定を返す。ObjectQueryMaxBytes はオブジェクト SQL 検索が取り込める 1 オブジェクトの
// サイズ上限 (バイト) で、環境変数 THIEF_OBJECT_QUERY_MAX_BYTES で変更できる。
type ClientConfigResponse struct {
	ObjectQueryMaxBytes int64 `json:"object_query_max_bytes"`
}

// handleClientConfig は frontend 向けの実行時設定を返す。認証もクラウド呼び出しも伴わない。
func (s *Server) handleClientConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, ClientConfigResponse{ObjectQueryMaxBytes: s.cfg.ObjectQueryMaxBytes})
}

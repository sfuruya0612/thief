package api

import "net/http"

// handleHealth はサーバの生存確認用エンドポイント。認証やクラウド呼び出しを伴わず、
// frontend が backend の起動待ちを検知するためだけに使う。
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

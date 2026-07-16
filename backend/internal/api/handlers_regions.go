package api

import (
	"net/http"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
)

// handleRegions は有効化済み AWS リージョン一覧を返す。
// キャッシュは長期 (24 時間)、プロファイル単位でキーを分ける。
func (s *Server) handleRegions(w http.ResponseWriter, r *http.Request) {
	profile := r.PathValue("profile")
	s.serveCached(w, r, cacheKey("regions", profile), regionsCacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListRegions(r.Context(), profile)
	})
}

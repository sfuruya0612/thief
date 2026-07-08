package api

import (
	"net/http"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
)

// handleRegions は有効化済み AWS リージョン一覧を返す。
// キャッシュは長期 (24 時間)、プロファイル単位でキーを分ける。
func (s *Server) handleRegions(w http.ResponseWriter, r *http.Request) {
	profile := r.PathValue("profile")
	key := cacheKey("regions", profile)
	entry, hit, err := s.resourceCache.Load(key, regionsCacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.ListRegions(r.Context(), profile)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

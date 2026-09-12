package api

import (
	"context"
	"log/slog"
	"net/http"

	ddclient "github.com/sfuruya0612/thief/backend/internal/datadog"
)

// datadogOrgResponse は /api/datadog/orgs が返す 1 組織分の情報。
// LoggedIn はその組織向けの OAuth トークンが保存されているかどうかで、frontend の
// Sub Organization タブが未ログインのバッジとログイン導線を出すために使う。
type datadogOrgResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	LoggedIn bool   `json:"logged_in"`
}

// handleDatadogOrgs は Datadog の組織一覧 (親組織と Sub Organization) を返す。
//
// 一覧の取得は親組織の認証で行う。Organizations API (GET /api/v1/org) は親組織の
// スコープで動作し、Sub Organization 個々の認証を必要としない。
//
// キャッシュするのは Datadog から取得した組織一覧だけで、ログイン済みかどうかは
// 応答を組み立てるたびに判定する。判定結果まで一緒にキャッシュすると、ログインを
// 終えた直後の再取得が「未ログイン」のままの応答を TTL の間返し続け、タブが
// ログイン導線を出したままになる。判定はローカルのファイルを読むだけなので、
// リクエストごとに行っても Datadog への呼び出しは増えない。
func (s *Server) handleDatadogOrgs(w http.ResponseWriter, r *http.Request) {
	entry, hit, err := s.resourceCache.Load(cacheKey("dd-orgs"), cacheTTL, s.refresh(r), func() (any, error) {
		// 認証の解決はキャッシュミス時 (実際に Datadog を呼ぶとき) だけ行う。
		authCtx, err := s.datadogAuthContext(r.Context(), datadogParentOrg)
		if err != nil {
			return nil, err
		}
		return s.datadogCall(r.Context(), authCtx, datadogParentOrg, func(ctx context.Context) (any, error) {
			return ddclient.ListOrgs(ctx, s.ddOrgV1)
		})
	})
	if err != nil {
		writeDatadogError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, s.datadogOrgsWithLoginState(entry.Value.([]ddclient.OrgInfo)))
}

// datadogOrgsWithLoginState は組織一覧に、保存済み OAuth トークンの有無を添える。
//
// トークンの期限は見ない。期限切れのトークンはリクエスト時に自動で更新されるため、
// ここで期限まで判定すると「更新すれば使えるセッション」を未ログインとして扱うことに
// なる。期限の確認のために Datadog を呼ぶこともしない (一覧の表示に外部への往復を
// 増やさない)。
func (s *Server) datadogOrgsWithLoginState(orgs []ddclient.OrgInfo) []datadogOrgResponse {
	site := s.cfg.Datadog.Site
	out := make([]datadogOrgResponse, 0, len(orgs))
	for _, org := range orgs {
		_, ok, err := s.ddAuth.loadToken(site, org.ID)
		if err != nil {
			// 読めたが壊れている。未ログインとして返すが、区別できるよう警告を残す
			// (datadogAuthContext がトークンを読むときと同じ扱い)。
			slog.Warn("stored datadog oauth token is unusable", "site", site, "org", org.ID, "err", err)
		}
		out = append(out, datadogOrgResponse{ID: org.ID, Name: org.Name, LoggedIn: ok})
	}
	return out
}

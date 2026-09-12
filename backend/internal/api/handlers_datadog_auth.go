package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/datadogauth"
)

const (
	// datadogAuthStartTimeout は login/start が行うネットワーク往復 (Dynamic Client
	// Registration) の上限。ブラウザでの承認は待たないため短くてよい。
	datadogAuthStartTimeout = 30 * time.Second
	// datadogAuthCallbackTimeout は callback が行う認可コードの引き換えの上限。
	datadogAuthCallbackTimeout = 30 * time.Second
	// datadogAuthErrorDescriptionMax は認可サーバが返した error_description を
	// 記録する際の上限。外部から来る文字列をそのままセッションに溜め込まない。
	datadogAuthErrorDescriptionMax = 200
)

// callback が返す固定の応答本文。認可サーバから渡されたクエリの値は一切埋め込まない。
// このページはブラウザが直接表示するため、値を反映させると反射型 XSS の経路になる。
// 失敗の詳細は login/status が JSON で返す。
const (
	datadogCallbackSuccessBody = "Datadog login succeeded. You can close this window and return to thief."
	datadogCallbackFailedBody  = "Datadog login failed. Close this window and start the login again from thief."
	datadogCallbackUnknownBody = "This Datadog login callback is unknown, already used, or expired. Close this window and start the login again from thief."
)

// datadogLoginStartResponse は login/start の応答。state は login/status で進行状態を
// 引くためのキーであり、認可要求の state そのものである。
type datadogLoginStartResponse struct {
	State            string `json:"state"`
	AuthorizationURL string `json:"authorization_url"`
}

// datadogLoginStatusResponse は login/status の応答。
type datadogLoginStatusResponse struct {
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// datadogServerRedirectURI は API サーバがコールバックを受ける redirect_uri を返す。
func (s *Server) datadogServerRedirectURI() string {
	return s.cfg.Datadog.OAuthRedirectBase + config.DatadogOAuthCallbackPath
}

// handleDatadogAuthLoginStart は認可の準備を行い、ブラウザに開かせる認可 URL を返す。
// トークンの取得は待たない (callback が受け、login/status が結果を知らせる)。
//
// org クエリパラメータで Sub Organization を指定する。省略時は親組織として扱う
// (OAuth を org 次元へ広げる前と同じ挙動)。
//
// 初回登録では CLI 用の redirect_uri も一緒に登録する。Dynamic Client Registration には
// 登録内容の更新エンドポイントが無く、後から URI を足すには新しい client_id を発行する
// しかない。CLI とサーバのどちらが先にログインしても同じクライアントを再利用できるよう、
// 両方の URI を最初にまとめて登録する。
func (s *Server) handleDatadogAuthLoginStart(w http.ResponseWriter, r *http.Request) {
	site := s.cfg.Datadog.Site
	org := r.URL.Query().Get("org")
	redirectURI := s.datadogServerRedirectURI()
	s.warnDatadogRedirectBaseReregistration(site, org, redirectURI)

	ctx, cancel := context.WithTimeout(r.Context(), datadogAuthStartTimeout)
	defer cancel()

	login, err := s.ddAuth.prepareLogin(ctx, datadogauth.PrepareParams{
		Site:                 site,
		Org:                  org,
		RedirectURI:          redirectURI,
		RegisterRedirectURIs: []string{config.DefaultDatadogOAuthCLIRedirectURI, redirectURI},
	})
	if err != nil {
		writeDatadogLoginStartError(w, err)
		return
	}

	s.ddLoginSessions.put(login)
	writeJSON(w, datadogLoginStartResponse{State: login.State, AuthorizationURL: login.AuthorizationURL})
}

// warnDatadogRedirectBaseReregistration は、既定値から変更された OAuthRedirectBase で
// クライアント登録がまだ無い場合に、これから起きる新規登録の影響を警告する。
// 変更後の redirect_uri を使うには新しい client_id を発行するしかなく、それ以前に
// 発行されたトークン (CLI 側のものを含む) は使えなくなるため、AWS SSO の start URL 変更と
// 同じく設定変更後は関係する全経路の再ログインが要る。
func (s *Server) warnDatadogRedirectBaseReregistration(site, org, redirectURI string) {
	if s.cfg.Datadog.OAuthRedirectBase == config.DefaultDatadogOAuthRedirectBase {
		return
	}
	if _, registered, err := s.ddAuth.loadClient(site, org); err != nil || registered {
		return
	}
	slog.Warn("registering a new datadog oauth client for a non-default redirect base; tokens issued to the previous client, including the CLI's, stop working",
		"site", site, "org", org, "redirect_uri", redirectURI, "default_redirect_base", config.DefaultDatadogOAuthRedirectBase)
}

// writeDatadogLoginStartError は認可準備の失敗を HTTP レスポンスへ変換する。
// 登録済みクライアントに今回の redirect_uri が無い場合は、再登録が既存トークンを
// 無効化するため PrepareLogin が自動では登録し直さない。利用者にログアウトを促せるよう
// 専用コードで返す。
func writeDatadogLoginStartError(w http.ResponseWriter, err error) {
	if errors.Is(err, datadogauth.ErrInvalidOrg) {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if errors.Is(err, datadogauth.ErrRedirectURINotRegistered) {
		writeError(w, http.StatusConflict, "DATADOG_REDIRECT_URI_NOT_REGISTERED", err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, "DATADOG_LOGIN_FAILED", err.Error())
}

// handleDatadogAuthCallback は認可サーバがブラウザをリダイレクトさせる先。state で
// 進行中のログインを引き当て、認可コードをトークンへ引き換えて保存する。
//
// 保存先の org は login/start が作った中間状態 (datadogauth.Login) が持つ。redirect_uri は
// Dynamic Client Registration で登録した文字列と完全一致していなければならず、org を
// クエリとして足せないためである。
//
// 応答はブラウザに表示される固定の文言で、クエリの値を一切含めない。frontend の fetch
// ではなくブラウザの遷移で到達するため、ここで受け取った値を書き戻すと反射型 XSS になる。
func (s *Server) handleDatadogAuthCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	state := q.Get("state")

	// take は取り出しと同時にセッションから中間状態を取り除く。state が一致しない、
	// 既に使われた、失効した、のいずれも同じ扱いにする (どれも正規のコールバックでは
	// なく、区別して伝えると state の総当たりの手掛かりになる)。
	login, ok := s.ddLoginSessions.take(state)
	if !ok {
		writeDatadogCallbackPage(w, http.StatusBadRequest, datadogCallbackUnknownBody)
		return
	}

	// RFC 6749 §4.1.2.1: 認可が拒否された場合は error / error_description が返る。
	if e := q.Get("error"); e != "" {
		s.ddLoginSessions.finish(state, datadogAuthorizationError(e, q.Get("error_description")))
		writeDatadogCallbackPage(w, http.StatusBadRequest, datadogCallbackFailedBody)
		return
	}
	code := q.Get("code")
	if code == "" {
		s.ddLoginSessions.finish(state, errors.New("datadog oauth callback has no authorization code"))
		writeDatadogCallbackPage(w, http.StatusBadRequest, datadogCallbackFailedBody)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), datadogAuthCallbackTimeout)
	defer cancel()

	if _, err := s.ddAuth.completeLogin(ctx, login, state, code); err != nil {
		s.ddLoginSessions.finish(state, err)
		slog.Warn("datadog oauth login failed at the callback", "site", s.cfg.Datadog.Site, "org", login.Org, "err", err)
		writeDatadogCallbackPage(w, http.StatusBadGateway, datadogCallbackFailedBody)
		return
	}

	s.ddLoginSessions.finish(state, nil)
	writeDatadogCallbackPage(w, http.StatusOK, datadogCallbackSuccessBody)
}

// datadogAuthorizationError は認可サーバが返した error / error_description をエラーに
// する。error_description は外部から来る文字列なので長さを切り詰める。
func datadogAuthorizationError(code, description string) error {
	if len(description) > datadogAuthErrorDescriptionMax {
		description = description[:datadogAuthErrorDescriptionMax]
	}
	if description == "" {
		return fmt.Errorf("datadog authorization failed: %s", code)
	}
	return fmt.Errorf("datadog authorization failed: %s: %s", code, description)
}

// writeDatadogCallbackPage は callback の固定文言をプレーンテキストで返す。
// HTML にしないのは、この応答に動的な値が混ざる余地を構造的に無くすためである。
func writeDatadogCallbackPage(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	fmt.Fprintln(w, body)
}

// handleDatadogAuthLoginStatus は login/start が返した state の進行状態を返す。
// frontend はこれをポーリングしてブラウザでの承認の完了を知る。
func (s *Server) handleDatadogAuthLoginStatus(w http.ResponseWriter, r *http.Request) {
	state, ok := requireQueryParam(w, r, "state")
	if !ok {
		return
	}
	status, errMsg, found := s.ddLoginSessions.lookup(state)
	if !found {
		writeError(w, http.StatusNotFound, "DATADOG_LOGIN_SESSION_NOT_FOUND",
			"datadog login session not found or expired; start a new login")
		return
	}
	writeJSON(w, datadogLoginStatusResponse{Status: string(status), ErrorMessage: errMsg})
}

// handleDatadogAuthLogout はローカルに保存した OAuth トークンとクライアント登録を
// 削除する。org クエリパラメータで対象の Sub Organization を指定し、省略時は親組織の
// 認証情報を削除する。他の org の認証情報は残る。Datadog 側にトークンを失効させる
// エンドポイントは確認できていないため、削除はローカルに限られる。既に未ログインでも
// 冪等な操作として 204 を返す。
// backend のリソースキャッシュには触れない (frontend が cache/invalidate で破棄する)。
func (s *Server) handleDatadogAuthLogout(w http.ResponseWriter, r *http.Request) {
	org := r.URL.Query().Get("org")
	if err := s.ddAuth.logout(s.cfg.Datadog.Site, org); err != nil {
		if errors.Is(err, datadogauth.ErrInvalidOrg) {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "DATADOG_LOGOUT_FAILED", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os/exec"
	"time"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/ssoauth"
)

// ssoLoginTimeout bounds how long `aws sso login` may run waiting for the
// user to complete browser authorization.
const ssoLoginTimeout = 5 * time.Minute

// ssoLoginStartTimeout はデバイス認可の開始呼び出し (RegisterClient /
// StartDeviceAuthorization) の上限。ブラウザでの認可を待たない純粋なネットワーク
// 往復なので、complete より大幅に短い上限を置く。
const ssoLoginStartTimeout = 30 * time.Second

func (s *Server) handleSSOLogin(w http.ResponseWriter, r *http.Request) {
	profile := r.PathValue("profile")
	// r.Context() はレスポンス送出直後にキャンセルされるが、本ハンドラはブラウザでの
	// 認可完了を待ってから応答するため、リクエストの生存期間とは無関係な独立 context を使う。
	ctx, cancel := context.WithTimeout(context.Background(), ssoLoginTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "aws", "sso", "login", "--profile", profile)
	err := cmd.Run()
	writeSSOLoginResult(w, err, ctx.Err())
}

// writeSSOLoginResult は `aws sso login` の実行結果を HTTP レスポンスへ変換する。
// フロントエンドはこの応答を受けてから profiles を再取得するため、ブラウザでの認可が
// 実際に完了した (または失敗が確定した) 後にのみ 2xx / エラーを返す必要がある。
func writeSSOLoginResult(w http.ResponseWriter, runErr, ctxErr error) {
	switch {
	case runErr == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(ctxErr, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, "SSO_LOGIN_TIMEOUT",
			"sso login timed out waiting for browser authorization")
	default:
		writeError(w, http.StatusInternalServerError, "SSO_LOGIN_FAILED",
			"sso login failed: "+runErr.Error())
	}
}

// ssoLoginDeps は start / complete エンドポイントが呼ぶ外部処理をまとめる。
// いずれも AWS への接続または ~/.aws 配下の読み書きを伴い、テストから実行できない
// ため関数値で差し替える (internal/ssoauth の Deps と同じ理由)。
type ssoLoginDeps struct {
	resolveConfig func(profile string) (*awsinternal.SSOConfig, error)
	start         func(ctx context.Context, region, startURL string) (*ssoauth.Session, error)
	wait          func(ctx context.Context, sess *ssoauth.Session) (*ssoauth.TokenCache, error)
}

// defaultSSOLoginDeps は本番で使う実装を返す。wait は ssoauth.Wait を DefaultDeps で
// 呼ぶため、トークン取得の成功時に AWS CLI 互換のキャッシュ (~/.aws/sso/cache) への
// 保存まで行う。
func defaultSSOLoginDeps() ssoLoginDeps {
	return ssoLoginDeps{
		resolveConfig: awsinternal.ResolveSSOConfig,
		start: func(ctx context.Context, region, startURL string) (*ssoauth.Session, error) {
			return ssoauth.Start(ctx, region, startURL, ssoauth.DefaultDeps())
		},
		wait: func(ctx context.Context, sess *ssoauth.Session) (*ssoauth.TokenCache, error) {
			return ssoauth.Wait(ctx, sess, ssoauth.DefaultDeps())
		},
	}
}

// ssoLoginStartResponse は start エンドポイントの応答。デバイスコードとクライアント
// シークレットは含めない (frontend に露出させる必要が無く、backend がセッション ID に
// 紐づけて保持する)。verification_uri_complete は RFC 8628 §3.2 で OPTIONAL のため
// 空になりうる。その場合 frontend は verification_uri と user_code を提示する。
type ssoLoginStartResponse struct {
	SessionID               string `json:"session_id"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	VerificationURI         string `json:"verification_uri"`
	UserCode                string `json:"user_code"`
}

// ssoLoginCompleteRequest は complete エンドポイントのリクエストボディ。
type ssoLoginCompleteRequest struct {
	SessionID string `json:"session_id"`
}

// handleSSOLoginStart は profile の SSO 設定を解決してデバイス認可フローを開始し、
// 認可 URL とセッション ID を返す。トークンの待機は行わない (complete が担う)。
func (s *Server) handleSSOLoginStart(w http.ResponseWriter, r *http.Request) {
	profile := r.PathValue("profile")
	if err := awsinternal.ValidateProfileName(profile); err != nil {
		writeBadRequest(w, err.Error())
		return
	}

	cfg, err := s.ssoLogin.resolveConfig(profile)
	if err != nil {
		writeSSOLoginStartError(w, err)
		return
	}

	// start は応答を返すまでの同期処理なので r.Context() を親にしてよいが、
	// AWS への往復が返らない場合に無制限に待たないよう上限を付ける。
	ctx, cancel := context.WithTimeout(r.Context(), ssoLoginStartTimeout)
	defer cancel()

	sess, err := s.ssoLogin.start(ctx, cfg.Region, cfg.StartURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SSO_LOGIN_FAILED", err.Error())
		return
	}

	id, err := s.ssoLoginSessions.put(profile, sess)
	if err != nil {
		writeInternalError(w, err.Error())
		return
	}

	writeJSON(w, ssoLoginStartResponse{
		SessionID:               id,
		VerificationURIComplete: sess.DeviceAuth.VerificationURIComplete,
		VerificationURI:         sess.DeviceAuth.VerificationURI,
		UserCode:                sess.DeviceAuth.UserCode,
	})
}

// writeSSOLoginStartError は SSO 設定の解決エラーを HTTP レスポンスへ変換する。
// profile が存在しない (404) と SSO 設定が無い (400) は frontend が別のメッセージを
// 出せるようコードを分ける。config の読み取り失敗などはサーバ側の問題として 500。
func writeSSOLoginStartError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, awsinternal.ErrProfileNotFound):
		writeError(w, http.StatusNotFound, "PROFILE_NOT_FOUND", err.Error())
	case errors.Is(err, awsinternal.ErrSSONotConfigured):
		writeError(w, http.StatusBadRequest, "SSO_NOT_CONFIGURED", err.Error())
	default:
		writeInternalError(w, err.Error())
	}
}

// handleSSOLoginComplete はセッション ID でデバイス認可の中間状態を引き、ブラウザでの
// 認可完了までトークンをポーリングして、成功時にキャッシュ保存済みで 204 を返す。
func (s *Server) handleSSOLoginComplete(w http.ResponseWriter, r *http.Request) {
	profile := r.PathValue("profile")
	if err := awsinternal.ValidateProfileName(profile); err != nil {
		writeBadRequest(w, err.Error())
		return
	}

	var req ssoLoginCompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "invalid request body: "+err.Error())
		return
	}
	if req.SessionID == "" {
		writeBadRequest(w, "session_id is required")
		return
	}

	// take は取り出しと同時にセッションを削除する (同一 ID の complete は 1 回だけ
	// 有効)。パスの profile と一致しないセッションは、他 profile 宛の ID の流用なので
	// 未知の ID と同じ扱いにする (このときセッションは消費されず、正しい profile での
	// complete は引き続き可能)。backend の再起動でセッションが消えた場合もここに落ち、
	// frontend は 404 を受けてログインをやり直す。
	entry, ok := s.ssoLoginSessions.take(profile, req.SessionID)
	if !ok {
		writeError(w, http.StatusNotFound, "SSO_LOGIN_SESSION_NOT_FOUND",
			"sso login session not found or expired; start a new login")
		return
	}

	// r.Context() はレスポンス送出直後にキャンセルされるが、本ハンドラはブラウザでの
	// 認可完了を待ってから応答するため、リクエストの生存期間とは無関係な独立 context を
	// 使う (現行 handleSSOLogin と同じ理由・同じ 5 分)。ポーリング自体の打ち切りは
	// device code の有効期限 (expiresIn) からも決まり、早い方が効く。
	ctx, cancel := context.WithTimeout(context.Background(), ssoLoginTimeout)
	defer cancel()

	// 戻り値のキャッシュは wait (本番実装では ssoauth.Wait) が保存まで済ませており、
	// この層で使う情報は無い。
	if _, err := s.ssoLogin.wait(ctx, entry.sess); err != nil {
		writeSSOLoginCompleteError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// writeSSOLoginCompleteError はトークン待機の失敗を HTTP レスポンスへ変換する。
// access_denied (ユーザが認可を拒否) は frontend が「認可タブを閉じてよい失敗」と
// 判別できるよう専用コードで返す (issue 0149 が利用する)。IsAccessDenied は smithy の
// エラーコードによる正確な分類なので最初に検査する。タイムアウトは、device code の
// 有効期限による打ち切り (ErrSSOTokenTimeout) とハンドラの 5 分上限
// (context.DeadlineExceeded) のどちらも 504 に分類する。
func writeSSOLoginCompleteError(w http.ResponseWriter, err error) {
	switch {
	case awsinternal.IsAccessDenied(err):
		writeError(w, http.StatusForbidden, "SSO_LOGIN_ACCESS_DENIED", err.Error())
	case errors.Is(err, awsinternal.ErrSSOTokenTimeout), errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, "SSO_LOGIN_TIMEOUT", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "SSO_LOGIN_FAILED", err.Error())
	}
}

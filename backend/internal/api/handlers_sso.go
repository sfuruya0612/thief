package api

import (
	"context"
	"errors"
	"net/http"
	"os/exec"
	"time"
)

// ssoLoginTimeout bounds how long `aws sso login` may run waiting for the
// user to complete browser authorization.
const ssoLoginTimeout = 5 * time.Minute

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

package api

import (
	"context"
	"net/http"
	"os/exec"
	"time"
)

// ssoLoginTimeout bounds how long the background `aws sso login` process may
// run waiting for the user to complete browser authorization.
const ssoLoginTimeout = 5 * time.Minute

func (s *Server) handleSSOLogin(w http.ResponseWriter, r *http.Request) {
	profile := r.PathValue("profile")
	// r.Context() はレスポンス送出直後にキャンセルされるため使えない。
	// ログイン処理はブラウザでの認可完了を待つ必要があり、リクエストの生存期間を超えて動く。
	ctx, cancel := context.WithTimeout(context.Background(), ssoLoginTimeout)
	cmd := exec.CommandContext(ctx, "aws", "sso", "login", "--profile", profile)
	if err := cmd.Start(); err != nil {
		cancel()
		writeInternalError(w, "start sso login: "+err.Error())
		return
	}
	go func() {
		defer cancel()
		_ = cmd.Wait()
	}()
	w.WriteHeader(http.StatusAccepted)
}

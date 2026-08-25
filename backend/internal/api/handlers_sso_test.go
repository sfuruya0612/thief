package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ssooidctypes "github.com/aws/aws-sdk-go-v2/service/ssooidc/types"
	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/ssoauth"
)

// TestWriteSSOLoginResult は aws sso login の実行結果 (成功 / タイムアウト / 一般エラー)
// が正しい HTTP ステータスとエラーコードへ変換されることを検証する。
func TestWriteSSOLoginResult(t *testing.T) {
	tests := []struct {
		name       string
		runErr     error
		ctxErr     error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "成功時は204",
			runErr:     nil,
			ctxErr:     nil,
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "コンテキストタイムアウト時は504",
			runErr:     errors.New("signal: killed"),
			ctxErr:     context.DeadlineExceeded,
			wantStatus: http.StatusGatewayTimeout,
			wantCode:   "SSO_LOGIN_TIMEOUT",
		},
		{
			name:       "aws cli の一般エラー時は500",
			runErr:     errors.New("exit status 1"),
			ctxErr:     nil,
			wantStatus: http.StatusInternalServerError,
			wantCode:   "SSO_LOGIN_FAILED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeSSOLoginResult(w, tt.runErr, tt.ctxErr)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantCode == "" {
				if w.Body.Len() != 0 {
					t.Fatalf("body = %q, want empty", w.Body.String())
				}
				return
			}

			var body ErrorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("unmarshal body: %v (body=%s)", err, w.Body.String())
			}
			if body.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", body.Code, tt.wantCode)
			}
		})
	}
}

// --- SSO デバイス認可 (start / complete) エンドポイント ---

// newSSOLoginTestServer は新設エンドポイントのテスト用に、注入した ssoLoginDeps と
// 空のセッションストアを持つ Server をルート登録済みで組み立てる。リクエストは
// s.mux.ServeHTTP へ流し、ルートパターン (メソッドとパス) の登録も含めて検証する。
func newSSOLoginTestServer(t *testing.T, deps ssoLoginDeps) *Server {
	t.Helper()
	s := newTestServer(t)
	s.ssoLoginSessions = newSSOLoginSessionStore()
	s.ssoLogin = deps
	s.mux = http.NewServeMux()
	s.registerRoutes()
	return s
}

// okSSOLoginDeps は start / complete が成功する ssoLoginDeps を返す。
// waited には complete が wait へ渡したセッションが積まれる。
func okSSOLoginDeps(waited *[]*ssoauth.Session) ssoLoginDeps {
	return ssoLoginDeps{
		resolveConfig: func(profile string) (*awsinternal.SSOConfig, error) {
			return &awsinternal.SSOConfig{Region: "ap-northeast-1", StartURL: "https://example.awsapps.com/start"}, nil
		},
		start: func(ctx context.Context, region, startURL string) (*ssoauth.Session, error) {
			return testSSOAuthSessionWithURIs(600), nil
		},
		wait: func(ctx context.Context, sess *ssoauth.Session) (*ssoauth.TokenCache, error) {
			if waited != nil {
				*waited = append(*waited, sess)
			}
			return &ssoauth.TokenCache{AccessToken: "token"}, nil
		},
	}
}

// testSSOAuthSessionWithURIs は認可 URL と user code を持つテスト用セッションを組む。
func testSSOAuthSessionWithURIs(expiresIn int32) *ssoauth.Session {
	sess := testSSOAuthSession(expiresIn)
	sess.DeviceAuth.VerificationURI = "https://device.sso.ap-northeast-1.amazonaws.com/"
	sess.DeviceAuth.VerificationURIComplete = "https://device.sso.ap-northeast-1.amazonaws.com/?user_code=USER-CODE"
	return sess
}

// postSSOLoginStart は start エンドポイントへ POST し、レコーダを返す。
func postSSOLoginStart(t *testing.T, s *Server, profile string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/aws/profiles/"+profile+"/sso/login/start", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, r)
	return w
}

// postSSOLoginComplete は complete エンドポイントへセッション ID 付きで POST し、
// レコーダを返す。
func postSSOLoginComplete(t *testing.T, s *Server, profile, sessionID string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(ssoLoginCompleteRequest{SessionID: sessionID})
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/aws/profiles/"+profile+"/sso/login/complete", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, r)
	return w
}

// decodeSSOLoginStartResponse は start の応答 JSON をデコードする。
func decodeSSOLoginStartResponse(t *testing.T, w *httptest.ResponseRecorder) ssoLoginStartResponse {
	t.Helper()
	var resp ssoLoginStartResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal start response: %v (body=%s)", err, w.Body.String())
	}
	return resp
}

// assertErrorCode は応答が期待したステータスとエラーコードであることを検証する。
func assertErrorCode(t *testing.T, w *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	if w.Code != wantStatus {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, wantStatus, w.Body.String())
	}
	var body ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal error body: %v (body=%s)", err, w.Body.String())
	}
	if body.Code != wantCode {
		t.Errorf("code = %q, want %q", body.Code, wantCode)
	}
}

// TestSSOLoginStartAndCompleteSucceed は start が認可 URL とセッション ID を返し、
// そのセッション ID での complete が start の作ったセッションでトークンを待機して
// 204 を返すことを検証する。
func TestSSOLoginStartAndCompleteSucceed(t *testing.T) {
	var waited []*ssoauth.Session
	s := newSSOLoginTestServer(t, okSSOLoginDeps(&waited))

	w := postSSOLoginStart(t, s, "dev")
	if w.Code != http.StatusOK {
		t.Fatalf("start status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	resp := decodeSSOLoginStartResponse(t, w)
	if resp.SessionID == "" {
		t.Fatal("session_id is empty")
	}
	if resp.VerificationURIComplete != "https://device.sso.ap-northeast-1.amazonaws.com/?user_code=USER-CODE" {
		t.Errorf("verification_uri_complete = %q", resp.VerificationURIComplete)
	}
	if resp.VerificationURI != "https://device.sso.ap-northeast-1.amazonaws.com/" {
		t.Errorf("verification_uri = %q", resp.VerificationURI)
	}
	if resp.UserCode != "USER-CODE" {
		t.Errorf("user_code = %q", resp.UserCode)
	}
	// デバイスコードとクライアントシークレットはレスポンスに含めない (完了条件)。
	for _, secret := range []string{"device-code", "secret"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Errorf("start response leaks %q: %s", secret, w.Body.String())
		}
	}

	cw := postSSOLoginComplete(t, s, "dev", resp.SessionID)
	if cw.Code != http.StatusNoContent {
		t.Fatalf("complete status = %d, want %d (body=%s)", cw.Code, http.StatusNoContent, cw.Body.String())
	}
	if len(waited) != 1 {
		t.Fatalf("wait called %d times, want 1", len(waited))
	}
	if waited[0].DeviceAuth == nil || waited[0].DeviceAuth.DeviceCode != "device-code" {
		t.Errorf("wait received session %+v, want the one built by start", waited[0])
	}
}

// TestSSOLoginStartErrors は start の失敗経路 (SSO 設定なし、profile 不在、不正な
// profile 名、デバイス認可の開始失敗) のステータスとエラーコードを検証する。
func TestSSOLoginStartErrors(t *testing.T) {
	tests := []struct {
		name       string
		profile    string
		deps       ssoLoginDeps
		wantStatus int
		wantCode   string
	}{
		{
			name:    "SSO 設定なし profile は 400 SSO_NOT_CONFIGURED",
			profile: "plain",
			deps: ssoLoginDeps{
				resolveConfig: func(profile string) (*awsinternal.SSOConfig, error) {
					return nil, fmt.Errorf("%w: profile %q has neither sso_session nor sso_start_url", awsinternal.ErrSSONotConfigured, profile)
				},
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "SSO_NOT_CONFIGURED",
		},
		{
			name:    "存在しない profile は 404 PROFILE_NOT_FOUND",
			profile: "missing",
			deps: ssoLoginDeps{
				resolveConfig: func(profile string) (*awsinternal.SSOConfig, error) {
					return nil, fmt.Errorf("%w: %q", awsinternal.ErrProfileNotFound, profile)
				},
			},
			wantStatus: http.StatusNotFound,
			wantCode:   "PROFILE_NOT_FOUND",
		},
		{
			name:       "不正な profile 名は 400 BAD_REQUEST",
			profile:    "bad%2Fname",
			deps:       ssoLoginDeps{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name:    "config 読み取り失敗は 500 INTERNAL_ERROR",
			profile: "dev",
			deps: ssoLoginDeps{
				resolveConfig: func(profile string) (*awsinternal.SSOConfig, error) {
					return nil, errors.New("read aws config: permission denied")
				},
			},
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
		},
		{
			name:    "デバイス認可の開始失敗は 500 SSO_LOGIN_FAILED",
			profile: "dev",
			deps: ssoLoginDeps{
				resolveConfig: func(profile string) (*awsinternal.SSOConfig, error) {
					return &awsinternal.SSOConfig{Region: "ap-northeast-1", StartURL: "https://example.awsapps.com/start"}, nil
				},
				start: func(ctx context.Context, region, startURL string) (*ssoauth.Session, error) {
					return nil, errors.New("register sso oidc client: connection refused")
				},
			},
			wantStatus: http.StatusInternalServerError,
			wantCode:   "SSO_LOGIN_FAILED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSSOLoginTestServer(t, tt.deps)
			w := postSSOLoginStart(t, s, tt.profile)
			assertErrorCode(t, w, tt.wantStatus, tt.wantCode)
		})
	}
}

// TestSSOLoginStartTimeout は start に渡る context に ssoLoginStartTimeout 以内の
// デッドラインが設定されることと、start がタイムアウトした場合の応答を検証する。
// start のタイムアウトはネットワーク起因の開始失敗であり、認可待ちの打ち切り
// (complete 側の 504 SSO_LOGIN_TIMEOUT) とは意味が違うため 500 SSO_LOGIN_FAILED に
// 分類する。この分類を変える場合はこのテストごと意図を見直すこと。
func TestSSOLoginStartTimeout(t *testing.T) {
	t.Run("start の context にはデッドラインが設定される", func(t *testing.T) {
		deps := okSSOLoginDeps(nil)
		var deadline time.Time
		var hasDeadline bool
		deps.start = func(ctx context.Context, region, startURL string) (*ssoauth.Session, error) {
			deadline, hasDeadline = ctx.Deadline()
			return testSSOAuthSessionWithURIs(600), nil
		}
		s := newSSOLoginTestServer(t, deps)

		before := time.Now()
		w := postSSOLoginStart(t, s, "dev")
		if w.Code != http.StatusOK {
			t.Fatalf("start status = %d (body=%s)", w.Code, w.Body.String())
		}
		if !hasDeadline {
			t.Fatal("start context has no deadline, want one within ssoLoginStartTimeout")
		}
		// before はリクエスト送信前の時刻なので、ハンドラがデッドラインを張るまでの
		// 経過ぶんだけ ssoLoginStartTimeout をわずかに超えうる。1 秒の余裕を持たせる。
		if remaining := deadline.Sub(before); remaining <= 0 || remaining > ssoLoginStartTimeout+time.Second {
			t.Errorf("deadline in %v, want within (0, %v]", remaining, ssoLoginStartTimeout+time.Second)
		}
	})

	t.Run("start のタイムアウトは 500 SSO_LOGIN_FAILED", func(t *testing.T) {
		deps := okSSOLoginDeps(nil)
		deps.start = func(ctx context.Context, region, startURL string) (*ssoauth.Session, error) {
			return nil, fmt.Errorf("start device authorization: %w", context.DeadlineExceeded)
		}
		s := newSSOLoginTestServer(t, deps)

		w := postSSOLoginStart(t, s, "dev")
		assertErrorCode(t, w, http.StatusInternalServerError, "SSO_LOGIN_FAILED")
	})
}

// TestSSOLoginStartSessionIDGenerationFails はセッション ID の乱数生成に失敗した場合に
// 500 INTERNAL_ERROR を返すことを検証する (セッションストア側のエラーパス)。
func TestSSOLoginStartSessionIDGenerationFails(t *testing.T) {
	s := newSSOLoginTestServer(t, okSSOLoginDeps(nil))
	s.ssoLoginSessions.randRead = func(_ []byte) (int, error) {
		return 0, errors.New("rand broken")
	}

	w := postSSOLoginStart(t, s, "dev")
	assertErrorCode(t, w, http.StatusInternalServerError, "INTERNAL_ERROR")
}

// TestSSOLoginCompleteWaitErrors は complete のトークン待機の失敗分類 (access_denied、
// ポーリングタイムアウト、5 分上限、キャッシュ保存失敗) を検証する。
// キャッシュ保存は本番実装では ssoauth.Wait の内部で行われるため、ハンドラ層からは
// 「分類対象外のエラー」としてしか観測できない。最後のケースはその代表例として
// 保存失敗のエラー形状を使っており、default 分岐 (未分類エラー全般) の検証を兼ねる。
func TestSSOLoginCompleteWaitErrors(t *testing.T) {
	tests := []struct {
		name       string
		waitErr    error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "認可拒否 (access_denied) は 403 SSO_LOGIN_ACCESS_DENIED",
			waitErr:    fmt.Errorf("create sso oidc token: %w", &ssooidctypes.AccessDeniedException{}),
			wantStatus: http.StatusForbidden,
			wantCode:   "SSO_LOGIN_ACCESS_DENIED",
		},
		{
			name:       "device code の有効期限によるポーリングタイムアウトは 504 SSO_LOGIN_TIMEOUT",
			waitErr:    awsinternal.ErrSSOTokenTimeout,
			wantStatus: http.StatusGatewayTimeout,
			wantCode:   "SSO_LOGIN_TIMEOUT",
		},
		{
			name:       "ハンドラの 5 分上限 (context.DeadlineExceeded) は 504 SSO_LOGIN_TIMEOUT",
			waitErr:    context.DeadlineExceeded,
			wantStatus: http.StatusGatewayTimeout,
			wantCode:   "SSO_LOGIN_TIMEOUT",
		},
		{
			name:       "キャッシュ保存失敗は 500 SSO_LOGIN_FAILED",
			waitErr:    fmt.Errorf("save cache file: %w", errors.New("failed to write cache file: no space left on device")),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "SSO_LOGIN_FAILED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := okSSOLoginDeps(nil)
			deps.wait = func(ctx context.Context, sess *ssoauth.Session) (*ssoauth.TokenCache, error) {
				return nil, tt.waitErr
			}
			s := newSSOLoginTestServer(t, deps)

			w := postSSOLoginStart(t, s, "dev")
			if w.Code != http.StatusOK {
				t.Fatalf("start status = %d (body=%s)", w.Code, w.Body.String())
			}
			resp := decodeSSOLoginStartResponse(t, w)

			cw := postSSOLoginComplete(t, s, "dev", resp.SessionID)
			assertErrorCode(t, cw, tt.wantStatus, tt.wantCode)
		})
	}
}

// TestSSOLoginCompleteSessionNotFound は未知・失効・使用済み・他 profile 宛の
// セッション ID とボディ不備が、待機を呼ばずに 4xx で返ることを検証する。
func TestSSOLoginCompleteSessionNotFound(t *testing.T) {
	t.Run("未知のセッション ID は 404", func(t *testing.T) {
		var waited []*ssoauth.Session
		s := newSSOLoginTestServer(t, okSSOLoginDeps(&waited))
		w := postSSOLoginComplete(t, s, "dev", "unknown-session-id")
		assertErrorCode(t, w, http.StatusNotFound, "SSO_LOGIN_SESSION_NOT_FOUND")
		if len(waited) != 0 {
			t.Errorf("wait called %d times, want 0", len(waited))
		}
	})

	t.Run("失効したセッション ID は 404", func(t *testing.T) {
		s := newSSOLoginTestServer(t, okSSOLoginDeps(nil))
		w := postSSOLoginStart(t, s, "dev")
		resp := decodeSSOLoginStartResponse(t, w)

		// ストアの時計を進めてデバイス認可の有効期限 (600 秒) を過ぎさせる。
		s.ssoLoginSessions.now = func() time.Time { return time.Now().Add(601 * time.Second) }
		cw := postSSOLoginComplete(t, s, "dev", resp.SessionID)
		assertErrorCode(t, cw, http.StatusNotFound, "SSO_LOGIN_SESSION_NOT_FOUND")
	})

	t.Run("使用済みセッション ID の 2 回目は 404", func(t *testing.T) {
		s := newSSOLoginTestServer(t, okSSOLoginDeps(nil))
		w := postSSOLoginStart(t, s, "dev")
		resp := decodeSSOLoginStartResponse(t, w)

		if cw := postSSOLoginComplete(t, s, "dev", resp.SessionID); cw.Code != http.StatusNoContent {
			t.Fatalf("first complete status = %d (body=%s)", cw.Code, cw.Body.String())
		}
		cw := postSSOLoginComplete(t, s, "dev", resp.SessionID)
		assertErrorCode(t, cw, http.StatusNotFound, "SSO_LOGIN_SESSION_NOT_FOUND")
	})

	t.Run("他 profile 宛のセッション ID は 404 でセッションを消費しない", func(t *testing.T) {
		var waited []*ssoauth.Session
		s := newSSOLoginTestServer(t, okSSOLoginDeps(&waited))
		w := postSSOLoginStart(t, s, "dev")
		resp := decodeSSOLoginStartResponse(t, w)

		cw := postSSOLoginComplete(t, s, "other", resp.SessionID)
		assertErrorCode(t, cw, http.StatusNotFound, "SSO_LOGIN_SESSION_NOT_FOUND")
		if len(waited) != 0 {
			t.Errorf("wait called %d times, want 0", len(waited))
		}

		// 誤った profile への complete がセッションを消費せず、正しい profile での
		// complete は引き続き成功する。
		if cw := postSSOLoginComplete(t, s, "dev", resp.SessionID); cw.Code != http.StatusNoContent {
			t.Fatalf("complete with correct profile after mismatch = %d, want %d", cw.Code, http.StatusNoContent)
		}
		if len(waited) != 1 {
			t.Errorf("wait called %d times, want 1", len(waited))
		}
	})

	t.Run("session_id の無いボディは 400", func(t *testing.T) {
		var waited []*ssoauth.Session
		s := newSSOLoginTestServer(t, okSSOLoginDeps(&waited))
		w := postSSOLoginComplete(t, s, "dev", "")
		assertErrorCode(t, w, http.StatusBadRequest, "BAD_REQUEST")
		if len(waited) != 0 {
			t.Errorf("wait called %d times, want 0", len(waited))
		}
	})

	t.Run("JSON でないボディは 400", func(t *testing.T) {
		var waited []*ssoauth.Session
		s := newSSOLoginTestServer(t, okSSOLoginDeps(&waited))
		r := httptest.NewRequest(http.MethodPost, "/api/aws/profiles/dev/sso/login/complete", strings.NewReader("not json"))
		w := httptest.NewRecorder()
		s.mux.ServeHTTP(w, r)
		assertErrorCode(t, w, http.StatusBadRequest, "BAD_REQUEST")
		if len(waited) != 0 {
			t.Errorf("wait called %d times, want 0", len(waited))
		}
	})

	t.Run("不正な profile 名は 400", func(t *testing.T) {
		var waited []*ssoauth.Session
		s := newSSOLoginTestServer(t, okSSOLoginDeps(&waited))
		w := postSSOLoginComplete(t, s, "bad%2Fname", "some-session-id")
		assertErrorCode(t, w, http.StatusBadRequest, "BAD_REQUEST")
		if len(waited) != 0 {
			t.Errorf("wait called %d times, want 0", len(waited))
		}
	})
}

// TestSSOLoginConcurrentSessionsSameProfile は同一 profile での複数の start がそれぞれ
// 独立したセッションになり、双方の complete が互いに干渉せず成功することを検証する
// (issue 0148 の設計判断: 後発の start が先発のセッションを上書きしない)。
func TestSSOLoginConcurrentSessionsSameProfile(t *testing.T) {
	var waited []*ssoauth.Session
	deps := okSSOLoginDeps(&waited)
	// start ごとに別のセッションを返し、complete がどちらを待機したか区別できるようにする。
	calls := 0
	deps.start = func(ctx context.Context, region, startURL string) (*ssoauth.Session, error) {
		calls++
		sess := testSSOAuthSessionWithURIs(600)
		sess.DeviceAuth.DeviceCode = fmt.Sprintf("device-code-%d", calls)
		return sess, nil
	}
	s := newSSOLoginTestServer(t, deps)

	resp1 := decodeSSOLoginStartResponse(t, postSSOLoginStart(t, s, "dev"))
	resp2 := decodeSSOLoginStartResponse(t, postSSOLoginStart(t, s, "dev"))
	if resp1.SessionID == resp2.SessionID {
		t.Fatalf("session ids collide: %q", resp1.SessionID)
	}

	// 先発・後発のどちらの順で complete しても、それぞれ自分のセッションで待機する。
	if w := postSSOLoginComplete(t, s, "dev", resp2.SessionID); w.Code != http.StatusNoContent {
		t.Fatalf("complete(resp2) status = %d (body=%s)", w.Code, w.Body.String())
	}
	if w := postSSOLoginComplete(t, s, "dev", resp1.SessionID); w.Code != http.StatusNoContent {
		t.Fatalf("complete(resp1) status = %d (body=%s)", w.Code, w.Body.String())
	}
	if len(waited) != 2 {
		t.Fatalf("wait called %d times, want 2", len(waited))
	}
	if got := waited[0].DeviceAuth.DeviceCode; got != "device-code-2" {
		t.Errorf("first wait device code = %q, want %q", got, "device-code-2")
	}
	if got := waited[1].DeviceAuth.DeviceCode; got != "device-code-1" {
		t.Errorf("second wait device code = %q, want %q", got, "device-code-1")
	}
}

// TestDefaultSSOLoginDepsIsFullyWired は本番配線の ssoLoginDeps に nil の依存が
// 無いことを検証する (実 AWS 接続を伴うため呼び出しはしない)。
func TestDefaultSSOLoginDepsIsFullyWired(t *testing.T) {
	deps := defaultSSOLoginDeps()
	if deps.resolveConfig == nil {
		t.Error("resolveConfig is nil")
	}
	if deps.start == nil {
		t.Error("start is nil")
	}
	if deps.wait == nil {
		t.Error("wait is nil")
	}
}

// TestExistingSSOLoginRouteStillRegistered は既存の POST /sso/login が新設ルートの
// 追加後も同じハンドラのまま登録されていることを検証する (issue 0148 の完了条件:
// 旧エンドポイントは変更せず残す)。パターンの一致だけを確認し、ハンドラは呼ばない。
func TestExistingSSOLoginRouteStillRegistered(t *testing.T) {
	s := newSSOLoginTestServer(t, ssoLoginDeps{})
	r := httptest.NewRequest(http.MethodPost, "/api/aws/profiles/dev/sso/login", nil)
	_, pattern := s.mux.Handler(r)
	if pattern != "POST /api/aws/profiles/{profile}/sso/login" {
		t.Errorf("pattern = %q, want the existing sso/login route", pattern)
	}
}

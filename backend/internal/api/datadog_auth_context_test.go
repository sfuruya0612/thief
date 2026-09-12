package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	ddapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/sfuruya0612/thief/backend/internal/datadogauth"
)

// testDatadogSite はテストで使う site。config.Defaults() の既定値と揃える。
const testDatadogSite = "datadoghq.com"

// testDatadogNow は期限判定の基準時刻。TokenSet.IsExpired は実際の期限の 300 秒前から
// 期限切れとみなすため、有効なトークンには十分長い expires_in を持たせる。
var testDatadogNow = time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)

// newTestDatadogToken は保存ファイルと同じ JSON からトークンを組み立てる。
// TokenSet.AccessToken / RefreshToken は internal/datadogauth の非公開型なので、
// package api からは復号でしか値を入れられない。
func newTestDatadogToken(t *testing.T, accessToken, refreshToken string, expiresIn int64) *datadogauth.TokenSet {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"expires_in":    expiresIn,
		"issued_at":     testDatadogNow,
		"client_id":     "client-id",
	})
	if err != nil {
		t.Fatalf("marshal test token: %v", err)
	}
	var tok datadogauth.TokenSet
	if err := json.Unmarshal(raw, &tok); err != nil {
		t.Fatalf("unmarshal test token: %v", err)
	}
	return &tok
}

// validTestToken は基準時刻の時点で期限内のトークンを返す。
func validTestToken(t *testing.T, accessToken string) *datadogauth.TokenSet {
	t.Helper()
	return newTestDatadogToken(t, accessToken, "refresh-token", 3600)
}

// expiredTestToken は基準時刻の時点で期限切れのトークンを返す。
func expiredTestToken(t *testing.T, accessToken string) *datadogauth.TokenSet {
	t.Helper()
	return newTestDatadogToken(t, accessToken, "refresh-token", 1)
}

// datadogAuthDisk はテスト用の datadogAuthDeps が読み書きする疑似的なトークンファイル。
// 実ファイルを使わずに「他プロセスが先に更新した」状況を作れるようにする。
type datadogAuthDisk struct {
	mu sync.Mutex

	token    *datadogauth.TokenSet
	loadErr  error
	client   *datadogauth.ClientCredentials
	saveErr  error
	loads    int
	saves    int
	refreshs int

	// orgs は読み書きで渡された org を現れた順に記録する。保存先が org ごとに
	// 分かれていることを、疑似ディスク側から確かめるために使う。
	orgs []string

	// refresh はリフレッシュの結果を決める。nil の場合は失敗しない既定の実装を使う。
	refresh func(disk *datadogAuthDisk) (*datadogauth.TokenSet, error)
}

func (d *datadogAuthDisk) deps(t *testing.T) datadogAuthDeps {
	t.Helper()
	return datadogAuthDeps{
		loadToken: func(_, org string) (*datadogauth.TokenSet, bool, error) {
			d.mu.Lock()
			defer d.mu.Unlock()
			d.loads++
			d.orgs = append(d.orgs, org)
			if d.loadErr != nil {
				return nil, false, d.loadErr
			}
			if d.token == nil {
				return nil, false, nil
			}
			return d.token, true, nil
		},
		saveToken: func(_, org string, tok *datadogauth.TokenSet) error {
			d.mu.Lock()
			defer d.mu.Unlock()
			d.saves++
			d.orgs = append(d.orgs, org)
			if d.saveErr != nil {
				return d.saveErr
			}
			d.token = tok
			return nil
		},
		loadClient: func(_, org string) (*datadogauth.ClientCredentials, bool, error) {
			d.mu.Lock()
			defer d.mu.Unlock()
			d.orgs = append(d.orgs, org)
			if d.client == nil {
				return nil, false, nil
			}
			return d.client, true, nil
		},
		refreshToken: func(context.Context, string, string, string) (*datadogauth.TokenSet, error) {
			d.mu.Lock()
			d.refreshs++
			refresh := d.refresh
			d.mu.Unlock()
			if refresh == nil {
				return validTestToken(t, "refreshed-token"), nil
			}
			return refresh(d)
		},
		now: func() time.Time { return testDatadogNow },
	}
}

func (d *datadogAuthDisk) counts() (loads, saves, refreshs int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.loads, d.saves, d.refreshs
}

// seenOrgs は読み書きで渡された org を重複なしで返す。
func (d *datadogAuthDisk) seenOrgs() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	seen := map[string]bool{}
	var orgs []string
	for _, org := range d.orgs {
		if seen[org] {
			continue
		}
		seen[org] = true
		orgs = append(orgs, org)
	}
	return orgs
}

// newDatadogAuthTestServer は疑似ディスクと静的キーの有無を指定して Server を組む。
func newDatadogAuthTestServer(t *testing.T, disk *datadogAuthDisk, staticKeys bool) *Server {
	t.Helper()
	s := newTestServer(t)
	s.ddAuth = disk.deps(t)
	if staticKeys {
		s.cfg.SetDatadogAPIKey("api-key")
		s.cfg.SetDatadogAppKey("app-key")
	}
	return s
}

// datadogAuthKind は認証コンテキストがどの資格情報を運んでいるかを返す。
// SDK が実際に読む context の値を直接見て、送られるヘッダと一致させる。
func datadogAuthKind(t *testing.T, ctx context.Context) string {
	t.Helper()
	token, hasToken := ctx.Value(ddapi.ContextAccessToken).(string)
	_, hasKeys := ctx.Value(ddapi.ContextAPIKeys).(map[string]ddapi.APIKey)
	switch {
	case hasToken && hasKeys:
		t.Fatal("auth context carries both an oauth token and static API keys")
		return ""
	case hasToken:
		return "oauth:" + token
	case hasKeys:
		return "static"
	default:
		return "none"
	}
}

// forbiddenDatadogError は Datadog が権限不足で返す 403 応答を模したエラー。
// 実 Datadog での本文の形は未検証であり、判定は状態コードだけで行う (datadogCall 参照)。
func forbiddenDatadogError() error {
	return fmt.Errorf("get datadog historical cost: %w", ddapi.GenericOpenAPIError{
		ErrorMessage: "403 Forbidden",
		ErrorBody:    []byte(`{"errors":["insufficient_scope"]}`),
	})
}

// TestDatadogAuthStates は issue 0165 の状態網羅表の全 9 状態を検証する。
// 各行は「トークンの状態 × 静的キーの有無 × 呼び出し結果」に対して、どの資格情報で
// 呼ぶことになるか、警告ログを出すか、エラーになるかを定める。
func TestDatadogAuthStates(t *testing.T) {
	refreshErr := errors.New("refresh endpoint rejected the refresh token")

	tests := []struct {
		name string

		// 疑似ディスクの初期状態
		token   func(t *testing.T) *datadogauth.TokenSet
		loadErr error
		refresh func(disk *datadogAuthDisk) (*datadogauth.TokenSet, error)

		staticKeys bool
		// callErr は Datadog 呼び出しが最初の 1 回で返すエラー。
		callErr error

		// wantKind は最終的に Datadog を呼んだ資格情報 ("oauth:<token>" / "static")。
		wantKind string
		// wantErr が真なら認証の解決または呼び出しがエラーで終わる。
		wantErr bool
		// wantWarn はログに出るべき警告の部分文字列。空なら警告が無いことを検証する。
		wantWarn string
	}{
		{
			name:     "1. token is valid",
			token:    func(t *testing.T) *datadogauth.TokenSet { return validTestToken(t, "valid-token") },
			wantKind: "oauth:valid-token",
		},
		{
			name:       "2. no token with static keys",
			staticKeys: true,
			wantKind:   "static",
		},
		{
			name:    "3. no token without static keys",
			wantErr: true,
		},
		{
			name:  "4. expired token refreshed successfully",
			token: func(t *testing.T) *datadogauth.TokenSet { return expiredTestToken(t, "stale-token") },
			refresh: func(*datadogAuthDisk) (*datadogauth.TokenSet, error) {
				return validTestToken(t, "refreshed-token"), nil
			},
			wantKind: "oauth:refreshed-token",
		},
		{
			name:       "5. refresh failed with static keys",
			token:      func(t *testing.T) *datadogauth.TokenSet { return expiredTestToken(t, "stale-token") },
			refresh:    func(*datadogAuthDisk) (*datadogauth.TokenSet, error) { return nil, refreshErr },
			staticKeys: true,
			wantKind:   "static",
			wantWarn:   "refresh datadog oauth token failed",
		},
		{
			name:     "6. refresh failed without static keys",
			token:    func(t *testing.T) *datadogauth.TokenSet { return expiredTestToken(t, "stale-token") },
			refresh:  func(*datadogAuthDisk) (*datadogauth.TokenSet, error) { return nil, refreshErr },
			wantErr:  true,
			wantWarn: "refresh datadog oauth token failed",
		},
		{
			name:       "7. corrupt token file with static keys",
			loadErr:    errors.New("parse token_datadoghq.com.json: invalid character 'x'"),
			staticKeys: true,
			wantKind:   "static",
			wantWarn:   "stored datadog oauth token is unusable",
		},
		{
			name:     "8. corrupt token file without static keys",
			loadErr:  errors.New("parse token_datadoghq.com.json: invalid character 'x'"),
			wantErr:  true,
			wantWarn: "stored datadog oauth token is unusable",
		},
		{
			name:       "9. valid token rejected with 403 falls back to static keys",
			token:      func(t *testing.T) *datadogauth.TokenSet { return validTestToken(t, "valid-token") },
			staticKeys: true,
			callErr:    forbiddenDatadogError(),
			wantKind:   "static",
			wantWarn:   "falling back to the static API keys",
		},
		{
			name:     "9. valid token rejected with 403 and no static keys keeps the error",
			token:    func(t *testing.T) *datadogauth.TokenSet { return validTestToken(t, "valid-token") },
			callErr:  forbiddenDatadogError(),
			wantErr:  true,
			wantWarn: "no static keys are available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := captureLogs(t)

			disk := &datadogAuthDisk{loadErr: tt.loadErr, refresh: tt.refresh}
			if tt.token != nil {
				disk.token = tt.token(t)
			}
			s := newDatadogAuthTestServer(t, disk, tt.staticKeys)

			var kinds []string
			call := func(ctx context.Context) (any, error) {
				kinds = append(kinds, datadogAuthKind(t, ctx))
				if len(kinds) == 1 && tt.callErr != nil {
					return nil, tt.callErr
				}
				return "cost", nil
			}

			var got any
			authCtx, err := s.datadogAuthContext(context.Background(), datadogParentOrg)
			if err == nil {
				got, err = s.datadogCall(context.Background(), authCtx, datadogParentOrg, call)
			}

			if tt.wantErr {
				if err == nil {
					t.Fatalf("error = nil, want an error (result=%v)", got)
				}
			} else {
				if err != nil {
					t.Fatalf("error = %v, want nil", err)
				}
				if got != "cost" {
					t.Errorf("result = %v, want %q", got, "cost")
				}
				if last := kinds[len(kinds)-1]; last != tt.wantKind {
					t.Errorf("credentials used = %q, want %q (all=%v)", last, tt.wantKind, kinds)
				}
			}
			assertDatadogWarn(t, logs, tt.wantWarn)
		})
	}
}

// assertDatadogWarn は want が空なら警告が無いこと、空でなければ want を含む警告が
// 出ていることを検証する。
func assertDatadogWarn(t *testing.T, logs *recordingHandler, want string) {
	t.Helper()
	logs.mu.Lock()
	defer logs.mu.Unlock()

	var warns []string
	for _, rec := range logs.records {
		if rec.level == slog.LevelWarn {
			warns = append(warns, rec.msg)
		}
	}
	if want == "" {
		if len(warns) > 0 {
			t.Errorf("warnings = %v, want none", warns)
		}
		return
	}
	for _, msg := range warns {
		if strings.Contains(msg, want) {
			return
		}
	}
	t.Errorf("warnings = %v, want one containing %q", warns, want)
}

// TestDatadogAuthContextNoCredentialsError は資格情報が 1 つも無い場合のエラーが
// ErrDatadogNoCredentials で識別でき、次に取るべき操作を示すことを確認する。
func TestDatadogAuthContextNoCredentialsError(t *testing.T) {
	s := newDatadogAuthTestServer(t, &datadogAuthDisk{}, false)

	_, err := s.datadogAuthContext(context.Background(), datadogParentOrg)
	if !errors.Is(err, ErrDatadogNoCredentials) {
		t.Fatalf("error = %v, want one matching ErrDatadogNoCredentials", err)
	}
	if !strings.Contains(err.Error(), "thief datadog auth login") {
		t.Errorf("error = %q, want it to mention the login command", err)
	}
}

// TestDatadogRefreshIsCollapsedBySingleflight は、同時に走った期限切れトークンの解決が
// トークンエンドポイントへの呼び出し 1 回に集約されることを確認する。
func TestDatadogRefreshIsCollapsedBySingleflight(t *testing.T) {
	const goroutines = 16

	release := make(chan struct{})
	disk := &datadogAuthDisk{token: expiredTestToken(t, "stale-token")}
	disk.refresh = func(*datadogAuthDisk) (*datadogauth.TokenSet, error) {
		// 全 goroutine が singleflight に集まるまでリフレッシュを終わらせない。
		<-release
		return validTestToken(t, "refreshed-token"), nil
	}
	s := newDatadogAuthTestServer(t, disk, false)

	var wg sync.WaitGroup
	kinds := make([]string, goroutines)
	errs := make([]error, goroutines)
	started := make(chan struct{}, goroutines)
	for i := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			started <- struct{}{}
			ctx, err := s.datadogAuthContext(context.Background(), datadogParentOrg)
			if err != nil {
				errs[i] = err
				return
			}
			kinds[i] = datadogAuthKind(t, ctx)
		}()
	}
	for range goroutines {
		<-started
	}
	// singleflight の集約は「実行中の 1 件へ後続が相乗りする」形なので、後続が
	// Do に到達する時間を与えてから解放する。到達が遅れて集約されなかった場合は
	// refreshs が 2 以上になり、下の検証が失敗する。
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: datadogAuthContext error = %v", i, err)
		}
	}
	for i, kind := range kinds {
		if kind != "oauth:refreshed-token" {
			t.Errorf("goroutine %d: credentials = %q, want %q", i, kind, "oauth:refreshed-token")
		}
	}
	if _, _, refreshs := disk.counts(); refreshs != 1 {
		t.Errorf("refresh calls = %d, want 1 (collapsed by singleflight)", refreshs)
	}
}

// TestDatadogRefreshSkippedWhenAnotherProcessAlreadyRefreshed は、リフレッシュを
// 決めた後にディスクを読み直す楽観的リトライを検証する。CLI が先に更新していれば
// トークンエンドポイントを呼ばずに済む。
func TestDatadogRefreshSkippedWhenAnotherProcessAlreadyRefreshed(t *testing.T) {
	disk := &datadogAuthDisk{token: expiredTestToken(t, "stale-token")}
	s := newDatadogAuthTestServer(t, disk, false)

	// datadogAuthContext が最初の読み取りで期限切れを見た後、リフレッシュ直前の
	// 読み直しの前に他プロセスが更新した状況を作る。
	loads := 0
	base := s.ddAuth.loadToken
	s.ddAuth.loadToken = func(site, org string) (*datadogauth.TokenSet, bool, error) {
		loads++
		if loads == 2 {
			disk.mu.Lock()
			disk.token = validTestToken(t, "token-from-the-cli")
			disk.mu.Unlock()
		}
		return base(site, org)
	}

	ctx, err := s.datadogAuthContext(context.Background(), datadogParentOrg)
	if err != nil {
		t.Fatalf("datadogAuthContext error = %v", err)
	}
	if got := datadogAuthKind(t, ctx); got != "oauth:token-from-the-cli" {
		t.Errorf("credentials = %q, want %q", got, "oauth:token-from-the-cli")
	}
	if _, saves, refreshs := disk.counts(); refreshs != 0 || saves != 0 {
		t.Errorf("refresh calls = %d, saves = %d, want 0 and 0 (another process had already refreshed)", refreshs, saves)
	}
}

// TestDatadogRefreshRecoversFromConcurrentRotation は、失敗を確定させる前の読み直しを
// 検証する。リフレッシュトークンをローテーションする認可サーバでは、他プロセスの成功が
// こちらの失敗の原因になる。その場合に「再ログインが必要」と誤診断しない。
func TestDatadogRefreshRecoversFromConcurrentRotation(t *testing.T) {
	disk := &datadogAuthDisk{token: expiredTestToken(t, "stale-token")}
	disk.refresh = func(d *datadogAuthDisk) (*datadogauth.TokenSet, error) {
		// 他プロセスが先に更新を終えて、こちらのリフレッシュトークンが失効した状況。
		d.mu.Lock()
		d.token = validTestToken(t, "token-from-the-cli")
		d.mu.Unlock()
		return nil, errors.New("invalid_grant: refresh token is no longer valid")
	}
	s := newDatadogAuthTestServer(t, disk, false)
	logs := captureLogs(t)

	ctx, err := s.datadogAuthContext(context.Background(), datadogParentOrg)
	if err != nil {
		t.Fatalf("datadogAuthContext error = %v", err)
	}
	if got := datadogAuthKind(t, ctx); got != "oauth:token-from-the-cli" {
		t.Errorf("credentials = %q, want %q", got, "oauth:token-from-the-cli")
	}
	assertDatadogWarn(t, logs, "")
}

// TestDatadogRefreshSavesRotatedToken は、更新したトークンが保存され、応答が
// refresh_token を省いた場合に元の値を引き継ぐことを確認する。保存しないと次の
// リクエストが毎回リフレッシュを走らせることになる。
func TestDatadogRefreshSavesRotatedToken(t *testing.T) {
	disk := &datadogAuthDisk{token: expiredTestToken(t, "stale-token")}
	disk.refresh = func(*datadogAuthDisk) (*datadogauth.TokenSet, error) {
		return newTestDatadogToken(t, "refreshed-token", "", 3600), nil
	}
	s := newDatadogAuthTestServer(t, disk, false)

	if _, err := s.datadogAuthContext(context.Background(), datadogParentOrg); err != nil {
		t.Fatalf("datadogAuthContext error = %v", err)
	}

	disk.mu.Lock()
	saved := disk.token
	disk.mu.Unlock()
	if saved.AccessTokenValue() != "refreshed-token" {
		t.Errorf("saved access token = %q, want %q", saved.AccessTokenValue(), "refreshed-token")
	}
	if saved.RefreshTokenValue() != "refresh-token" {
		t.Errorf("saved refresh token = %q, want the previous value %q", saved.RefreshTokenValue(), "refresh-token")
	}

	// 2 回目の解決はリフレッシュを走らせない (保存済みのトークンが期限内なので)。
	if _, err := s.datadogAuthContext(context.Background(), datadogParentOrg); err != nil {
		t.Fatalf("second datadogAuthContext error = %v", err)
	}
	if _, _, refreshs := disk.counts(); refreshs != 1 {
		t.Errorf("refresh calls = %d, want 1", refreshs)
	}
}

// TestDatadogRefreshWithoutRefreshToken はリフレッシュトークンを持たない期限切れの
// トークンが、トークンエンドポイントを呼ばずに再ログインを促すことを確認する。
func TestDatadogRefreshWithoutRefreshToken(t *testing.T) {
	disk := &datadogAuthDisk{token: newTestDatadogToken(t, "stale-token", "", 1)}
	s := newDatadogAuthTestServer(t, disk, false)
	logs := captureLogs(t)

	_, err := s.datadogAuthContext(context.Background(), datadogParentOrg)
	if !errors.Is(err, ErrDatadogNoCredentials) {
		t.Fatalf("error = %v, want one matching ErrDatadogNoCredentials", err)
	}
	if _, _, refreshs := disk.counts(); refreshs != 0 {
		t.Errorf("refresh calls = %d, want 0", refreshs)
	}
	assertDatadogWarn(t, logs, "refresh datadog oauth token failed")
}

// TestDatadogCallKeepsNonForbiddenErrors は、権限不足以外の失敗で静的キーへ倒さない
// ことを確認する。倒すと、静的キーでも同じ理由で失敗する呼び出しを 2 回投げるだけになる。
func TestDatadogCallKeepsNonForbiddenErrors(t *testing.T) {
	disk := &datadogAuthDisk{token: validTestToken(t, "valid-token")}
	s := newDatadogAuthTestServer(t, disk, true)

	callErr := fmt.Errorf("get datadog historical cost: %w", ddapi.GenericOpenAPIError{ErrorMessage: "500 Internal Server Error"})
	calls := 0
	authCtx, err := s.datadogAuthContext(context.Background(), datadogParentOrg)
	if err != nil {
		t.Fatalf("datadogAuthContext error = %v", err)
	}
	_, err = s.datadogCall(context.Background(), authCtx, datadogParentOrg, func(context.Context) (any, error) {
		calls++
		return nil, callErr
	})
	if !errors.Is(err, callErr) {
		t.Errorf("error = %v, want %v", err, callErr)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (no static key retry)", calls)
	}
}

// TestDatadogCallDoesNotRetryStaticKeyForbidden は、静的キーで呼んで 403 になった場合に
// 再試行しないことを確認する (倒す先が無い)。
func TestDatadogCallDoesNotRetryStaticKeyForbidden(t *testing.T) {
	s := newDatadogAuthTestServer(t, &datadogAuthDisk{}, true)

	calls := 0
	authCtx, err := s.datadogAuthContext(context.Background(), datadogParentOrg)
	if err != nil {
		t.Fatalf("datadogAuthContext error = %v", err)
	}
	_, err = s.datadogCall(context.Background(), authCtx, datadogParentOrg, func(context.Context) (any, error) {
		calls++
		return nil, forbiddenDatadogError()
	})
	if err == nil {
		t.Fatal("error = nil, want the 403 error")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

// TestDatadogAuthContextSubOrgDoesNotFallBackToStaticKeys は、静的 API キーが
// Sub Organization の代わりにならないことを状態ごとに検証する。静的キーは発行元の
// 組織でしか読めず、Sub Organization を指定した要求に使うと、黙って親組織のデータを
// 返すことになる。親組織 (org == "") の行は、この変更で従来の挙動が変わらないことの対照。
func TestDatadogAuthContextSubOrgDoesNotFallBackToStaticKeys(t *testing.T) {
	const subOrg = "suborg1"

	tests := []struct {
		name string

		org     string
		token   func(t *testing.T) *datadogauth.TokenSet
		loadErr error
		refresh func(disk *datadogAuthDisk) (*datadogauth.TokenSet, error)

		// wantKind は資格情報の解決に成功した場合に運ばれる資格情報。
		wantKind string
		// wantErr が真なら ErrDatadogNoCredentials で終わる。
		wantErr bool
		// wantHint はエラーメッセージに含まれるべき文字列。
		wantHint string
	}{
		{
			name:     "parent org without a token falls back to the static keys",
			token:    nil,
			wantKind: "static",
		},
		{
			name:     "sub org without a token does not fall back",
			org:      subOrg,
			wantErr:  true,
			wantHint: "thief datadog auth login --org suborg1",
		},
		{
			name:     "sub org with a valid token uses it",
			org:      subOrg,
			token:    func(t *testing.T) *datadogauth.TokenSet { return validTestToken(t, "suborg-token") },
			wantKind: "oauth:suborg-token",
		},
		{
			name:     "parent org with a broken token file falls back to the static keys",
			loadErr:  errors.New("parse token_datadoghq.com.json: invalid character 'x'"),
			wantKind: "static",
		},
		{
			name:     "sub org with a broken token file does not fall back",
			org:      subOrg,
			loadErr:  errors.New("parse token_datadoghq.com_suborg1.json: invalid character 'x'"),
			wantErr:  true,
			wantHint: "log in to Datadog again",
		},
		{
			name:     "parent org whose refresh failed falls back to the static keys",
			token:    func(t *testing.T) *datadogauth.TokenSet { return expiredTestToken(t, "stale-token") },
			refresh:  func(*datadogAuthDisk) (*datadogauth.TokenSet, error) { return nil, errors.New("invalid_grant") },
			wantKind: "static",
		},
		{
			name:     "sub org whose refresh failed does not fall back",
			org:      subOrg,
			token:    func(t *testing.T) *datadogauth.TokenSet { return expiredTestToken(t, "stale-token") },
			refresh:  func(*datadogAuthDisk) (*datadogauth.TokenSet, error) { return nil, errors.New("invalid_grant") },
			wantErr:  true,
			wantHint: "could not be refreshed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			disk := &datadogAuthDisk{loadErr: tt.loadErr, refresh: tt.refresh}
			if tt.token != nil {
				disk.token = tt.token(t)
			}
			// 静的キーは常に用意しておき、org の違いだけで挙動が変わることを見る。
			s := newDatadogAuthTestServer(t, disk, true)

			ctx, err := s.datadogAuthContext(context.Background(), tt.org)
			if tt.wantErr {
				if !errors.Is(err, ErrDatadogNoCredentials) {
					t.Fatalf("error = %v, want one matching ErrDatadogNoCredentials", err)
				}
				if !strings.Contains(err.Error(), tt.org) {
					t.Errorf("error = %q, want it to mention the organization %q", err, tt.org)
				}
				if !strings.Contains(err.Error(), tt.wantHint) {
					t.Errorf("error = %q, want it to contain %q", err, tt.wantHint)
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v, want nil", err)
			}
			if got := datadogAuthKind(t, ctx); got != tt.wantKind {
				t.Errorf("credentials = %q, want %q", got, tt.wantKind)
			}
			if got := disk.seenOrgs(); len(got) != 1 || got[0] != tt.org {
				t.Errorf("orgs passed to the storage = %v, want only %q", got, tt.org)
			}
		})
	}
}

// TestDatadogCallSubOrgForbiddenDoesNotRetryWithStaticKeys は、Sub Organization への
// 呼び出しが 403 で拒否されたときに静的キーで投げ直さないことを確認する。投げ直すと
// 親組織のデータが Sub Organization の結果として返る。
func TestDatadogCallSubOrgForbiddenDoesNotRetryWithStaticKeys(t *testing.T) {
	const subOrg = "suborg1"

	disk := &datadogAuthDisk{token: validTestToken(t, "suborg-token")}
	s := newDatadogAuthTestServer(t, disk, true)
	logs := captureLogs(t)

	var kinds []string
	authCtx, err := s.datadogAuthContext(context.Background(), subOrg)
	if err != nil {
		t.Fatalf("datadogAuthContext error = %v", err)
	}
	_, err = s.datadogCall(context.Background(), authCtx, subOrg, func(ctx context.Context) (any, error) {
		kinds = append(kinds, datadogAuthKind(t, ctx))
		return nil, forbiddenDatadogError()
	})
	if err == nil {
		t.Fatal("error = nil, want the 403 error")
	}
	want := []string{"oauth:suborg-token"}
	if len(kinds) != 1 || kinds[0] != want[0] {
		t.Errorf("credentials used = %v, want %v (no static key retry)", kinds, want)
	}
	assertDatadogWarn(t, logs, "the static API keys are not used as a fallback")
}

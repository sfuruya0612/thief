package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/datadogauth"
	"github.com/spf13/cobra"
)

const (
	// datadogAuthCallbackAddr は CLI のログインコールバックを待ち受ける固定の
	// ループバックアドレス。ポートを実行のたびに変えると redirect_uri が変わり、
	// Dynamic Client Registration の登録済みクライアントを再利用できなくなる
	// (Datadog 側が RFC 8252 §7.3 のポート一致の緩和をサポートするかは未確認)。
	// 使用中の場合は別ポートへ黙って移らず、明確なエラーで失敗させる。
	datadogAuthCallbackAddr = "127.0.0.1:8400"
	// datadogAuthCallbackPath は CLI のコールバックのパス。
	datadogAuthCallbackPath = "/callback"
	// datadogCLIRedirectURI は CLI 用の redirect_uri。
	datadogCLIRedirectURI = "http://" + datadogAuthCallbackAddr + datadogAuthCallbackPath
	// datadogAuthLoginTimeout はログイン 1 回の上限。ブラウザでの承認を待つ間に
	// 無期限で止まらないようにする。
	datadogAuthLoginTimeout = 5 * time.Minute
	// datadogAuthShutdownTimeout はコールバック受理後にローカルサーバを閉じる上限。
	datadogAuthShutdownTimeout = 5 * time.Second
)

// datadogServerRedirectURI は API サーバ (issue 0165) 用の redirect_uri。
// Dynamic Client Registration の初回登録で CLI 用と一緒に登録する。
func datadogServerRedirectURI() string {
	return config.DefaultDatadogOAuthRedirectBase + config.DatadogOAuthCallbackPath
}

// datadogAuthDeps は datadog auth の 3 つのサブコマンドが呼ぶ外部処理をまとめる。
// prepareLogin / completeLogin / ensureFreshToken / logout は Datadog への接続と
// config.Dir() 配下への読み書きを、openBrowser はブラウザの起動を、listen と
// serveCallback はループバックポートの待ち受けを、readPastedCode は標準入力の
// 読み取りを伴い、いずれもテストからは実行できないため差し替える。
type datadogAuthDeps struct {
	prepareLogin     func(ctx context.Context, p datadogauth.PrepareParams) (*datadogauth.Login, error)
	completeLogin    func(ctx context.Context, login *datadogauth.Login, state, code string) (*datadogauth.TokenSet, error)
	ensureFreshToken func(ctx context.Context, site string) (*datadogauth.TokenSet, bool, error)
	logout           func(site string) error
	openBrowser      func(url string) error
	listen           func(addr string) (net.Listener, error)
	serveCallback    func(ctx context.Context, ln net.Listener) (state, code string, err error)
	readPastedCode   func(ctx context.Context) (string, error)
}

// defaultDatadogAuthDeps は本番で使う実装を返す。標準入力と標準出力はコマンドから
// 取り、テストで差し替えられる経路 (cmd.InOrStdin / cmd.OutOrStdout) を保つ。
func defaultDatadogAuthDeps(cmd *cobra.Command) datadogAuthDeps {
	authDeps := datadogauth.DefaultDeps()
	return datadogAuthDeps{
		prepareLogin: func(ctx context.Context, p datadogauth.PrepareParams) (*datadogauth.Login, error) {
			return datadogauth.PrepareLogin(ctx, p, authDeps)
		},
		completeLogin: func(ctx context.Context, login *datadogauth.Login, state, code string) (*datadogauth.TokenSet, error) {
			return datadogauth.CompleteLogin(ctx, login, state, code, authDeps)
		},
		ensureFreshToken: func(ctx context.Context, site string) (*datadogauth.TokenSet, bool, error) {
			return datadogauth.EnsureFreshToken(ctx, site, authDeps)
		},
		logout: func(site string) error {
			return datadogauth.Logout(site, authDeps)
		},
		openBrowser:   openBrowser,
		listen:        func(addr string) (net.Listener, error) { return net.Listen("tcp", addr) },
		serveCallback: serveDatadogAuthCallback,
		readPastedCode: func(ctx context.Context) (string, error) {
			return readDatadogAuthCode(ctx, cmd)
		},
	}
}

// newDatadogAuthCmd は `thief datadog auth` のコマンド木を返す。
func newDatadogAuthCmd() *cobra.Command {
	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage Datadog OAuth 2.0 login",
		Long:  "Log in to Datadog with OAuth 2.0 (Authorization Code + PKCE + Dynamic Client Registration), refresh the stored token, or remove it.",
	}

	loginCmd := &cobra.Command{
		Use:   "login",
		Short: "Log in to Datadog with OAuth 2.0",
		Long:  "Open the Datadog authorization page in a browser and store the issued tokens under the thief config directory with file mode 0600.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return datadogAuthLoginWith(cmd, defaultDatadogAuthDeps(cmd))
		},
	}

	logoutCmd := &cobra.Command{
		Use:   "logout",
		Short: "Remove the locally stored Datadog OAuth credentials",
		Long:  "Remove the locally stored Datadog OAuth token and client registration. Datadog exposes no token revocation endpoint, so only the local files are removed.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return datadogAuthLogoutWith(cmd, defaultDatadogAuthDeps(cmd))
		},
	}

	refreshCmd := &cobra.Command{
		Use:   "refresh",
		Short: "Refresh the stored Datadog OAuth token",
		Long:  "Refresh the stored Datadog OAuth access token when it is expired or about to expire, and show the resulting expiry.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return datadogAuthRefreshWith(cmd, defaultDatadogAuthDeps(cmd))
		},
	}

	authCmd.AddCommand(loginCmd, logoutCmd, refreshCmd)
	return authCmd
}

// datadogAuthLoginWith は `thief datadog auth login` の本体。
//
// 手順は次のとおり。ループバックの待ち受けを Dynamic Client Registration より先に
// 行うのは、ポートが使えないときにクライアント登録という副作用を残さないためである。
//
//  1. 固定ポートで待ち受ける (使用中なら別ポートへ移らずエラー)
//  2. 認可の準備 (クライアント登録の再利用または初回登録、PKCE と state の生成)
//  3. 認可 URL を提示し、ブラウザの起動を試みる
//  4. コールバック、またはブラウザを開けなかった場合は標準入力から認可コードを受け取る
//  5. state を検証してトークンへ引き換え、保存する
func datadogAuthLoginWith(cmd *cobra.Command, deps datadogAuthDeps) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(commandContext(cmd), datadogAuthLoginTimeout)
	defer cancel()

	ln, err := deps.listen(datadogAuthCallbackAddr)
	if err != nil {
		// 別ポートへ黙ってフォールバックしない。登録済みの redirect_uri と食い違う
		// ポートで待ち受けても、認可サーバがコールバックを拒否するだけである。
		return fmt.Errorf("listen on %s for the datadog oauth callback: %w", datadogAuthCallbackAddr, err)
	}
	defer ln.Close()

	login, err := deps.prepareLogin(ctx, datadogauth.PrepareParams{
		Site:                 cfg.Datadog.Site,
		RedirectURI:          datadogCLIRedirectURI,
		RegisterRedirectURIs: []string{datadogCLIRedirectURI, datadogServerRedirectURI()},
	})
	if err != nil {
		return err
	}

	cmd.Println("Attempting to automatically open the Datadog authorization page in your default browser.")
	cmd.Println("If the browser does not open, open the following URL:")
	cmd.Println()
	cmd.Println(login.AuthorizationURL)
	cmd.Println()

	// ブラウザの起動の失敗はフローを止める理由にならない。URL は既に提示済みであり、
	// 別の端末のブラウザで承認して認可コードを貼り付ける経路が残っている。
	allowPaste := false
	if err := deps.openBrowser(login.AuthorizationURL); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: open browser: %v; open the URL shown above manually to continue.\n", err)
		allowPaste = true
	}

	state, code, err := awaitDatadogAuthCode(ctx, cmd, deps, ln, login, allowPaste)
	if err != nil {
		return err
	}

	if _, err := deps.completeLogin(ctx, login, state, code); err != nil {
		return err
	}

	cmd.Printf("Successfully logged into Datadog site: %s\n", login.Site)
	return nil
}

// awaitDatadogAuthCode は認可コードを受け取る。ループバックのコールバックを待ち、
// allowPaste が真の場合は標準入力への貼り付けも並行して待って、早い方を採用する。
//
// ブラウザを自動起動できなかった環境では、利用者が別の端末で承認してこの端末の
// ループバックへ到達できないことがある。その場合の退路として貼り付けを受け付けるが、
// 同じ端末のブラウザを手動で開いた場合はコールバックが届くので、どちらも待つ。
func awaitDatadogAuthCode(ctx context.Context, cmd *cobra.Command, deps datadogAuthDeps, ln net.Listener, login *datadogauth.Login, allowPaste bool) (string, string, error) {
	type result struct {
		state string
		code  string
		err   error
	}

	callbackCh := make(chan result, 1)
	go func() {
		state, code, err := deps.serveCallback(ctx, ln)
		callbackCh <- result{state: state, code: code, err: err}
	}()

	pasteCh := make(chan result, 1)
	if allowPaste {
		cmd.Println("After approving the request, paste the authorization code (or the full redirect URL) here and press Enter:")
		go func() {
			line, err := deps.readPastedCode(ctx)
			if err != nil {
				pasteCh <- result{err: err}
				return
			}
			code, state, err := parseDatadogAuthCodeInput(line)
			if err != nil {
				pasteCh <- result{err: err}
				return
			}
			// 認可コードだけを貼り付けた場合、state は帯域外で運ばれたことになる。
			// 検証する相手が無いので、こちらが生成した値をそのまま使う。
			if state == "" {
				state = login.State
			}
			pasteCh <- result{state: state, code: code}
		}()
	}

	select {
	case r := <-callbackCh:
		return r.state, r.code, r.err
	case r := <-pasteCh:
		return r.state, r.code, r.err
	case <-ctx.Done():
		return "", "", ctx.Err()
	}
}

// serveDatadogAuthCallback は ln で 1 回だけコールバックを受理し、state と code を返す。
// 受理した時点でサーバを閉じる。
func serveDatadogAuthCallback(ctx context.Context, ln net.Listener) (string, string, error) {
	type result struct {
		state string
		code  string
		err   error
	}
	done := make(chan result, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("GET "+datadogAuthCallbackPath, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		// RFC 6749 §4.1.2.1: 認可が拒否された場合は error / error_description が返る。
		if e := q.Get("error"); e != "" {
			http.Error(w, "Authorization failed. Return to the terminal.", http.StatusBadRequest)
			send(done, result{err: authorizationError(e, q.Get("error_description"))})
			return
		}
		code, state := q.Get("code"), q.Get("state")
		if code == "" {
			http.Error(w, "Authorization callback has no code. Return to the terminal.", http.StatusBadRequest)
			send(done, result{err: errors.New("datadog oauth callback has no authorization code")})
			return
		}
		fmt.Fprintln(w, "Login successful. You can close this window and return to the terminal.")
		send(done, result{state: state, code: code})
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	defer func() {
		// ctx が既にキャンセルされていても応答を返し切ってから閉じたいので、
		// キャンセルを引き継がない context に期限だけを掛ける。
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), datadogAuthShutdownTimeout)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	select {
	case r := <-done:
		return r.state, r.code, r.err
	case err := <-serveErr:
		return "", "", fmt.Errorf("serve datadog oauth callback: %w", err)
	case <-ctx.Done():
		return "", "", ctx.Err()
	}
}

// send はバッファ 1 のチャネルへ非ブロッキングで送る。1 回目のコールバックだけを
// 採用し、2 回目以降のリクエストでハンドラが送信のまま残らないようにする。
func send[T any](ch chan T, v T) {
	select {
	case ch <- v:
	default:
	}
}

// authorizationError は認可サーバが返した error / error_description をエラーにする。
func authorizationError(code, description string) error {
	if description == "" {
		return fmt.Errorf("datadog authorization failed: %s", code)
	}
	return fmt.Errorf("datadog authorization failed: %s: %s", code, description)
}

// readDatadogAuthCode は標準入力から 1 行読む。ctx がキャンセルされたら入力を待たずに
// 戻る (readWithContext を参照)。
func readDatadogAuthCode(ctx context.Context, cmd *cobra.Command) (string, error) {
	r := bufio.NewReader(cmd.InOrStdin())
	return readWithContext(ctx, func() (string, error) {
		line, err := r.ReadString('\n')
		// EOF は改行の前に読めていた分を入力として扱う (promptSelection と同じ)。
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("read authorization code: %w", err)
		}
		return strings.TrimSpace(line), nil
	})
}

// parseDatadogAuthCodeInput は貼り付けられた 1 行から認可コードを取り出す。
//
// ブラウザのアドレスバーからリダイレクト先の URL 全体を貼り付けた場合は、そこから
// code と state を取り出す (error パラメータがあればエラーにする)。それ以外は行全体を
// 認可コードとして扱い、state は空で返す。
func parseDatadogAuthCodeInput(line string) (code, state string, err error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", "", errors.New("authorization code is empty")
	}
	if !strings.HasPrefix(line, "http://") && !strings.HasPrefix(line, "https://") {
		return line, "", nil
	}
	u, perr := url.Parse(line)
	if perr != nil {
		return "", "", fmt.Errorf("parse pasted redirect URL: %w", perr)
	}
	q := u.Query()
	if e := q.Get("error"); e != "" {
		return "", "", authorizationError(e, q.Get("error_description"))
	}
	code = q.Get("code")
	if code == "" {
		return "", "", errors.New("pasted redirect URL has no code parameter")
	}
	return code, q.Get("state"), nil
}

// datadogAuthLogoutWith は `thief datadog auth logout` の本体。
func datadogAuthLogoutWith(cmd *cobra.Command, deps datadogAuthDeps) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	if err := deps.logout(cfg.Datadog.Site); err != nil {
		return err
	}
	cmd.Printf("Removed the local Datadog OAuth credentials for site: %s\n", cfg.Datadog.Site)
	return nil
}

// datadogAuthRefreshWith は `thief datadog auth refresh` の本体。
// 期限切れ (5 分前からの早期バッファを含む) のときだけ実際の更新が走る。
func datadogAuthRefreshWith(cmd *cobra.Command, deps datadogAuthDeps) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	tok, ok, err := deps.ensureFreshToken(commandContext(cmd), cfg.Datadog.Site)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no Datadog OAuth token found for site %s. Run 'thief datadog auth login' first", cfg.Datadog.Site)
	}
	expiresAt := tok.ExpiresAt()
	if expiresAt.IsZero() {
		cmd.Printf("Datadog OAuth token for site %s is available (expiry unknown)\n", cfg.Datadog.Site)
		return nil
	}
	cmd.Printf("Datadog OAuth token for site %s is valid until %s\n", cfg.Datadog.Site, expiresAt.UTC().Format(time.RFC3339))
	return nil
}

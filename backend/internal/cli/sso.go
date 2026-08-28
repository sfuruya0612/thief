package cli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/template"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/ssoauth"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

const ssoProfileTemplate = `
[profile {{.Name}}]
sso_start_url = {{.StartUrl}}
sso_region = {{.Region}}
sso_account_id = {{.AccountId}}
sso_role_name = {{.RoleName}}
region = {{.Region}}
`

// ProfileConfig は generate-config が ~/.aws/config に追記するプロファイル定義。
type ProfileConfig struct {
	Name      string
	StartUrl  string
	Region    string
	AccountId string
	RoleName  string
}

var ssoAccountColumns = []util.Column{
	{Header: "ID"},
	{Header: "Name"},
	{Header: "Email"},
	{Header: "Roles"},
}

func newSSOCmd() *cobra.Command {
	ssoCmd := &cobra.Command{
		Use:   "sso",
		Short: "Manage SSO",
	}

	loginCmd := &cobra.Command{
		Use:   "login",
		Short: "Login to SSO",
		Long:  "Authenticate with AWS SSO to obtain access credentials for AWS services.",
		RunE:  ssoLogin,
	}
	loginCmd.Flags().StringP("url", "", "", "AWS access portal URL")

	logoutCmd := &cobra.Command{
		Use:   "logout",
		Short: "Logout from SSO. Revoke sessions on AWS and remove all cache files.",
		Long:  "Sign out of AWS SSO: revoke every cached access token on AWS (sso:Logout), then remove all cached credentials and tokens.",
		RunE:  ssoLogout,
	}

	generateConfigCmd := &cobra.Command{
		Use:   "generate-config",
		Short: "Generate `~/.aws/config` file for the AWS CLI that uses the SSO profile.",
		RunE:  ssoGenerateConfig,
	}
	generateConfigCmd.Flags().StringP("url", "", "", "AWS access portal URL")

	// backend 専用: キャッシュ済みトークンでアクセス可能なアカウント一覧を表示する。
	lsCmd := &cobra.Command{
		Use:   "ls",
		Short: "List SSO accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, ListConfig[awsinternal.SSOAccountResource]{
				Columns:  ssoAccountColumns,
				EmptyMsg: "No SSO accounts found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.SSOAccountResource, error) {
					return awsinternal.ListSSOAccounts(ctx, cfg.Profile, cfg.Region)
				},
			})
		},
	}

	ssoCmd.AddCommand(loginCmd, logoutCmd, generateConfigCmd, lsCmd)
	return ssoCmd
}

// ssoLogin authenticates with AWS SSO and caches the credentials.
func ssoLogin(cmd *cobra.Command, args []string) error {
	return ssoLoginWith(cmd, defaultSSOTokenDeps(ssoauth.DefaultDeps()))
}

// ssoLoginWith は ssoLogin の本体。
// デバイス認可フロー (開始、表示とブラウザ起動、トークン待機とキャッシュ保存) は
// getSSOTokenWith が internal/ssoauth の 2 つの公開関数を合成して行う。
// トークン取得へ渡す context はコマンドから取る。デバイス認可フローはユーザが
// ブラウザで承認するまで待つため、Ctrl-C がここへ届かないと待ち続ける。
func ssoLoginWith(cmd *cobra.Command, deps ssoTokenDeps) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	region := cfg.Region
	url := cmd.Flag("url").Value.String()

	if url == "" {
		return fmt.Errorf("please specify the AWS SSO access portal URL with --url flag")
	}

	startUrl := fmt.Sprintf("https://%s.awsapps.com/start/", url)

	// トークンの取得と ~/.aws/sso/cache へのキャッシュ保存は ssoauth.Wait の中で行われる。
	if _, err := getSSOTokenWith(commandContext(cmd), region, startUrl, deps); err != nil {
		return fmt.Errorf("get token: %w", err)
	}

	// aws sso login コマンドと同じ出力にする。
	cmd.Printf("Successfully logged into Start URL: %s\n", startUrl)
	return nil
}

// ssoLogout revokes every cached SSO access token on AWS and removes all SSO cache files.
func ssoLogout(cmd *cobra.Command, args []string) error {
	return ssoLogoutWith(cmd, ssoLogoutDeps{
		logoutAll: func(ctx context.Context) (ssoauth.LogoutResult, error) {
			return ssoauth.LogoutAll(ctx, ssoauth.DefaultDeps())
		},
	})
}

// ssoLogoutDeps は ssoLogout が呼ぶ外部処理をまとめる。logoutAll は AWS への通信と
// ~/.aws/sso/cache の削除を伴い、テストから実行できないため差し替える。
type ssoLogoutDeps struct {
	logoutAll func(ctx context.Context) (ssoauth.LogoutResult, error)
}

// ssoLogoutWith は ssoLogout の本体。失効はトークンごとに AWS への往復を伴うため、
// Ctrl-C が届くようコマンドの context を渡す。失効に失敗したトークンがあっても
// ローカルの削除は行われるので、警告を stderr に出して正常終了する。キャッシュの
// 列挙または削除に失敗した場合だけエラーを返す (その場合も失効の失敗があれば先に
// 警告を出す)。キャッシュディレクトリが無い場合は削除するものが無いので正常終了する。
func ssoLogoutWith(cmd *cobra.Command, deps ssoLogoutDeps) error {
	result, err := deps.logoutAll(commandContext(cmd))
	if n := len(result.RevokeFailed); n > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to revoke %d SSO session(s) on AWS; the sign-in session may remain valid until it expires\n", n)
	}
	if err != nil {
		// LogoutAll が返すのはキャッシュディレクトリの解決、列挙、削除のいずれかの失敗で、
		// 削除の失敗は対象ファイル名まで含む。この層から足せる情報が無いため包み直さず、
		// そのまま伝播させる。
		return err
	}

	cmd.Println("Successfully signed out of all SSO profiles.")
	return nil
}

// ssoGenerateConfigDeps は ssoGenerateConfig が呼ぶ外部処理をまとめる。
// getToken はブラウザの起動と AWS への往復を伴い、listAccounts / listRoles は AWS への
// 通信を伴い、configPath / readConfig / writeConfig は $HOME/.aws/config を読み書きする。
// いずれもテストからは実行できないため、アカウント選択・ロール取得・既存設定の
// 読み取り失敗時のフォールバックといった分岐を検証するために差し替える。
type ssoGenerateConfigDeps struct {
	getToken     func(ctx context.Context, region, url string) (*ssoauth.TokenCache, error)
	listAccounts func(ctx context.Context, region, accessToken string) ([]awsinternal.SSOAccountInfo, error)
	listRoles    func(ctx context.Context, region, accessToken, accountID string) ([]string, error)
	configPath   func() (string, error)
	readConfig   func(path string) (string, error)
	writeConfig  func(path, content string) error
}

// defaultSSOGenerateConfigDeps は本番で使う実装を返す。
func defaultSSOGenerateConfigDeps() ssoGenerateConfigDeps {
	return ssoGenerateConfigDeps{
		getToken:     getSSOTokenWithoutSaving,
		listAccounts: awsinternal.ListSSOAccountInfos,
		listRoles:    awsinternal.ListSSOAccountRoleNames,
		configPath:   getAwsConfigPath,
		readConfig:   readAwsConfig,
		writeConfig: func(path, content string) error {
			return os.WriteFile(path, []byte(content), 0600)
		},
	}
}

func ssoGenerateConfig(cmd *cobra.Command, args []string) error {
	return ssoGenerateConfigWith(cmd, args, defaultSSOGenerateConfigDeps())
}

// ssoGenerateConfigWith は ssoGenerateConfig の本体。
func ssoGenerateConfigWith(cmd *cobra.Command, args []string, deps ssoGenerateConfigDeps) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	region := cfg.Region
	url := cmd.Flag("url").Value.String()

	if url == "" {
		return fmt.Errorf("please specify the AWS SSO access portal URL with --url flag")
	}

	startUrl := fmt.Sprintf("https://%s.awsapps.com/start/", url)

	ctx := commandContext(cmd)
	cache, err := deps.getToken(ctx, region, startUrl)
	if err != nil {
		return fmt.Errorf("get token: %w", err)
	}

	accounts, err := deps.listAccounts(ctx, region, cache.AccessToken)
	if err != nil {
		return fmt.Errorf("list accounts: %w", err)
	}

	cmd.Printf("Found %d accounts from AWS SSO\n", len(accounts))

	// アカウント選択とロール選択の両方で使い回す。呼ぶたびに bufio.Reader を作り直すと、
	// 先読みされた分が使い捨てられた Reader のバッファに閉じ込められたまま失われる。
	// *bufio.Reader はスレッドセーフではないため、promptSelection がエラーを返したら
	// 以後この stdin へ読み取りを仕掛けてはならない (下の 2 箇所とも直後に return している
	// のはこの前提を保つため)。
	stdin := bufio.NewReader(cmd.InOrStdin())

	// 選択肢としてアカウントを表示する。
	cmd.Println("\nAvailable AWS accounts:")
	for i, account := range accounts {
		cmd.Printf("[%d] %s (%s)\n", i+1, account.AccountName, account.AccountID)
	}

	// 対話式のアカウント選択。
	cmd.Print("\nSelect accounts to configure (comma-separated numbers, or 'all' for all accounts): ")
	accountInput, err := promptSelection(ctx, stdin)
	if err != nil {
		return err
	}

	accountsToProcess := selectIndices(cmd, accountInput, len(accounts), "account")
	if len(accountsToProcess) == 0 {
		return fmt.Errorf("no valid accounts selected")
	}

	// 選択されたアカウントのロールを取得しプロファイルを作成する。
	profiles := make([]ProfileConfig, 0)
	for _, accountIndex := range accountsToProcess {
		account := accounts[accountIndex]
		cmd.Printf("\nProcessing account %s (%s)...\n", account.AccountName, account.AccountID)

		roles, err := deps.listRoles(ctx, region, cache.AccessToken, account.AccountID)
		if err != nil {
			return fmt.Errorf("list account roles for %s: %w", account.AccountID, err)
		}

		if len(roles) == 0 {
			cmd.Printf("No roles found for account %s (%s)\n", account.AccountName, account.AccountID)
			continue
		}

		// アカウントごとに利用可能なロールを表示する。
		cmd.Printf("Available roles for %s:\n", account.AccountName)
		for i, role := range roles {
			cmd.Printf("[%d] %s\n", i+1, role)
		}

		// 対話式のロール選択。
		cmd.Print("Select roles to configure (comma-separated numbers, or 'all' for all roles): ")
		roleInput, err := promptSelection(ctx, stdin)
		if err != nil {
			return err
		}

		selectedRoles := selectIndices(cmd, roleInput, len(roles), "role")

		// 選択されたロールのプロファイルを作成する。
		for _, roleIndex := range selectedRoles {
			role := roles[roleIndex]
			profileName := fmt.Sprintf("%s-%s", account.AccountName, role)
			profileName = strings.ToLower(strings.ReplaceAll(profileName, " ", "-"))

			profiles = append(profiles, ProfileConfig{
				Name:      profileName,
				StartUrl:  startUrl,
				Region:    region,
				AccountId: account.AccountID,
				RoleName:  role,
			})
		}
	}

	if len(profiles) == 0 {
		return fmt.Errorf("no roles selected for any accounts")
	}

	cmd.Printf("\nFound %d role configurations to add\n", len(profiles))

	// 既存の設定を読み込む。
	configPath, err := deps.configPath()
	if err != nil {
		return fmt.Errorf("get AWS config path: %w", err)
	}

	existingConfig, err := deps.readConfig(configPath)
	if err != nil {
		cmd.Printf("Warning: Reading existing config: %v\n", err)
		existingConfig = ""
	}

	// 設定にプロファイルを追記する。
	newConfig, err := appendProfiles(existingConfig, profiles)
	if err != nil {
		return fmt.Errorf("append profiles: %w", err)
	}

	// 設定ファイルを書き込む。
	if err := deps.writeConfig(configPath, newConfig); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	cmd.Printf("Successfully updated AWS config file at %s\n", configPath)
	return nil
}

// selectIndices は "1,3" や "all" 形式の入力を 0 始まりのインデックス一覧へ変換する。
// 不正な番号は警告を表示してスキップする。
func selectIndices(cmd *cobra.Command, input string, max int, kind string) []int {
	selected := make([]int, 0)
	if strings.ToLower(input) == "all" {
		for i := 0; i < max; i++ {
			selected = append(selected, i)
		}
		return selected
	}

	for _, indexStr := range strings.Split(input, ",") {
		indexStr = strings.TrimSpace(indexStr)
		if indexStr == "" {
			continue
		}

		index, err := strconv.Atoi(indexStr)
		if err != nil || index < 1 || index > max {
			cmd.Printf("Warning: Invalid %s number '%s', skipped\n", kind, indexStr)
			continue
		}

		// 0 始まりのインデックスへ変換する。
		selected = append(selected, index-1)
	}
	return selected
}

// ssoTokenDeps は getSSOTokenWith がデバイス認可フローの合成で呼ぶ外部処理をまとめる。
// 差し替え可能にしている理由はフィールドによって 2 つある。
// start / wait は internal/ssoauth の公開関数 (AWS への接続とキャッシュ保存を伴う) で、
// openBrowser はブラウザの起動を伴い、いずれもテストからは実行できない。エラーの伝播と
// 合成の順序を検証するために差し替える。
// display / reportBrowserFailure は標準出力・標準エラー出力へ書くだけでエラーを返さないが、
// テスト実行時の出力を汚さないために差し替える。
// これらは元から自由関数であり、絞り込む対象の具象型が無い。internal/aws のように
// SDK クライアントをコンシューマ定義インターフェースで受けるのではなく、関数値を
// 持たせているのはそのためである。
type ssoTokenDeps struct {
	start       func(ctx context.Context, region, startURL string) (*ssoauth.Session, error)
	wait        func(ctx context.Context, sess *ssoauth.Session) (*ssoauth.TokenCache, error)
	openBrowser func(url string) error
	display     func(verificationURI, userCode string, attemptingBrowser bool)
	// reportBrowserFailure は openBrowser の失敗を利用者へ伝える。RFC 8628 §3.3.1 は
	// ブラウザ等による非テキストでの提示を MAY と定めており、失敗はフローを止める理由に
	// ならない。display で verification_uri と user_code は既に提示済みのため、ここでは
	// 警告を出すだけで処理を継続する。
	reportBrowserFailure func(err error)
}

// defaultSSOTokenDeps は本番で使う実装を返す。
// ssoauth 側の依存 (AWS への接続とキャッシュ保存) は authDeps で受け取る。sso login は
// 保存まで行う ssoauth.DefaultDeps() を、generate-config は保存だけを無効化した依存を渡す。
func defaultSSOTokenDeps(authDeps ssoauth.Deps) ssoTokenDeps {
	return ssoTokenDeps{
		start: func(ctx context.Context, region, startURL string) (*ssoauth.Session, error) {
			return ssoauth.Start(ctx, region, startURL, authDeps)
		},
		wait: func(ctx context.Context, sess *ssoauth.Session) (*ssoauth.TokenCache, error) {
			return ssoauth.Wait(ctx, sess, authDeps)
		},
		openBrowser: openBrowser,
		display: func(verificationURI, userCode string, attemptingBrowser bool) {
			writeSSOLoginPrompt(os.Stdout, verificationURI, userCode, attemptingBrowser)
		},
		reportBrowserFailure: func(err error) {
			writeSSOBrowserFailureWarning(os.Stderr, err)
		},
	}
}

// getSSOTokenWithoutSaving は generate-config 用にデバイス認可フローを実行し、取得した
// トークンを返す。generate-config は従来からトークンをキャッシュへ保存しない (保存は
// sso login の責務)。ssoauth.Wait は保存まで含むため、保存だけを何もしない実装に
// 差し替えて従来の挙動を保つ。
func getSSOTokenWithoutSaving(ctx context.Context, region, url string) (*ssoauth.TokenCache, error) {
	return getSSOTokenWith(ctx, region, url, ssoTokenDepsWithoutSaving(ssoauth.DefaultDeps()))
}

// ssoTokenDepsWithoutSaving は authDeps の SaveCache を何もしない実装に差し替えてから
// 合成の依存を組む。差し替えを関数として切り出しているのは、「generate-config は
// キャッシュを保存しない」という外部挙動の不変条件をテストで直接検証できるように
// するためである。
func ssoTokenDepsWithoutSaving(authDeps ssoauth.Deps) ssoTokenDeps {
	authDeps.SaveCache = func(*ssoauth.TokenCache) error { return nil }
	return defaultSSOTokenDeps(authDeps)
}

// getSSOTokenWith は internal/ssoauth の 2 つの公開関数 (Start, Wait) と CLI 固有の
// 提示手段 (display, openBrowser) を合成して、デバイス認可フロー全体を実行する。
// sso login と sso generate-config の両方がこの合成を使う。
// ssoauth の 2 つの呼び出しは、どの段で失敗したかを示す文言で既にラップされて返る。
// この層から足せる情報が無いため包み直さず、そのまま伝播させる。
// openBrowser の失敗はフローを中断しない (下記コメント参照)。
func getSSOTokenWith(ctx context.Context, region, url string, deps ssoTokenDeps) (*ssoauth.TokenCache, error) {
	sess, err := deps.start(ctx, region, url)
	if err != nil {
		return nil, err
	}
	deviceAuth := sess.DeviceAuth

	// 提示する URI はサーバが返した verification_uri である (RFC 8628 §3.2 / §3.3)。
	// start URL から組み立てた値を渡すと、サーバの指示と食い違ったときに
	// ブラウザが開けなかった利用者の退路が塞がる。
	// テキストでの提示 (§3.3、user_code は §3.3.1 で MUST) をブラウザの起動より先に行う。
	// ブラウザの起動が失敗しても、利用者は既に verification_uri と user_code を見ている
	// 状態になる。
	// verification_uri_complete が空の場合はこの後ブラウザの起動を試みないため、
	// その旨を display にも伝える (試みない場合に「開こうとしています」と出すと
	// 実際の動作と矛盾する)。
	attemptingBrowser := deviceAuth.VerificationURIComplete != ""
	deps.display(deviceAuth.VerificationURI, deviceAuth.UserCode, attemptingBrowser)

	// verification_uri_complete は RFC 8628 §3.2 で OPTIONAL である。空文字列を
	// openBrowser に渡しても開く先が無く、失敗の文言も分かりにくいので試みない。
	if attemptingBrowser {
		if err := deps.openBrowser(deviceAuth.VerificationURIComplete); err != nil {
			// ブラウザによる非テキストでの提示は RFC 8628 §3.3.1 で MAY であり、
			// 失敗はフローを止める理由にならない。デバイス認可フローはそもそも
			// ブラウザを開けない環境のために存在する仕組みである。exec の裸の
			// エラーには文脈が無いため、何をしようとしたかをここで足して報告する。
			deps.reportBrowserFailure(fmt.Errorf("open browser: %w", err))
		}
	}

	return deps.wait(ctx, sess)
}

// writeSSOLoginPrompt はデバイス認可の承認手順を w へ書き出す。
//
// 書き出す内容は RFC 8628 §3.3 の User Interaction にあたる。認可サーバが返した
// verification_uri と user_code をそのまま提示する。
//
// 書き出し先を引数で受け取るのは、内容をテストから読めるようにするためである。
// 本番では os.Stdout を渡す。ここまでコマンドの出力先 (cmd.OutOrStdout) が届いて
// いないのは、getSSOTokenWith が cobra のコマンドを受け取らないためである。
//
// attemptingBrowser はこの後ブラウザの起動を試みるかどうかを表す。verification_uri_complete
// が空で試みない場合に「ブラウザを開こうとしています」と告げると、実際には何も起きず
// 利用者を混乱させる。試みるかどうかは呼び出し元がこの関数を呼ぶ時点で既に確定している。
func writeSSOLoginPrompt(w io.Writer, verificationURI, userCode string, attemptingBrowser bool) {
	if attemptingBrowser {
		fmt.Fprintln(w, "Attempting to automatically open the SSO authorization page in your default browser.")
	}

	// verification_uri は RFC 8628 §3.2 で REQUIRED である。欠けているのはサーバ側の
	// 仕様違反であり、こちらで start URL から URI を組み立てて補うことはしない。
	// 組み立てた値には仕様上の裏付けが無く、サーバの指示として見せることになる。
	// ポーリングは続行できるため、欠けていることを伝えて URI の行だけを省く。
	// この時点ではブラウザの起動を試みたかも成否も分からない (この関数はブラウザの
	// 起動より先に呼ばれる) ため、ブラウザ側の状態には触れない。
	if verificationURI == "" {
		fmt.Fprintln(w, "warning: the authorization server did not return a verification URI.")
	} else {
		fmt.Fprintln(w, "If the browser does not open or you wish to use a different device to authorize this request, open the following URL:")
		fmt.Fprintln(w)
		fmt.Fprintln(w, verificationURI)
	}

	// user_code の表示は RFC 8628 §3.3.1 の MUST である。verification_uri の有無に
	// 関わらず出す。認可サーバは利用者にこのコードの確認を要求する。
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Then enter the code:")
	fmt.Fprintln(w)
	fmt.Fprintln(w, userCode)
}

// writeSSOBrowserFailureWarning は openBrowser の失敗を w へ書き出す。
//
// ブラウザによる非テキストでの提示は RFC 8628 §3.3.1 で MAY であり、失敗はフローを
// 止める理由にならない。writeSSOLoginPrompt で提示済みの verification_uri と
// user_code を使って別の端末からでも承認できることを伝える。
//
// 書き出し先を引数で受け取るのは、内容をテストから読めるようにするためである。
// 本番では os.Stderr を渡す。
func writeSSOBrowserFailureWarning(w io.Writer, err error) {
	fmt.Fprintf(w, "warning: %v; open the URL shown above manually to continue.\n", err)
}

func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}

	return cmd.Run()
}

// getAwsConfigPath returns the path to the AWS config file.
func getAwsConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".aws", "config"), nil
}

// readAwsConfig reads the AWS config file and returns its content.
func readAwsConfig(configPath string) (string, error) {
	// ディレクトリが存在しない場合は作成する。
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", fmt.Errorf("create aws directory: %w", err)
	}

	// ファイルが存在しない場合は作成する。
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := os.WriteFile(configPath, []byte(""), 0600); err != nil {
			return "", fmt.Errorf("create config file: %w", err)
		}
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("read config file: %w", err)
	}

	return string(content), nil
}

// appendProfiles appends SSO profiles to the existing AWS config.
func appendProfiles(existingConfig string, profiles []ProfileConfig) (string, error) {
	var config strings.Builder

	// 既存設定の末尾を改行で揃える。
	if existingConfig != "" {
		existingConfig = strings.TrimSpace(existingConfig) + "\n\n"
	}

	config.WriteString(existingConfig)

	tmpl, err := template.New("profile").Parse(ssoProfileTemplate)
	if err != nil {
		return "", fmt.Errorf("parse profile template: %w", err)
	}

	for _, profile := range profiles {
		// 既に同名プロファイルが存在する場合はスキップする。
		profileHeader := fmt.Sprintf("[profile %s]", profile.Name)
		if strings.Contains(existingConfig, profileHeader) {
			continue
		}

		var profileContent bytes.Buffer
		if err := tmpl.Execute(&profileContent, profile); err != nil {
			return "", fmt.Errorf("execute template for %s: %w", profile.Name, err)
		}

		config.WriteString(profileContent.String())
		config.WriteString("\n")
	}

	return config.String(), nil
}

package cli

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/template"
	"time"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

const (
	ssoClientName = "thief"
	ssoClientType = "public"
	ssoGrantType  = "urn:ietf:params:oauth:grant-type:device_code"
)

// SSOTokenCache は ~/.aws/sso/cache に保存するトークンキャッシュの JSON 形状。
// AWS CLI (aws sso login) と互換のフォーマットを維持する。
type SSOTokenCache struct {
	StartURL              string `json:"startUrl"`
	Region                string `json:"region"`
	AccessToken           string `json:"accessToken"`
	ExpiresAt             string `json:"expiresAt"`
	ClientID              string `json:"clientId"`
	ClientSecret          string `json:"clientSecret"`
	RegistrationExpiresAt string `json:"registrationExpiresAt"`
}

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
		Short: "Logout from SSO. Remove all cache files.",
		Long:  "Sign out of AWS SSO by removing all cached credentials and tokens.",
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

// ssoLoginDeps は ssoLogin が呼ぶ外部処理をまとめる。
// getToken はブラウザの起動と AWS への往復を伴い、saveCache は $HOME/.aws/sso/cache へ
// 書き込むため、いずれもテストからは実行できない。コマンドに載った context が
// トークン取得まで届いていることを検証するために差し替える。
type ssoLoginDeps struct {
	getToken  func(ctx context.Context, region, url string) (*SSOTokenCache, error)
	saveCache func(cache *SSOTokenCache) error
}

// defaultSSOLoginDeps は本番で使う実装を返す。
func defaultSSOLoginDeps() ssoLoginDeps {
	return ssoLoginDeps{
		getToken:  getSSOToken,
		saveCache: saveSSOCacheFile,
	}
}

// ssoLogin authenticates with AWS SSO and caches the credentials.
func ssoLogin(cmd *cobra.Command, args []string) error {
	return ssoLoginWith(cmd, defaultSSOLoginDeps())
}

// ssoLoginWith は ssoLogin の本体。
// トークン取得へ渡す context はコマンドから取る。デバイス認可フローはユーザが
// ブラウザで承認するまで待つため、Ctrl-C がここへ届かないと待ち続ける。
func ssoLoginWith(cmd *cobra.Command, deps ssoLoginDeps) error {
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

	cache, err := deps.getToken(commandContext(cmd), region, startUrl)
	if err != nil {
		return fmt.Errorf("get token: %w", err)
	}

	// ~/.aws/sso/cache 配下にキャッシュファイルを作成する。
	if err = deps.saveCache(cache); err != nil {
		return fmt.Errorf("save cache file: %w", err)
	}

	// aws sso login コマンドと同じ出力にする。
	cmd.Printf("Successfully logged into Start URL: %s\n", startUrl)
	return nil
}

// ssoLogout removes all SSO credential cache files.
func ssoLogout(cmd *cobra.Command, args []string) error {
	cacheDir, err := getSSOCacheDir()
	if err != nil {
		return fmt.Errorf("get cache directory: %w", err)
	}

	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		return fmt.Errorf("directory does not exist: %s", cacheDir)
	}

	err = filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk directory: %w", err)
		}

		if info.IsDir() {
			return nil
		}

		if err := os.Remove(path); err != nil {
			return fmt.Errorf("delete file %s: %w", path, err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("error walking directory: %w", err)
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
	getToken     func(ctx context.Context, region, url string) (*SSOTokenCache, error)
	listAccounts func(ctx context.Context, region, accessToken string) ([]awsinternal.SSOAccountInfo, error)
	listRoles    func(ctx context.Context, region, accessToken, accountID string) ([]string, error)
	configPath   func() (string, error)
	readConfig   func(path string) (string, error)
	writeConfig  func(path, content string) error
}

// defaultSSOGenerateConfigDeps は本番で使う実装を返す。
func defaultSSOGenerateConfigDeps() ssoGenerateConfigDeps {
	return ssoGenerateConfigDeps{
		getToken:     getSSOToken,
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

// ssoTokenDeps は getSSOToken がデバイス認可フローで呼ぶ外部処理をまとめる。
// 差し替え可能にしている理由はフィールドによって 2 つある。
// registerClient / startDeviceAuth / waitForToken / openBrowser は AWS への接続かブラウザの
// 起動を伴い、テストからは実行できない。エラーの伝播を検証するために差し替える。
// display は標準出力へ書くだけでエラーを返さないが、テスト実行時の出力を汚さないために
// 差し替える。
// これらは元から自由関数であり、絞り込む対象の具象型が無い。internal/aws のように
// SDK クライアントをコンシューマ定義インターフェースで受けるのではなく、関数値を
// 持たせているのはそのためである。
type ssoTokenDeps struct {
	registerClient  func(ctx context.Context, region, clientName, clientType string) (*awsinternal.SSOClientRegistration, error)
	startDeviceAuth func(ctx context.Context, region string, reg *awsinternal.SSOClientRegistration, startURL string) (*awsinternal.SSODeviceAuthorization, error)
	openBrowser     func(url string) error
	waitForToken    func(ctx context.Context, region string, reg *awsinternal.SSOClientRegistration, deviceAuth *awsinternal.SSODeviceAuthorization, grantType string) (*awsinternal.SSOToken, error)
	display         func(verificationURI, userCode string)
}

// defaultSSOTokenDeps は本番で使う実装を返す。
func defaultSSOTokenDeps() ssoTokenDeps {
	return ssoTokenDeps{
		registerClient:  awsinternal.RegisterSSOClient,
		startDeviceAuth: awsinternal.StartSSODeviceAuthorization,
		openBrowser:     openBrowser,
		waitForToken:    awsinternal.WaitForSSOToken,
		display: func(verificationURI, userCode string) {
			writeSSOLoginPrompt(os.Stdout, verificationURI, userCode)
		},
	}
}

// getSSOToken はデバイス認可フローでアクセストークンを取得する。
func getSSOToken(ctx context.Context, region, url string) (*SSOTokenCache, error) {
	return getSSOTokenWith(ctx, region, url, defaultSSOTokenDeps())
}

// getSSOTokenWith は getSSOToken の本体。
// awsinternal の 3 つの呼び出しは、どの API で失敗したかを示す文言で既にラップされて返る。
// この層から足せる情報が無いため包み直さず、そのまま伝播させる。
// openBrowser だけは exec の裸のエラーを返すため、何をしようとしたかをここで足す。
func getSSOTokenWith(ctx context.Context, region, url string, deps ssoTokenDeps) (*SSOTokenCache, error) {
	registration, err := deps.registerClient(ctx, region, ssoClientName, ssoClientType)
	if err != nil {
		return nil, err
	}

	deviceAuth, err := deps.startDeviceAuth(ctx, region, registration, url)
	if err != nil {
		return nil, err
	}

	if err := deps.openBrowser(deviceAuth.VerificationURIComplete); err != nil {
		return nil, fmt.Errorf("open browser: %w", err)
	}

	// 提示する URI はサーバが返した verification_uri である (RFC 8628 §3.2 / §3.3)。
	// start URL から組み立てた値を渡すと、サーバの指示と食い違ったときに
	// ブラウザが開けなかった利用者の退路が塞がる。
	deps.display(deviceAuth.VerificationURI, deviceAuth.UserCode)

	// deviceAuth を丸ごと渡す。waitForToken は device code だけでなく、サーバが指示した
	// interval と expires_in からポーリング間隔と打ち切り期限を決める (RFC 8628 §3.2 / §3.5)。
	// ここで DeviceCode だけ取り出すと、その指示が捨てられて既定値に落ちる。
	token, err := deps.waitForToken(ctx, region, registration, deviceAuth, ssoGrantType)
	if err != nil {
		return nil, err
	}

	expireAt := time.Now().UTC().Add(time.Duration(token.ExpiresIn) * time.Second)

	return &SSOTokenCache{
		StartURL:              url,
		Region:                region,
		AccessToken:           token.AccessToken,
		ExpiresAt:             expireAt.Format(time.RFC3339),
		ClientID:              registration.ClientID,
		ClientSecret:          registration.ClientSecret,
		RegistrationExpiresAt: time.Unix(registration.ClientSecretExpiresAt, 0).UTC().Format(time.RFC3339),
	}, nil
}

// writeSSOLoginPrompt はデバイス認可の承認手順を w へ書き出す。
//
// 書き出す内容は RFC 8628 §3.3 の User Interaction にあたる。認可サーバが返した
// verification_uri と user_code をそのまま提示する。
//
// 書き出し先を引数で受け取るのは、内容をテストから読めるようにするためである。
// 本番では os.Stdout を渡す。ここまでコマンドの出力先 (cmd.OutOrStdout) が届いて
// いないのは、getSSOToken が cobra のコマンドを受け取らないためである。
func writeSSOLoginPrompt(w io.Writer, verificationURI, userCode string) {
	fmt.Fprintln(w, "Attempting to automatically open the SSO authorization page in your default browser.")

	// verification_uri は RFC 8628 §3.2 で REQUIRED である。欠けているのはサーバ側の
	// 仕様違反であり、こちらで start URL から URI を組み立てて補うことはしない。
	// 組み立てた値には仕様上の裏付けが無く、サーバの指示として見せることになる。
	// ポーリングは続行できるため、欠けていることを伝えて URI の行だけを省く。
	if verificationURI == "" {
		fmt.Fprintln(w, "warning: the authorization server did not return a verification URI; authorize in the browser page that was opened.")
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

func saveSSOCacheFile(cache *SSOTokenCache) error {
	cacheDir, err := getSSOCacheDir()
	if err != nil {
		return fmt.Errorf("failed to get cache directory: %w", err)
	}

	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return fmt.Errorf("failed to create sso cache directory: %w", err)
	}

	cacheKey := generateSSOCacheKey(cache.StartURL)
	cacheFile := filepath.Join(cacheDir, cacheKey+".json")

	jsonData, err := json.Marshal(cache)
	if err != nil {
		return fmt.Errorf("failed to marshal cache data: %w", err)
	}

	if err := os.WriteFile(cacheFile, jsonData, 0600); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

func getSSOCacheDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".aws", "sso", "cache"), nil
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

// generateSSOCacheKey は startUrl から AWS CLI 互換のキャッシュファイル名 (SHA-1) を生成する。
func generateSSOCacheKey(cacheKey string) string {
	hasher := sha1.New()
	hasher.Write([]byte(cacheKey))
	return hex.EncodeToString(hasher.Sum(nil))
}

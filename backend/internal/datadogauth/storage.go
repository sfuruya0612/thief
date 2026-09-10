package datadogauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/sfuruya0612/thief/backend/internal/config"
)

// ErrInvalidSite は site 名がホスト名として妥当でないことを表す。site 名は保存先の
// パスの一部になるため、パスの組み立てより前に検証する。
var ErrInvalidSite = errors.New("invalid datadog site")

// siteRe は Datadog の site (datadoghq.com、us3.datadoghq.com、ddog-gov.com など) が
// 満たすべき形。小文字の英数字とハイフンからなるラベルをドットで 2 つ以上つないだ形だけを
// 通す。パス区切り、"..", 空文字、大文字、先頭と末尾のハイフンやドットはすべて弾く。
var siteRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*(\.[a-z0-9]+(-[a-z0-9]+)*)+$`)

// maxSiteLength は site 名の長さの上限 (RFC 1035 のホスト名の上限)。
const maxSiteLength = 253

// dirName は config.Dir() 配下に作る、Datadog の認証情報の置き場。
const dirName = "datadog"

// ValidateSite は site 名を検証する。妥当でなければ ErrInvalidSite でラップした
// エラーを返す。
func ValidateSite(site string) error {
	if len(site) > maxSiteLength || !siteRe.MatchString(site) {
		return fmt.Errorf("%w: %q", ErrInvalidSite, site)
	}
	return nil
}

// Dir は Datadog の認証情報の保存先ディレクトリ (config.Dir()/datadog) を返す。
func Dir() (string, error) {
	base, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, dirName), nil
}

// TokenPath は site のトークンファイルのパスを返す。
func TokenPath(dir, site string) (string, error) {
	return sitePath(dir, "token_", site)
}

// ClientPath は site のクライアント登録ファイルのパスを返す。
func ClientPath(dir, site string) (string, error) {
	return sitePath(dir, "client_", site)
}

func sitePath(dir, prefix, site string) (string, error) {
	if err := ValidateSite(site); err != nil {
		return "", err
	}
	return filepath.Join(dir, prefix+site+".json"), nil
}

// LoadToken は dir 配下の site のトークンを読む。ファイルが無い場合は
// (nil, false, nil) を返す (未ログイン)。JSON として壊れている場合と、
// access_token を欠く場合はエラーを返す。
//
// 壊れたファイルを「無かったこと」にして再ログインへ倒さないのは、認証情報の消失を
// 黙って握り潰すと、利用者が気付かないうちに保存が壊れ続ける状態を招くためである。
func LoadToken(dir, site string) (*TokenSet, bool, error) {
	var tok TokenSet
	ok, err := loadJSON(dir, "token_", site, &tok)
	if err != nil || !ok {
		return nil, false, err
	}
	if err := tok.Validate(); err != nil {
		path, _ := TokenPath(dir, site)
		return nil, false, fmt.Errorf("load datadog oauth token %s: %w", path, err)
	}
	return &tok, true, nil
}

// SaveToken は dir 配下へ site のトークンをファイル権限 0600 で保存する。
func SaveToken(dir, site string, tok *TokenSet) error {
	if err := tok.Validate(); err != nil {
		return fmt.Errorf("save datadog oauth token: %w", err)
	}
	path, err := TokenPath(dir, site)
	if err != nil {
		return err
	}
	return saveJSON(path, tok)
}

// DeleteToken は dir 配下の site のトークンファイルを削除する。
// ファイルが無い場合は何もせず nil を返す。
func DeleteToken(dir, site string) error {
	path, err := TokenPath(dir, site)
	if err != nil {
		return err
	}
	return removeFile(path)
}

// LoadClient は dir 配下の site のクライアント登録を読む。ファイルが無い場合は
// (nil, false, nil) を返す (未登録)。JSON として壊れている場合と、client_id または
// redirect_uris を欠く場合はエラーを返す。
func LoadClient(dir, site string) (*ClientCredentials, bool, error) {
	var creds ClientCredentials
	ok, err := loadJSON(dir, "client_", site, &creds)
	if err != nil || !ok {
		return nil, false, err
	}
	if err := creds.Validate(); err != nil {
		path, _ := ClientPath(dir, site)
		return nil, false, fmt.Errorf("load datadog oauth client %s: %w", path, err)
	}
	return &creds, true, nil
}

// SaveClient は dir 配下へ site のクライアント登録をファイル権限 0600 で保存する。
// client_secret が返る認可サーバでもファイルへは平文で書くため、トークンと同じ権限にする。
func SaveClient(dir, site string, creds *ClientCredentials) error {
	if err := creds.Validate(); err != nil {
		return fmt.Errorf("save datadog oauth client: %w", err)
	}
	path, err := ClientPath(dir, site)
	if err != nil {
		return err
	}
	return saveJSON(path, creds)
}

// DeleteClient は dir 配下の site のクライアント登録ファイルを削除する。
// ファイルが無い場合は何もせず nil を返す。
func DeleteClient(dir, site string) error {
	path, err := ClientPath(dir, site)
	if err != nil {
		return err
	}
	return removeFile(path)
}

// loadJSON は dir/prefix+site.json を読んで out へ復号する。ファイルが無ければ
// (false, nil) を返す。
func loadJSON(dir, prefix, site string, out any) (bool, error) {
	path, err := sitePath(dir, prefix, site)
	if err != nil {
		return false, err
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return false, fmt.Errorf("parse %s: %w", path, err)
	}
	return true, nil
}

// saveJSON は v を path へ原子的に書き込む。同一ディレクトリへ一時ファイルを書き、
// 0600 へ変更してから rename するため、並行する読み手が書きかけの内容を見ることは無い
// (internal/pricecache の Save と同じ手順)。ディレクトリは 0700 で作成する。
func saveJSON(path string, v any) error {
	destDir := filepath.Dir(path)
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return fmt.Errorf("create datadog auth dir %s: %w", destDir, err)
	}
	payload, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	tmp, err := os.CreateTemp(destDir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", destDir, err)
	}
	defer os.Remove(tmp.Name()) // rename 成功後は ENOENT になるだけなので常に呼んでよい
	if _, err := tmp.Write(payload); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file for %s: %w", path, err)
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return fmt.Errorf("chmod temp file for %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("rename %s: %w", path, err)
	}
	return nil
}

func removeFile(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

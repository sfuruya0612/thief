package aws

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ssoCacheMaxFileSize は SSO キャッシュとして読み込む JSON の上限サイズ。
// 正常なキャッシュは数 KB であり、これを超えるファイルは対象外として扱う。
const ssoCacheMaxFileSize = 1 << 20 // 1MB

// ssoCacheStatus は 1 つの startUrl に対するローカルトークンの状態。
type ssoCacheStatus struct {
	Status    SSOStatus
	ExpiresAt time.Time
}

// ssoCacheEntry はトークンキャッシュ JSON から読み取る最小フィールド。
// accessToken / clientSecret 等の秘密情報はフィールドに持たないことで、
// デコード時に Go のメモリへ展開されること自体を避ける。
type ssoCacheEntry struct {
	StartURL  string `json:"startUrl"`
	ExpiresAt string `json:"expiresAt"`
}

// normalizeStartURL は startUrl 比較用のキーを返す (末尾スラッシュの有無を
// 吸収する)。AWS CLI と thief 自身でキャッシュ書き込み時の表記が揺れるため、
// 突き合わせは常に正規化後の値で行う。
func normalizeStartURL(u string) string {
	return strings.TrimRight(strings.TrimSpace(u), "/")
}

// readSSOCacheStatuses は cacheDir (~/.aws/sso/cache) の JSON を 1 パスで
// 全走査し、正規化済み startUrl → トークン状態の map を返す。2 番目の返り値は
// ディレクトリを読み取れたかどうか (not-exist は「未ログイン」の正常系として
// true)。false のとき呼び出し側は SSO 状態を判定不能 (欠落) として扱う。
//
// キャッシュのファイル名は AWS CLI の形式 (legacy=SHA1(startUrl) /
// sso-session=SHA1(セッション名)) や書き込み元により SHA1 の入力が異なり
// 推測できないため、ファイル名ではなく中身の startUrl で突き合わせる。
// client registration ファイルは startUrl を持たないことで除外される。
//
// なお既存の loadSSOAccessToken (sso_token.go) は startUrl を見ずに最初の
// 有効トークンを返す別実装であり、意図的に統合していない (別 issue で扱う)。
func readSSOCacheStatuses(cacheDir string, now time.Time) (map[string]ssoCacheStatus, bool) {
	statuses := make(map[string]ssoCacheStatus)
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			// 一度もログインしていない正常系。
			return statuses, true
		}
		slog.Warn("read sso cache dir failed", "err", err)
		return statuses, false
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if info, err := entry.Info(); err == nil && info.Size() > ssoCacheMaxFileSize {
			slog.Warn("skip oversized sso cache file", "file", entry.Name(), "size", info.Size())
			continue
		}
		data, err := os.ReadFile(filepath.Join(cacheDir, entry.Name()))
		if err != nil {
			slog.Warn("read sso cache file failed", "file", entry.Name(), "err", err)
			continue
		}
		var ce ssoCacheEntry
		if err := json.Unmarshal(data, &ce); err != nil {
			slog.Warn("parse sso cache file failed", "file", entry.Name(), "err", err)
			continue
		}
		if ce.StartURL == "" {
			// client registration 等トークン以外のキャッシュ。常在する正常系
			// なのでログは出さない。
			continue
		}

		st := ssoCacheStatus{Status: SSOStatusExpired}
		if exp, err := time.Parse(time.RFC3339, ce.ExpiresAt); err != nil {
			// botocore 旧形式 ("2020-06-14T05:26:13UTC") 等。有効と確認できない
			// ため期限切れ扱いに落とす (安全側の degrade)。
			slog.Warn("parse sso cache expiresAt failed", "file", entry.Name(), "err", err)
		} else {
			st.ExpiresAt = exp
			if exp.After(now) {
				st.Status = SSOStatusValid
			}
		}

		// 同一 startUrl に複数ファイルがある場合は期限が最も先のものを採用する。
		key := normalizeStartURL(ce.StartURL)
		if prev, ok := statuses[key]; ok && prev.ExpiresAt.After(st.ExpiresAt) {
			continue
		}
		statuses[key] = st
	}
	return statuses, true
}

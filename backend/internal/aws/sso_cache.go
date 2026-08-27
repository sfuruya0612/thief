package aws

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ssoCacheMaxFileSize は SSO キャッシュとして読み込む JSON の上限サイズ。
// 正常なキャッシュは数 KB であり、これを超えるファイルは対象外として扱う。
const ssoCacheMaxFileSize = 1 << 20 // 1MB

// ssoCacheRemove は RemoveSSOTokenCache が使うファイル削除関数。走査と削除の間に
// 別プロセスが同じファイルを消した状況 (fs.ErrNotExist) と削除失敗をテストで
// 決定的に再現するために差し替え可能にしている。
var ssoCacheRemove = os.Remove

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
	err := walkSSOCacheTokens(cacheDir, func(fileName string, ce ssoCacheEntry) {
		st := ssoCacheStatus{Status: SSOStatusExpired}
		if exp, err := time.Parse(time.RFC3339, ce.ExpiresAt); err != nil {
			// botocore 旧形式 ("2020-06-14T05:26:13UTC") 等。有効と確認できない
			// ため期限切れ扱いに落とす (安全側の degrade)。
			slog.Warn("parse sso cache expiresAt failed", "file", fileName, "err", err)
		} else {
			st.ExpiresAt = exp
			if exp.After(now) {
				st.Status = SSOStatusValid
			}
		}

		// 同一 startUrl に複数ファイルがある場合は期限が最も先のものを採用する。
		key := normalizeStartURL(ce.StartURL)
		if prev, ok := statuses[key]; ok && prev.ExpiresAt.After(st.ExpiresAt) {
			return
		}
		statuses[key] = st
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// 一度もログインしていない正常系。
			return statuses, true
		}
		slog.Warn("read sso cache dir failed", "err", err)
		return statuses, false
	}
	return statuses, true
}

// walkSSOCacheTokens は cacheDir の JSON を 1 パスで走査し、startUrl を持つ
// トークンキャッシュごとに visit を呼ぶ。対象の判定規則 (.json のみ、1MB 超と
// 読めないファイルと壊れた JSON は slog.Warn を出して対象外、startUrl を持たない
// client registration は対象外) をここに集約し、状態の読み取り
// (readSSOCacheStatuses) と削除 (RemoveSSOTokenCache) で同じファイル集合を扱う。
// 返すエラーは os.ReadDir のものだけで、not-exist の扱いは呼び出し側に委ねる。
func walkSSOCacheTokens(cacheDir string, visit func(fileName string, ce ssoCacheEntry)) error {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return err
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
		visit(entry.Name(), ce)
	}
	return nil
}

// RemoveSSOTokenCache は cacheDir (~/.aws/sso/cache) のトークンキャッシュのうち、
// 中身の startUrl が startURL と一致する (normalizeStartURL 後の比較) ファイルを
// すべて削除する。他の startUrl のファイルと startUrl を持たない client
// registration は残す。ログアウトの目的は対象ファイルが無い状態にすることなので、
// 一致するファイルが無い場合、cacheDir 自体が無い場合、削除時に既に消えていた
// 場合 (fs.ErrNotExist) はいずれも成功として nil を返す。一致が複数ある場合は
// 1 件の削除失敗で止めずに残りも削除し、失敗を errors.Join でまとめて返す。
// cacheDir の読み取りに not-exist 以外で失敗した場合は、対象を特定できていない
// ためエラーを返す。
func RemoveSSOTokenCache(cacheDir, startURL string) error {
	want := normalizeStartURL(startURL)
	var targets []string
	err := walkSSOCacheTokens(cacheDir, func(fileName string, ce ssoCacheEntry) {
		if normalizeStartURL(ce.StartURL) == want {
			targets = append(targets, fileName)
		}
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read sso cache dir: %w", err)
	}

	var errs []error
	for _, name := range targets {
		if err := ssoCacheRemove(filepath.Join(cacheDir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, fmt.Errorf("remove sso cache file %s: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

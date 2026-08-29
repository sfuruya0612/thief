// Package snippet はクエリスニペットのファイルベース永続化を提供する。
// スニペットはベースディレクトリ配下のサービス別ディレクトリ (athena / bigquery) に
// <name>.sql として保存されるため、手動で配置した .sql ファイルもそのまま一覧に載る。
// ファイル名として安全でない文字を含む名前はパーセントエンコードしてファイル名に
// する (encodeName / decodeFileName)。エンコード規則に従わない手動配置のファイル名は
// そのまま名前として扱う。
package snippet

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// ErrInvalidService はサービスキーが未対応の場合のエラー。
var ErrInvalidService = errors.New("invalid snippet service")

// ErrInvalidName は名前が空、またはエンコード後のファイル名がバイト長上限を
// 超える場合のエラー。文字種はエンコードで吸収するため制限しない。
var ErrInvalidName = errors.New("invalid snippet name")

// ErrNotFound は指定名のスニペットが存在しない場合のエラー。
var ErrNotFound = errors.New("snippet not found")

// renameFile は renameSnippetFile が一時ファイルを保存先へ置くときに使う rename 関数。
// rename の直後に別リクエストが同じ名前を上書きした状況 (issue 0160) と、rename が ENOENT を
// 返す状況 (issue 0161) をテストで決定的に再現するために差し替え可能にしている。本番コード
// からは代入しないこと。パッケージ全体で共有する変数なので、これを差し替えるテストと Save を
// 呼ぶテストが t.Parallel で並列に走ると、代入 (書き込み) と renameSnippetFile 内の呼び出し
// (読み取り) がこの変数へのデータ競合になる。
var renameFile = os.Rename

// renameAttemptLimit は renameSnippetFile が rename を試みる回数の上限 (初回 + 再試行)。
// 同一宛先への競合下で成功までに要した試行回数の実測値の最大が 5 回だったため、その約 3 倍の
// 余裕を持たせた値にしている (issue 0161 の調査結果)。
const renameAttemptLimit = 16

// maxNameLength はエンコード後のファイル名 (拡張子 .sql を除く部分) の最大バイト長
// (ファイルシステムのファイル名長制限より十分小さい値)。名前そのものではなく
// エンコード後の長さに適用する。ファイルシステムの制限が対象とするのは
// エンコード後のファイル名であるため。
const maxNameLength = 128

// services は保存を許可するサービスキー (= ベースディレクトリ直下のサブディレクトリ名)。
var services = map[string]bool{
	"athena":   true,
	"bigquery": true,
}

// Snippet は 1 つのクエリスニペット。UpdatedAt はファイルの更新日時。
type Snippet struct {
	Name      string    `json:"name"`
	SQL       string    `json:"sql"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Store はサービス別ディレクトリ配下の .sql ファイルとしてスニペットを読み書きする。
type Store struct {
	baseDir string
}

// NewStore は baseDir を保存先ベースディレクトリとする Store を返す。
func NewStore(baseDir string) *Store {
	return &Store{baseDir: baseDir}
}

func validateService(service string) error {
	if !services[service] {
		return fmt.Errorf("%w: %q", ErrInvalidService, service)
	}
	return nil
}

// validateName はスニペット名を検証する。ファイル名として使えない文字は
// encodeName が置き換えるため文字種は制限せず、空でないことと、エンコード後の
// ファイル名のバイト長上限だけを確認する。
func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: must not be empty", ErrInvalidName)
	}
	if len(encodeName(name)) > maxNameLength {
		return fmt.Errorf("%w: encoded file name must be at most %d bytes", ErrInvalidName, maxNameLength)
	}
	return nil
}

// encodeName はスニペット名をファイル名 (拡張子 .sql を除く部分) へ変換する。
// ファイル名として安全でない文字 (パス区切り `/` `\`、NUL)、エンコードに用いる
// `%` 自体、および先頭のドット (隠しファイル / 相対パス) を大文字 16 進の
// パーセントエンコードに置き換える。それ以外の文字 (全角文字を含む) は
// エンコードせず、ファイル名の可読性を保つ。
func encodeName(name string) string {
	var sb strings.Builder
	for i := 0; i < len(name); i++ {
		b := name[i]
		if b == '/' || b == '\\' || b == 0 || b == '%' || (i == 0 && b == '.') {
			fmt.Fprintf(&sb, "%%%02X", b)
			continue
		}
		sb.WriteByte(b)
	}
	return sb.String()
}

// decodeFileName はファイル名 (拡張子 .sql を除いた部分) をスニペット名へ戻す。
// デコードは正規形 (デコード結果を encodeName で再エンコードすると元のファイル名に
// 一致する) の場合に限って適用し、非正規形のファイル名 (デコードできない `%` 並び、
// 16 進が小文字の `%2f` 形式、エンコードなしで `%` を含む手動配置のファイル名) は
// エラーにせず、そのまま名前として扱う。この規則により、どのファイルでも
// 一覧に出た名前で削除できるラウンドトリップが成り立つ。
func decodeFileName(stem string) string {
	decoded, err := url.PathUnescape(stem)
	if err != nil || encodeName(decoded) != stem {
		return stem
	}
	return decoded
}

// rawFileNameSafe は name をそのままファイル名として使ってよいか
// (パス区切り・NUL を含まず、先頭がドットでない) を返す。
// 手動配置された非正規形のファイル名への後方互換の削除パスにだけ使う。
func rawFileNameSafe(name string) bool {
	return !strings.ContainsAny(name, "/\\\x00") && !strings.HasPrefix(name, ".")
}

func (s *Store) dir(service string) string {
	return filepath.Join(s.baseDir, service)
}

func (s *Store) path(service, name string) string {
	return filepath.Join(s.baseDir, service, encodeName(name)+".sql")
}

// List は service のディレクトリ直下の .sql ファイルを更新日時の降順
// (同時刻は名前順) で返す。ディレクトリが存在しない場合は空リストを返す (初回起動時)。
func (s *Store) List(service string) ([]Snippet, error) {
	if err := validateService(service); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.dir(service))
	if errors.Is(err, os.ErrNotExist) {
		return []Snippet{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read snippets dir %s: %w", s.dir(service), err)
	}
	snippets := make([]Snippet, 0, len(entries))
	for _, e := range entries {
		// 隠しファイル (Save の一時ファイル .tmp-* を含む) と .sql 以外は一覧に載せない
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		name := decodeFileName(strings.TrimSuffix(e.Name(), ".sql"))
		// ReadDir と Open は別のシステムコールのため、その間に別リクエストや手動操作で
		// 削除されたファイルは fs.ErrNotExist になる。もう存在しないスニペットとして一覧
		// から外し、1 ファイルの消失で一覧全体を失敗させない。それ以外のエラー (権限不足
		// など) は従来どおり返す。
		data, modTime, err := readSnippetFile(filepath.Join(s.dir(service), e.Name()))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read snippet %s: %w", e.Name(), err)
		}
		snippets = append(snippets, Snippet{
			Name:      name,
			SQL:       string(data),
			UpdatedAt: modTime.UTC(),
		})
	}
	sort.Slice(snippets, func(i, j int) bool {
		if !snippets[i].UpdatedAt.Equal(snippets[j].UpdatedAt) {
			return snippets[i].UpdatedAt.After(snippets[j].UpdatedAt)
		}
		return snippets[i].Name < snippets[j].Name
	})
	return snippets, nil
}

// readSnippetFile は path を 1 回だけ開き、同じファイル記述子から本文と更新日時を取得する。
// Save は一時ファイルの rename で上書きするため、開いた後に上書きが起きても記述子は
// 旧版の inode を指し続け、本文と更新日時が別の版に属することはない (os.ReadFile と
// DirEntry.Info のように別々にパスを解決すると、その間の上書きで組がずれる)。
func readSnippetFile(path string) ([]byte, time.Time, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, time.Time{}, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, time.Time{}, err
	}
	return data, info.ModTime(), nil
}

// renameSnippetFile は oldpath を newpath へ rename する。macOS (Darwin 25.5.0、APFS で実測)
// では、宛先が同時に別の rename で置き換えられている間、ソースが存在するのに rename が ENOENT
// を返すことがある (issue 0161)。この ENOENT は宛先側の解決に由来する一時的な失敗なので、
// ソースが存在することを確認できる場合に限り renameAttemptLimit 回まで即時に再試行する。
// バックオフを入れないのは、実測で即時再試行が収束しており、待機は通常経路の遅延になるため。
//
// ソースが存在しない場合 (保存先ディレクトリごと消えた場合を含む。一時ファイルは保存先と
// 同じディレクトリに作るため、ディレクトリが消えていればソースも存在しない) と、ソースの
// os.Stat が ENOENT 以外で失敗して存在を確認できない場合は、再試行せずそのまま返す。宛先側の
// 一時的な失敗以外の ENOENT を再試行で隠さないためである。
//
// 上限に達した場合は試行回数を添えて返す。ディレクトリ欠落による ENOENT (初回で返る) と
// 競合による ENOENT (上限まで再試行して返る) を、運用時にエラーメッセージで切り分けられる
// ようにするため。
func renameSnippetFile(oldpath, newpath string) error {
	for attempt := 1; ; attempt++ {
		err := renameFile(oldpath, newpath)
		if err == nil {
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if _, serr := os.Stat(oldpath); serr != nil {
			return err
		}
		if attempt >= renameAttemptLimit {
			return fmt.Errorf("give up after %d rename attempts: %w", attempt, err)
		}
	}
}

// Save は service 配下に name のスニペットを作成または上書きし、保存結果を返す。
// 一時ファイルへ書き込んでから rename することで部分書き込みを防ぐ。rename は
// renameSnippetFile 経由で行い、同時に同じ名前が置き換えられている間の ENOENT を吸収する。
func (s *Store) Save(service, name, sql string) (Snippet, error) {
	if err := validateService(service); err != nil {
		return Snippet{}, err
	}
	if err := validateName(name); err != nil {
		return Snippet{}, err
	}
	dir := s.dir(service)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Snippet{}, fmt.Errorf("create snippets dir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return Snippet{}, fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	defer os.Remove(tmp.Name()) // rename 成功後は ENOENT になるだけなので常に呼んでよい
	if _, err := tmp.WriteString(sql); err != nil {
		tmp.Close()
		return Snippet{}, fmt.Errorf("write snippet %s: %w", name, err)
	}
	// 更新日時は rename 前に、自分が書き込んだ一時ファイルの記述子から取る。rename は
	// inode を変えないため保存先の更新日時と同じ値になり、rename 後にパスを再解決すると
	// その間に別リクエストが同じ名前を上書きした版の更新日時を返してしまう (issue 0160)。
	info, err := tmp.Stat()
	if err != nil {
		// 返すのは Stat のエラーで、後始末の Close の戻り値は使わない (記述子の解放だけが
		// 目的で、報告すべき失敗は既に Stat が示しているため)。
		tmp.Close()
		return Snippet{}, fmt.Errorf("stat snippet %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return Snippet{}, fmt.Errorf("close snippet %s: %w", name, err)
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return Snippet{}, fmt.Errorf("chmod snippet %s: %w", name, err)
	}
	if err := renameSnippetFile(tmp.Name(), s.path(service, name)); err != nil {
		return Snippet{}, fmt.Errorf("rename snippet %s: %w", name, err)
	}
	return Snippet{Name: name, SQL: sql, UpdatedAt: info.ModTime().UTC()}, nil
}

// Delete は service 配下の name のスニペットを削除する。存在しない場合は ErrNotFound を返す。
// まず名前をエンコードしたファイル名を探し、無ければ名前をそのままファイル名とみなす
// 後方互換のパスも探す (List がそのまま名前として返す、手動配置された非正規形の
// ファイル名に対応する)。エンコード後の長さ検証は行わない。一覧に載った名前は
// エンコード後の長さにかかわらず削除できる必要があるため。
func (s *Store) Delete(service, name string) error {
	if err := validateService(service); err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("%w: must not be empty", ErrInvalidName)
	}
	err := os.Remove(s.path(service, name))
	if errors.Is(err, os.ErrNotExist) && rawFileNameSafe(name) {
		err = os.Remove(filepath.Join(s.dir(service), name+".sql"))
	}
	// ファイル名 (単一コンポーネント) が NAME_MAX を超える名前はファイルとして
	// 存在しえないため、ENAMETOOLONG を存在しない場合と同じ ErrNotFound に写像する
	// (500 にしない)。ENAMETOOLONG はパス全体の PATH_MAX 超過でも発生し、その場合は
	// baseDir の設定不備を 404 が隠すことになるが、同じ baseDir では Save も
	// 同様に失敗して保存自体ができないため、実在するファイルを 404 にする余地は
	// 実質的にない。
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENAMETOOLONG) {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if err != nil {
		return fmt.Errorf("delete snippet %s: %w", name, err)
	}
	return nil
}

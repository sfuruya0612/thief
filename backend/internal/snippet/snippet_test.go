package snippet

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestValidateService(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{name: "athena", input: "athena"},
		{name: "bigquery", input: "bigquery"},
		{name: "empty", input: "", wantErr: ErrInvalidService},
		{name: "unknown", input: "redshift", wantErr: ErrInvalidService},
		{name: "traversal", input: "../athena", wantErr: ErrInvalidService},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateService(tt.input); !errors.Is(err, tt.wantErr) {
				t.Fatalf("validateService(%q) = %v, want %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{name: "ok", input: "monthly cost"},
		{name: "ok japanese", input: "月次コスト集計"},
		{name: "ok slash", input: "a/b"},
		{name: "ok backslash", input: `a\b`},
		{name: "ok traversal", input: "../evil"},
		{name: "ok leading dot", input: ".hidden"},
		{name: "ok percent", input: "50%off"},
		{name: "ok nul", input: "a\x00b"},
		{name: "ok max length", input: strings.Repeat("a", maxNameLength)},
		{name: "empty", input: "", wantErr: ErrInvalidName},
		{name: "raw too long", input: strings.Repeat("a", maxNameLength+1), wantErr: ErrInvalidName},
		// エンコードで 3 倍に膨らみ、生の長さは上限内でもエンコード後に超える
		{name: "encoded too long", input: strings.Repeat("/", maxNameLength/3+1), wantErr: ErrInvalidName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateName(tt.input); !errors.Is(err, tt.wantErr) {
				t.Fatalf("validateName(%q) = %v, want %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestEncodeName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain", input: "monthly cost", want: "monthly cost"},
		{name: "japanese", input: "月次コスト集計", want: "月次コスト集計"},
		{name: "slash", input: "a/b", want: "a%2Fb"},
		{name: "backslash", input: `a\b`, want: "a%5Cb"},
		{name: "nul", input: "a\x00b", want: "a%00b"},
		{name: "percent", input: "50%off", want: "50%25off"},
		{name: "leading dot", input: ".hidden", want: "%2Ehidden"},
		{name: "inner dot", input: "v1.2", want: "v1.2"},
		{name: "traversal", input: "../evil", want: "%2E.%2Fevil"},
		{
			name:  "repro name",
			input: "virtual_money_issue_refund / virtual_money_use_refund（取消・返金）",
			want:  "virtual_money_issue_refund %2F virtual_money_use_refund（取消・返金）",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := encodeName(tt.input); got != tt.want {
				t.Fatalf("encodeName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDecodeFileName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain", input: "monthly cost", want: "monthly cost"},
		{name: "canonical slash", input: "a%2Fb", want: "a/b"},
		{name: "canonical percent", input: "50%25off", want: "50%off"},
		{name: "canonical leading dot", input: "%2Ehidden", want: ".hidden"},
		// 非正規形はデコードせずファイル名をそのまま名前とする
		{name: "lowercase hex", input: "a%2fb", want: "a%2fb"},
		{name: "undecodable percent", input: "bad%zz", want: "bad%zz"},
		{name: "unencoded percent", input: "foo%20bar", want: "foo%20bar"},
		{name: "over encoded", input: "%41", want: "%41"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decodeFileName(tt.input); got != tt.want {
				t.Fatalf("decodeFileName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestStoreRoundTripUnsafeNames はファイル名に使えない文字を含む名前の
// Save / List / Delete のラウンドトリップを athena / bigquery の両サービスで検証する。
func TestStoreRoundTripUnsafeNames(t *testing.T) {
	names := []struct {
		name     string
		input    string
		wantStem string
	}{
		{
			name:     "slash",
			input:    "virtual_money_issue_refund / virtual_money_use_refund（取消・返金）",
			wantStem: "virtual_money_issue_refund %2F virtual_money_use_refund（取消・返金）",
		},
		{name: "backslash", input: `a\b`, wantStem: "a%5Cb"},
		{name: "nul", input: "a\x00b", wantStem: "a%00b"},
		{name: "percent", input: "50%off", wantStem: "50%25off"},
		{name: "traversal", input: "../evil", wantStem: "%2E.%2Fevil"},
		{name: "leading dot", input: ".hidden", wantStem: "%2Ehidden"},
	}
	for _, service := range []string{"athena", "bigquery"} {
		for _, tt := range names {
			t.Run(service+"/"+tt.name, func(t *testing.T) {
				dir := t.TempDir()
				s := NewStore(dir)
				if _, err := s.Save(service, tt.input, "SELECT 1"); err != nil {
					t.Fatalf("Save(%q): %v", tt.input, err)
				}
				// エンコード済みファイル名でサービスディレクトリ直下に保存される
				if _, err := os.Stat(filepath.Join(dir, service, tt.wantStem+".sql")); err != nil {
					t.Fatalf("encoded file: %v", err)
				}
				// 上書きも成立する
				if _, err := s.Save(service, tt.input, "SELECT 2"); err != nil {
					t.Fatalf("Save overwrite: %v", err)
				}
				got, err := s.List(service)
				if err != nil {
					t.Fatalf("List: %v", err)
				}
				if len(got) != 1 || got[0].Name != tt.input || got[0].SQL != "SELECT 2" {
					t.Fatalf("List = %+v, want single %q with SELECT 2", got, tt.input)
				}
				if err := s.Delete(service, tt.input); err != nil {
					t.Fatalf("Delete(%q): %v", tt.input, err)
				}
				got, err = s.List(service)
				if err != nil {
					t.Fatalf("List after delete: %v", err)
				}
				if len(got) != 0 {
					t.Fatalf("List after delete = %+v, want empty", got)
				}
			})
		}
	}
}

// TestStoreListAndDeleteNonCanonicalFileNames は手動配置された非正規形のファイル名が
// エラーにならず、ファイル名そのままの名前で一覧に載り、その名前で削除できることを
// athena / bigquery の両サービスで検証する。
func TestStoreListAndDeleteNonCanonicalFileNames(t *testing.T) {
	stems := []string{"foo%20bar", "bad%zz", "a%2fb"}
	for _, service := range []string{"athena", "bigquery"} {
		t.Run(service, func(t *testing.T) {
			base := t.TempDir()
			dir := filepath.Join(base, service)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			for _, stem := range stems {
				if err := os.WriteFile(filepath.Join(dir, stem+".sql"), []byte("SELECT 1"), 0o644); err != nil {
					t.Fatalf("WriteFile(%s): %v", stem, err)
				}
			}
			s := NewStore(base)
			got, err := s.List(service)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(got) != len(stems) {
				t.Fatalf("List returned %d snippets, want %d: %+v", len(got), len(stems), got)
			}
			byName := map[string]bool{}
			for _, sn := range got {
				byName[sn.Name] = true
			}
			for _, stem := range stems {
				if !byName[stem] {
					t.Fatalf("List does not contain %q: %+v", stem, got)
				}
				if err := s.Delete(service, stem); err != nil {
					t.Fatalf("Delete(%q): %v", stem, err)
				}
			}
			got, err = s.List(service)
			if err != nil {
				t.Fatalf("List after delete: %v", err)
			}
			if len(got) != 0 {
				t.Fatalf("List after delete = %+v, want empty", got)
			}
		})
	}
}

// TestStoreListDecodesCanonicalFileNames は手動配置のファイル名がたまたま正規形の
// エンコード列である場合に、一覧の名前がデコード結果になり、その名前で削除できる
// ことを athena / bigquery の両サービスで検証する (issue 0150 の修正方針が許容する
// 表示名の差)。
func TestStoreListDecodesCanonicalFileNames(t *testing.T) {
	for _, service := range []string{"athena", "bigquery"} {
		t.Run(service, func(t *testing.T) {
			base := t.TempDir()
			dir := filepath.Join(base, service)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "100%2Foff.sql"), []byte("SELECT 1"), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			s := NewStore(base)
			got, err := s.List(service)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(got) != 1 || got[0].Name != "100/off" {
				t.Fatalf("List = %+v, want single 100/off", got)
			}
			if err := s.Delete(service, "100/off"); err != nil {
				t.Fatalf("Delete: %v", err)
			}
		})
	}
}

// TestStoreListPercentCollision は正規形ファイル (50%25off.sql → 表示名 50%off) と
// 非正規形ファイル (50%off.sql → 表示名 50%off) が同居して表示名が衝突した場合の
// 挙動を athena / bigquery の両サービスで固定する。一覧は両方を同じ名前で返し、
// その名前の Delete は正規形 → 非正規形の順に 1 回 1 ファイルずつ削除できる
// (行き止まりにならない)。
func TestStoreListPercentCollision(t *testing.T) {
	for _, service := range []string{"athena", "bigquery"} {
		t.Run(service, func(t *testing.T) {
			base := t.TempDir()
			dir := filepath.Join(base, service)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			for _, stem := range []string{"50%25off", "50%off"} {
				if err := os.WriteFile(filepath.Join(dir, stem+".sql"), []byte("SELECT 1"), 0o644); err != nil {
					t.Fatalf("WriteFile(%s): %v", stem, err)
				}
			}
			s := NewStore(base)
			got, err := s.List(service)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(got) != 2 || got[0].Name != "50%off" || got[1].Name != "50%off" {
				t.Fatalf("List = %+v, want two entries named 50%%off", got)
			}
			// 1 回目はエンコード済みパス (正規形の 50%25off.sql) が消える
			if err := s.Delete(service, "50%off"); err != nil {
				t.Fatalf("Delete first: %v", err)
			}
			if _, err := os.Stat(filepath.Join(dir, "50%25off.sql")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("canonical file must be removed first: %v", err)
			}
			if _, err := os.Stat(filepath.Join(dir, "50%off.sql")); err != nil {
				t.Fatalf("non-canonical file must survive first delete: %v", err)
			}
			// 2 回目は後方互換パス (非正規形の 50%off.sql) が消える
			if err := s.Delete(service, "50%off"); err != nil {
				t.Fatalf("Delete second: %v", err)
			}
			if err := s.Delete(service, "50%off"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("Delete third = %v, want ErrNotFound", err)
			}
		})
	}
}

// TestStoreDeleteLongName は List が長さ制限なしで一覧に載せた名前を
// Delete がエンコード後の長さ検証なしで削除できることを athena / bigquery の
// 両サービスで検証する (一覧に出た名前で削除できるラウンドトリップの一部)。
func TestStoreDeleteLongName(t *testing.T) {
	for _, service := range []string{"athena", "bigquery"} {
		t.Run(service, func(t *testing.T) {
			base := t.TempDir()
			dir := filepath.Join(base, service)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			stem := strings.Repeat("a", maxNameLength+10)
			if err := os.WriteFile(filepath.Join(dir, stem+".sql"), []byte("SELECT 1"), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			s := NewStore(base)
			got, err := s.List(service)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(got) != 1 || got[0].Name != stem {
				t.Fatalf("List = %+v, want single %q", got, stem)
			}
			if err := s.Delete(service, stem); err != nil {
				t.Fatalf("Delete: %v", err)
			}
		})
	}
}

// TestStoreDeleteDoesNotEscapeServiceDir は後方互換の削除パスがサービスディレクトリの
// 外や隠しファイルへ届かないことを検証する。
func TestStoreDeleteDoesNotEscapeServiceDir(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "athena")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// サービスディレクトリの 1 つ上 (ベースディレクトリ直下) のファイル
	outside := filepath.Join(base, "evil.sql")
	if err := os.WriteFile(outside, []byte("SELECT 1"), 0o644); err != nil {
		t.Fatalf("WriteFile(outside): %v", err)
	}
	// サービスディレクトリ直下の隠しファイル
	hidden := filepath.Join(dir, ".hidden.sql")
	if err := os.WriteFile(hidden, []byte("SELECT 1"), 0o644); err != nil {
		t.Fatalf("WriteFile(hidden): %v", err)
	}
	// バックスラッシュを字面に含む手動配置のファイル (POSIX では合法なファイル名)
	backslash := filepath.Join(dir, `a\b.sql`)
	if err := os.WriteFile(backslash, []byte("SELECT 1"), 0o644); err != nil {
		t.Fatalf("WriteFile(backslash): %v", err)
	}
	s := NewStore(base)
	if err := s.Delete("athena", "../evil"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete(../evil) = %v, want ErrNotFound", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside file must survive: %v", err)
	}
	if err := s.Delete("athena", ".hidden"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete(.hidden) = %v, want ErrNotFound", err)
	}
	if _, err := os.Stat(hidden); err != nil {
		t.Fatalf("hidden file must survive: %v", err)
	}
	// 後方互換の削除パスは `\` をパス区切りとして防御的に拒否する (issue 0150 の
	// 境界条件の追記のとおり、`\` を字面に含む手動配置ファイルは API から削除できない)
	if err := s.Delete("athena", `a\b`); !errors.Is(err, ErrNotFound) {
		t.Fatalf(`Delete(a\b) = %v, want ErrNotFound`, err)
	}
	if _, err := os.Stat(backslash); err != nil {
		t.Fatalf("backslash file must survive: %v", err)
	}
	// NUL を含む名前も後方互換パスへ流さない (流すと os.Remove が EINVAL を返し
	// ErrNotFound にならないため、この期待値が拒否条件の欠落を検出する)
	if err := s.Delete("athena", "x\x00y"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete(nul) = %v, want ErrNotFound", err)
	}
}

func TestStoreSaveListRoundTrip(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "snippets"))

	if _, err := s.Save("athena", "first", "SELECT 1"); err != nil {
		t.Fatalf("Save(first): %v", err)
	}
	if _, err := s.Save("athena", "second", "SELECT 2"); err != nil {
		t.Fatalf("Save(second): %v", err)
	}

	got, err := s.List("athena")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List returned %d snippets, want 2", len(got))
	}
	byName := map[string]Snippet{}
	for _, sn := range got {
		byName[sn.Name] = sn
	}
	if byName["first"].SQL != "SELECT 1" || byName["second"].SQL != "SELECT 2" {
		t.Errorf("List content mismatch: %+v", got)
	}
	for _, sn := range got {
		if sn.UpdatedAt.IsZero() {
			t.Errorf("UpdatedAt of %s is zero", sn.Name)
		}
	}
}

func TestStoreSeparatesServices(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if _, err := s.Save("athena", "a", "SELECT 1"); err != nil {
		t.Fatalf("Save(athena): %v", err)
	}
	if _, err := s.Save("bigquery", "b", "SELECT 2"); err != nil {
		t.Fatalf("Save(bigquery): %v", err)
	}

	// サービス別のサブディレクトリに保存される
	if _, err := os.Stat(filepath.Join(dir, "athena", "a.sql")); err != nil {
		t.Errorf("athena/a.sql: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "bigquery", "b.sql")); err != nil {
		t.Errorf("bigquery/b.sql: %v", err)
	}

	// 一覧は互いに混ざらない
	athena, err := s.List("athena")
	if err != nil {
		t.Fatalf("List(athena): %v", err)
	}
	if len(athena) != 1 || athena[0].Name != "a" {
		t.Errorf("List(athena) = %+v, want only a", athena)
	}
	bigquery, err := s.List("bigquery")
	if err != nil {
		t.Fatalf("List(bigquery): %v", err)
	}
	if len(bigquery) != 1 || bigquery[0].Name != "b" {
		t.Errorf("List(bigquery) = %+v, want only b", bigquery)
	}
}

func TestStoreRejectsInvalidService(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, err := s.List("redshift"); !errors.Is(err, ErrInvalidService) {
		t.Errorf("List = %v, want ErrInvalidService", err)
	}
	if _, err := s.Save("redshift", "q", "SELECT 1"); !errors.Is(err, ErrInvalidService) {
		t.Errorf("Save = %v, want ErrInvalidService", err)
	}
	if err := s.Delete("redshift", "q"); !errors.Is(err, ErrInvalidService) {
		t.Errorf("Delete = %v, want ErrInvalidService", err)
	}
}

func TestStoreSaveOverwrites(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, err := s.Save("athena", "q", "SELECT 1"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := s.Save("athena", "q", "SELECT 2"); err != nil {
		t.Fatalf("Save overwrite: %v", err)
	}
	got, err := s.List("athena")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].SQL != "SELECT 2" {
		t.Errorf("List = %+v, want single snippet with SELECT 2", got)
	}
}

func TestStoreSaveRejectsInvalidName(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, err := s.Save("athena", "", "SELECT 1"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("Save(empty) = %v, want ErrInvalidName", err)
	}
	// エンコード後のファイル名がバイト長上限を超える名前は拒否する
	long := strings.Repeat("/", maxNameLength/3+1)
	if _, err := s.Save("athena", long, "SELECT 1"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("Save(encoded too long) = %v, want ErrInvalidName", err)
	}
}

func TestStoreListSortsByModTimeDesc(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if _, err := s.Save("athena", "old", "SELECT 1"); err != nil {
		t.Fatalf("Save(old): %v", err)
	}
	if _, err := s.Save("athena", "new", "SELECT 2"); err != nil {
		t.Fatalf("Save(new): %v", err)
	}
	// mtime を明示的にずらしてソート順を検証する
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "athena", "old.sql"), past, past); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	got, err := s.List("athena")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 || got[0].Name != "new" || got[1].Name != "old" {
		t.Errorf("List order = %+v, want [new old]", got)
	}
}

func TestStoreListMissingDirReturnsEmpty(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "does-not-exist"))
	got, err := s.List("athena")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("List = %+v, want empty", got)
	}
}

func TestStoreListSkipsNonSnippetEntries(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "athena")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	s := NewStore(base)
	// 手動配置の .sql は一覧に載る
	if err := os.WriteFile(filepath.Join(dir, "manual.sql"), []byte("SELECT 3"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// .sql 以外・隠しファイル・ディレクトリは無視される
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".tmp-123"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".hidden.sql"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub.sql"), 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	got, err := s.List("athena")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].Name != "manual" || got[0].SQL != "SELECT 3" {
		t.Errorf("List = %+v, want only manual", got)
	}
}

func TestStoreListSkipsEntryRemovedAfterReadDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on windows")
	}
	base := t.TempDir()
	dir := filepath.Join(base, "athena")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "alive.sql"), []byte("SELECT 1"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// 列挙 (ReadDir) には載るが読み取り (Open) が fs.ErrNotExist になる状態を、
	// 参照先の無いシンボリックリンクで決定的に再現する。ReadDir と Open の間に
	// 別リクエストがファイルを削除した場合と Open が返すエラーが同じになる。
	if err := os.Symlink(filepath.Join(dir, "missing-target.sql"), filepath.Join(dir, "gone.sql")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	got, err := NewStore(base).List("athena")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].Name != "alive" || got[0].SQL != "SELECT 1" {
		t.Errorf("List = %+v, want only alive", got)
	}
}

func TestStoreListReturnsEmptyWhenAllEntriesRemovedAfterReadDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on windows")
	}
	base := t.TempDir()
	dir := filepath.Join(base, "athena")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// 列挙された全エントリが読み取り前に消えた場合も、エラーや nil ではなく
	// 空スライスを返す (JSON では null ではなく [] になる)
	if err := os.Symlink(filepath.Join(dir, "missing-target.sql"), filepath.Join(dir, "gone.sql")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	got, err := NewStore(base).List("athena")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("List = %#v, want empty non-nil slice", got)
	}
}

func TestStoreListReturnsSQLAndUpdatedAtFromSameVersionUnderConcurrentOverwrite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("renaming over a file that is open for reading is not guaranteed on windows")
	}
	base := t.TempDir()
	dir := filepath.Join(base, "athena")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	target := filepath.Join(dir, "q.sql")
	if err := os.WriteFile(target, []byte("0"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Save と同じく一時ファイルの rename で上書きし、版ごとに本文 (連番) と更新日時
	// (1 秒刻み) を一意にする。List が返す組が同じ版に属さなければ、本文と更新日時を
	// 別々にパスから解決している (issue 0158 の TOCTOU) ことになる。
	const versions = 1000
	var mu sync.Mutex
	modTimes := map[string]time.Time{"0": {}}
	// 書き込み goroutine は停止要求 (stop) を見て抜け、終了時に必ず done へ結果を送る。
	// 検証側は t.Fatalf の前に必ず stop を閉じて done を待つため、失敗時にも goroutine
	// が t.TempDir のクリーンアップと競合しない。
	stop := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		tmp := filepath.Join(dir, ".tmp-overwrite")
		for i := 1; i <= versions; i++ {
			select {
			case <-stop:
				done <- nil
				return
			default:
			}
			sql := strconv.Itoa(i)
			ts := time.Unix(1_700_000_000+int64(i), 0)
			if err := os.WriteFile(tmp, []byte(sql), 0o644); err != nil {
				done <- err
				return
			}
			if err := os.Chtimes(tmp, ts, ts); err != nil {
				done <- err
				return
			}
			mu.Lock()
			modTimes[sql] = ts.UTC()
			mu.Unlock()
			if err := os.Rename(tmp, target); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	// 検証側の失敗は goroutine を止めてから報告する
	fatalf := func(format string, args ...any) {
		t.Helper()
		close(stop)
		<-done
		t.Fatalf(format, args...)
	}
	store := NewStore(base)
	for {
		// 書き込み側が途中でエラーになると modTimes が増えなくなるため、先に検知して抜ける
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("overwrite: %v", err)
			}
			return
		default:
		}
		got, err := store.List("athena")
		if err != nil {
			fatalf("List: %v", err)
		}
		if len(got) != 1 {
			fatalf("List returned %d snippets, want 1", len(got))
		}
		mu.Lock()
		want, ok := modTimes[got[0].SQL]
		mu.Unlock()
		if !ok {
			fatalf("List returned unknown sql %q", got[0].SQL)
		}
		if got[0].SQL != "0" && !want.Equal(got[0].UpdatedAt) {
			fatalf("List returned sql %q with updated_at %v, want %v (sql and updated_at from different versions)", got[0].SQL, got[0].UpdatedAt, want)
		}
	}
}

func TestStoreListReturnsNonNotExistReadError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits do not restrict the owner on windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root can read files regardless of permission bits")
	}
	base := t.TempDir()
	dir := filepath.Join(base, "athena")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "alive.sql"), []byte("SELECT 1"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// 読み取り権限の無いファイルは fs.ErrNotExist ではなく fs.ErrPermission になり、
	// 従来どおり一覧全体のエラーとして返る
	if err := os.WriteFile(filepath.Join(dir, "locked.sql"), []byte("SELECT 2"), 0o000); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := NewStore(base).List("athena")
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("List err = %v, want fs.ErrPermission", err)
	}
	if got != nil {
		t.Errorf("List = %+v, want nil on error", got)
	}
}

func TestStoreDelete(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, err := s.Save("athena", "q", "SELECT 1"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Delete("athena", "q"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err := s.List("athena")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("List after delete = %+v, want empty", got)
	}
}

func TestStoreDeleteMissingReturnsNotFound(t *testing.T) {
	s := NewStore(t.TempDir())
	if err := s.Delete("athena", "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete(nope) = %v, want ErrNotFound", err)
	}
}

func TestStoreDeleteRejectsEmptyName(t *testing.T) {
	s := NewStore(t.TempDir())
	if err := s.Delete("athena", ""); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("Delete(empty) = %v, want ErrInvalidName", err)
	}
}

// TestStoreDeleteNameTooLongReturnsNotFound はファイル名長超過 (ENAMETOOLONG) になる
// 巨大な名前の削除が、OS エラーのままの 500 相当ではなく ErrNotFound になることを
// 検証する (存在しえない名前のため)。
func TestStoreDeleteNameTooLongReturnsNotFound(t *testing.T) {
	s := NewStore(t.TempDir())
	// 5000 バイトは主要なファイルシステムの NAME_MAX (darwin / linux とも一般に 255)
	// を確実に超え、os.Remove が ENAMETOOLONG を返す値
	if err := s.Delete("athena", strings.Repeat("a", 5000)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete(huge name) = %v, want ErrNotFound", err)
	}
}

func TestStoreSaveReturnsUpdatedAtMatchingList(t *testing.T) {
	s := NewStore(t.TempDir())
	saved, err := s.Save("athena", "q", "SELECT 1")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.UpdatedAt.IsZero() {
		t.Fatal("Save returned zero UpdatedAt")
	}
	if saved.UpdatedAt.Location() != time.UTC {
		t.Errorf("Save returned UpdatedAt in %v, want UTC", saved.UpdatedAt.Location())
	}
	got, err := s.List("athena")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("List returned %d snippets, want 1", len(got))
	}
	// Save は rename 前の一時ファイルから、List は rename 後の q.sql から更新日時を
	// 取るが、rename は inode を変えないため同じ値になる。
	if !got[0].UpdatedAt.Equal(saved.UpdatedAt) {
		t.Errorf("List UpdatedAt = %v, Save UpdatedAt = %v, want equal", got[0].UpdatedAt, saved.UpdatedAt)
	}
}

func TestStoreSaveReturnsOwnUpdatedAtWhenOverwrittenBetweenRenameAndStat(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "athena")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	target := filepath.Join(dir, "q.sql")
	// 別リクエストの後続の保存を、Save の rename の直後に q.sql を別の版 (本文 other、
	// 更新日時は過去の 2023 年) で上書きする renameFile の差し替えで決定的に再現する。
	// 確率的な同時実行では、macOS の rename(2) が同じ宛先への同時 rename で ENOENT を
	// 返す (issue 0161) ため再現に使えない。
	other := filepath.Join(dir, ".tmp-other")
	otherTime := time.Unix(1_700_000_000, 0)
	if err := os.WriteFile(other, []byte("other"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Chtimes(other, otherTime, otherTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	var tmpModTime time.Time
	prev := renameFile
	t.Cleanup(func() { renameFile = prev })
	renameFile = func(oldpath, newpath string) error {
		info, err := os.Stat(oldpath)
		if err != nil {
			return err
		}
		tmpModTime = info.ModTime()
		if err := os.Rename(oldpath, newpath); err != nil {
			return err
		}
		return os.Rename(other, newpath)
	}
	got, err := NewStore(base).Save("athena", "q", "mine")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got.SQL != "mine" {
		t.Errorf("Save returned sql %q, want %q", got.SQL, "mine")
	}
	if got.UpdatedAt.Equal(otherTime) {
		t.Fatalf("Save returned updated_at %v of the other version (taken from the path after rename)", got.UpdatedAt)
	}
	if !got.UpdatedAt.Equal(tmpModTime) {
		t.Errorf("Save returned updated_at %v, want %v (mod time of the temp file it wrote)", got.UpdatedAt, tmpModTime)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "other" {
		t.Errorf("q.sql = %q, want %q (the later version must win on disk)", data, "other")
	}
}

// TestStoreSaveSucceedsUnderConcurrentRenameToSameDestination は、保存先を別スレッドが
// 同時に rename で置き換えている間も Save が失敗しないことを確認する (issue 0161)。
// macOS では rename が宛先の解決に失敗して ENOENT を返すことがあり、修正前はこの
// テストが Save のエラーで失敗した。ENOENT が起きない環境 (Linux で実測) では、競合下でも
// Save が成功し続けることの確認になる。
func TestStoreSaveSucceedsUnderConcurrentRenameToSameDestination(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	serviceDir := filepath.Join(dir, "athena")
	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// 宛先を連続で置き換える writer 用の一時ファイルをあらかじめ用意する。writer 側を
	// rename だけのループにすることで、Save の rename と競合する時間の割合を上げる。
	const overwrites = 5000
	others := make([]string, overwrites)
	for i := range others {
		others[i] = filepath.Join(serviceDir, ".tmp-overwrite-"+strconv.Itoa(i))
		if err := os.WriteFile(others[i], []byte("other"), 0o644); err != nil {
			t.Fatalf("write %s: %v", others[i], err)
		}
	}

	target := filepath.Join(serviceDir, "q.sql")
	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, p := range others {
			// 競合を作ることが目的で、置き換えが成功したかは問わないため戻り値は使わない。
			_ = os.Rename(p, target)
		}
	}()

	var saveErr error
	saves := 0
loop:
	for {
		// writer の状態を見る前に Save を実行することで、writer が先に終わりきった環境でも
		// 保存が 1 回は行われることを保証する (保存が 0 回だと何も検証できずに通ってしまう)。
		if _, err := s.Save("athena", "q", "mine"); err != nil {
			saveErr = err
			break loop
		}
		saves++
		select {
		case <-done:
			break loop
		default:
		}
	}
	// 一時ディレクトリの削除が writer の rename と競合しないよう、writer の終了を待つ。
	<-done

	if saveErr != nil {
		t.Fatalf("Save under concurrent rename = %v (after %d successful saves), want nil", saveErr, saves)
	}
	// 保存先の本文が、Save か writer のどちらかが書いた本文そのものであること
	// (部分書き込みや消失が無いこと) を確認する。
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read %s: %v", target, err)
	}
	if string(got) != "mine" && string(got) != "other" {
		t.Errorf("q.sql = %q, want %q or %q", string(got), "mine", "other")
	}
}

// TestStoreSaveAbsorbsRenameEnoentFromDestinationContention は、Save が rename を
// renameSnippetFile 経由で呼び、宛先の競合に由来する ENOENT を吸収することを決定的に
// 検証する (issue 0161)。
//
// この配線 (Save が renameSnippetFile を通ること) を検証するのはこのテストだけである。
// renameSnippetFile を直接呼ぶ TestRenameSnippetFile 系は、Save の呼び出しが
// renameFile の直接呼び出しへ戻る退行を検出できない。
// TestStoreSaveSucceedsUnderConcurrentRenameToSameDestination は同じ退行を検出できるが、
// カーネルが ENOENT を返すかどうかがタイミングに依存し、修正前のコードでも 10 回中 2 回は
// 通過する (issue 0161 の実測)。
//
// renameFile はパッケージ全体で共有する変数のため、このテストでは t.Parallel を使わない。
func TestStoreSaveAbsorbsRenameEnoentFromDestinationContention(t *testing.T) {
	s := NewStore(t.TempDir())

	calls := 0
	prev := renameFile
	t.Cleanup(func() { renameFile = prev })
	renameFile = func(o, n string) error {
		calls++
		if calls == 1 {
			// 宛先が同時に置き換えられている間に macOS が返す ENOENT と同じ形にする。
			// ソースの一時ファイルは消さないので、再試行の条件を満たす。
			return &os.LinkError{Op: "rename", Old: o, New: n, Err: syscall.ENOENT}
		}
		return prev(o, n)
	}

	got, err := s.Save("athena", "q", "mine")
	if err != nil {
		t.Fatalf("Save() = %v, want nil", err)
	}
	if calls != 2 {
		t.Errorf("renameFile called %d times, want 2", calls)
	}
	if got.SQL != "mine" {
		t.Errorf("Save().SQL = %q, want %q", got.SQL, "mine")
	}

	// 吸収された ENOENT の後に、保存先が実際に書かれていること (Save が成功を返しただけで
	// 終わっていないこと) を読み出しで確認する。
	list, err := s.List("athena")
	if err != nil {
		t.Fatalf("List() = %v, want nil", err)
	}
	if len(list) != 1 {
		t.Fatalf("List() returned %d snippets, want 1", len(list))
	}
	if list[0].Name != "q" || list[0].SQL != "mine" {
		t.Errorf("List()[0] = {Name: %q, SQL: %q}, want {Name: %q, SQL: %q}", list[0].Name, list[0].SQL, "q", "mine")
	}
}

// TestRenameSnippetFile は rename の失敗に対する再試行の判別条件を、renameFile を
// 差し替えて決定的に検証する (issue 0161)。renameFile はパッケージ全体で共有する変数の
// ため、このテストでは t.Parallel を使わない。
func TestRenameSnippetFile(t *testing.T) {
	tests := []struct {
		name string
		// failures は rename が失敗を返す回数。これを超えた呼び出しは実際に rename する。
		failures int
		// errno は失敗時に返すエラー番号。
		errno syscall.Errno
		// removeSource は失敗を返す前にソースを削除するか (ソースが消えている状況の再現)。
		removeSource bool
		wantCalls    int
		wantErr      error
		// wantMsgContains はエラーメッセージに含まれるべき文字列 (空なら検査しない)。
		wantMsgContains string
		// wantRenamed は最終的にソースが保存先へ移動しているべきか。
		wantRenamed bool
	}{
		{
			name:        "succeeds on the first attempt",
			wantCalls:   1,
			wantRenamed: true,
		},
		{
			name:        "retries enoent while the source exists",
			failures:    3,
			errno:       syscall.ENOENT,
			wantCalls:   4,
			wantRenamed: true,
		},
		{
			name:            "gives up at the attempt limit",
			failures:        renameAttemptLimit,
			errno:           syscall.ENOENT,
			wantCalls:       renameAttemptLimit,
			wantErr:         os.ErrNotExist,
			wantMsgContains: "give up after " + strconv.Itoa(renameAttemptLimit) + " rename attempts",
		},
		{
			name:         "does not retry enoent when the source is gone",
			failures:     1,
			errno:        syscall.ENOENT,
			removeSource: true,
			wantCalls:    1,
			wantErr:      os.ErrNotExist,
		},
		{
			name:      "does not retry errors other than enoent",
			failures:  1,
			errno:     syscall.EACCES,
			wantCalls: 1,
			wantErr:   os.ErrPermission,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			oldpath := filepath.Join(dir, ".tmp-source")
			newpath := filepath.Join(dir, "q.sql")
			if err := os.WriteFile(oldpath, []byte("mine"), 0o644); err != nil {
				t.Fatalf("write source: %v", err)
			}

			calls := 0
			prev := renameFile
			t.Cleanup(func() { renameFile = prev })
			renameFile = func(o, n string) error {
				calls++
				if calls > tt.failures {
					return prev(o, n)
				}
				if tt.removeSource {
					if err := os.Remove(o); err != nil {
						t.Fatalf("remove source: %v", err)
					}
				}
				// os.Rename が返すのと同じ形のエラーにする (errors.Is が Errno まで辿れること
				// 自体も検証の対象になる)。
				return &os.LinkError{Op: "rename", Old: o, New: n, Err: tt.errno}
			}

			err := renameSnippetFile(oldpath, newpath)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("renameSnippetFile() = %v, want error matching %v", err, tt.wantErr)
			}
			if tt.wantMsgContains != "" && !strings.Contains(err.Error(), tt.wantMsgContains) {
				t.Errorf("error message = %q, want it to contain %q", err.Error(), tt.wantMsgContains)
			}
			if calls != tt.wantCalls {
				t.Errorf("renameFile called %d times, want %d", calls, tt.wantCalls)
			}

			got, readErr := os.ReadFile(newpath)
			if !tt.wantRenamed {
				if readErr == nil {
					t.Errorf("%s exists with %q, want it not to be created", newpath, string(got))
				}
				return
			}
			if readErr != nil {
				t.Fatalf("read %s: %v", newpath, readErr)
			}
			if string(got) != "mine" {
				t.Errorf("%s = %q, want %q", newpath, string(got), "mine")
			}
		})
	}
}

// TestRenameSnippetFileDoesNotRetryWhenSourceCannotBeStatted は、ソースの os.Stat が
// fs.ErrNotExist 以外の理由で失敗し、ソースが存在するとは確認できない場合に再試行しない
// ことを検証する (issue 0161 の修正方針)。存在の確認を「Stat が fs.ErrNotExist ではない」で
// 代用すると、この経路で上限まで再試行してしまう。
//
// Stat を失敗させる理由には、ファイル名長の上限超過 (ENAMETOOLONG) を使う。ディレクトリの
// 権限を落とす方法は root で実行した場合に Stat が成功してしまうため、実行ユーザに依存しない
// こちらを選んだ。
//
// renameFile はパッケージ全体で共有する変数のため、このテストでは t.Parallel を使わない
// (TestRenameSnippetFile と同じ理由)。
func TestRenameSnippetFileDoesNotRetryWhenSourceCannotBeStatted(t *testing.T) {
	dir := t.TempDir()
	// 単一コンポーネントの長さが NAME_MAX (macOS と Linux はいずれも 255) を超えるパス。
	oldpath := filepath.Join(dir, strings.Repeat("a", 512))
	newpath := filepath.Join(dir, "q.sql")
	if _, err := os.Stat(oldpath); err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("os.Stat(oldpath) = %v, want a failure other than fs.ErrNotExist", err)
	}

	calls := 0
	prev := renameFile
	t.Cleanup(func() { renameFile = prev })
	renameFile = func(o, n string) error {
		calls++
		return &os.LinkError{Op: "rename", Old: o, New: n, Err: syscall.ENOENT}
	}

	err := renameSnippetFile(oldpath, newpath)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("renameSnippetFile() = %v, want error matching %v", err, os.ErrNotExist)
	}
	if calls != 1 {
		t.Errorf("renameFile called %d times, want 1", calls)
	}
}

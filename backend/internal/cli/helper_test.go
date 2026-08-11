package cli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

func TestStripOneTrailingNewline(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "no trailing newline", in: "secret", want: "secret"},
		{name: "lf", in: "secret\n", want: "secret"},
		{name: "crlf", in: "secret\r\n", want: "secret"},
		{name: "only one of multiple lf", in: "secret\n\n", want: "secret\n"},
		{name: "empty", in: "", want: ""},
		{name: "single lf", in: "\n", want: ""},
		{name: "internal newlines preserved", in: "a\nb\n", want: "a\nb"},
		{name: "lone cr is not stripped", in: "secret\r", want: "secret\r"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripOneTrailingNewline(tt.in); got != tt.want {
				t.Errorf("stripOneTrailingNewline(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// blockingReader は release が閉じられるまで Read から返らない。端末で入力を待っている
// 状態を模す。
type blockingReader struct {
	release chan struct{}
}

func (r blockingReader) Read([]byte) (int, error) {
	<-r.release
	return 0, io.EOF
}

// newBlockingReader は入力待ちで止まる Reader を返し、テストの終了時に解放する。
// 解放しないと読み取りの goroutine がテストプロセスの終了まで残る。
func newBlockingReader(t *testing.T) blockingReader {
	t.Helper()
	r := blockingReader{release: make(chan struct{})}
	t.Cleanup(func() { close(r.release) })
	return r
}

// unreadableReader は Read が呼ばれるとテストを失敗させる。stdin を読まないことの検証に使う。
type unreadableReader struct {
	t *testing.T
}

func (r unreadableReader) Read([]byte) (int, error) {
	r.t.Helper()
	r.t.Error("stdin was read, want it left untouched")
	return 0, io.EOF
}

func TestReadWithContextReturnsWhatReadReturns(t *testing.T) {
	wantErr := errors.New("boom")

	got, err := readWithContext(context.Background(), func() (int, error) { return 42, wantErr })
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if got != 42 {
		t.Errorf("value = %d, want 42", got)
	}
}

// TestReadWithContextDoesNotWaitForAnAlreadyCanceledContext は、キャンセル済みの
// context では読み取りの完了を待たないことを検証する。
func TestReadWithContextDoesNotWaitForAnAlreadyCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	got, err := readWithContext(ctx, func() (string, error) {
		<-release
		return "too late", nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want it to wrap %v", err, context.Canceled)
	}
	if got != "" {
		t.Errorf("value = %q, want the zero value", got)
	}
}

// TestReadWithContextStopsWaitingWhenCanceledWhileReading は、読み取りの途中で
// context がキャンセルされた場合にその場で戻ることを検証する。
//
// 実際の Ctrl-C はこの順序で届く。キャンセル済みの context を渡す検証だけでは、
// select を素の受信に落としても気付けない。
func TestReadWithContextStopsWaitingWhenCanceledWhileReading(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	reading := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	// 入力待ちに入ったのを見てからシグナル相当のキャンセルを起こす。
	go func() {
		<-reading
		cancel()
	}()

	got, err := readWithContext(ctx, func() (string, error) {
		close(reading)
		<-release
		return "too late", nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want it to wrap %v", err, context.Canceled)
	}
	if got != "" {
		t.Errorf("value = %q, want the zero value", got)
	}
}

// TestReadWithContextLetsTheReaderFinishAfterCancellation は、キャンセルで先に戻った後も
// 読み取りの goroutine が結果の送信で詰まらずに終われることを検証する。
//
// 結果を渡すチャネルのバッファを外すと、誰も受け取らない送信で goroutine が永久に残る。
// 残っても他の検証には影響しないため、goroutine の数が戻ることを直接見る。
//
// internal/cli のテストは t.Parallel() を使っていないため、この検証と並行して走る
// テストは無い。数が一時的に増えることはあるので、落ち着くまで待って判定する。
func TestReadWithContextLetsTheReaderFinishAfterCancellation(t *testing.T) {
	before := settledGoroutineCount(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	release := make(chan struct{})
	if _, err := readWithContext(ctx, func() (string, error) {
		<-release
		return "too late", nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want it to wrap %v", err, context.Canceled)
	}

	// 読み取りを終わらせる。ここから goroutine は結果を送って終了できるはずである。
	close(release)

	if got := settledGoroutineCount(t); got > before {
		t.Errorf("goroutines = %d, want it back to %d; the reader goroutine is stuck sending its result", got, before)
	}
}

// settledGoroutineCount は goroutine の数が落ち着くまで待ってその数を返す。
//
// goroutine の終了は非同期であり、直前のテストが残したものが消えるまでの間に数を取ると
// 基準値が多めに出る。基準値が 1 つ多いと、リークした 1 つを見逃す。
//
// internal/util の executer_test.go に同名のヘルパーがある。パッケージをまたいで
// テスト用のヘルパーを共有する手段が無いため、それぞれに置いている。
func settledGoroutineCount(t *testing.T) int {
	t.Helper()

	const (
		interval      = 10 * time.Millisecond
		stableSamples = 3
	)

	deadline := time.Now().Add(2 * time.Second)
	count := runtime.NumGoroutine()
	stable := 0
	for stable < stableSamples && time.Now().Before(deadline) {
		time.Sleep(interval)
		got := runtime.NumGoroutine()
		if got == count {
			stable++
			continue
		}
		count = got
		stable = 0
	}
	return count
}

func TestPromptSelection(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "all", input: "all\n", want: "all"},
		{name: "comma separated numbers", input: "1,2\n", want: "1,2"},
		{name: "no trailing newline", input: "all", want: "all"},
		{name: "reads only the first line", input: "1,2\n3\n", want: "1,2"},
		// 読み取れなかった入力は空として扱い、選択なしの判断を呼び出し側へ渡す。
		{name: "empty line", input: "\n", want: ""},
		{name: "whitespace only line", input: "   \n", want: ""},
		{name: "eof", input: "", want: ""},
		{name: "no trailing newline with surrounding whitespace", input: "  1,2  ", want: "1,2"},
		// カンマの後ろに空白を挟んだ書き方 (プロンプトの文言どおりの入力) を通す。
		// 各要素の TrimSpace は selectIndices が担う。
		{name: "space inside the selection", input: "1, 2\n", want: "1, 2"},
		{name: "space around each element", input: "1 , 2\n", want: "1 , 2"},
		{name: "all with surrounding whitespace", input: "  all  \n", want: "all"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := promptSelection(context.Background(), bufio.NewReader(strings.NewReader(tt.input)))
			if err != nil {
				t.Fatalf("promptSelection() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("promptSelection(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestPromptSelectionReusesTheSameReaderAcrossCalls は、sso generate-config がアカウント
// 選択とロール選択で promptSelection を複数回呼ぶ際に、1 回目の呼び出しが先読みした分を
// 2 回目以降の呼び出しでも読めることを検証する。
//
// bufio.Reader は下層の Reader から一度の Read で複数行分をまとめて読み込むことがある。
// 呼び出しのたびに新しい bufio.Reader を作ると、その先読み分は使い捨てられた Reader の
// 内部バッファに閉じ込められたまま失われ、2 回目の呼び出しはまだ入力が残っているのに
// 空扱いになる (sso generate-config ではロール選択がこれに当たる)。
func TestPromptSelectionReusesTheSameReaderAcrossCalls(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("1,2\n3\n"))

	first, err := promptSelection(context.Background(), r)
	if err != nil {
		t.Fatalf("promptSelection() first call error = %v", err)
	}
	if first != "1,2" {
		t.Fatalf("first call = %q, want %q", first, "1,2")
	}

	second, err := promptSelection(context.Background(), r)
	if err != nil {
		t.Fatalf("promptSelection() second call error = %v", err)
	}
	if second != "3" {
		t.Errorf("second call = %q, want %q; the same *bufio.Reader must be reused across calls", second, "3")
	}
}

// erroringReader は Read のたびに常に同じエラーを返す。
type erroringReader struct {
	err error
}

func (r erroringReader) Read([]byte) (int, error) {
	return 0, r.err
}

// TestPromptSelectionPropagatesNonEOFReadErrors は、EOF 以外の読み取りエラーが空文字へ
// 握り潰されず呼び出し側へ伝わることを検証する。
//
// bufio.Reader は下層のエラーを内部に保持し以降の呼び出しでも返し続けるため、ここで
// 空文字に握り潰すと、一度エラーが起きた後の選択がすべてエラーの表示無く空になる。
func TestPromptSelectionPropagatesNonEOFReadErrors(t *testing.T) {
	wantErr := errors.New("boom")
	r := bufio.NewReader(erroringReader{err: wantErr})

	got, err := promptSelection(context.Background(), r)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want it to wrap %v", err, wantErr)
	}
	if got != "" {
		t.Errorf("value = %q, want the zero value", got)
	}
}

// TestPromptSelectionStopsWaitingWhenContextIsCanceled は、対話式の選択の入力待ちが
// context のキャンセルで打ち切られることを検証する。
//
// これが効かないと、sso generate-config のアカウント選択とロール選択で Ctrl-C が
// 無反応になる。
func TestPromptSelectionStopsWaitingWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := promptSelection(ctx, bufio.NewReader(newBlockingReader(t)))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want it to wrap %v", err, context.Canceled)
	}
	if got != "" {
		t.Errorf("value = %q, want the zero value", got)
	}
}

func TestReadUpdateValue(t *testing.T) {
	tests := []struct {
		name  string
		value string
		stdin string
		want  string
	}{
		{name: "value flag wins", value: "from-flag", stdin: "from-stdin\n", want: "from-flag"},
		{name: "stdin with trailing newline", stdin: "secret\n", want: "secret"},
		{name: "stdin without trailing newline", stdin: "secret", want: "secret"},
		{name: "empty stdin", stdin: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newUpdateValueCmd(t, tt.value)

			got, err := readUpdateValue(context.Background(), cmd, strings.NewReader(tt.stdin))
			if err != nil {
				t.Fatalf("readUpdateValue() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("readUpdateValue() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestReadUpdateValueSkipsStdinWhenValueFlagIsSet は、--value がある場合に stdin を
// 読まないことを検証する。読んでしまうと、端末から実行したときに入力待ちで止まる。
func TestReadUpdateValueSkipsStdinWhenValueFlagIsSet(t *testing.T) {
	cmd := newUpdateValueCmd(t, "from-flag")

	got, err := readUpdateValue(context.Background(), cmd, unreadableReader{t: t})
	if err != nil {
		t.Fatalf("readUpdateValue() error = %v", err)
	}
	if got != "from-flag" {
		t.Errorf("readUpdateValue() = %q, want %q", got, "from-flag")
	}
}

// TestReadUpdateValueStopsWaitingWhenContextIsCanceled は、stdin の読み取り待ちが
// context のキャンセルで打ち切られることを検証する。
//
// --value を渡し忘れて端末から実行すると入力待ちになる。そこから Ctrl-C で戻れる必要がある。
func TestReadUpdateValueStopsWaitingWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cmd := newUpdateValueCmd(t, "")

	got, err := readUpdateValue(ctx, cmd, newBlockingReader(t))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want it to wrap %v", err, context.Canceled)
	}
	if got != "" {
		t.Errorf("value = %q, want the zero value", got)
	}
}

// newUpdateValueCmd は --value を持つコマンドを返す。value が空文字でない場合は
// 明示指定されたものとして Changed を立てる。
func newUpdateValueCmd(t *testing.T, value string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "put"}
	cmd.Flags().String("value", "", "New value")
	if value != "" {
		if err := cmd.Flags().Set("value", value); err != nil {
			t.Fatalf("set --value: %v", err)
		}
	}
	return cmd
}

// runListCtxKey は runList に渡した context を同一性で見分けるための鍵。
type runListCtxKey struct{}

// stubRow は runList の型引数を満たすだけの最小の実装。
type stubRow struct{}

func (stubRow) ToRow() []string { return nil }

// TestRunListPassesCommandContextToFetch は runList が Fetch へコマンドに載った
// context をそのまま渡すことを検証する。
//
// runList は ls 系のほぼ全コマンドが通る共通経路である。ここで context が差し替わると、
// 個々のコマンドが正しく書かれていても Ctrl-C と SIGTERM がどれにも届かなくなる。
//
// Fetch が空の一覧を返す経路を使う。printRowsOrGroupBy の先の TableFormatter は
// os.Stdout へ直接書くため、空でない一覧ではテストから出力を確認できない。
func TestRunListPassesCommandContextToFetch(t *testing.T) {
	// loadConfig の先の config.Load は $XDG_CONFIG_HOME/thief/config.yaml と
	// $HOME/.thief/config.yaml、それにカレントディレクトリの config.yaml を読む。
	// 実行環境の設定でテストの結果が変わらないよう、3 つすべてを空に向ける。
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir())

	want := context.WithValue(context.Background(), runListCtxKey{}, "carried")

	cmd := &cobra.Command{Use: "ls"}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetContext(want)

	var got context.Context
	calls := 0
	err := runList(cmd, ListConfig[stubRow]{
		Columns:  []util.Column{{Header: "NAME"}},
		EmptyMsg: "No resources found",
		Fetch: func(ctx context.Context, cfg *config.Config) ([]stubRow, error) {
			calls++
			got = ctx
			if cfg == nil {
				t.Error("Fetch received cfg = nil, want a loaded config")
			}
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("runList() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("Fetch called %d times, want 1", calls)
	}
	// 同一性で比較する。派生や差し替えを行っていれば別のインスタンスになる。
	if got != want {
		t.Errorf("Fetch received ctx = %v, want the context set on the command", got)
	}
	if out.String() != "No resources found\n" {
		t.Errorf("output = %q, want %q", out.String(), "No resources found\n")
	}
}

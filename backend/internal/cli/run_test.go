package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/status"
)

// runCtxKey は context に載せた値を取り出して同一性を確かめるためのキー。
type runCtxKey struct{}

func TestCommandContextReturnsTheContextSetOnTheCommand(t *testing.T) {
	cmd := &cobra.Command{}
	want := context.WithValue(context.Background(), runCtxKey{}, "carried")
	cmd.SetContext(want)

	got := commandContext(cmd)
	if got != want {
		t.Fatalf("commandContext() = %v, want the context set on the command", got)
	}
	// 値まで確かめる。別の context を返していないことを、同一性とは別の観点で押さえる。
	if v := got.Value(runCtxKey{}); v != "carried" {
		t.Errorf("ctx.Value(runCtxKey{}) = %v, want %q", v, "carried")
	}
}

func TestCommandContextFallsBackToBackground(t *testing.T) {
	cmd := &cobra.Command{}

	// cobra の Command.Context は内部フィールドをそのまま返すため、Execute 系も
	// SetContext も通らないコマンドでは nil になる。フォールバックが必要な理由は
	// この前提にある。cobra 側が Background を返すようになったら、ここが落ちて
	// commandContext の存在意義を見直すきっかけになる。
	if cmd.Context() != nil {
		t.Fatal("cobra.Command.Context() != nil; commandContext のフォールバックの前提が変わった")
	}

	got := commandContext(cmd)
	if got == nil {
		t.Fatal("commandContext() = nil, want non-nil")
	}
	if err := got.Err(); err != nil {
		t.Errorf("commandContext().Err() = %v, want nil", err)
	}
	if _, ok := got.Deadline(); ok {
		t.Error("commandContext() has a deadline, want none")
	}
}

func TestRunPassesContextToTheResolvedCommand(t *testing.T) {
	want := context.WithValue(context.Background(), runCtxKey{}, "from-run")

	var got context.Context
	root := newQuietRoot()
	root.AddCommand(&cobra.Command{
		Use: "probe",
		RunE: func(cmd *cobra.Command, args []string) error {
			got = commandContext(cmd)
			return nil
		},
	})
	root.SetArgs([]string{"probe"})

	if code := Run(want, root, io.Discard); code != 0 {
		t.Fatalf("Run() = %d, want 0", code)
	}
	if got != want {
		t.Fatalf("commandContext(cmd) = %v, want the context passed to Run", got)
	}
}

func TestRunMapsCanceledToInterruptExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "bare", err: context.Canceled},
		// AWS SDK は中断を自前のエラーに包んで返す。errors.Is で辿れることを押さえる。
		{name: "wrapped", err: fmt.Errorf("operation error EC2: DescribeInstances: %w", context.Canceled)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newQuietRoot()
			root.AddCommand(&cobra.Command{
				Use:  "probe",
				RunE: func(cmd *cobra.Command, args []string) error { return tt.err },
			})
			root.SetArgs([]string{"probe"})

			var stderr bytes.Buffer
			code := Run(context.Background(), root, &stderr)

			if code != interruptExitCode {
				t.Errorf("Run() = %d, want %d", code, interruptExitCode)
			}
			// 中断は異常ではない。context canceled の生の文言も usage への誘導も出さない。
			if got := stderr.String(); got != "interrupted\n" {
				t.Errorf("stderr = %q, want %q", got, "interrupted\n")
			}
		})
	}
}

// TestRunTreatsFailureAfterCancellationAsInterrupt は、ラップの連鎖が切れていて
// errors.Is では中断と判別できない失敗も、ctx がキャンセルされていれば中断として
// 扱うことを検証する。
//
// gRPC を使う Google Cloud の呼び出しは context のキャンセルを codes.Canceled の
// ステータスへ写して返す。この形は Unwrap で context.Canceled へ到達できないため、
// errors.Is だけに頼ると Cloud Logging などの Ctrl-C が中断として扱われない。
func TestRunTreatsFailureAfterCancellationAsInterrupt(t *testing.T) {
	// gRPC が実際に返す形。errors.Is で辿れないという前提をここで固定する。
	// grpc-go が Unwrap を持つようになったら落ちて、フォールバックの必要性を見直せる。
	grpcErr := status.FromContextError(context.Canceled).Err()
	if errors.Is(grpcErr, context.Canceled) {
		t.Fatal("errors.Is(grpc canceled, context.Canceled) = true; フォールバックの前提が変わった")
	}

	tests := []struct {
		name string
		err  error
		// wantStderr は stderr の全文。中断は errors.Is で辿れる場合と揃えて
		// interrupted で始める 1 行にし、辿れない分だけ元の文言を添える。
		// 文言を捨てると何が起きたか分からなくなる。
		wantStderr string
	}{
		{
			name:       "grpc canceled status",
			err:        grpcErr,
			wantStderr: "interrupted: rpc error: code = Canceled desc = context canceled\n",
		},
		{
			// 連鎖を保たない任意の失敗。中断後に浮上したものは中断として扱う。
			name:       "unrelated error surfaced after cancellation",
			err:        errors.New("list log entries: boom"),
			wantStderr: "interrupted: list log entries: boom\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 中断でキャンセル済みになった ctx を再現する。
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			root := newQuietRoot()
			root.AddCommand(&cobra.Command{
				Use:  "probe",
				RunE: func(cmd *cobra.Command, args []string) error { return tt.err },
			})
			root.SetArgs([]string{"probe"})

			var stderr bytes.Buffer
			code := Run(ctx, root, &stderr)

			if code != interruptExitCode {
				t.Errorf("Run() = %d, want %d", code, interruptExitCode)
			}
			// Error: を名乗らないこと自体が仕様である。実行時の失敗と混ざらない。
			if got := stderr.String(); got != tt.wantStderr {
				t.Errorf("stderr = %q, want %q", got, tt.wantStderr)
			}
		})
	}
}

func TestRunReportsRuntimeFailureWithoutUsageHint(t *testing.T) {
	root := newQuietRoot()
	root.AddCommand(&cobra.Command{
		Use:  "probe",
		RunE: func(cmd *cobra.Command, args []string) error { return errors.New("list EC2 instances: boom") },
	})
	root.SetArgs([]string{"probe"})

	var stderr bytes.Buffer
	code := Run(context.Background(), root, &stderr)

	if code != 1 {
		t.Errorf("Run() = %d, want 1", code)
	}
	// 実行時の失敗で help を案内しても、利用者にできることは無い。
	if got := stderr.String(); got != "Error: list EC2 instances: boom\n" {
		t.Errorf("stderr = %q, want %q", got, "Error: list EC2 instances: boom\n")
	}
}

// TestRunReportsFailureOfALazilyRegisteredCommandWithoutUsageHint は cobra が自前で足す
// コマンドの実行時の失敗も、使い方の誤りとして扱わないことを検証する。
//
// cobra は help と completion を ExecuteContextC の中で足す。Run が markCommandBodyRun を
// 呼ぶ前にこれらを登録させていないと、木を辿った時点でまだ存在せず本体が包まれない。
// 本体に入ったことが記録されないため、実行時の失敗が使い方の誤りとして扱われる。
//
// completion bash を使うのは、本体に入ってから失敗する経路を外から作れる唯一の
// 自前コマンドだからである。本体は登録時の出力先へ書き込み、その失敗をそのまま返す。
func TestRunReportsFailureOfALazilyRegisteredCommandWithoutUsageHint(t *testing.T) {
	root := newQuietRoot()
	// completion コマンドが登録されるのは、他に本物のサブコマンドがある場合だけである。
	root.AddCommand(&cobra.Command{
		Use:  "probe",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	})
	// 出力先は completion コマンドの登録時に捕まえられる。Run を呼ぶ前に差し替える。
	root.SetOut(failingWriter{err: errors.New("no space left on device")})
	root.SetArgs([]string{"completion", "bash"})

	var stderr bytes.Buffer
	code := Run(context.Background(), root, &stderr)

	if code != 1 {
		t.Fatalf("Run() = %d, want 1 (stderr = %q)", code, stderr.String())
	}
	want := "Error: no space left on device\n"
	if got := stderr.String(); got != want {
		t.Errorf("stderr = %q, want %q; 本体に入った後の失敗に help を案内してはならない", got, want)
	}
}

// TestRunReportsFailureOfACustomHelpCommandWithoutUsageHint は help コマンドも
// markCommandBodyRun より前に登録させていることを検証する。
//
// cobra が既定で使う help コマンドは Run (非 E) であり、内部の失敗を CheckErr で
// プロセスの終了に直接変えるため、Run へはエラーが届かない。既定のままでは包んでも
// 挙動は変わらないが、SetHelpCommand で差し替えれば RunE を持つ help コマンドになり、
// そこでの失敗は Run が受け取る。後から足されるという性質は completion と同じであり、
// 登録を先に済ませていなければ包まれず、使い方の誤りとして扱われる。
func TestRunReportsFailureOfACustomHelpCommandWithoutUsageHint(t *testing.T) {
	root := newQuietRoot()
	// help コマンドが登録されるのは、サブコマンドがある場合だけである。
	root.AddCommand(&cobra.Command{
		Use:  "probe",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	})
	// SetHelpCommand は木に足さず、差し替え先を覚えるだけである。木に入るのは
	// InitDefaultHelpCmd が呼ばれた時点である。
	root.SetHelpCommand(&cobra.Command{
		Use:  "help",
		RunE: func(cmd *cobra.Command, args []string) error { return errors.New("render help: boom") },
	})
	root.SetArgs([]string{"help"})

	var stderr bytes.Buffer
	code := Run(context.Background(), root, &stderr)

	if code != 1 {
		t.Fatalf("Run() = %d, want 1 (stderr = %q)", code, stderr.String())
	}
	want := "Error: render help: boom\n"
	if got := stderr.String(); got != want {
		t.Errorf("stderr = %q, want %q; 本体に入った後の失敗に help を案内してはならない", got, want)
	}
}

// failingWriter は書き込みが必ず失敗する io.Writer。
type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

// TestMarkCommandBodyRunWrapsBothBodyKinds は包んだ本体が元のとおり呼ばれ、かつ呼ばれた
// ことが記録されることを Run と RunE の両方で検証する。
//
// Run (非 E) の分岐は cli の本番コードには無く、cobra が自前で足す help コマンドが使う。
// エラーを返せないコマンドは本体に入った後に失敗しないため、記録の有無を Run の
// 終了コードからは観測できない。ここで直接確かめる。
func TestMarkCommandBodyRunWrapsBothBodyKinds(t *testing.T) {
	tests := []struct {
		name string
		// newCmd は本体が呼ばれたら called を立てるサブコマンドを返す。
		newCmd func(called *bool) *cobra.Command
	}{
		{
			name: "RunE",
			newCmd: func(called *bool) *cobra.Command {
				return &cobra.Command{
					Use:  "sub",
					RunE: func(cmd *cobra.Command, args []string) error { *called = true; return nil },
				}
			},
		},
		{
			name: "Run",
			newCmd: func(called *bool) *cobra.Command {
				return &cobra.Command{
					Use: "sub",
					Run: func(cmd *cobra.Command, args []string) { *called = true },
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var called bool
			root := newQuietRoot()
			root.AddCommand(tt.newCmd(&called))
			root.SetArgs([]string{"sub"})

			var ran bool
			markCommandBodyRun(root, &ran)

			if ran {
				t.Fatal("ran = true before the command ran")
			}
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() = %v, want nil", err)
			}
			if !ran {
				t.Error("ran = false, want true; 本体に入ったことが記録されていない")
			}
			if !called {
				t.Error("元の本体が呼ばれていない; 包むときに呼び出しを落としている")
			}
		})
	}
}

// TestRunOnRealRoot は本番の root コマンドを通して終了コードと出力を検証する。
// 引数は AWS へ接続する前に処理が終わるものだけを選んでいる。
func TestRunOnRealRoot(t *testing.T) {
	tests := []struct {
		name string
		args []string
		// wantCode は Run の返り値。
		wantCode int
		// wantErrContains は stderr に含まれていてほしい断片。文言は cobra が決めるため
		// 全文は固定しない。
		wantErrContains []string
		// wantHint は Run が足す help への誘導。空文字列なら出ないことを要求する。
		wantHint string
	}{
		{
			name:     "help succeeds",
			args:     []string{"--help"},
			wantCode: 0,
		},
		{
			// cobra が自前で足す help コマンド。Run (非 E) で定義されており、Run が
			// markCommandBodyRun より前に登録させていなければ包まれない。
			name:     "help subcommand succeeds",
			args:     []string{"help", "ec2"},
			wantCode: 0,
		},
		{
			// 未知のコマンドは Find がコマンドを解決する前に失敗する。本体に入っていない
			// ため、Run は使い方の誤りと判定する。
			name:            "unknown command",
			args:            []string{"no-such-command"},
			wantCode:        1,
			wantErrContains: []string{`unknown command "no-such-command"`},
			wantHint:        "Run 'thief --help' for usage.\n",
		},
		{
			// ルートの永続フラグの解析失敗。cobra は FlagErrorFunc を通して返すが、
			// 既定の FlagErrorFunc は受け取ったエラーをそのまま返すため、他の検査の
			// 失敗と区別できる目印は付かない。
			name:            "unknown flag on root",
			args:            []string{"--no-such-flag"},
			wantCode:        1,
			wantErrContains: []string{"unknown flag: --no-such-flag"},
			wantHint:        "Run 'thief --help' for usage.\n",
		},
		{
			// サブコマンドの解析失敗では、そのサブコマンドのパスを案内する。
			// ルートのパスを案内しても目的のフラグ一覧にたどり着けない。
			name:            "unknown flag on subcommand",
			args:            []string{"ec2", "ls", "--no-such-flag"},
			wantCode:        1,
			wantErrContains: []string{"unknown flag: --no-such-flag"},
			wantHint:        "Run 'thief ec2 ls --help' for usage.\n",
		},
		{
			// 位置引数の個数の検証失敗。cobra は Args をコマンドの本体より前に評価し、
			// FlagErrorFunc を通さない素のエラーを返す。
			name:            "wrong number of positional args",
			args:            []string{"cfn", "describe"},
			wantCode:        1,
			wantErrContains: []string{"accepts 1 arg(s), received 0"},
			wantHint:        "Run 'thief cfn describe --help' for usage.\n",
		},
		{
			// 必須フラグの検証失敗。これも本体より前で、FlagErrorFunc を通らない。
			name:            "required flag not set",
			args:            []string{"bq", "table", "ls"},
			wantCode:        1,
			wantErrContains: []string{`required flag(s) "dataset" not set`},
			wantHint:        "Run 'thief bq table ls --help' for usage.\n",
		},
		{
			// RunE が返した実行時の失敗。AWS へ接続する前に返るものを選んでいる。
			// 本体に入っているため誘導は出ない。
			name:            "runtime failure gets no hint",
			args:            []string{"sso", "login"},
			wantCode:        1,
			wantErrContains: []string{"AWS SSO access portal URL"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// loadConfig の先の config.Load が $XDG_CONFIG_HOME/thief/config.yaml と
			// $HOME/.thief/config.yaml、それにカレントディレクトリの config.yaml を
			// 読む。実行環境に設定が置かれているとテストの意図と無関係に結果が変わるため、
			// 3 つすべてを空のディレクトリへ向ける。
			t.Setenv("HOME", t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Chdir(t.TempDir())

			root := NewRootCmd()
			// cobra 自身の出力先。エラーと usage の表示は Run に集約するため、
			// エラー時にはここへ何も書かれてはならない。
			var cobraOut, cobraErr bytes.Buffer
			root.SetOut(&cobraOut)
			root.SetErr(&cobraErr)
			root.SetArgs(tt.args)

			var stderr bytes.Buffer
			code := Run(context.Background(), root, &stderr)

			if code != tt.wantCode {
				t.Errorf("Run() = %d, want %d (stderr = %q)", code, tt.wantCode, stderr.String())
			}
			for _, want := range tt.wantErrContains {
				if !strings.Contains(stderr.String(), want) {
					t.Errorf("stderr = %q, want it to contain %q", stderr.String(), want)
				}
			}
			if tt.wantHint == "" {
				if strings.Contains(stderr.String(), "--help' for usage.") {
					t.Errorf("stderr = %q, want no usage hint", stderr.String())
				}
			} else if !strings.HasSuffix(stderr.String(), tt.wantHint) {
				t.Errorf("stderr = %q, want it to end with %q", stderr.String(), tt.wantHint)
			}

			if tt.wantCode == 0 {
				return
			}
			// SilenceErrors が外れると Error: の行が cobra 側にも出て 2 回並ぶ。
			if got := cobraErr.String(); got != "" {
				t.Errorf("cobra stderr = %q, want empty; Run が表示を担う", got)
			}
			// SilenceUsage が外れると usage 全文が cobra の標準出力へ出る。
			if got := cobraOut.String(); got != "" {
				t.Errorf("cobra stdout = %q, want empty; Run が表示を担う", got)
			}
		})
	}
}

// newQuietRoot は Run の分岐だけを見たいテスト用に、本番と同じ表示方針を持つ
// 最小の root を返す。cobra 自身は何も表示しない。
func newQuietRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "probe-root",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	return root
}

// TestNoRootContextOutsideDesignatedFunctions は internal/cli の本番コードが
// 独自に根の context を作る場所を固定する。
//
// 各コマンドは main が signal.NotifyContext で作った context を commandContext 経由で
// 受け取らなければ、Ctrl-C と SIGTERM が実行中の AWS 呼び出しに届かない。
// context.Background / context.TODO を各コマンドで呼べば、その場でシグナル連動が
// 切れるが、コンパイルも lint も通ってしまう。ここで作れる場所を列挙して固定し、
// 増えたら落ちるようにする。
//
// 意図して増やす場合は、なぜシグナル連動から切り離す必要があるのかをコードに
// コメントで残したうえで、この一覧に追記する。
func TestNoRootContextOutsideDesignatedFunctions(t *testing.T) {
	want := []string{
		// 中断でコマンドの context がキャンセル済みでも SSM セッションの切断は通す。
		"ec2.go:startEC2SessionWith",
		// 中断でコマンドの context がキャンセル済みでも SSM セッションの切断は通す。
		"ecs.go:ecsExecuteCommandWith",
		// コマンドに context が載っていない場合のフォールバック。
		"run.go:commandContext",
		// 中断でコマンドの context がキャンセル済みでもシャットダウンの猶予を与える。
		"server.go:newServerCmd",
	}

	got := rootContextCallSites(t, ".")
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("context.Background / context.TODO の呼び出し場所が変わった (-want +got):\n%s", diff)
	}
}

// TestNoContextBlindStdinReadOutsideDesignatedFunctions は internal/cli の本番コードが
// context を見ずに標準入力を読む場所を固定する。
//
// 標準入力の読み取りは context を一切見ない。main が signal.NotifyContext を使うと
// シグナルの既定の動作が止まるため、context を見ない読み取りの前で Ctrl-C を押しても
// 何も起きず、利用者が入力を与えるまで戻ってこない。読み取りを readWithContext で
// 包む 2 つのヘルパーに集約し、増えたら落ちるようにする。
//
// 意図して増やす場合は readWithContext を通すこと。通せない理由がある場合は、
// なぜ入力待ちで Ctrl-C が効かなくてよいのかをコードにコメントで残したうえで、
// この一覧に追記する。
func TestNoContextBlindStdinReadOutsideDesignatedFunctions(t *testing.T) {
	want := []string{
		// 値更新コマンドの標準入力の読み取り。readWithContext の中で呼ぶ。
		"helper.go:readUpdateValue",
		// アカウント選択とロール選択で使い回す *bufio.Reader の構築。構築そのものは
		// ブロックしない。実際に読む ReadString は promptSelection (readWithContext の中) が
		// 呼ぶ。呼び出しのたびに構築し直すと bufio.Reader の先読み分が失われるため、
		// ssoGenerateConfigWith の 1 箇所で構築して両方の呼び出しへ使い回す。
		"sso.go:ssoGenerateConfigWith",
	}

	got := contextBlindReadCallSites(t, ".")
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("context を見ない標準入力の読み取りの場所が変わった (-want +got):\n%s", diff)
	}
}

// TestContextBlindReadCallSitesFindsEveryShape は
// TestNoContextBlindStdinReadOutsideDesignatedFunctions が使う検査そのものを検証する。
//
// 検査が取りこぼすようになると、context を見ない読み取りが混入してもテストは通ってしまう。
func TestContextBlindReadCallSitesFindsEveryShape(t *testing.T) {
	dir := t.TempDir()
	src := `package p

import (
	"bufio"
	"fmt"
	"io"
	"os"
)

func implicitStdin() { fmt.Scanln(new(string)) }
func explicitReader(r io.Reader) { fmt.Fscanln(r, new(string)) }
func formatted(r io.Reader) { fmt.Fscanf(r, "%s", new(string)) }
func readAll(r io.Reader) { io.ReadAll(r) }
func referencesStdin() io.Reader { return os.Stdin }
func bufferedLine(r io.Reader) { bufio.NewReader(r).ReadString('\n') }
func sizedBuffer(r io.Reader) { bufio.NewReaderSize(r, 16).ReadString('\n') }
func scanned(r io.Reader) { bufio.NewScanner(r).Scan() }

// context を見ない読み取りを含まない関数は挙がらない。
func clean(r io.Reader) { fmt.Fprintln(os.Stderr, r) }
`
	if err := os.WriteFile(filepath.Join(dir, "reads.go"), []byte(src), 0o600); err != nil {
		t.Fatalf("write reads.go: %v", err)
	}

	want := []string{
		"reads.go:bufferedLine",
		"reads.go:explicitReader",
		"reads.go:formatted",
		"reads.go:implicitStdin",
		"reads.go:readAll",
		"reads.go:referencesStdin",
		"reads.go:scanned",
		"reads.go:sizedBuffer",
	}
	if diff := cmp.Diff(want, contextBlindReadCallSites(t, dir)); diff != "" {
		t.Errorf("contextBlindReadCallSites() mismatch (-want +got):\n%s", diff)
	}
}

// TestRootContextCallSitesFindsEveryShape は
// TestNoRootContextOutsideDesignatedFunctions が使う検査そのものを検証する。
//
// 検査が取りこぼすようになると、本番コードへ context.Background が混入しても
// テストは通ってしまう。検出する形と対象外にする形を、細工したソースで固定する。
func TestRootContextCallSitesFindsEveryShape(t *testing.T) {
	dir := t.TempDir()
	writeGoFile := func(rel, src string) {
		t.Helper()
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir for %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	writeGoFile("top.go", `package p

import "context"

var pkgLevel = context.Background()

type a struct{}
type b struct{}

// 同名のメソッドはレシーバの型で区別する。
func (*a) New() context.Context { return context.Background() }
func (b) New() context.Context  { return context.TODO() }

// 関数リテラルの中からの呼び出しは、囲む関数に寄せる。
func withLiteral() func() context.Context {
	return func() context.Context { return context.Background() }
}

// 根の context を作らない関数は挙がらない。
func clean(ctx context.Context) context.Context { return ctx }
`)
	// サブディレクトリも辿る。
	writeGoFile("sub/nested.go", `package sub

import "context"

func nested() context.Context { return context.Background() }
`)
	// テストコードは対象外。
	writeGoFile("skipped_test.go", `package p

import "context"

func inTest() context.Context { return context.Background() }
`)
	// Go 自身がビルド対象から外すディレクトリは辿らない。
	for _, d := range []string{"testdata", "_ignored", ".hidden"} {
		writeGoFile(d+"/ignored.go", `package ignored

import "context"

func ignored() context.Context { return context.Background() }
`)
	}

	want := []string{
		"sub/nested.go:nested",
		"top.go:(a).New",
		"top.go:(b).New",
		"top.go:<package level>",
		"top.go:withLiteral",
	}
	if diff := cmp.Diff(want, rootContextCallSites(t, dir)); diff != "" {
		t.Errorf("rootContextCallSites() mismatch (-want +got):\n%s", diff)
	}
}

// rootContextCallSites は dir 以下の本番コードから context.Background / context.TODO を
// 呼んでいる箇所を返す。
func rootContextCallSites(t *testing.T, dir string) []string {
	t.Helper()
	return callSites(t, dir, isRootContextCall)
}

// contextBlindReadCallSites は dir 以下の本番コードで、context を見ない標準入力の
// 読み取りを行っている箇所を返す。
func contextBlindReadCallSites(t *testing.T, dir string) []string {
	t.Helper()
	return callSites(t, dir, isContextBlindRead)
}

// callSites は dir 以下の本番コードで match が真になるノードのある箇所を
// "<dir からの相対パス>:<囲む関数名>" の形で集めて返す。
// 同じ関数に複数あっても 1 件に畳む。
//
// サブディレクトリも辿る。コマンドを internal/cli/<something>/ へ分割したときに、
// そこに書いたコードが検査から漏れると、呼び出し元のテストの意味が無くなる。
func callSites(t *testing.T, dir string, match func(ast.Node) bool) []string {
	t.Helper()

	fset := token.NewFileSet()
	seen := make(map[string]struct{})

	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Go 自身がビルド対象から外すディレクトリは辿らない。
			if path != dir && (d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".") || strings.HasPrefix(d.Name(), "_")) {
				return fs.SkipDir
			}
			return nil
		}

		name := d.Name()
		// テストコードは対象外。テストは根の context を自由に作ってよい。
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("relativize %s: %w", path, err)
		}
		rel = filepath.ToSlash(rel)

		for _, decl := range file.Decls {
			// 関数の外 (パッケージ変数の初期化式など) からの呼び出しも取りこぼさない。
			owner := "<package level>"
			var body ast.Node = decl
			if fn, ok := decl.(*ast.FuncDecl); ok {
				if fn.Body == nil {
					continue
				}
				owner = funcOwner(fn)
				// 関数リテラルの中も本体に含まれるため、囲む関数に寄せて数える。
				body = fn.Body
			}

			ast.Inspect(body, func(n ast.Node) bool {
				if match(n) {
					seen[rel+":"+owner] = struct{}{}
				}
				return true
			})
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk %s: %v", dir, walkErr)
	}

	sites := make([]string, 0, len(seen))
	for s := range seen {
		sites = append(sites, s)
	}
	sort.Strings(sites)
	return sites
}

// funcOwner は fn を一意に指す名前を返す。メソッドにはレシーバの型を付ける。
// 同じファイルに同名のメソッドを持つ型が複数あっても 1 件に畳まれないようにする。
func funcOwner(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	return "(" + receiverTypeName(fn.Recv.List[0].Type) + ")." + fn.Name.Name
}

// receiverTypeName はレシーバの型名を返す。ポインタと型引数は剥がす。
// 想定外の形は "?" を返す。呼び出し場所の一覧に "?" が現れれば、期待値と一致せず
// テストが落ちるため、黙って畳まれることはない。
func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return receiverTypeName(t.X)
	case *ast.IndexExpr:
		// 型引数が 1 つのジェネリック型のレシーバ (Foo[T])。
		return receiverTypeName(t.X)
	case *ast.IndexListExpr:
		// 型引数が複数のジェネリック型のレシーバ (Foo[T, U])。
		return receiverTypeName(t.X)
	case *ast.Ident:
		return t.Name
	default:
		return "?"
	}
}

// isRootContextCall は n が context.Background() または context.TODO() の呼び出しかを返す。
//
// import のエイリアス (import c "context" として c.Background()) や関数値経由の呼び出し
// (var bg = context.Background として bg()) は検出しない。目的はうっかり書いた
// context.Background を止めることであり、意図的な回避への対策は狙っていない。
func isRootContextCall(n ast.Node) bool {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return false
	}
	return isPkgSelector(call.Fun, "context", "Background", "TODO")
}

// isContextBlindRead は n が context を見ない標準入力の読み取りかを返す。
//
// fmt.Scan 系は os.Stdin を暗黙に読む。os.Stdin を直に参照する経路も、コマンドに
// 載った context から切り離される点で同じである。fmt.Fscan 系と io.ReadAll は
// 読み取り先を引数で受け取るが、いずれも context を見ないため対象に含める。
//
// bufio.NewReader / bufio.NewScanner は読み取り自体を返り値のメソッド (ReadString、
// Scan) で行う。メソッドの呼び出しだけを見て標準入力の読み取りと判別するには型情報が
// 必要になるため、構築の時点で捕まえる。bufio.NewReader(os.Stdin) は os.Stdin の参照で
// 既に挙がるが、bufio.NewReader(cmd.InOrStdin()) は構築を見ないと挙がらない。
//
// isRootContextCall と同じく、import のエイリアスや関数値経由の呼び出しは検出しない。
// io.Reader を引数で受け取ってメソッドを直に呼ぶ形 (r.Read、io.Copy) も検出しない。
// 型情報が無いと reader かどうかを判別できないためである。internal/cli の本番コードは
// 読み取りを 2 つのヘルパーに集約しており、この形は現れない。
func isContextBlindRead(n ast.Node) bool {
	// os.Stdin は呼び出しではなく参照として現れる。
	if isPkgSelector(n, "os", "Stdin") {
		return true
	}

	call, ok := n.(*ast.CallExpr)
	if !ok {
		return false
	}
	return isPkgSelector(call.Fun, "fmt", "Scan", "Scanln", "Scanf", "Fscan", "Fscanln", "Fscanf") ||
		isPkgSelector(call.Fun, "io", "ReadAll") ||
		isPkgSelector(call.Fun, "bufio", "NewReader", "NewReaderSize", "NewScanner")
}

// isPkgSelector は expr が pkg.<names のいずれか> の形かを返す。
func isPkgSelector(expr ast.Node, pkg string, names ...string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok || ident.Name != pkg {
		return false
	}
	return slices.Contains(names, sel.Sel.Name)
}

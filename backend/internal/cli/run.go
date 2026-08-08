package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// interruptExitCode は SIGINT / SIGTERM による中断時の終了コード。
// 128 + SIGINT(2) というシェルの慣習に従う。
//
// signal.NotifyContext はどのシグナルで解除されたかを呼び出し側に伝えないため、
// SIGTERM で中断した場合も同じ値になる。正確を期すには通知チャネルを自前で持つ
// 必要があるが、対話的な CLI に SIGTERM を送るのは稀であり、AGENTS.md が名指しする
// signal.NotifyContext を使う方を採る。
const interruptExitCode = 130

// Run は root コマンドを ctx 付きで実行し、プロセスの終了コードを返す。
//
// エラーの表示はこの関数に集約する。root が SilenceErrors と SilenceUsage を
// 立てているため Cobra 自身は何も表示しない。表示先を引数で受け取るのは、
// 終了コードと出力の対応をテストから検証できるようにするためである。
//
// 使い方の誤りを見分けるために配下のコマンドの本体を包むため、渡した root は変更される。
func Run(ctx context.Context, root *cobra.Command, stderr io.Writer) int {
	// cobra が自前で足すコマンドを先に登録させる。ExecuteContextC は help と completion を
	// 実行の直前に足すため、先に足しておかないと markCommandBodyRun が木を辿った時点では
	// まだ存在せず、本体が包まれない。包まれないコマンドが実行時に失敗すると、本体に
	// 入ったことが記録されず、使い方の誤りとして help へ誘導してしまう。
	//
	// どちらも冪等である。InitDefaultHelpCmd は同じインスタンスを付け替えるだけであり、
	// InitDefaultCompletionCmd は completion が既にあれば何もしない。したがって
	// ExecuteContextC が後で呼び直しても、ここで包んだ本体は残る。
	//
	// __complete (initCompleteCmd) は非公開のため先に足せないが、Run (非 E) で定義されて
	// おりエラーを返せない。本体の中で失敗しても Run へは伝わらないため、包めなくても
	// 判定を誤らない。
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()

	var bodyRan bool
	markCommandBodyRun(root, &bodyRan)

	cmd, err := root.ExecuteContextC(ctx)
	if err == nil {
		return 0
	}

	// シグナルによる中断は異常ではない。context canceled の生の文言は情報を持たないため
	// 見せず、中断であることだけを伝えて終了コードで区別する。
	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(stderr, "interrupted")
		return interruptExitCode
	}

	// ctx がキャンセルされているなら、原因はシグナル以外に無い。それでも errors.Is で
	// 辿れないのは、途中の SDK がラップの連鎖を保っていないためである。gRPC を使う
	// Google Cloud の呼び出しは context のキャンセルを codes.Canceled のステータスへ
	// 写して返すため、context.Canceled へは到達できない。
	//
	// 中断であることは上の分岐と同じなので 1 行目の語を揃え、辿れない分だけ元のエラーを
	// 添えて何が起きたかを判別できるようにする。Error: は実行時の失敗だけが名乗る。
	if ctx.Err() != nil {
		fmt.Fprintf(stderr, "interrupted: %v\n", err)
		return interruptExitCode
	}

	fmt.Fprintln(stderr, "Error:", err)

	// 使い方の誤りにだけ help への誘導を足す。AWS の呼び出し失敗のような実行時の
	// エラーで help を案内しても、利用者にできることは無い。
	//
	// cobra は未知のコマンド、フラグ解析の失敗、位置引数の個数、必須フラグの有無を
	// いずれもコマンドの本体より前に検査する。このうち FlagErrorFunc を通るのは
	// フラグ解析の失敗だけで、残りは素の fmt.Errorf であり型も共通の目印も持たない。
	// 文言で見分けるのは cobra の実装に縛られるため、本体に入ったかどうかで分ける。
	// 本体に入る前に落ちた失敗は、すべて使い方の誤りである。
	if cmd != nil && !bodyRan {
		fmt.Fprintf(stderr, "Run '%s --help' for usage.\n", cmd.CommandPath())
	}

	return 1
}

// markCommandBodyRun は cmd とその配下すべてについて、コマンドの本体 (Run / RunE) が
// 呼ばれたことを ran に記録するように包む。
//
// cobra は本体より前の検査の失敗を、実行時の失敗と区別できる形では返さない。本体に
// 入ったかどうかが唯一の確実な手掛かりであり、Run はこの記録で help への誘導を出す
// かどうかを決める。
//
// PersistentPreRunE と PreRunE も本体より前に走るため、そこで失敗すると使い方の誤りと
// して扱われる。本リポジトリはいずれも使っていない。使い始める場合は、そこでの失敗が
// 使い方の誤りなのかを判断し、必要ならここで併せて包むこと。
//
// 呼ぶ前に cobra の遅延登録を済ませておくこと。辿るのは呼んだ時点の木だけであり、
// 後から足されたコマンドは包まれない。
func markCommandBodyRun(cmd *cobra.Command, ran *bool) {
	for _, sub := range cmd.Commands() {
		markCommandBodyRun(sub, ran)
	}

	// cobra は RunE があれば Run を呼ばないため、包むのは実際に呼ばれる方だけでよい。
	switch {
	case cmd.RunE != nil:
		body := cmd.RunE
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			*ran = true
			return body(cmd, args)
		}
	case cmd.Run != nil:
		body := cmd.Run
		cmd.Run = func(cmd *cobra.Command, args []string) {
			*ran = true
			body(cmd, args)
		}
	}
}

// commandContext は cmd に載っている context を返す。
//
// cobra の Command.Context は内部フィールドをそのまま返すため、ExecuteContext /
// SetContext / Execute のいずれも経由しない呼び出しでは nil になる。テストが RunE 相当の
// 関数を直接呼ぶ経路がこれに当たる。nil の context をそのまま AWS SDK へ渡すと
// 実行時に壊れるため、境界で Background に倒す。
//
// 本番の経路では main が signal.NotifyContext で作った context が入っており、
// Ctrl-C と SIGTERM でキャンセルされる。
func commandContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

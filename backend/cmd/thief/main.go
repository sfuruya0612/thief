package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/sfuruya0612/thief/backend/internal/cli"
)

func main() {
	// os.Exit は defer を実行しないため、run を分けて stop の呼び出しを保証する。
	os.Exit(run())
}

// run はシグナル連動の context を作って CLI を実行し、終了コードを返す。
//
// ここで作った context は cli.Run が呼ぶ cobra の ExecuteContextC を通って各コマンドへ
// 渡り、commandContext 経由で AWS や Google Cloud の呼び出しに使われる。これにより
// Ctrl-C と SIGTERM が実行中の処理に届く。
func run() int {
	ctx, stop := interruptContext()
	defer stop()

	return cli.Run(ctx, cli.NewRootCmd(), os.Stderr)
}

// interruptContext は SIGINT と SIGTERM でキャンセルされる context を返す。
// 返された stop は必ず呼ぶこと。呼ばないとシグナルの購読が残り続ける。
//
// これが Ctrl-C と SIGTERM を実行中の処理へ届ける唯一の入口である。
// signal.NotifyContext を context.Background に差し替えても、コンパイルも lint も
// 通ってしまい、中断が届かないことは実行するまで分からない。関数に切り出して
// テストから実際にシグナルを送れるようにしている。
func interruptContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

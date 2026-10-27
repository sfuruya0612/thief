package tui

import (
	"context"
	"fmt"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"
)

// Run は TUI を全画面で起動し、終了まで待つ。
//
// 呼び出し側の ctx (main の signal.NotifyContext に連動する) は意図的に受け取らない。
// セッションは TUI の中のフロートウィンドウで動くため、セッション中も端末は代替画面のままである。
// TUI の Ctrl-C は raw モードのキー入力として Update に届き、SIGTERM は bubbletea 自身の
// シグナル処理が QuitMsg に変換するため、外部の ctx が無くてもシグナルで終了できる。
func Run(deps Deps, target Target) error {
	// TUI は代替画面を占有するため、取得層や SDK のログが stderr に出ると画面が壊れる。
	// TUI の実行中だけ既定のロガーを破棄し、終了後に元へ戻す。
	restoreLogger := discardLogs()
	defer restoreLogger()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m, err := New(ctx, deps, target)
	if err != nil {
		return err
	}
	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if finalModel, ok := final.(Model); ok {
		// セッションを開いたまま SIGTERM などで終了した場合も、AWS 側のセッションを残さない。
		finalModel.shutdownSession()
	}
	if err != nil {
		return fmt.Errorf("run tui: %w", err)
	}
	return nil
}

// discardLogs は既定のロガーを破棄するハンドラへ置き換え、元に戻す関数を返す。
func discardLogs() func() {
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.DiscardHandler))
	return func() { slog.SetDefault(previous) }
}

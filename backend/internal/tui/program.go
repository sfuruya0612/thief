package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// Run は TUI を全画面で起動し、終了まで待つ。
//
// 呼び出し側の ctx (main の signal.NotifyContext に連動する) は意図的に受け取らない。
// 対話セッション中は bubbletea が端末を一時的に通常モードへ戻すため、Ctrl-C は SIGINT として
// プロセスグループ全体に届き、main の ctx をキャンセルする。その ctx を AWS の呼び出しに使うと、
// セッションから戻った後のすべての取得が失敗する。TUI では Ctrl-C は raw モードのキー入力として
// Update に届き (終了として扱う)、SIGTERM は bubbletea 自身のシグナル処理が QuitMsg に変換する
// ため、外部の ctx が無くてもシグナルで終了できる。
func Run(deps Deps, profile, region string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m, err := New(ctx, deps, profile, region)
	if err != nil {
		return err
	}
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		return fmt.Errorf("run tui: %w", err)
	}
	return nil
}

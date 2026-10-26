package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// Run は TUI を全画面で起動し、終了まで待つ。
//
// 呼び出し側の ctx (main の signal.NotifyContext に連動する) は意図的に受け取らない。
// セッションは TUI の中のフロートウィンドウで動くため、セッション中も端末は代替画面のままである。
// TUI の Ctrl-C は raw モードのキー入力として Update に届き、SIGTERM は bubbletea 自身の
// シグナル処理が QuitMsg に変換するため、外部の ctx が無くてもシグナルで終了できる。
func Run(deps Deps, profile, region string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m, err := New(ctx, deps, profile, region)
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

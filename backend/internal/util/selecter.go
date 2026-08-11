package util

import (
	"context"
	"errors"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// Item は対話式セレクタで選択可能な要素を表す。
type Item interface {
	Title() string
	ID() string
}

type model struct {
	items    []Item
	cursor   int
	selected Item
	prompt   string
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		// ctrl+c は SIGINT / SIGTERM による中断と同じ扱いにする。tea.Interrupt を返すと
		// Program.Run が tea.ErrInterrupted を含むエラーで戻り、selectWith がそれを
		// context.Canceled を包んだエラーに変換するため、cli.Run の中断判定
		// (errors.Is(err, context.Canceled)) にそのまま乗り、interrupted / exit 130 になる。
		// q は「選択せずに終了」という利用者の意思決定であり中断ではないため、
		// 従来どおり tea.Quit のまま no item selected を返す。
		case "ctrl+c":
			return m, tea.Interrupt
		case "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "enter":
			m.selected = m.items[m.cursor]
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) View() string {
	s := m.prompt + "\n\n"

	for i, item := range m.items {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		s += fmt.Sprintf("%s %s\n", cursor, item.Title())
	}

	s += "\nPress q to quit.\n"
	return s
}

// selectRunner は対話式プログラムの実行を抽象化する。
type selectRunner func(context.Context, tea.Model) (tea.Model, error)

// runTeaProgram は bubbletea のプログラムを組んで実行する本番の実装。
// ctx を tea.WithContext に渡すことで、選択画面の表示中でも ctx のキャンセル
// (SIGINT / SIGTERM) で Program.Run が戻るようにする。
func runTeaProgram(ctx context.Context, m tea.Model) (tea.Model, error) {
	return tea.NewProgram(m, tea.WithContext(ctx)).Run()
}

// Select は items を対話式リストで表示し、ユーザーが選択した要素を返す。
// ctx がキャンセルされた場合、または選択画面で ctrl+c が押された場合は、
// errors.Is(err, context.Canceled) が真になるエラーを返す。
func Select(ctx context.Context, items []Item, prompt string) (Item, error) {
	return selectWith(ctx, items, prompt, runTeaProgram)
}

// selectWith は実行を差し替えられる Select のコア。
func selectWith(ctx context.Context, items []Item, prompt string, run selectRunner) (Item, error) {
	if len(items) == 0 {
		return nil, errors.New("no items to select")
	}

	initialModel := model{
		items:  items,
		cursor: 0,
		prompt: prompt,
	}

	m, err := run(ctx, initialModel)
	if err != nil {
		// ctrl+c による中断は context.Canceled を包んだエラーとして返し、cli.Run の
		// 中断判定 (errors.Is(err, context.Canceled)) にそのまま乗せる。ctx のキャンセルに
		// よる中断は tea.ErrProgramKilled が既に context.Canceled を包んで戻ってくるため
		// (bubbletea 側の挙動)、下の汎用ラップだけで errors.Is が辿れる。
		if errors.Is(err, tea.ErrInterrupted) {
			return nil, fmt.Errorf("select interrupted: %w", context.Canceled)
		}
		return nil, fmt.Errorf("run bubble tea program: %w", err)
	}

	finalModel, ok := m.(model)
	if !ok {
		return nil, fmt.Errorf("unexpected select program result type %T", m)
	}
	if finalModel.selected == nil {
		return nil, errors.New("no item selected")
	}

	return finalModel.selected, nil
}

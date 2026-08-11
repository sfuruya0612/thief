package util

import (
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
		case "ctrl+c", "q":
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
type selectRunner func(tea.Model) (tea.Model, error)

// runTeaProgram は bubbletea のプログラムを組んで実行する本番の実装。
func runTeaProgram(m tea.Model) (tea.Model, error) {
	return tea.NewProgram(m).Run()
}

// Select は items を対話式リストで表示し、ユーザーが選択した要素を返す。
func Select(items []Item, prompt string) (Item, error) {
	return selectWith(items, prompt, runTeaProgram)
}

// selectWith は実行を差し替えられる Select のコア。
func selectWith(items []Item, prompt string, run selectRunner) (Item, error) {
	if len(items) == 0 {
		return nil, errors.New("no items to select")
	}

	initialModel := model{
		items:  items,
		cursor: 0,
		prompt: prompt,
	}

	m, err := run(initialModel)
	if err != nil {
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

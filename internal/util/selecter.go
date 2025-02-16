package util

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

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

func Select(items []Item, prompt string) (Item, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no items to select")
	}

	initialModel := model{
		items:  items,
		cursor: 0,
		prompt: prompt,
	}

	p := tea.NewProgram(initialModel)
	m, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to start bubble tea program: %v", err)
	}

	finalModel := m.(model)
	if finalModel.selected == nil {
		return nil, fmt.Errorf("no item selected")
	}

	return finalModel.selected, nil
}

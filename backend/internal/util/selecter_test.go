package util

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Mock Item implementation for testing
type TestItem struct {
	title string
	id    string
}

func (t TestItem) Title() string {
	return t.title
}

func (t TestItem) ID() string {
	return t.id
}

func TestSelect_EmptyItems(t *testing.T) {
	items := []Item{}
	prompt := "Select an item:"

	_, err := Select(items, prompt)

	if err == nil {
		t.Error("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no items to select") {
		t.Errorf("expected error to contain 'no items to select', got %q", err.Error())
	}
}

// fakeSelectRunnerError は run bubble tea program の失敗を模す。selectWith が %w で
// ラップすることを errors.As で検証するための独自型。
type fakeSelectRunnerError struct{}

func (fakeSelectRunnerError) Error() string { return "fake tea program error" }

func TestSelectWith_RunFailureIsWrapped(t *testing.T) {
	items := []Item{TestItem{title: "Item 1", id: "1"}}

	_, err := selectWith(items, "Select an item:", func(tea.Model) (tea.Model, error) {
		return nil, fakeSelectRunnerError{}
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var target fakeSelectRunnerError
	if !errors.As(err, &target) {
		t.Errorf("errors.As(fakeSelectRunnerError) = false, want true; the chain is severed (run bubble tea program: %%w is not wrapping): %v", err)
	}
}

func TestSelectWith_NoSelectionReturnsError(t *testing.T) {
	items := []Item{TestItem{title: "Item 1", id: "1"}}

	_, err := selectWith(items, "Select an item:", func(m tea.Model) (tea.Model, error) {
		// ユーザーが q や Ctrl-C で終了した場合、model.selected は nil のまま Quit する。
		return m, nil
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no item selected") {
		t.Errorf("expected error to contain 'no item selected', got %q", err.Error())
	}
}

func TestSelectWith_ReturnsSelectedItem(t *testing.T) {
	items := []Item{
		TestItem{title: "Item 1", id: "1"},
		TestItem{title: "Item 2", id: "2"},
	}

	got, err := selectWith(items, "Select an item:", func(m tea.Model) (tea.Model, error) {
		finalModel := m.(model)
		finalModel.selected = items[1]
		return finalModel, nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != items[1] {
		t.Errorf("selected = %v, want %v", got, items[1])
	}
}

// fakeTeaModel は tea.Model を実装するが util.model ではない型。selectWith の型アサーションが
// panic せずエラーを返すことを検証するために使う。
type fakeTeaModel struct{}

func (fakeTeaModel) Init() tea.Cmd                       { return nil }
func (fakeTeaModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return fakeTeaModel{}, nil }
func (fakeTeaModel) View() string                        { return "" }

func TestSelectWith_UnexpectedResultTypeReturnsErrorWithoutPanic(t *testing.T) {
	items := []Item{TestItem{title: "Item 1", id: "1"}}

	_, err := selectWith(items, "Select an item:", func(tea.Model) (tea.Model, error) {
		return fakeTeaModel{}, nil
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected select program result type") {
		t.Errorf("expected error to contain 'unexpected select program result type', got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "fakeTeaModel") {
		t.Errorf("expected error to contain the actual type name 'fakeTeaModel', got %q", err.Error())
	}
}

func TestModel_Init(t *testing.T) {
	items := []Item{
		TestItem{title: "Item 1", id: "1"},
		TestItem{title: "Item 2", id: "2"},
	}

	m := model{
		items:  items,
		cursor: 0,
		prompt: "Select an item:",
	}

	cmd := m.Init()

	if cmd != nil {
		t.Errorf("expected nil cmd, got %v", cmd)
	}
}

func TestModel_Update_Navigation(t *testing.T) {
	items := []Item{
		TestItem{title: "Item 1", id: "1"},
		TestItem{title: "Item 2", id: "2"},
		TestItem{title: "Item 3", id: "3"},
	}

	m := model{
		items:  items,
		cursor: 1,
		prompt: "Select an item:",
	}

	// Test moving up
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if cmd != nil {
		t.Errorf("expected nil cmd, got %v", cmd)
	}
	if newModel.(model).cursor != 0 {
		t.Errorf("expected cursor 0, got %d", newModel.(model).cursor)
	}

	// Test moving down from initial position
	newModel, cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Errorf("expected nil cmd, got %v", cmd)
	}
	if newModel.(model).cursor != 2 {
		t.Errorf("expected cursor 2, got %d", newModel.(model).cursor)
	}

	// Test boundaries (already at top)
	m.cursor = 0
	newModel, cmd = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if cmd != nil {
		t.Errorf("expected nil cmd, got %v", cmd)
	}
	if newModel.(model).cursor != 0 {
		t.Errorf("expected cursor 0, got %d", newModel.(model).cursor)
	}

	// Test boundaries (already at bottom)
	m.cursor = 2
	newModel, cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Errorf("expected nil cmd, got %v", cmd)
	}
	if newModel.(model).cursor != 2 {
		t.Errorf("expected cursor 2, got %d", newModel.(model).cursor)
	}
}

func TestModel_Update_Select(t *testing.T) {
	items := []Item{
		TestItem{title: "Item 1", id: "1"},
		TestItem{title: "Item 2", id: "2"},
	}

	m := model{
		items:  items,
		cursor: 1,
		prompt: "Select an item:",
	}

	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if newModel.(model).selected != items[1] {
		t.Errorf("expected selected %v, got %v", items[1], newModel.(model).selected)
	}
	if cmd == nil {
		t.Error("expected non-nil cmd, got nil")
	}
	if cmd() != tea.Quit() {
		t.Errorf("expected Quit command")
	}
}

func TestModel_View(t *testing.T) {
	items := []Item{
		TestItem{title: "Item 1", id: "1"},
		TestItem{title: "Item 2", id: "2"},
	}

	m := model{
		items:  items,
		cursor: 0,
		prompt: "Select an item:",
	}

	output := m.View()

	if !strings.Contains(output, "Select an item:") {
		t.Errorf("expected output to contain 'Select an item:', got %q", output)
	}
	if !strings.Contains(output, "> Item 1") {
		t.Errorf("expected output to contain '> Item 1', got %q", output)
	}
	if !strings.Contains(output, "  Item 2") {
		t.Errorf("expected output to contain '  Item 2', got %q", output)
	}
	if !strings.Contains(output, "Press q to quit.") {
		t.Errorf("expected output to contain 'Press q to quit.', got %q", output)
	}
}

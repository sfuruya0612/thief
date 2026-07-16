package util

import (
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

package util

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
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

	// Call the function
	_, err := Select(items, prompt)

	// Verify an error is returned for empty items
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no items to select")
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

	// Call Init
	cmd := m.Init()

	// Init should return nil for this model
	assert.Nil(t, cmd)
}

func TestModel_Update_Navigation(t *testing.T) {
	items := []Item{
		TestItem{title: "Item 1", id: "1"},
		TestItem{title: "Item 2", id: "2"},
		TestItem{title: "Item 3", id: "3"},
	}

	m := model{
		items:  items,
		cursor: 1, // Start at the middle item
		prompt: "Select an item:",
	}

	// Test moving up
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Nil(t, cmd)
	assert.Equal(t, 0, newModel.(model).cursor) // Cursor should move up to index 0

	// Test moving down from initial position
	newModel, cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Nil(t, cmd)
	assert.Equal(t, 2, newModel.(model).cursor) // Cursor should move down to index 2

	// Test boundaries (already at top)
	m.cursor = 0
	newModel, cmd = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Nil(t, cmd)
	assert.Equal(t, 0, newModel.(model).cursor) // Cursor should stay at 0

	// Test boundaries (already at bottom)
	m.cursor = 2
	newModel, cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Nil(t, cmd)
	assert.Equal(t, 2, newModel.(model).cursor) // Cursor should stay at 2
}

func TestModel_Update_Select(t *testing.T) {
	items := []Item{
		TestItem{title: "Item 1", id: "1"},
		TestItem{title: "Item 2", id: "2"},
	}

	m := model{
		items:  items,
		cursor: 1, // Point to the second item
		prompt: "Select an item:",
	}

	// Test selecting an item
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Verify the item was selected and quit command was returned
	assert.Equal(t, items[1], newModel.(model).selected)
	assert.NotNil(t, cmd)
	assert.Equal(t, tea.Quit(), cmd())
}

func TestModel_View(t *testing.T) {
	items := []Item{
		TestItem{title: "Item 1", id: "1"},
		TestItem{title: "Item 2", id: "2"},
	}

	m := model{
		items:  items,
		cursor: 0, // Point to the first item
		prompt: "Select an item:",
	}

	// Get the view output
	output := m.View()

	// Verify it contains the expected components
	assert.Contains(t, output, "Select an item:")
	assert.Contains(t, output, "> Item 1") // First item should have cursor
	assert.Contains(t, output, "  Item 2") // Second item should not have cursor
	assert.Contains(t, output, "Press q to quit.")
}

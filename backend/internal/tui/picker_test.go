package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func testPicker() picker {
	return newPicker(pickerScope, "Select", []pickerItem{
		{Label: "a", Detail: "A", Value: "a"},
		{Label: "b", Detail: "B", Value: "b"},
		{Label: "c", Detail: "C", Value: "c"},
	}, "b")
}

func TestNewPicker_CursorStartsOnSelected(t *testing.T) {
	if p := testPicker(); p.cursor != 1 {
		t.Errorf("cursor = %d, want 1", p.cursor)
	}
	if p := newPicker(pickerScope, "Select", testPicker().items, "missing"); p.cursor != 0 {
		t.Errorf("cursor for unknown selection = %d, want 0", p.cursor)
	}
}

func TestPicker_Update(t *testing.T) {
	tests := []struct {
		name       string
		keys       []string
		wantDone   bool
		wantChosen string
		wantCursor int
	}{
		{name: "down moves", keys: []string{"down"}, wantCursor: 2},
		{name: "ctrl+n moves", keys: []string{"ctrl+n"}, wantCursor: 2},
		{name: "down clamps", keys: []string{"down", "down", "down"}, wantCursor: 2},
		{name: "up clamps", keys: []string{"up", "ctrl+p", "up"}, wantCursor: 0},
		{name: "end jumps to end", keys: []string{"end"}, wantCursor: 2},
		{name: "home jumps to start", keys: []string{"home"}, wantCursor: 0},
		{name: "enter chooses", keys: []string{"enter"}, wantDone: true, wantChosen: "b"},
		{name: "esc cancels", keys: []string{"esc"}, wantDone: true},
		{name: "typing filters and resets the cursor", keys: []string{"c"}, wantCursor: 0},
		{name: "typing then enter chooses the match", keys: []string{"C", "enter"}, wantDone: true, wantChosen: "c"},
		{name: "detail matches too", keys: []string{"A", "enter"}, wantDone: true, wantChosen: "a"},
		{name: "esc clears the filter before cancelling", keys: []string{"c", "esc"}, wantCursor: 0},
		{name: "enter with no match does nothing", keys: []string{"z", "enter"}, wantCursor: 0},
		{name: "backspace restores items", keys: []string{"z", "backspace", "down"}, wantCursor: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := testPicker()
			var done bool
			var chosen *pickerItem
			for _, k := range tt.keys {
				p, done, chosen, _ = p.update(keyMsg(k))
			}
			if done != tt.wantDone {
				t.Errorf("done = %v, want %v", done, tt.wantDone)
			}
			got := ""
			if chosen != nil {
				got = chosen.Value
			}
			if got != tt.wantChosen {
				t.Errorf("chosen = %q, want %q", got, tt.wantChosen)
			}
			if !done && p.cursor != tt.wantCursor {
				t.Errorf("cursor = %d, want %d", p.cursor, tt.wantCursor)
			}
		})
	}
}

func TestPicker_EmptyAndNoMatchViews(t *testing.T) {
	p := newPicker(pickerRegion, "Select", nil, "")
	if _, done, chosen, _ := p.update(tea.KeyMsg{Type: tea.KeyEnter}); done || chosen != nil {
		t.Errorf("done=%v chosen=%v, enter on an empty picker must do nothing", done, chosen)
	}
	if !strings.Contains(p.view(5), "(no items)") {
		t.Errorf("empty picker should say so\n%s", p.view(5))
	}
	if _, done, _, _ := p.update(tea.KeyMsg{Type: tea.KeyEsc}); !done {
		t.Error("esc must still close an empty picker")
	}

	p = testPicker()
	p, _, _, _ = p.update(keyMsg("z"))
	if view := p.view(5); !strings.Contains(view, "(no matches)") || !strings.Contains(view, "(0/3)") {
		t.Errorf("no-match view should say so with the count\n%s", view)
	}
}

func TestPicker_EscClearsFilterThenCancels(t *testing.T) {
	p := testPicker()
	p, done, _, _ := p.update(keyMsg("a"))
	if done {
		t.Fatal("typing must not close")
	}
	p, done, _, _ = p.update(keyMsg("esc"))
	if done || p.input.Value() != "" {
		t.Fatalf("first esc should only clear the filter: done=%v value=%q", done, p.input.Value())
	}
	if len(p.filtered()) != 3 {
		t.Errorf("filtered = %d, want all 3 after clearing", len(p.filtered()))
	}
	if _, done, _, _ = p.update(keyMsg("esc")); !done {
		t.Error("second esc should cancel")
	}
}

func TestPicker_ViewScrollsToCursor(t *testing.T) {
	items := make([]pickerItem, 0, 10)
	for i := 0; i < 10; i++ {
		items = append(items, pickerItem{Label: string(rune('a' + i)), Value: string(rune('a' + i))})
	}
	p := newPicker(pickerScope, "Select", items, "j")
	view := p.view(5)
	if !strings.Contains(view, "> j") {
		t.Errorf("cursor row should be visible\n%s", view)
	}
	if strings.Contains(view, "  a ") {
		t.Errorf("rows far above the cursor should scroll out\n%s", view)
	}
}

func TestPicker_ViewShowsWholePlaceholder(t *testing.T) {
	if view := testPicker().view(10); !strings.Contains(view, "type to filter") {
		t.Errorf("placeholder should be shown in full\n%s", view)
	}
}

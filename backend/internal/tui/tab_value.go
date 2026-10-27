package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type valueLoadedMsg struct {
	seq   int
	value string
	err   error
}

// valueTab は Secrets Manager / Parameter Store の Value タブ。読み取り専用で、
// 秘密値は初期状態でマスクし、v で取得 / 表示 / 非表示を切り替える。
type valueTab struct {
	ctx      context.Context
	metadata []KV
	masked   bool
	load     func(ctx context.Context) (string, error)

	value    string
	loaded   bool
	revealed bool
	loading  bool
	err      error
	loadSeq  int
	viewport viewport.Model
	width    int
	height   int
}

func newValueTab(ctx context.Context, metadata []KV, masked bool, load func(ctx context.Context) (string, error)) *valueTab {
	return &valueTab{
		ctx:      ctx,
		metadata: metadata,
		masked:   masked,
		load:     load,
		loading:  !masked,
		viewport: viewport.New(0, 0),
	}
}

func (t *valueTab) Init() tea.Cmd {
	if t.masked {
		return nil
	}
	return t.loadCmd()
}

func (t *valueTab) Update(msg tea.Msg) (Tab, tea.Cmd, *TabIntent, bool) {
	switch msg := msg.(type) {
	case valueLoadedMsg:
		if msg.seq != t.loadSeq {
			return t, nil, nil, true
		}
		t.loading = false
		t.loaded = true
		t.err = msg.err
		if msg.err != nil {
			// 取得に失敗したときは値を受け取っていても保持しない。
			t.value = ""
			t.loaded = false
			t.revealed = false
			t.SetSize(t.width, t.height)
			return t, nil, nil, true
		}
		t.value = msg.value
		t.revealed = true
		t.SetSize(t.width, t.height)
		t.viewport.SetContent(wrap(t.value, t.width))
		t.viewport.GotoTop()
		return t, nil, nil, true
	case tea.KeyMsg:
		switch msg.String() {
		case "v":
			if !t.masked {
				return t, nil, nil, true
			}
			if !t.loaded && !t.loading {
				return t, t.loadCmd(), nil, true
			}
			if t.loaded {
				t.revealed = !t.revealed
				t.SetSize(t.width, t.height)
				if t.revealed {
					t.viewport.SetContent(wrap(t.value, t.width))
				}
			}
			return t, nil, nil, true
		case "up", "k", "down", "j", "pgup", "pgdown", "home", "end":
			if t.masked && !t.revealed {
				return t, nil, nil, true
			}
			var cmd tea.Cmd
			t.viewport, cmd = t.viewport.Update(msg)
			return t, cmd, nil, true
		}
	}
	return t, nil, nil, false
}

func (t *valueTab) loadCmd() tea.Cmd {
	t.loadSeq++
	t.loading = true
	t.err = nil
	seq := t.loadSeq
	load := t.load
	ctx := t.ctx
	return func() tea.Msg {
		value, err := load(ctx)
		return valueLoadedMsg{seq: seq, value: value, err: err}
	}
}

func (t *valueTab) View(width, height int) string {
	if width != t.width || height != t.height {
		t.SetSize(width, height)
	}
	var b strings.Builder
	if len(t.metadata) > 0 {
		b.WriteString(renderTabContent(TabContent{Blocks: []TabBlock{{KV: t.metadata}}}, width))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	switch {
	case t.loading:
		b.WriteString(mutedStyle.Render("Loading..."))
	case t.err != nil:
		b.WriteString(errorStyle.Render(wrap(t.err.Error(), width)))
	case t.masked && !t.revealed:
		if t.loaded {
			b.WriteString(mutedStyle.Render("Hidden; press v to reveal"))
		} else {
			b.WriteString(mutedStyle.Render("Press v to reveal"))
		}
	case t.loaded && t.value == "":
		b.WriteString(mutedStyle.Render("Value is empty"))
	default:
		b.WriteString(t.viewport.View())
	}
	if t.masked && t.revealed && t.value != "" {
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render("Press v to hide"))
	}
	return b.String()
}

func (t *valueTab) SetSize(width, height int) {
	t.width, t.height = width, height
	reserved := kvLines(t.metadata, width) + 1 // メタデータと空行
	if t.masked && t.revealed && t.value != "" {
		reserved++ // 非表示に戻す案内
	}
	viewportHeight := height - reserved
	if viewportHeight < 1 {
		viewportHeight = 1
	}
	t.viewport.Width = width
	t.viewport.Height = viewportHeight
	if t.loaded {
		t.viewport.SetContent(wrap(t.value, width))
	}
}

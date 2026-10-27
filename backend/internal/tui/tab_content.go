package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// contentTabSpec は contentTab を使う TabSpec を返す。load はタブを開いた時点の主スコープ、
// 副スコープ、行を受け取る。
func contentTabSpec(id, title string, load func(ctx context.Context, scope, region string, row Row) (TabContent, error)) TabSpec {
	return TabSpec{
		ID:    id,
		Title: title,
		New: func(ctx context.Context, scope, region string, row Row) Tab {
			return newContentTab(ctx, func(ctx context.Context) (TabContent, error) {
				return load(ctx, scope, region, row)
			})
		},
	}
}

// contentTabLoadedMsg は contentTab の取得結果。
type contentTabLoadedMsg struct {
	content TabContent
	err     error
}

// contentTab は読み取り専用の表とテキストを表示するタブ。タブごとの取得関数を Init で呼び、
// 読み込み中、失敗、空、成功を区別する。
type contentTab struct {
	ctx      context.Context
	load     func(ctx context.Context) (TabContent, error)
	viewport viewport.Model
	content  TabContent
	loading  bool
	err      error
	width    int
	height   int
}

// newContentTab は取得関数を持つ contentTab を返す。
func newContentTab(ctx context.Context, load func(ctx context.Context) (TabContent, error)) *contentTab {
	return &contentTab{
		ctx:      ctx,
		load:     load,
		viewport: viewport.New(0, 0),
		loading:  true,
	}
}

// Init は取得を始める。
func (t *contentTab) Init() tea.Cmd {
	t.loading = true
	t.err = nil
	load := t.load
	ctx := t.ctx
	return func() tea.Msg {
		content, err := load(ctx)
		return contentTabLoadedMsg{content: content, err: err}
	}
}

// Update は取得結果とスクロールのキーを処理する。
func (t *contentTab) Update(msg tea.Msg) (Tab, tea.Cmd, *TabIntent, bool) {
	switch msg := msg.(type) {
	case contentTabLoadedMsg:
		t.loading = false
		t.content = msg.content
		t.err = msg.err
		t.refresh()
		return t, nil, nil, true
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k", "down", "j", "pgup", "pgdown", "home", "end":
			var cmd tea.Cmd
			t.viewport, cmd = t.viewport.Update(msg)
			return t, cmd, nil, true
		}
	}
	return t, nil, nil, false
}

// View はタブ本文を返す。
func (t *contentTab) View(width, height int) string {
	if width != t.width || height != t.height {
		t.SetSize(width, height)
	}
	switch {
	case t.loading:
		return mutedStyle.Render("Loading...")
	case t.err != nil:
		return errorStyle.Render(wrap(t.err.Error(), width))
	case tabContentEmpty(t.content):
		return mutedStyle.Render("No data")
	default:
		return t.viewport.View()
	}
}

// SetSize は本文の大きさを反映し、表示中の内容を描き直す。
func (t *contentTab) SetSize(width, height int) {
	t.width, t.height = width, height
	t.viewport.Width = width
	t.viewport.Height = height
	t.refresh()
}

// refresh は保持している内容を viewport に流し込む。
func (t *contentTab) refresh() {
	if t.loading || t.err != nil {
		t.viewport.SetContent("")
		return
	}
	t.viewport.SetContent(renderTabContent(t.content, t.width))
}

// renderTabContent はタブ本文のブロックを文字列にする。
func renderTabContent(content TabContent, width int) string {
	var b strings.Builder
	for i, block := range content.Blocks {
		if i > 0 {
			b.WriteString("\n\n")
		}
		if block.Title != "" {
			b.WriteString(headerStyle.Render(block.Title))
			b.WriteString("\n")
		}
		if len(block.KV) > 0 {
			keys := make([]string, 0, len(block.KV))
			for _, kv := range block.KV {
				keys = append(keys, kv.Key)
			}
			keyWidth := maxLen(keys)
			valueWidth := width - keyWidth - 2
			if valueWidth < 1 {
				valueWidth = 1
			}
			for _, kv := range block.KV {
				prefix := mutedStyle.Render(padRight(kv.Key, keyWidth)) + "  "
				lines := []string{""}
				if kv.Value != "" {
					lines = strings.Split(wrap(kv.Value, valueWidth), "\n")
				}
				for j, line := range lines {
					if j > 0 {
						b.WriteString(strings.Repeat(" ", keyWidth+2))
					} else {
						b.WriteString(prefix)
					}
					b.WriteString(line)
					b.WriteString("\n")
				}
			}
		}
		if block.Table != nil {
			if len(block.Table.Rows) == 0 {
				b.WriteString(mutedStyle.Render("No data"))
				b.WriteString("\n")
			} else {
				b.WriteString(renderTabTable(*block.Table, width))
				b.WriteString("\n")
			}
		}
		if block.Text != "" {
			b.WriteString(wrap(block.Text, width))
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// wrappedLines は幅で折り返したときの行数を返す。
func wrappedLines(text string, width int) int {
	if text == "" {
		return 0
	}
	return strings.Count(wrap(text, width), "\n") + 1
}

// kvLines は KV を width で描画したときの行数を返す。
func kvLines(kvs []KV, width int) int {
	if len(kvs) == 0 {
		return 0
	}
	keys := make([]string, 0, len(kvs))
	for _, kv := range kvs {
		keys = append(keys, kv.Key)
	}
	valueWidth := width - maxLen(keys) - 2
	if valueWidth < 1 {
		valueWidth = 1
	}
	total := 0
	for _, kv := range kvs {
		if kv.Value == "" {
			total++
			continue
		}
		total += wrappedLines(kv.Value, valueWidth)
	}
	return total
}

// fitColumnCount は幅に収まる列数を返す。収まらないときは右端の列を落とし、
// widths の末尾を必要なら縮める。perColumn は列ごとの余白、between は列間の余白、
// reserve は切り落としを示す記号の分。
func fitColumnCount(widths []int, width, perColumn, between, reserve int) int {
	if len(widths) == 0 {
		return 0
	}
	cost := func(n int) int {
		total := reserve
		for i := 0; i < n; i++ {
			total += widths[i] + perColumn
		}
		if n > 1 {
			total += between * (n - 1)
		}
		return total
	}
	n := len(widths)
	for n > 1 && cost(n) > width {
		n--
	}
	if n > 0 && cost(n) > width {
		others := cost(n) - widths[n-1]
		maxLast := width - others
		if maxLast < 1 {
			maxLast = 1
		}
		if widths[n-1] > maxLast {
			widths[n-1] = maxLast
		}
	}
	return n
}

// renderTabTable は TabTable を幅に収まる文字列にする。すべての列が最小幅まで縮んでも
// 収まらないときは右端の列を切り落とし、末尾に … を付ける。
func renderTabTable(table TabTable, width int) string {
	if len(table.Columns) == 0 {
		return mutedStyle.Render("No data")
	}
	widths := make([]int, len(table.Columns))
	for i, col := range table.Columns {
		widths[i] = lipgloss.Width(col.Title)
	}
	for _, row := range table.Rows {
		for i, cell := range fitCells(row, len(table.Columns)) {
			if w := lipgloss.Width(cell); w > widths[i] {
				widths[i] = w
			}
		}
	}
	shrinkToFit(widths, width-cellPadding*len(widths))

	visible := fitColumnCount(widths, width, 0, 2, 2)
	truncated := visible < len(table.Columns)

	var b strings.Builder
	for i := 0; i < visible; i++ {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(mutedStyle.Render(fitCell(table.Columns[i].Title, widths[i])))
	}
	if truncated {
		b.WriteString(" …")
	}
	b.WriteString("\n")
	for i := 0; i < visible; i++ {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(strings.Repeat("─", widths[i]))
	}
	if truncated {
		b.WriteString(" …")
	}
	b.WriteString("\n")

	for _, row := range table.Rows {
		for i, cell := range fitCells(row, len(table.Columns)) {
			if i >= visible {
				break
			}
			if i > 0 {
				b.WriteString("  ")
			}
			cell = fitCell(cell, widths[i])
			if table.Columns[i].Align == AlignRight {
				cell = padLeft(cell, widths[i])
			}
			b.WriteString(cell)
		}
		if truncated {
			b.WriteString(" …")
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func fitCell(s string, width int) string {
	if width < 1 {
		return ""
	}
	return ansi.Truncate(sanitizeCell(s), width, "…")
}

// sanitizeCell はセル内の改行とタブを 1 行に潰す。表の行数と列の対応が崩れるのを防ぐ。
func sanitizeCell(s string) string {
	return strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ").Replace(s)
}

func padLeft(s string, width int) string {
	if pad := width - lipgloss.Width(s); pad > 0 {
		return strings.Repeat(" ", pad) + s
	}
	return s
}

func tabContentEmpty(content TabContent) bool {
	for _, block := range content.Blocks {
		if block.Title != "" || len(block.KV) > 0 || block.Text != "" {
			return false
		}
		if block.Table != nil && len(block.Table.Rows) > 0 {
			return false
		}
	}
	return true
}

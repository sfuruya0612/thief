package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// listRow は一覧と詳細を持つタブの 1 行。value は詳細を作るための元データ。
type listRow struct {
	cells []string
	value any
}

// TabDetail は一覧の行を開いたときに表示する内容。
type TabDetail struct {
	Title      string
	KVs        []KV
	Text       string
	Table      *TabTable
	Selectable bool
	Intent     func(index int) *TabIntent
}

type listDetailLoadedMsg struct {
	rows []listRow
	err  error
}

type listDetailFetchedMsg struct {
	seq    int
	detail *TabDetail
	err    error
}

type listDetailFocus int

const (
	listDetailFocusList listDetailFocus = iota
	listDetailFocusDetail
)

// listDetailTab は一覧と詳細を持つタブ。ECS Tasks / Instances、ELB、WAF で使う。
type listDetailTab struct {
	ctx      context.Context
	columns  []Column
	loadList func(ctx context.Context) ([]listRow, error)
	detailOf func(row listRow) *TabDetail
	load     func(ctx context.Context, row listRow) (*TabDetail, error)
	empty    string

	list          table.Model
	rows          []listRow
	loading       bool
	err           error
	detail        *TabDetail
	detailSeq     int
	detailLoad    bool
	detailErr     error
	detailTable   table.Model
	viewport      viewport.Model
	listHeight    int
	detailHeight  int
	detailKVLimit int
	focus         listDetailFocus
	width         int
	height        int
}

// newListDetailTab は一覧と詳細を持つタブを作る。detailOf が非 nil なら詳細を即時に作り、
// load が非 nil なら詳細を非同期で取る。両方 nil なら詳細を開かない。
func newListDetailTab(
	ctx context.Context,
	columns []Column,
	loadList func(ctx context.Context) ([]listRow, error),
	detailOf func(row listRow) *TabDetail,
	load func(ctx context.Context, row listRow) (*TabDetail, error),
) *listDetailTab {
	return &listDetailTab{
		ctx:      ctx,
		columns:  columns,
		loadList: loadList,
		detailOf: detailOf,
		load:     load,
		list:     newTabList(columns, nil, 1, 1),
		viewport: viewport.New(0, 0),
		loading:  true,
	}
}

// Init は一覧の取得を始める。
func (t *listDetailTab) Init() tea.Cmd {
	t.loading = true
	t.err = nil
	load := t.loadList
	ctx := t.ctx
	return func() tea.Msg {
		rows, err := load(ctx)
		return listDetailLoadedMsg{rows: rows, err: err}
	}
}

// Update は一覧、詳細、キー入力を処理する。
func (t *listDetailTab) Update(msg tea.Msg) (Tab, tea.Cmd, *TabIntent, bool) {
	switch msg := msg.(type) {
	case listDetailLoadedMsg:
		t.loading = false
		t.err = msg.err
		t.rows = msg.rows
		t.detail = nil
		t.detailLoad = false
		t.detailErr = nil
		t.focus = listDetailFocusList
		t.layout(true)
		return t, nil, nil, true
	case listDetailFetchedMsg:
		if msg.seq != t.detailSeq {
			return t, nil, nil, true
		}
		t.detailLoad = false
		t.detail = msg.detail
		t.detailErr = msg.err
		t.layout(false)
		return t, nil, nil, true
	case tea.KeyMsg:
		return t.updateKey(msg)
	}
	return t, nil, nil, false
}

func (t *listDetailTab) updateKey(msg tea.KeyMsg) (Tab, tea.Cmd, *TabIntent, bool) {
	if t.focus == listDetailFocusDetail {
		return t.updateDetailKey(msg)
	}

	switch msg.String() {
	case "up", "k", "down", "j":
		var cmd tea.Cmd
		t.list, cmd = t.list.Update(msg)
		return t, cmd, nil, true
	case "pgup", "pgdown", "home", "end":
		var cmd tea.Cmd
		t.list, cmd = t.list.Update(msg)
		return t, cmd, nil, true
	case "enter":
		index := t.list.Cursor()
		if index < 0 || index >= len(t.rows) {
			return t, nil, nil, true
		}
		t.detailSeq++
		t.focus = listDetailFocusDetail
		t.detail = nil
		t.detailErr = nil
		if t.detailOf != nil {
			t.detail = t.detailOf(t.rows[index])
			t.layout(false)
			return t, nil, nil, true
		}
		if t.load != nil {
			t.detailLoad = true
			t.layout(false)
			seq := t.detailSeq
			ctx := t.ctx
			row := t.rows[index]
			load := t.load
			return t, func() tea.Msg {
				detail, err := load(ctx, row)
				return listDetailFetchedMsg{seq: seq, detail: detail, err: err}
			}, nil, true
		}
		return t, nil, nil, true
	case "esc":
		if t.detail != nil || t.detailLoad {
			t.detailSeq++
			t.detail = nil
			t.detailLoad = false
			t.detailErr = nil
			t.focus = listDetailFocusList
			t.layout(false)
			return t, nil, nil, true
		}
	}
	return t, nil, nil, false
}

func (t *listDetailTab) updateDetailKey(msg tea.KeyMsg) (Tab, tea.Cmd, *TabIntent, bool) {
	switch msg.String() {
	case "esc":
		t.detailSeq++
		t.detail = nil
		t.detailLoad = false
		t.detailErr = nil
		t.focus = listDetailFocusList
		t.layout(false)
		return t, nil, nil, true
	case "up", "k", "down", "j":
		if t.detail != nil && t.detail.Selectable {
			var cmd tea.Cmd
			t.detailTable, cmd = t.detailTable.Update(msg)
			return t, cmd, nil, true
		}
		var cmd tea.Cmd
		t.viewport, cmd = t.viewport.Update(msg)
		return t, cmd, nil, true
	case "pgup", "pgdown", "home", "end":
		var cmd tea.Cmd
		t.viewport, cmd = t.viewport.Update(msg)
		return t, cmd, nil, true
	case "s", "enter":
		if t.detail != nil && t.detail.Selectable && t.detail.Intent != nil {
			return t, nil, t.detail.Intent(t.detailTable.Cursor()), true
		}
		// 詳細にフォーカスがある間の s と enter はタブが消費する。ECS の Instances のように
		// Exec の導線を持たない詳細で、グローバルの s がクラスタのセッションを始めないようにする。
		return t, nil, nil, true
	}
	return t, nil, nil, false
}

// View は一覧と詳細を返す。
func (t *listDetailTab) View(width, height int) string {
	if width != t.width || height != t.height {
		t.SetSize(width, height)
	}
	if t.loading {
		return mutedStyle.Render("Loading...")
	}
	if t.err != nil {
		return errorStyle.Render(wrap(t.err.Error(), width))
	}
	if len(t.rows) == 0 {
		if t.empty != "" {
			return mutedStyle.Render(t.empty)
		}
		return mutedStyle.Render("No data")
	}

	list := t.list.View()
	if t.detail == nil && !t.detailLoad && t.detailErr == nil {
		return list
	}

	var b strings.Builder
	b.WriteString(list)
	b.WriteString("\n")
	if t.detailLoad {
		b.WriteString(mutedStyle.Render("Loading detail..."))
		return b.String()
	}
	if t.detailErr != nil {
		b.WriteString(errorStyle.Render(wrap(t.detailErr.Error(), width)))
		return b.String()
	}
	if t.detail == nil {
		return b.String()
	}
	if t.detail.Table != nil && t.detail.Selectable {
		if t.detail.Title != "" {
			b.WriteString(headerStyle.Render(t.detail.Title))
			b.WriteString("\n")
		}
		kvs := t.detail.KVs
		if t.detailKVLimit >= 0 && len(kvs) > t.detailKVLimit {
			kvs = kvs[:t.detailKVLimit]
		}
		if len(kvs) > 0 {
			b.WriteString(renderTabContent(TabContent{Blocks: []TabBlock{{KV: kvs}}}, width))
			b.WriteString("\n")
		}
		if t.detail.Table != nil && len(t.detail.Table.Rows) == 0 && t.detail.Text != "" {
			b.WriteString(mutedStyle.Render(wrap(t.detail.Text, width)))
		} else {
			b.WriteString(t.detailTable.View())
		}
		return b.String()
	}
	// 選択できない詳細は、タイトル、KV、表、テキストをまとめて viewport に入れ、
	// Drawer の高さを超えた分をスクロールできるようにする。
	b.WriteString(t.viewport.View())
	return b.String()
}

// SetSize は本文の大きさを反映する。
func (t *listDetailTab) SetSize(width, height int) {
	t.width, t.height = width, height
	t.layout(false)
}

// layout は一覧と詳細の高さを配分し、表と詳細を作り直す。resetCursor は一覧の選択を
// 先頭に戻すとき (一覧の取得後) に真にする。
func (t *listDetailTab) layout(resetCursor bool) {
	if t.width < 1 || t.height < 1 {
		return
	}
	listHeight := t.height
	if t.detail != nil || t.detailLoad || t.detailErr != nil {
		// 詳細は KV と表を合わせて 10 行以上になることがあるため、一覧を 1/4 に抑えて
		// 詳細へ 3/4 を割り当てる。
		listHeight = t.height / 4
		if listHeight < 3 {
			listHeight = 3
		}
	}
	if listHeight >= t.height {
		listHeight = t.height - 1
	}
	if listHeight < 1 {
		listHeight = 1
	}
	cursor := t.list.Cursor()
	t.list = newTabList(t.columns, t.rows, t.width, listHeight)
	if !resetCursor {
		t.list.SetCursor(cursor)
	}
	t.listHeight = listHeight

	t.detailHeight = t.height - listHeight - 1
	if t.detailHeight < 1 {
		t.detailHeight = 1
	}
	t.viewport.Width = t.width
	t.viewport.Height = t.detailHeight
	t.rebuildDetail()
}

// rebuildDetail は詳細の表またはスクロール用の内容を作る。
func (t *listDetailTab) rebuildDetail() {
	if t.detail == nil {
		t.viewport.SetContent("")
		return
	}
	if t.detail.Table != nil && t.detail.Selectable {
		titleLines := 0
		if t.detail.Title != "" {
			titleLines = 1
		}
		// 表のヘッダと 1 行は必ず見えるように、KV の行数を制限する。
		available := t.detailHeight - titleLines - 3
		if available < 1 {
			available = 1
		}
		limit := 0
		used := 0
		for limit < len(t.detail.KVs) {
			nextUsed := kvLines(t.detail.KVs[:limit+1], t.width)
			if nextUsed > available && limit > 0 {
				break
			}
			used = nextUsed
			limit++
		}
		t.detailKVLimit = limit
		remaining := t.detailHeight - titleLines - used
		if remaining < 3 {
			remaining = 3
		}
		t.detailTable = newTabList(t.detail.Table.Columns, detailTableRows(*t.detail.Table), t.width, remaining)
		t.viewport.SetContent("")
		return
	}
	t.detailKVLimit = len(t.detail.KVs)

	var b strings.Builder
	if t.detail.Title != "" {
		b.WriteString(headerStyle.Render(t.detail.Title))
		b.WriteString("\n")
	}
	if len(t.detail.KVs) > 0 {
		b.WriteString(renderTabContent(TabContent{Blocks: []TabBlock{{KV: t.detail.KVs}}}, t.width))
		b.WriteString("\n")
	}
	if t.detail.Table != nil {
		b.WriteString(renderTabTable(*t.detail.Table, t.width))
		b.WriteString("\n")
	}
	if t.detail.Text != "" {
		b.WriteString(wrap(t.detail.Text, t.width))
	}
	t.viewport.SetContent(strings.TrimRight(b.String(), "\n"))
}

// newTabList はタブの一覧用の table を作る。幅に収まらないときは右端の列を落とし、
// 末尾に … の列を足す。height はヘッダを含む表全体の高さ。
func newTabList(columns []Column, rows []listRow, width, height int) table.Model {
	styles := table.DefaultStyles()
	styles.Header = styles.Header.Bold(true).Foreground(mutedColor).BorderStyle(lipgloss.NormalBorder()).BorderBottom(true).BorderForeground(mutedColor)
	styles.Selected = styles.Selected.Foreground(lipgloss.Color("0")).Background(accentColor).Bold(false)

	widths := make([]int, len(columns))
	for i, col := range columns {
		widths[i] = lipgloss.Width(col.Title)
	}
	for _, row := range rows {
		for i, cell := range fitCells(row.cells, len(columns)) {
			if w := lipgloss.Width(cell); w > widths[i] {
				widths[i] = w
			}
		}
	}
	shrinkToFit(widths, width-cellPadding*len(columns))

	visible := fitColumnCount(widths, width, cellPadding, 0, cellPadding+1)
	truncated := visible < len(columns)

	tableCols := make([]table.Column, 0, visible+1)
	for i := 0; i < visible; i++ {
		tableCols = append(tableCols, table.Column{Title: sanitizeCell(columns[i].Title), Width: widths[i]})
	}
	if truncated {
		tableCols = append(tableCols, table.Column{Title: "…", Width: 1})
	}
	tableRows := make([]table.Row, 0, len(rows))
	for _, row := range rows {
		cells := fitCells(row.cells, visible)
		if truncated {
			cells = append(cells, "…")
		}
		tableRows = append(tableRows, table.Row(cells))
	}

	t := table.New(table.WithFocused(true), table.WithStyles(styles))
	t.SetRows(nil)
	t.SetColumns(tableCols)
	t.SetRows(tableRows)
	t.SetWidth(width)
	// bubbles の table は View がヘッダの次の行に viewport を足すため、SetHeight に
	// height-1 を渡すと描画がちょうど height 行になる。ヘッダ 2 行と 1 行は確保する。
	if height < 3 {
		height = 3
	}
	t.SetHeight(height - 1)
	return t
}

// detailTableRows は TabTable の行を listRow に変換する。
func detailTableRows(table TabTable) []listRow {
	rows := make([]listRow, 0, len(table.Rows))
	for i, cells := range table.Rows {
		rows = append(rows, listRow{cells: cells, value: i})
	}
	return rows
}

// listSummary は KVs と表をまとめた詳細を作る。
func listSummary(title string, kvs []KV, table *TabTable) *TabDetail {
	return &TabDetail{Title: title, KVs: kvs, Table: table}
}

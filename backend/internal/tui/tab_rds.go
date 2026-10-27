package tui

import (
	"context"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
)

type rdsParamsLoadedMsg struct {
	seq    int
	params [][]string
	err    error
}

// rdsInstanceParametersTab は RDS の Instance Parameters タブ。インスタンスに紐づく
// パラメータグループを ← / → で切り替え、選択中のグループのパラメータを表示する。
type rdsInstanceParametersTab struct {
	ctx     context.Context
	profile string
	region  string
	groups  []string
	load    func(ctx context.Context, profile, region, group string) ([][]string, error)

	index   int
	rows    [][]string
	loading bool
	err     error
	loadSeq int
	table   table.Model
	width   int
	height  int
}

func newRDSInstanceParametersTab(
	ctx context.Context,
	profile, region string,
	groups []string,
	load func(ctx context.Context, profile, region, group string) ([][]string, error),
) *rdsInstanceParametersTab {
	return &rdsInstanceParametersTab{
		ctx:     ctx,
		profile: profile,
		region:  region,
		groups:  groups,
		load:    load,
		table:   newTabList(rdsParameterColumns(), nil, 1, 1),
	}
}

func (t *rdsInstanceParametersTab) Init() tea.Cmd {
	if len(t.groups) == 0 {
		return nil
	}
	return t.reload()
}

func (t *rdsInstanceParametersTab) Update(msg tea.Msg) (Tab, tea.Cmd, *TabIntent, bool) {
	switch msg := msg.(type) {
	case rdsParamsLoadedMsg:
		if msg.seq != t.loadSeq {
			return t, nil, nil, true
		}
		t.loading = false
		t.err = msg.err
		t.rows = msg.params
		t.SetSize(t.width, t.height)
		t.table.SetCursor(0)
		return t, nil, nil, true
	case tea.KeyMsg:
		switch msg.String() {
		case "left":
			if t.index > 0 {
				t.index--
				return t, t.reload(), nil, true
			}
			return t, nil, nil, true
		case "right":
			if t.index < len(t.groups)-1 {
				t.index++
				return t, t.reload(), nil, true
			}
			return t, nil, nil, true
		case "r":
			return t, t.reload(), nil, true
		case "up", "k", "down", "j", "pgup", "pgdown", "home", "end":
			var cmd tea.Cmd
			t.table, cmd = t.table.Update(msg)
			return t, cmd, nil, true
		}
	}
	return t, nil, nil, false
}

func (t *rdsInstanceParametersTab) View(width, height int) string {
	if width != t.width || height != t.height {
		t.SetSize(width, height)
	}
	if len(t.groups) == 0 {
		return mutedStyle.Render("No parameter groups.")
	}

	var b strings.Builder
	b.WriteString(mutedStyle.Render(t.groupHeader()))
	b.WriteString("\n")
	switch {
	case t.loading:
		b.WriteString(mutedStyle.Render("Loading..."))
	case t.err != nil:
		b.WriteString(errorStyle.Render(wrap(t.err.Error(), width)))
	case len(t.rows) == 0:
		b.WriteString(mutedStyle.Render("No parameters."))
	default:
		b.WriteString(t.table.View())
	}
	return b.String()
}

// groupHeader は Group: <name> と、2 つ以上あるときの現在位置を返す。
func (t *rdsInstanceParametersTab) groupHeader() string {
	if len(t.groups) == 0 {
		return ""
	}
	header := "Group: " + t.groups[t.index]
	if len(t.groups) > 1 {
		header += " (" + strconv.Itoa(t.index+1) + "/" + strconv.Itoa(len(t.groups)) + ")"
	}
	return header
}

func (t *rdsInstanceParametersTab) SetSize(width, height int) {
	t.width, t.height = width, height
	tableHeight := height - 1
	if tableHeight < 1 {
		tableHeight = 1
	}
	cursor := t.table.Cursor()
	t.table = newTabList(rdsParameterColumns(), tabRows(t.rows), width, tableHeight)
	t.table.SetCursor(cursor)
}

func (t *rdsInstanceParametersTab) reload() tea.Cmd {
	t.loadSeq++
	t.loading = true
	t.err = nil
	seq := t.loadSeq
	group := t.groups[t.index]
	ctx, profile, region, load := t.ctx, t.profile, t.region, t.load
	return func() tea.Msg {
		params, err := load(ctx, profile, region, group)
		return rdsParamsLoadedMsg{seq: seq, params: params, err: err}
	}
}

func rdsParameterColumns() []Column {
	return []Column{
		{Title: "Name"}, {Title: "Value"}, {Title: "Apply type"}, {Title: "Data type"}, {Title: "Modifiable"}, {Title: "Source"},
	}
}

// tabRows は文字列の表を listRow に変換する。
func tabRows(rows [][]string) []listRow {
	result := make([]listRow, 0, len(rows))
	for _, cells := range rows {
		result = append(result, listRow{cells: cells})
	}
	return result
}

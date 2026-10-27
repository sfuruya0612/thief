package tui

import (
	"context"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfuruya0612/thief/backend/internal/aws"
)

type s3LoadedMsg struct {
	seq       int
	objects   []aws.S3ObjectResource
	prefixes  []string
	truncated bool
	err       error
}

type s3ItemKind int

const (
	s3ItemUp s3ItemKind = iota
	s3ItemFolder
	s3ItemObject
)

type s3Item struct {
	kind   s3ItemKind
	value  string
	object aws.S3ObjectResource
}

// s3ObjectsTab は S3 の Objects タブ。prefix の階層移動、検索、階層 / フラット表示を持つ。
type s3ObjectsTab struct {
	ctx         context.Context
	profile     string
	region      string
	bucket      string
	listObjects func(ctx context.Context, profile, region, bucket, prefix, delimiter string) ([]aws.S3ObjectResource, []string, bool, error)

	currentPrefix string
	searchPrefix  string
	flat          bool
	searching     bool
	searchInput   textinput.Model

	table     table.Model
	items     []s3Item
	loading   bool
	err       error
	truncated bool
	loadSeq   int
	width     int
	height    int
}

func newS3ObjectsTab(
	ctx context.Context,
	profile, region, bucket, bucketRegion string,
	listObjects func(ctx context.Context, profile, region, bucket, prefix, delimiter string) ([]aws.S3ObjectResource, []string, bool, error),
) *s3ObjectsTab {
	si := textinput.New()
	si.Prompt = "prefix: "
	si.Placeholder = "reports/"
	si.CharLimit = 256
	t := &s3ObjectsTab{
		ctx:         ctx,
		profile:     profile,
		region:      bucketRegion,
		bucket:      bucket,
		listObjects: listObjects,
		searchInput: si,
		table:       newTabList([]Column{{Title: "Key"}, {Title: "Size"}, {Title: "Last modified"}, {Title: "Storage class"}}, nil, 1, 1),
		loading:     true,
	}
	return t
}

func (t *s3ObjectsTab) Init() tea.Cmd { return t.reload() }

func (t *s3ObjectsTab) Update(msg tea.Msg) (Tab, tea.Cmd, *TabIntent, bool) {
	switch msg := msg.(type) {
	case s3LoadedMsg:
		if msg.seq != t.loadSeq {
			return t, nil, nil, true
		}
		t.loading = false
		t.err = msg.err
		t.truncated = msg.truncated
		t.items = nil
		if msg.err == nil {
			t.items = t.buildItems(msg.prefixes, msg.objects)
		}
		t.SetSize(t.width, t.height)
		t.table.SetCursor(0)
		return t, nil, nil, true
	case tea.KeyMsg:
		return t.updateKey(msg)
	}
	return t, nil, nil, false
}

func (t *s3ObjectsTab) updateKey(msg tea.KeyMsg) (Tab, tea.Cmd, *TabIntent, bool) {
	if t.searching {
		switch msg.String() {
		case "enter":
			t.searching = false
			t.searchInput.Blur()
			t.searchPrefix = normalizeS3Prefix(t.searchInput.Value())
			t.SetSize(t.width, t.height)
			return t, t.reload(), nil, true
		case "esc":
			t.searching = false
			t.searchInput.Blur()
			t.SetSize(t.width, t.height)
			return t, nil, nil, true
		case "tab":
			// 検索入力中の tab は Drawer のフォーカス移動に使わない。S3 の prefix に
			// タブ文字を入れる用途は無いため、入力欄が消費するだけにする。
			return t, nil, nil, true
		}
		var cmd tea.Cmd
		t.searchInput, cmd = t.searchInput.Update(msg)
		return t, cmd, nil, true
	}

	switch msg.String() {
	case "f":
		t.searching = true
		t.searchInput.SetValue(t.searchPrefix)
		t.searchInput.CursorEnd()
		t.SetSize(t.width, t.height)
		return t, t.searchInput.Focus(), nil, true
	case "m":
		t.flat = !t.flat
		return t, t.reload(), nil, true
	case "r":
		return t, t.reload(), nil, true
	case "enter":
		if row := t.selected(); row != nil {
			switch row.kind {
			case s3ItemUp:
				return t, t.goParent(), nil, true
			case s3ItemFolder:
				t.currentPrefix = row.value
				t.searchPrefix = ""
				return t, t.reload(), nil, true
			}
		}
		return t, nil, nil, true
	case "esc", "backspace":
		if t.currentPrefix+t.searchPrefix != "" {
			return t, t.goParent(), nil, true
		}
		return t, nil, nil, true
	case "up", "k", "down", "j", "pgup", "pgdown", "home", "end":
		var cmd tea.Cmd
		t.table, cmd = t.table.Update(msg)
		return t, cmd, nil, true
	}
	return t, nil, nil, false
}

func (t *s3ObjectsTab) View(width, height int) string {
	if width != t.width || height != t.height {
		t.SetSize(width, height)
	}
	var b strings.Builder
	if t.searching {
		b.WriteString(t.searchInput.View())
		b.WriteString("\n")
	}
	switch {
	case t.loading:
		b.WriteString(mutedStyle.Render("Loading..."))
	case t.err != nil:
		b.WriteString(errorStyle.Render(wrap(t.err.Error(), width)))
	default:
		b.WriteString(t.table.View())
		if !t.hasEntries() {
			b.WriteString("\n")
			b.WriteString(mutedStyle.Render("No objects"))
		}
		if t.truncated {
			b.WriteString("\n")
			b.WriteString(mutedStyle.Render("Showing the first 1000 entries; narrow the prefix to see more"))
		}
	}
	mode := "hierarchy"
	if t.flat {
		mode = "flat"
	}
	header := strings.TrimSuffix(t.currentPrefix+t.searchPrefix, "/")
	if header == "" {
		header = "/"
	}
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render(header + "  [" + mode + "]"))
	return b.String()
}

func (t *s3ObjectsTab) SetSize(width, height int) {
	t.width, t.height = width, height
	reserved := 1 // 現在の prefix と表示モード
	if t.searching {
		reserved++
	}
	if t.truncated {
		reserved++
	}
	tableHeight := height - reserved
	if tableHeight < 1 {
		tableHeight = 1
	}
	cursor := t.table.Cursor()
	t.table = newTabList(t.columns(), t.itemRows(t.items), width, tableHeight)
	if cursor > 0 {
		t.table.SetCursor(cursor)
	}
	t.searchInput.Width = width - len(t.searchInput.Prompt)
	if t.searchInput.Width < 1 {
		t.searchInput.Width = 1
	}
}

// hasEntries は .. 以外のフォルダかオブジェクトがあるかを返す。
func (t *s3ObjectsTab) hasEntries() bool {
	for _, item := range t.items {
		if item.kind != s3ItemUp {
			return true
		}
	}
	return false
}

// goParent は表示中の prefix (今いるフォルダと検索 prefix の連結) の親へ移動する。
func (t *s3ObjectsTab) goParent() tea.Cmd {
	combined := t.currentPrefix + t.searchPrefix
	if combined == "" {
		return nil
	}
	t.currentPrefix = parentS3Prefix(combined)
	t.searchPrefix = ""
	return t.reload()
}

func (t *s3ObjectsTab) reload() tea.Cmd {
	t.loadSeq++
	t.loading = true
	t.err = nil
	seq := t.loadSeq
	prefix := t.currentPrefix + t.searchPrefix
	delimiter := "/"
	if t.flat {
		delimiter = ""
	}
	ctx, profile, region, bucket, list := t.ctx, t.profile, t.region, t.bucket, t.listObjects
	return func() tea.Msg {
		objects, prefixes, truncated, err := list(ctx, profile, region, bucket, prefix, delimiter)
		return s3LoadedMsg{seq: seq, objects: objects, prefixes: prefixes, truncated: truncated, err: err}
	}
}

func (t *s3ObjectsTab) buildItems(prefixes []string, objects []aws.S3ObjectResource) []s3Item {
	items := make([]s3Item, 0, len(prefixes)+len(objects)+1)
	if t.currentPrefix+t.searchPrefix != "" {
		items = append(items, s3Item{kind: s3ItemUp})
	}
	sortedPrefixes := append([]string(nil), prefixes...)
	sort.Strings(sortedPrefixes)
	for _, prefix := range sortedPrefixes {
		items = append(items, s3Item{kind: s3ItemFolder, value: prefix})
	}
	sortedObjects := append([]aws.S3ObjectResource(nil), objects...)
	sort.Slice(sortedObjects, func(i, j int) bool { return sortedObjects[i].Key < sortedObjects[j].Key })
	for _, object := range sortedObjects {
		items = append(items, s3Item{kind: s3ItemObject, value: object.Key, object: object})
	}
	return items
}

func (t *s3ObjectsTab) itemRows(items []s3Item) []listRow {
	rows := make([]listRow, 0, len(items))
	for _, item := range items {
		switch item.kind {
		case s3ItemUp:
			rows = append(rows, listRow{cells: []string{"..", dash, dash, dash}, value: item})
		case s3ItemFolder:
			rows = append(rows, listRow{cells: []string{item.value, dash, dash, dash}, value: item})
		default:
			rows = append(rows, listRow{cells: []string{item.object.Key, formatBytes(item.object.Size), orDash(item.object.LastModified), item.object.StorageClass}, value: item})
		}
	}
	return rows
}

func (t *s3ObjectsTab) columns() []Column {
	return []Column{{Title: "Key"}, {Title: "Size", Align: AlignRight}, {Title: "Last modified"}, {Title: "Storage class"}}
}

func (t *s3ObjectsTab) selected() *s3Item {
	index := t.table.Cursor()
	if index < 0 || index >= len(t.items) {
		return nil
	}
	return &t.items[index]
}

func normalizeS3Prefix(s string) string {
	s = strings.TrimSpace(s)
	return strings.TrimLeft(s, "/")
}

func parentS3Prefix(prefix string) string {
	trimmed := strings.TrimSuffix(prefix, "/")
	if i := strings.LastIndex(trimmed, "/"); i >= 0 {
		return trimmed[:i+1]
	}
	return ""
}

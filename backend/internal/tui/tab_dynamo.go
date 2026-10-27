package tui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

const (
	dynamoFieldPK = iota
	dynamoFieldSK
	dynamoFieldAttrName
	dynamoFieldAttrValue
	dynamoFieldLimit
	dynamoFieldCount
)

type dynamoLoadedMsg struct {
	seq       int
	schema    aws.DynamoTableSchema
	schemaErr error
	items     []map[string]any
	itemsErr  error
}

type dynamoQueryInput struct {
	pk        string
	sk        string
	attrName  string
	attrValue string
	limit     int32
}

// dynamoItemsTab は DynamoDB Items タブ。既定のスキャン、検索フォーム、項目の JSON 詳細を持つ。
type dynamoItemsTab struct {
	ctx        context.Context
	profile    string
	region     string
	tableName  string
	describe   func(ctx context.Context, profile, region, table string) (aws.DynamoTableSchema, error)
	queryItems func(ctx context.Context, profile, region, table string, req aws.DynamoItemQuery) ([]map[string]any, error)

	schema    aws.DynamoTableSchema
	schemaErr error

	items      []map[string]any
	columns    []string
	table      table.Model
	listHeight int
	viewport   viewport.Model
	loading    bool
	itemsErr   error
	loadSeq    int

	form      bool
	fields    []textinput.Model
	focus     int
	formErr   string
	submitted dynamoQueryInput

	detail     bool
	detailText string

	width  int
	height int
}

func newDynamoItemsTab(
	ctx context.Context,
	profile, region, tableName string,
	describe func(ctx context.Context, profile, region, table string) (aws.DynamoTableSchema, error),
	queryItems func(ctx context.Context, profile, region, table string, req aws.DynamoItemQuery) ([]map[string]any, error),
) *dynamoItemsTab {
	fields := make([]textinput.Model, dynamoFieldCount)
	for i := range fields {
		ti := textinput.New()
		ti.CharLimit = 256
		fields[i] = ti
	}
	fields[dynamoFieldLimit].SetValue("10")
	return &dynamoItemsTab{
		ctx:        ctx,
		profile:    profile,
		region:     region,
		tableName:  tableName,
		describe:   describe,
		queryItems: queryItems,
		fields:     fields,
		submitted:  dynamoQueryInput{limit: 10},
		table:      newTabList([]Column{{Title: "Item"}}, nil, 1, 1),
		viewport:   viewport.New(0, 0),
		loading:    true,
	}
}

func (t *dynamoItemsTab) Init() tea.Cmd { return t.reload(t.submitted) }

func (t *dynamoItemsTab) Update(msg tea.Msg) (Tab, tea.Cmd, *TabIntent, bool) {
	switch msg := msg.(type) {
	case dynamoLoadedMsg:
		if msg.seq != t.loadSeq {
			return t, nil, nil, true
		}
		t.loading = false
		if msg.schemaErr == nil {
			t.schema = msg.schema
			t.schemaErr = nil
			// スキーマが変わって SK が無くなった場合に、無効な入力欄へフォーカスを残さない。
			if t.focus == dynamoFieldSK && !t.hasSortKey() {
				t.moveFocus(1)
			}
		} else {
			t.schema = aws.DynamoTableSchema{}
			t.schemaErr = msg.schemaErr
		}
		t.itemsErr = msg.itemsErr
		if msg.itemsErr == nil {
			t.items = msg.items
			t.SetSize(t.width, t.height)
			t.table.SetCursor(0)
		}
		return t, nil, nil, true
	case tea.KeyMsg:
		return t.updateKey(msg)
	}
	return t, nil, nil, false
}

func (t *dynamoItemsTab) updateKey(msg tea.KeyMsg) (Tab, tea.Cmd, *TabIntent, bool) {
	if t.form {
		return t.updateForm(msg)
	}
	if t.detail {
		switch msg.String() {
		case "esc":
			t.detail = false
			return t, nil, nil, true
		case "up", "k", "down", "j", "pgup", "pgdown", "home", "end":
			var cmd tea.Cmd
			t.viewport, cmd = t.viewport.Update(msg)
			return t, cmd, nil, true
		}
		return t, nil, nil, false
	}

	switch msg.String() {
	case "f":
		t.openForm()
		return t, nil, nil, true
	case "l":
		switch t.submitted.limit {
		case 10:
			t.submitted.limit = 50
		case 50:
			t.submitted.limit = 100
		default:
			t.submitted.limit = 10
		}
		return t, t.reload(t.submitted), nil, true
	case "r":
		return t, t.reload(t.submitted), nil, true
	case "enter":
		if item := t.selectedItem(); item != nil {
			text, err := json.MarshalIndent(item, "", "  ")
			if err != nil {
				t.detailText = err.Error()
			} else {
				t.detailText = string(text)
			}
			t.detail = true
			t.viewport.SetContent(t.detailText)
			t.viewport.GotoTop()
			return t, nil, nil, true
		}
		return t, nil, nil, true
	case "up", "k", "down", "j", "pgup", "pgdown", "home", "end":
		var cmd tea.Cmd
		t.table, cmd = t.table.Update(msg)
		return t, cmd, nil, true
	}
	return t, nil, nil, false
}

func (t *dynamoItemsTab) updateForm(msg tea.KeyMsg) (Tab, tea.Cmd, *TabIntent, bool) {
	switch msg.String() {
	case "esc":
		t.closeForm()
		return t, nil, nil, true
	case "enter":
		input, err := t.formInput()
		if err != nil {
			t.formErr = err.Error()
			return t, nil, nil, true
		}
		t.formErr = ""
		t.submitted = input
		// 検索中もフォームと前回の一覧を残す。成功してもフォームは esc で閉じるまで開いたままにする。
		return t, t.reload(input), nil, true
	case "tab":
		t.moveFocus(1)
		return t, nil, nil, true
	case "shift+tab":
		t.moveFocus(-1)
		return t, nil, nil, true
	}
	if t.focus == dynamoFieldLimit {
		switch msg.String() {
		case "l", "right":
			t.setLimitField(nextDynamoLimit(t.limitField(), 1))
			return t, nil, nil, true
		case "left":
			t.setLimitField(nextDynamoLimit(t.limitField(), -1))
			return t, nil, nil, true
		}
	}
	if t.focus == dynamoFieldSK && !t.hasSortKey() {
		return t, nil, nil, true
	}
	var cmd tea.Cmd
	t.fields[t.focus], cmd = t.fields[t.focus].Update(msg)
	return t, cmd, nil, true
}

func (t *dynamoItemsTab) View(width, height int) string {
	if width != t.width || height != t.height {
		t.SetSize(width, height)
	}
	if t.detail {
		return t.viewport.View()
	}
	if t.form {
		var b strings.Builder
		b.WriteString(t.formView(width))
		b.WriteString("\n")
		b.WriteString(t.listView(width))
		return b.String()
	}
	return t.listView(width)
}

func (t *dynamoItemsTab) listView(width int) string {
	var b strings.Builder
	switch {
	case t.loading:
		b.WriteString(mutedStyle.Render("Loading..."))
		b.WriteString("\n")
	case t.itemsErr != nil:
		b.WriteString(errorStyle.Render(wrap(t.itemsErr.Error(), width)))
		b.WriteString("\n")
	}
	if t.schemaErr != nil {
		b.WriteString(errorStyle.Render(wrap("schema: "+t.schemaErr.Error(), width)))
		b.WriteString("\n")
	}
	if len(t.items) == 0 && !t.loading && t.itemsErr == nil {
		b.WriteString(mutedStyle.Render("No items"))
		b.WriteString("\n")
	}
	b.WriteString(t.table.View())
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render(t.summary()))
	return b.String()
}

func (t *dynamoItemsTab) summary() string {
	return fmt.Sprintf("PK: %s  SK: %s  Filter: %s=%s  Limit: %d  [f query]",
		orDash(t.submitted.pk), orDash(t.submitted.sk), orDash(t.submitted.attrName), orDash(t.submitted.attrValue), t.submitted.limit)
}

func (t *dynamoItemsTab) SetSize(width, height int) {
	t.width, t.height = width, height
	t.listHeight = height - t.reservedLines(width)
	if t.listHeight < 1 {
		t.listHeight = 1
	}
	t.rebuildTable()
	t.viewport.Width = width
	t.viewport.Height = height
}

// reservedLines は表の外に使う行数を見積もる。
func (t *dynamoItemsTab) reservedLines(width int) int {
	lines := 1 // summary
	if t.loading {
		lines++
	}
	if t.itemsErr != nil {
		lines += wrappedLines(t.itemsErr.Error(), width)
	}
	if t.schemaErr != nil {
		lines += wrappedLines("schema: "+t.schemaErr.Error(), width)
	}
	if len(t.items) == 0 && !t.loading && t.itemsErr == nil {
		lines++
	}
	if t.form {
		lines += t.formHeight(width) + 1
	}
	return lines
}

func (t *dynamoItemsTab) reload(input dynamoQueryInput) tea.Cmd {
	t.loadSeq++
	t.loading = true
	t.itemsErr = nil
	t.formErr = ""
	seq := t.loadSeq
	ctx, profile, region, tableName := t.ctx, t.profile, t.region, t.tableName
	describe, queryItems := t.describe, t.queryItems
	req := aws.DynamoItemQuery{
		PKValue:   input.pk,
		AttrName:  input.attrName,
		AttrValue: input.attrValue,
		Limit:     input.limit,
	}
	// SK は PK があり、スキーマにソートキーがあるときだけ送る。PK が空の Scan 経路では使われず、
	// スキーマの取得に失敗したときはソートキーの有無が分からないため送らない。
	if input.pk != "" && t.hasSortKey() {
		req.SKValue = input.sk
	}
	return func() tea.Msg {
		schema, schemaErr := describe(ctx, profile, region, tableName)
		items, itemsErr := queryItems(ctx, profile, region, tableName, req)
		return dynamoLoadedMsg{seq: seq, schema: schema, schemaErr: schemaErr, items: items, itemsErr: itemsErr}
	}
}

func (t *dynamoItemsTab) rebuildTable() {
	cursor := t.table.Cursor()
	if len(t.items) == 0 {
		t.table = newTabList(t.columnDefs(), nil, t.width, t.listHeight)
		return
	}
	keys := map[string]bool{}
	for _, item := range t.items {
		for key := range item {
			keys[key] = true
		}
	}
	t.columns = make([]string, 0, len(keys))
	for key := range keys {
		t.columns = append(t.columns, key)
	}
	sort.Strings(t.columns)

	rows := make([]listRow, 0, len(t.items))
	for _, item := range t.items {
		cells := make([]string, 0, len(t.columns))
		for _, key := range t.columns {
			value, ok := item[key]
			if !ok {
				cells = append(cells, "")
				continue
			}
			cells = append(cells, formatDynamoValue(value))
		}
		rows = append(rows, listRow{cells: cells, value: item})
	}
	t.table = newTabList(t.columnDefs(), rows, t.width, t.listHeight)
	t.table.SetCursor(cursor)
}

func (t *dynamoItemsTab) columnDefs() []Column {
	columns := make([]Column, 0, len(t.columns))
	for _, key := range t.columns {
		columns = append(columns, Column{Title: key})
	}
	return columns
}

func (t *dynamoItemsTab) selectedItem() map[string]any {
	index := t.table.Cursor()
	if index < 0 || index >= len(t.items) {
		return nil
	}
	return t.items[index]
}

func (t *dynamoItemsTab) openForm() {
	t.form = true
	t.formErr = ""
	values := []string{t.submitted.pk, t.submitted.sk, t.submitted.attrName, t.submitted.attrValue, strconv.FormatInt(int64(t.submitted.limit), 10)}
	for i := range t.fields {
		t.fields[i].SetValue(values[i])
		t.fields[i].Blur()
	}
	t.focus = dynamoFieldPK
	t.fields[t.focus].Focus()
	t.SetSize(t.width, t.height)
}

func (t *dynamoItemsTab) closeForm() {
	t.form = false
	for i := range t.fields {
		t.fields[i].Blur()
	}
	t.SetSize(t.width, t.height)
}

func (t *dynamoItemsTab) formInput() (dynamoQueryInput, error) {
	limit, err := strconv.ParseInt(strings.TrimSpace(t.fields[dynamoFieldLimit].Value()), 10, 32)
	if err != nil || (limit != 10 && limit != 50 && limit != 100) {
		return dynamoQueryInput{}, fmt.Errorf("limit must be 10, 50, or 100")
	}
	return dynamoQueryInput{
		pk:        strings.TrimSpace(t.fields[dynamoFieldPK].Value()),
		sk:        strings.TrimSpace(t.fields[dynamoFieldSK].Value()),
		attrName:  strings.TrimSpace(t.fields[dynamoFieldAttrName].Value()),
		attrValue: strings.TrimSpace(t.fields[dynamoFieldAttrValue].Value()),
		limit:     int32(limit),
	}, nil
}

func (t *dynamoItemsTab) formView(width int) string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Query items"))
	b.WriteString("\n")
	pkLabel := "PK"
	skLabel := "SK"
	if t.schema.Table.PartitionKey.Name != "" {
		pkLabel = t.schema.Table.PartitionKey.Name
	}
	if t.hasSortKey() {
		skLabel = t.schema.Table.SortKey.Name
	}
	labels := []string{pkLabel, skLabel, "Attribute", "Value", "Limit"}
	for i, field := range t.fields {
		if i == dynamoFieldSK && !t.hasSortKey() {
			reason := "no sort key"
			if t.schemaErr != nil {
				reason = "schema unavailable"
			}
			b.WriteString(mutedStyle.Render("SK: disabled (" + reason + ")"))
			b.WriteString("\n")
			continue
		}
		b.WriteString(mutedStyle.Render(labels[i] + ": "))
		b.WriteString(field.View())
		b.WriteString("\n")
	}
	if t.formErr != "" {
		b.WriteString(errorStyle.Render(wrap(t.formErr, width)))
		b.WriteString("\n")
	}
	b.WriteString(mutedStyle.Render("tab next  shift+tab previous  enter search  esc close"))
	return strings.TrimSuffix(b.String(), "\n")
}

func (t *dynamoItemsTab) formHeight(width int) int {
	lines := 1 + dynamoFieldCount + 1 // 見出し、入力欄、ヒント
	if t.formErr != "" {
		lines += wrappedLines(t.formErr, width)
	}
	return lines
}

func (t *dynamoItemsTab) hasSortKey() bool {
	return t.schemaErr == nil && t.schema.Table.SortKey != nil && t.schema.Table.SortKey.Name != ""
}

func (t *dynamoItemsTab) moveFocus(delta int) {
	if delta == 0 {
		return
	}
	t.fields[t.focus].Blur()
	for i := 0; i < len(t.fields); i++ {
		t.focus = (t.focus + delta + len(t.fields)) % len(t.fields)
		if t.focus != dynamoFieldSK || t.hasSortKey() {
			break
		}
	}
	t.fields[t.focus].Focus()
}

func (t *dynamoItemsTab) limitField() int {
	value, err := strconv.Atoi(strings.TrimSpace(t.fields[dynamoFieldLimit].Value()))
	if err != nil {
		return 10
	}
	return value
}

func (t *dynamoItemsTab) setLimitField(limit int) {
	t.fields[dynamoFieldLimit].SetValue(strconv.Itoa(limit))
	t.fields[dynamoFieldLimit].CursorEnd()
}

// nextDynamoLimit は 10、50、100 の範囲で 1 段階進める。端では止まる。
func nextDynamoLimit(current, delta int) int {
	values := []int{10, 50, 100}
	index := 0
	for i, v := range values {
		if v == current {
			index = i
			break
		}
	}
	index += delta
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}

func formatDynamoValue(value any) string {
	if value == nil {
		return "null"
	}
	switch v := value.(type) {
	case []byte:
		return base64.StdEncoding.EncodeToString(v)
	case string:
		return v
	case map[string]any, []any:
		var b strings.Builder
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(v); err != nil {
			return fmt.Sprint(v)
		}
		return strings.TrimRight(b.String(), "\n")
	default:
		return fmt.Sprint(v)
	}
}

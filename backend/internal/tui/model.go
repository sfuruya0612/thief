package tui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

// Deps は TUI が外部 (AWS、プロセス起動) と接する処理をまとめる。テストで差し替える。
type Deps struct {
	// ListProfiles は ~/.aws/config のプロファイルを返す (aws.ListProfiles)。
	ListProfiles func() ([]aws.Profile, error)
	// ListRegions はプロファイルで有効なリージョンを返す (aws.ListRegions)。
	ListRegions func(ctx context.Context, profile string) ([]aws.RegionResource, error)
	// Services は表示できるサービス。先頭を初期表示にする。
	Services []Service
	// StartSession は対話セッションの子プロセスを組み立てる。返す finalize は子プロセスの
	// 終了後に必ず呼ぶ (AWS 側のセッションの切断)。
	StartSession func(ctx context.Context, profile, region, target string) (*exec.Cmd, func() error, error)
}

type focusArea int

const (
	focusTable focusArea = iota
	focusDrawer
)

type drawerTab int

const (
	tabOverview drawerTab = iota
	tabTags
)

var drawerTabTitles = []string{"Overview", "Tags"}

// 画面の固定部分の行数。ヘッダ 1 行、フッタ 1 行、ペインの枠の上下 2 行。
const (
	headerLines = 1
	footerLines = 1
	borderLines = 2
	// cellPadding は bubbles の table が各列に足す左右の余白 (Padding(0, 1)) の合計。
	cellPadding = 2
	// minBodyHeight は本文に最低限確保する行数。これより小さい端末では描画を省く。
	minBodyHeight = 3
	// defaultStatusTTL はステータス行を表示し続ける時間。過ぎると自動で消える。
	defaultStatusTTL = 6 * time.Second
)

// Model は TUI 全体の状態。bubbletea の Elm 風の構成に従い、Update は値渡しで新しい状態を返す。
type Model struct {
	deps Deps
	ctx  context.Context

	service Service
	profile string
	region  string

	profiles []aws.Profile

	rows    []Row
	visible []Row
	table   table.Model

	filter    textinput.Model
	filtering bool

	drawer viewport.Model
	tab    drawerTab

	focus  focusArea
	picker *picker

	loading bool
	loadSeq int
	spinner spinner.Model

	err        error
	ssoExpired bool

	// status は一時的な通知。statusTTL が過ぎるか、キーを押すと消える。
	// statusSeq は古い自動消去のタイマーを無視するための通し番号。
	status    string
	statusSeq int
	statusTTL time.Duration

	width  int
	height int

	// sessionStarting は s を押してから sessionReadyMsg を受け取るまで真。二度押しで 2 つ目の
	// セッションを開始しないためのガード。inSession は子プロセスの実行中に真。
	sessionStarting bool
	inSession       bool
	sessionFinalize func() error

	// profilePickerPending はプロファイルの一覧が届く前に p を押したことを表す。
	// 一覧が届いたら picker を開く。
	profilePickerPending bool
}

// New は初期状態の Model を返す。deps.Services は 1 つ以上必要。
func New(ctx context.Context, deps Deps, profile, region string) (Model, error) {
	if len(deps.Services) == 0 {
		return Model{}, errors.New("no services configured")
	}

	ti := textinput.New()
	ti.Prompt = "/ "
	ti.Placeholder = "filter"
	ti.CharLimit = 128

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	sp.Style = lipgloss.NewStyle().Foreground(accentColor)

	styles := table.DefaultStyles()
	styles.Header = styles.Header.Bold(true).Foreground(mutedColor).BorderStyle(lipgloss.NormalBorder()).BorderBottom(true).BorderForeground(mutedColor)
	styles.Selected = styles.Selected.Foreground(lipgloss.Color("0")).Background(accentColor).Bold(false)

	m := Model{
		deps:    deps,
		ctx:     ctx,
		service: deps.Services[0],
		profile: profile,
		region:  region,
		filter:  ti,
		spinner: sp,
		table:   table.New(table.WithFocused(true), table.WithStyles(styles)),
		drawer:  viewport.New(0, 0),
		loading: true,

		statusTTL: defaultStatusTTL,
	}
	m.table.SetColumns(m.tableColumns(0))
	return m, nil
}

// --- メッセージ ---

type profilesLoadedMsg struct {
	profiles []aws.Profile
	err      error
}

type regionsLoadedMsg struct {
	regions []aws.RegionResource
	err     error
}

type rowsLoadedMsg struct {
	seq  int
	rows []Row
	err  error
}

type sessionReadyMsg struct {
	cmd      *exec.Cmd
	finalize func() error
	err      error
}

type sessionExitMsg struct {
	err error
}

type sessionClosedMsg struct {
	err error
}

// statusExpiredMsg はステータス行の自動消去のタイマー。seq が現在の statusSeq と
// 一致するときだけ消す。
type statusExpiredMsg struct {
	seq int
}

// Init は起動時にプロファイルと一覧の取得を始める。
// Init は Model を変更できないため、New が loading を立て loadSeq を 0 に置いた状態を
// そのまま使い、loadCmd が seq 0 の結果を返す。
func (m Model) Init() tea.Cmd {
	// spinner.Tick は loadCmd が含むため、ここでは重ねない。
	return tea.Batch(m.loadProfilesCmd(), m.loadCmd())
}

func (m Model) loadProfilesCmd() tea.Cmd {
	list := m.deps.ListProfiles
	if list == nil {
		return nil
	}
	return func() tea.Msg {
		profiles, err := list()
		return profilesLoadedMsg{profiles: profiles, err: err}
	}
}

func (m Model) loadRegionsCmd() tea.Cmd {
	list := m.deps.ListRegions
	if list == nil {
		return nil
	}
	ctx, profile := m.ctx, m.profile
	return func() tea.Msg {
		regions, err := list(ctx, profile)
		return regionsLoadedMsg{regions: regions, err: err}
	}
}

// startLoad は一覧の取得をやり直す。seq を進めるため、先に走っていた取得の結果は捨てられる。
func (m Model) startLoad() (Model, tea.Cmd) {
	m.loadSeq++
	m.loading = true
	m.err = nil
	return m, m.loadCmd()
}

// loadCmd は現在のサービス、プロファイル、リージョンで一覧を取得する Cmd を返す。
// 結果には現在の loadSeq を添え、Update は loadSeq が一致する結果だけを受け取る。
func (m Model) loadCmd() tea.Cmd {
	seq, ctx, svc, profile, region := m.loadSeq, m.ctx, m.service, m.profile, m.region
	load := func() tea.Msg {
		rows, err := svc.Load(ctx, profile, region)
		return rowsLoadedMsg{seq: seq, rows: rows, err: err}
	}
	return tea.Batch(load, m.spinner.Tick)
}

// Update はメッセージを処理して次の状態を返す。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m = m.resize()
		return m, nil

	case spinner.TickMsg:
		if !m.loading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case profilesLoadedMsg:
		pending := m.profilePickerPending
		m.profilePickerPending = false
		if msg.err != nil {
			return m.setStatus(fmt.Sprintf("list profiles: %v", msg.err))
		}
		m.profiles = msg.profiles
		if pending {
			m, _ = m.setStatus("")
			return m.openProfilePicker()
		}
		return m, nil

	case regionsLoadedMsg:
		if msg.err != nil {
			return m.setStatus(fmt.Sprintf("list regions: %v", msg.err))
		}
		items := make([]pickerItem, 0, len(msg.regions))
		for _, r := range msg.regions {
			items = append(items, pickerItem{Label: r.Code, Detail: r.Name, Value: r.Code})
		}
		p := newPicker(pickerRegion, "Select a region", items, m.region)
		m.picker = &p
		return m.setStatus("")

	case rowsLoadedMsg:
		if msg.seq != m.loadSeq {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			// Web UI の writeAWSError と同じ順で判定する。aws.IsSSOTokenExpired は "not authorized" の
			// 部分一致で IAM の権限不足にも当たるため、先に aws.IsAccessDenied で除く。
			m.ssoExpired = !aws.IsAccessDenied(msg.err) && aws.IsSSOTokenExpired(msg.err)
			m.rows = nil
		} else {
			m.ssoExpired = false
			m.rows = msg.rows
		}
		// バナーの有無で本文の高さが変わるため、寸法を取り直してから行を流し込む。
		m = m.resize()
		m = m.applyFilter()
		return m, nil

	case sessionReadyMsg:
		m.sessionStarting = false
		if msg.err != nil {
			return m.setStatus(fmt.Sprintf("session: %v", msg.err))
		}
		m.inSession = true
		m.sessionFinalize = msg.finalize
		m, _ = m.setStatus("")
		return m, tea.ExecProcess(msg.cmd, func(err error) tea.Msg { return sessionExitMsg{err: err} })

	case sessionExitMsg:
		m.inSession = false
		finalize := m.sessionFinalize
		m.sessionFinalize = nil
		exitErr := msg.err
		return m, func() tea.Msg {
			var err error
			if finalize != nil {
				err = finalize()
			}
			if exitErr != nil {
				err = errors.Join(fmt.Errorf("session process: %w", exitErr), err)
			}
			return sessionClosedMsg{err: err}
		}

	case sessionClosedMsg:
		if msg.err != nil {
			return m.setStatus(fmt.Sprintf("session closed with error: %v", msg.err))
		}
		return m.setStatus("session closed")

	case statusExpiredMsg:
		if msg.seq != m.statusSeq {
			return m, nil
		}
		return m.setStatus("")

	case tea.KeyMsg:
		return m.updateKey(msg)
	}

	return m, nil
}

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// キーを押したら通知は読んだものとみなして消す。この後の処理が新しい通知を出す
	// 場合はそちらが残る。
	if m.status != "" {
		m, _ = m.setStatus("")
	}
	if m.picker != nil {
		return m.updatePicker(msg)
	}
	if m.filtering {
		return m.updateFilter(msg)
	}

	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "/":
		m.filtering = true
		m.focus = focusTable
		return m, m.filter.Focus()
	case "tab":
		if m.focus == focusTable {
			m.focus = focusDrawer
		} else {
			m.focus = focusTable
		}
		return m, nil
	case "1", "2":
		m.tab = drawerTab(int(msg.String()[0] - '1'))
		m = m.refreshDrawer()
		return m, nil
	case "r":
		return m.startLoad()
	case "p":
		if len(m.profiles) == 0 && m.deps.ListProfiles != nil {
			// 起動直後で一覧が届いていないか、取得に失敗した後。取り直し、届いたら picker を開く。
			m.profilePickerPending = true
			m, statusCmd := m.setStatus("loading profiles...")
			return m, tea.Batch(statusCmd, m.loadProfilesCmd())
		}
		return m.openProfilePicker()
	case "R":
		m, statusCmd := m.setStatus("loading regions...")
		return m, tea.Batch(statusCmd, m.loadRegionsCmd())
	case "S":
		return m.openServicePicker()
	case "s":
		return m.startSession()
	}

	switch m.focus {
	case focusDrawer:
		var cmd tea.Cmd
		m.drawer, cmd = m.drawer.Update(msg)
		return m, cmd
	default:
		before := m.table.Cursor()
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		if m.table.Cursor() != before {
			m = m.refreshDrawer()
		}
		return m, cmd
	}
}

func (m Model) updatePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// ctrl+c はどの画面でもアプリの終了にする。
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	next, done, chosen, cmd := m.picker.update(msg)
	if !done {
		m.picker = &next
		return m, cmd
	}
	kind := m.picker.kind
	m.picker = nil
	if chosen == nil {
		return m, nil
	}
	switch kind {
	case pickerProfile:
		if chosen.Value == m.profile {
			return m, nil
		}
		m.profile = chosen.Value
		return m.reloadForNewTarget()
	case pickerRegion:
		if chosen.Value == m.region {
			return m, nil
		}
		m.region = chosen.Value
		return m.reloadForNewTarget()
	case pickerService:
		if chosen.Value == m.service.ID() {
			return m, nil
		}
		for _, svc := range m.deps.Services {
			if svc.ID() == chosen.Value {
				m.service = svc
				break
			}
		}
		return m.reloadForNewTarget()
	}
	return m, nil
}

// reloadForNewTarget はプロファイル、リージョン、サービスの切替後に一覧を取り直す。
// 旧い対象の行を先に消し、取得中に s を押しても旧い行 (別の対象のリソース ID) で
// セッションを始めないようにする。r の再取得は同じ対象なので行を残す (startLoad)。
func (m Model) reloadForNewTarget() (Model, tea.Cmd) {
	m.rows = nil
	m = m.applyFilter()
	return m.startLoad()
}

func (m Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filter.SetValue("")
		m.filtering = false
		m.filter.Blur()
		m = m.applyFilter()
		return m, nil
	case "enter":
		m.filtering = false
		m.filter.Blur()
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	m = m.applyFilter()
	return m, cmd
}

func (m Model) openProfilePicker() (tea.Model, tea.Cmd) {
	items := make([]pickerItem, 0, len(m.profiles))
	for _, p := range m.profiles {
		detail := p.AccountID
		if p.AuthType == aws.AuthTypeSSO {
			detail = fmt.Sprintf("%s  %s  sso:%s", p.AccountID, p.SSORoleName, p.SSOStatus)
		}
		items = append(items, pickerItem{Label: p.Name, Detail: detail, Value: p.Name})
	}
	p := newPicker(pickerProfile, "Select a profile", items, m.profile)
	m.picker = &p
	return m, nil
}

func (m Model) openServicePicker() (tea.Model, tea.Cmd) {
	items := make([]pickerItem, 0, len(m.deps.Services))
	for _, svc := range m.deps.Services {
		items = append(items, pickerItem{Label: svc.ID(), Detail: svc.Title(), Value: svc.ID()})
	}
	p := newPicker(pickerService, "Select a service", items, m.service.ID())
	m.picker = &p
	return m, nil
}

func (m Model) startSession() (tea.Model, tea.Cmd) {
	if m.sessionStarting || m.inSession {
		return m, nil
	}
	if !m.service.SupportsSession() {
		return m.setStatus(fmt.Sprintf("%s does not support sessions", m.service.Title()))
	}
	if m.deps.StartSession == nil {
		return m.setStatus("sessions are not available")
	}
	row := m.selectedRow()
	if row == nil {
		return m.setStatus("no row selected")
	}
	m.sessionStarting = true
	m, statusCmd := m.setStatus(fmt.Sprintf("starting session to %s...", row.ID()))
	start, ctx, profile, region, target := m.deps.StartSession, m.ctx, m.profile, m.region, row.ID()
	return m, tea.Batch(statusCmd, func() tea.Msg {
		cmd, finalize, err := start(ctx, profile, region, target)
		return sessionReadyMsg{cmd: cmd, finalize: finalize, err: err}
	})
}

// statusLineReplacer は通知を 1 行にするための置換。CRLF を先に置き、CR と LF が別々に 2 つの空白にならないようにする。
var statusLineReplacer = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ")

// setStatus は通知を差し替え、自動消去のタイマーを返す。空文字は通知を消す。
// 通知は 1 行に収めてヒントの行を隠さないため、改行 (errors.Join でまとめた失敗など) は空白に置き換える。
// 子プロセスの出力に由来する CRLF と CR も同じく空白にする。
// 通知行の有無で本文の高さが変わるため、寸法も取り直す。
func (m Model) setStatus(s string) (Model, tea.Cmd) {
	m.status = statusLineReplacer.Replace(s)
	m.statusSeq++
	m = m.resize()
	if s == "" {
		return m, nil
	}
	seq := m.statusSeq
	return m, tea.Tick(m.statusTTL, func(time.Time) tea.Msg { return statusExpiredMsg{seq: seq} })
}

// --- 状態の導出 ---

// applyFilter は rows を filter の値で絞り込んで table に反映する。
func (m Model) applyFilter() Model {
	query := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	m.visible = m.visible[:0]
	for _, r := range m.rows {
		if query == "" || strings.Contains(strings.ToLower(strings.Join(r.Cells(), " ")), query) {
			m.visible = append(m.visible, r)
		}
	}
	ncols := len(m.service.Columns())
	trows := make([]table.Row, 0, len(m.visible))
	for _, r := range m.visible {
		trows = append(trows, table.Row(fitCells(r.Cells(), ncols)))
	}
	// 列を入れ替える前に行を空にする。bubbles の table は SetColumns でも行を描き直し、
	// 行のセルの数が列の数を超えると panic するため (サービスの切替で列数が減るとき、
	// 旧いサービスの行が新しい列で描かれる)。
	m.table.SetRows(nil)
	m.table.SetColumns(m.tableColumns(m.tableInnerWidth()))
	m.table.SetRows(trows)
	// 行が無いときの SetCursor は -1 に丸められるため、負のカーソルも先頭に戻す。
	// 戻さないと、次に行が届いても選択が無いままで Drawer が "No row selected" になる。
	if c := m.table.Cursor(); c < 0 || c >= len(trows) {
		m.table.SetCursor(0)
	}
	return m.refreshDrawer()
}

// selectedRow は table のカーソル位置の行を返す。行が無ければ nil。
func (m Model) selectedRow() Row {
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.visible) {
		return nil
	}
	return m.visible[idx]
}

// refreshDrawer は選択中の行とタブから Drawer の内容を組み直す。
func (m Model) refreshDrawer() Model {
	row := m.selectedRow()
	if row == nil {
		m.drawer.SetContent(mutedStyle.Render("No row selected"))
		m.drawer.GotoTop()
		return m
	}

	var b strings.Builder
	switch m.tab {
	case tabTags:
		tags := row.Tags()
		if len(tags) == 0 {
			b.WriteString(mutedStyle.Render("No tags"))
			break
		}
		keys := make([]string, 0, len(tags))
		for k := range tags {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		width := maxLen(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "%s  %s\n", mutedStyle.Render(padRight(k, width)), tags[k])
		}
	default:
		entries := row.Overview()
		keys := make([]string, 0, len(entries))
		for _, e := range entries {
			keys = append(keys, e.Key)
		}
		width := maxLen(keys)
		fmt.Fprintf(&b, "%s  %s\n", mutedStyle.Render(padRight("Name", width)), row.Name())
		fmt.Fprintf(&b, "%s  %s\n", mutedStyle.Render(padRight("State", width)), stateStyle(aws.NormalizeState(row.State())).Render(row.State()))
		for _, e := range entries {
			fmt.Fprintf(&b, "%s  %s\n", mutedStyle.Render(padRight(e.Key, width)), e.Value)
		}
	}
	m.drawer.SetContent(b.String())
	m.drawer.GotoTop()
	return m
}

// resize は端末の大きさから各部品の寸法を決める。
func (m Model) resize() Model {
	bodyHeight := m.bodyHeight()
	if bodyHeight < minBodyHeight {
		return m
	}
	leftInner := m.tableInnerWidth()
	rightInner := m.width - leftInner - 2*borderLines
	if rightInner < 1 {
		rightInner = 1
	}

	// ペインの 1 行目はタイトルに使う。
	m.table.SetColumns(m.tableColumns(leftInner))
	m.table.SetWidth(leftInner)
	m.table.SetHeight(bodyHeight - 1)
	m.filter.Width = leftInner - len(m.filter.Prompt) - 1

	m.drawer.Width = rightInner
	m.drawer.Height = bodyHeight - 1
	return m
}

// bodyHeight はペインの枠の内側に使える行数。SSO のバナーと通知行はそれぞれ 1 行を取る。
func (m Model) bodyHeight() int {
	h := m.height - headerLines - footerLines - borderLines
	if m.ssoExpired {
		h--
	}
	if m.status != "" {
		h--
	}
	return h
}

// tableInnerWidth は左ペインの枠の内側の幅。画面の 3/5 を左に割り当てる。
func (m Model) tableInnerWidth() int {
	w := m.width*3/5 - borderLines
	if w < 1 {
		w = 1
	}
	return w
}

// minColumnWidth は列を縮めるときの下限。これより狭いと見出しも値も読めない。
const minColumnWidth = 4

// tableColumns は見出しと表示中の行の内容から列幅を決める。
// すべての列が自然幅 (見出しと値の最大幅) で収まればそれを使い、収まらなければ
// 広い列から順に削って innerWidth に合わせる。bubbles は各列に左右 1 文字ずつ余白を
// 足すため、その分も幅に含めて計算する。
func (m Model) tableColumns(innerWidth int) []table.Column {
	cols := m.service.Columns()
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = lipgloss.Width(c.Title)
	}
	for _, r := range m.visible {
		for i, cell := range fitCells(r.Cells(), len(cols)) {
			if w := lipgloss.Width(cell); w > widths[i] {
				widths[i] = w
			}
		}
	}

	shrinkToFit(widths, innerWidth-cellPadding*len(cols))

	out := make([]table.Column, 0, len(cols))
	for i, c := range cols {
		out = append(out, table.Column{Title: c.Title, Width: widths[i]})
	}
	return out
}

// shrinkToFit は widths の合計が budget を超えるとき、最も広い列から 1 文字ずつ削って
// 合計を budget 以下にする。各列は minColumnWidth より狭くしない。すべての列が下限に
// 達しても収まらない場合はそのまま返す (bubbles が末尾の列を切り落とす)。
func shrinkToFit(widths []int, budget int) {
	if len(widths) == 0 {
		return
	}
	total := 0
	for _, w := range widths {
		total += w
	}
	for total > budget {
		widest := -1
		for i, w := range widths {
			if w > minColumnWidth && (widest < 0 || w > widths[widest]) {
				widest = i
			}
		}
		if widest < 0 {
			return
		}
		widths[widest]--
		total--
	}
}

// --- 描画 ---

// View は画面全体を描画する。
func (m Model) View() string {
	if m.width == 0 || m.bodyHeight() < minBodyHeight {
		return "terminal too small"
	}

	var b strings.Builder
	b.WriteString(m.headerView())
	b.WriteString("\n")
	if m.ssoExpired {
		b.WriteString(bannerStyle.Render(fmt.Sprintf("SSO token expired for profile %s. Run `thief sso login` and press r to reload.", m.profile)))
		b.WriteString("\n")
	}
	if m.picker != nil {
		b.WriteString(paneFocusedStyle.Width(m.width - borderLines).Height(m.bodyHeight()).Render(m.picker.view(m.bodyHeight())))
	} else {
		b.WriteString(m.bodyView())
	}
	b.WriteString("\n")
	if m.status != "" {
		b.WriteString(lipgloss.NewStyle().MaxWidth(m.width).Render(statusStyle.Render(m.status)))
		b.WriteString("\n")
	}
	b.WriteString(m.footerView())
	return b.String()
}

func (m Model) headerView() string {
	left := fmt.Sprintf("%s  %s  %s  %s",
		titleStyle.Render("thief"),
		headerStyle.Render(m.service.Title()),
		mutedStyle.Render("profile:")+" "+m.profile,
		mutedStyle.Render("region:")+" "+m.region,
	)
	right := ""
	if m.loading {
		right = m.spinner.View() + " loading"
	}
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return lipgloss.NewStyle().MaxWidth(m.width).Render(left + strings.Repeat(" ", gap) + right)
}

func (m Model) bodyView() string {
	leftInner := m.tableInnerWidth()
	rightInner := m.width - leftInner - 2*borderLines
	bodyHeight := m.bodyHeight()

	var leftTitle string
	if m.filtering {
		leftTitle = m.filter.View()
	} else if m.filter.Value() != "" {
		leftTitle = fmt.Sprintf("%s (%d/%d)  %s", headerStyle.Render(m.service.Title()), len(m.visible), len(m.rows), mutedStyle.Render("/ "+m.filter.Value()))
	} else {
		leftTitle = fmt.Sprintf("%s (%d)", headerStyle.Render(m.service.Title()), len(m.rows))
	}

	var leftBody string
	switch {
	case m.err != nil:
		leftBody = errorStyle.Render(wrap(m.err.Error(), leftInner))
	case m.loading && len(m.rows) == 0:
		leftBody = mutedStyle.Render("loading...")
	case len(m.rows) == 0:
		leftBody = mutedStyle.Render("No " + m.service.Title() + " resources found")
	default:
		leftBody = m.table.View()
	}

	tabs := make([]string, 0, len(drawerTabTitles))
	for i, t := range drawerTabTitles {
		if drawerTab(i) == m.tab {
			tabs = append(tabs, selectedStyle.Render(t))
		} else {
			tabs = append(tabs, mutedStyle.Render(t))
		}
	}
	rightTitle := strings.Join(tabs, mutedStyle.Render(" | "))

	leftStyle, rightStyle := paneStyle, paneStyle
	if m.focus == focusTable {
		leftStyle = paneFocusedStyle
	} else {
		rightStyle = paneFocusedStyle
	}
	left := leftStyle.Width(leftInner).Height(bodyHeight).Render(leftTitle + "\n" + leftBody)
	right := rightStyle.Width(rightInner).Height(bodyHeight).Render(rightTitle + "\n" + m.drawer.View())
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m Model) footerView() string {
	keys := []struct{ key, desc string }{
		{"/", "filter"}, {"j/k", "move"}, {"tab", "focus"}, {"1/2", "tabs"},
		{"s", "session"}, {"r", "reload"}, {"p", "profile"}, {"R", "region"},
	}
	if len(m.deps.Services) > 1 {
		keys = append(keys, struct{ key, desc string }{"S", "service"})
	}
	keys = append(keys, struct{ key, desc string }{"q", "quit"})
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, keyStyle.Render(k.key)+" "+mutedStyle.Render(k.desc))
	}
	return lipgloss.NewStyle().MaxWidth(m.width).Render(strings.Join(parts, "  "))
}

// --- 文字列ヘルパー ---

// fitCells はセル数を列数に揃える。bubbles の table は行のセル数が列数を超えると
// 描画時に panic するため、Service の実装の不一致を描画の崩れに留める。
func fitCells(cells []string, n int) []string {
	if len(cells) == n {
		return cells
	}
	out := make([]string, n)
	copy(out, cells)
	return out
}

func maxLen(ss []string) int {
	n := 0
	for _, s := range ss {
		if w := lipgloss.Width(s); w > n {
			n = w
		}
	}
	return n
}

func padRight(s string, width int) string {
	if pad := width - lipgloss.Width(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

// wrap は幅で折り返す。lipgloss の Width は折り返しも行うため、そのまま使う。
func wrap(s string, width int) string {
	if width < 1 {
		return s
	}
	return lipgloss.NewStyle().Width(width).Render(s)
}

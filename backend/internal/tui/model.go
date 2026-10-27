package tui

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

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
	// OpenEC2Session は EC2 の Session Manager のセッションを開始し、TUI のフロートウィンドウが
	// 使う InteractiveSession を返す。
	OpenEC2Session func(ctx context.Context, profile, region, target string) (InteractiveSession, error)
	// ListECSTasks はクラスタのタスクを返す (aws.ListECSTasks)。TUI は service に空文字列を渡す。
	ListECSTasks func(ctx context.Context, profile, region, cluster, service string) ([]aws.ECSTaskResource, error)
	// ListECSContainers はタスクのコンテナを返す (aws.ListECSContainers)。task はタスク ID。
	ListECSContainers func(ctx context.Context, profile, region, cluster, task string) ([]aws.ECSContainerResource, error)
	// OpenECSExecSession は ECS Exec のセッションを開始し、TUI のフロートウィンドウが使う
	// InteractiveSession を返す。cluster はクラスタ名、task はタスク ID、container はコンテナ名。
	OpenECSExecSession func(ctx context.Context, profile, region, cluster, task, container, command string) (InteractiveSession, error)
}

// ecsExecState は ECS Exec の接続先の選択の進行状態。s を押してから OpenECSExecSession を呼ぶまで active が真で、
// その間 sessionStarting も真。seq は選択を始めたときの ecsSeq で、Update は一致する結果だけを受け取る。
type ecsExecState struct {
	active  bool
	seq     int
	cluster string
	task    string
}

type focusArea int

const (
	focusTable focusArea = iota
	focusDrawer
)

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

	drawer    viewport.Model
	tabID     string
	tabSeq    int
	tabCancel context.CancelFunc
	activeTab Tab

	focus  focusArea
	picker *picker

	loading bool
	loadSeq int
	spinner spinner.Model

	// tags は TagFetcher を実装する行から取ったタグ (か失敗) を行の ID() ごとに保持する。
	// 一覧の読み込み (startLoad) で捨て、同じ読み込みの中では同じ行に FetchTags を呼び直さない。
	// tagsPending は取得中の行の ID()。結果が届く前に同じ行を選び直しても二重に呼ばないためのガード。
	tags        map[string]tagsResult
	tagsPending map[string]bool

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
	// セッションを開始しないためのガード。sessionSeq は開始の世代で、対象の切替や終了後に
	// 届いた古い結果を捨てる。sessionStartCancel は開始中のセッション開始を取り消す。
	sessionStarting    bool
	sessionSeq         int
	sessionStartCancel context.CancelFunc
	sessionStartSlot   *sessionStartSlot
	session            *sessionWindow
	closedSessionSeq   int

	// ecsExec は ECS Exec の接続先の選択の進行状態。ecsSeq は選択を始めるたびに進め、ecsExec.seq に写す。
	// 届いた結果は ecsExec.active が真で seq が一致するときだけ使う。選択の取り消しと対象の切替
	// (reloadForNewTarget) は cancelECSExec で ecsExec を空にし (active が偽、seq が 0)、
	// 旧い選択のタスクとコンテナの結果を捨てる。
	ecsExec ecsExecState
	ecsSeq  int

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
		deps:             deps,
		ctx:              ctx,
		service:          deps.Services[0],
		profile:          profile,
		region:           region,
		filter:           ti,
		spinner:          sp,
		table:            table.New(table.WithFocused(true), table.WithStyles(styles)),
		drawer:           viewport.New(0, 0),
		tabID:            tabOverview,
		loading:          true,
		sessionStartSlot: newSessionStartSlot(),

		tags:        map[string]tagsResult{},
		tagsPending: map[string]bool{},

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

// tagsResult は TagFetcher.FetchTags の結果。err が nil でなければ取得失敗。
type tagsResult struct {
	tags map[string]string
	err  error
}

// tagsFetchedMsg は FetchTags の結果。seq は取得を始めたときの loadSeq で、Update は一致する結果だけを保持する。
type tagsFetchedMsg struct {
	seq  int
	id   string
	tags map[string]string
	err  error
}

// ecsTasksMsg は ListECSTasks の結果。seq は ecsExecState.seq。
type ecsTasksMsg struct {
	seq   int
	tasks []aws.ECSTaskResource
	err   error
}

// ecsContainersMsg は ListECSContainers の結果。task は取得したタスク ID。
type ecsContainersMsg struct {
	seq        int
	task       string
	containers []aws.ECSContainerResource
	err        error
}

// sessionReadyMsg はセッションの開始結果。seq は開始時の sessionSeq。
type sessionReadyMsg struct {
	seq     int
	title   string
	session InteractiveSession
	err     error
}

// sessionOutputMsg はセッションの読み取り結果。seq はウィンドウの世代。
type sessionOutputMsg struct {
	seq    int
	output SessionOutput
	err    error
}

// sessionClosedMsg はセッションの終了結果。seq はウィンドウの世代。
type sessionClosedMsg struct {
	seq int
	err error
}

// sessionCleanupMsg はウィンドウを開けなかったセッションの後始末の結果。
type sessionCleanupMsg struct {
	err error
}

// tabScopedMsg はタブの Cmd の結果をタブの世代とともに運ぶ。
type tabScopedMsg struct {
	instance int
	msg      tea.Msg
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
	// 取得済みのタグは一覧と同じ世代のものなので捨てる。取得中の結果も seq が合わず捨てられる。
	m.tags = map[string]tagsResult{}
	m.tagsPending = map[string]bool{}
	// 捨てた世代のタグを一覧が届くまで Drawer に残さない (Tags タブは取得中の表示に戻る)。
	m = m.resetActiveTab()
	m = m.refreshDrawer()
	return m, m.loadCmd()
}

// ensureTags は選択中の行が TagFetcher を実装し、タグが未取得で取得中でもなければ FetchTags の Cmd を返す。
// 一覧の読み込み中は、表示中の行が次の読み込みで入れ替わるため呼ばない。
func (m Model) ensureTags() (Model, tea.Cmd) {
	if m.loading {
		return m, nil
	}
	row := m.selectedRow()
	if row == nil {
		return m, nil
	}
	f, ok := row.(TagFetcher)
	if !ok {
		return m, nil
	}
	id := row.ID()
	if _, done := m.tags[id]; done || m.tagsPending[id] {
		return m, nil
	}
	m.tagsPending[id] = true
	seq, ctx := m.loadSeq, m.ctx
	return m, func() tea.Msg {
		tags, err := f.FetchTags(ctx)
		return tagsFetchedMsg{seq: seq, id: id, tags: tags, err: err}
	}
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
		if m.session != nil {
			areaWidth, areaHeight := m.sessionAreaSize()
			if width, height, ok := sessionWindowSize(areaWidth, areaHeight); ok &&
				(width != m.session.width || height != m.session.height) {
				m.session.resize(width, height)
			}
			// 本文が最小サイズを下回ったときは、最後のウィンドウサイズを維持する。
			// ここで閉じると、一瞬の端末縮小でセッションを失う。
		}
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
		m = m.resetActiveTab()
		m = m.applyFilter()
		m, tabCmd := m.ensureActiveTab()
		m, tagsCmd := m.ensureTags()
		return m, tea.Batch(tabCmd, tagsCmd)

	case tagsFetchedMsg:
		// r や対象の切替で世代が進んだ後に届いた結果は、旧い一覧のものなので表示も保持もしない。
		if msg.seq != m.loadSeq {
			return m, nil
		}
		delete(m.tagsPending, msg.id)
		m.tags[msg.id] = tagsResult{tags: msg.tags, err: msg.err}
		// 選択中の行の結果だけ Drawer を組み直す。別の行の結果で組み直すと refreshDrawer の GotoTop で
		// 表示中の Drawer のスクロール位置が先頭に戻る。別の行の結果は保持し、その行に戻ったときに出す。
		if row := m.selectedRow(); row != nil && row.ID() == msg.id {
			m = m.refreshDrawer()
		}
		return m, nil

	case ecsTasksMsg:
		if !m.ecsExec.active || msg.seq != m.ecsExec.seq {
			return m, nil
		}
		if msg.err != nil {
			m = m.cancelECSExec()
			return m.setStatus(fmt.Sprintf("list tasks: %v", msg.err))
		}
		if len(msg.tasks) == 0 {
			m = m.cancelECSExec()
			return m.setStatus("no running tasks")
		}
		if len(msg.tasks) == 1 {
			return m.listECSContainers(ecsTaskID(msg.tasks[0].ARN))
		}
		items := make([]pickerItem, 0, len(msg.tasks))
		for _, t := range msg.tasks {
			id := ecsTaskID(t.ARN)
			items = append(items, pickerItem{Label: id, Detail: strings.TrimSpace(t.Group + "  " + t.LastStatus), Value: id})
		}
		p := newPicker(pickerECSTask, fmt.Sprintf("Select a task in %s", m.ecsExec.cluster), items, "")
		m.picker = &p
		m, _ = m.setStatus("")
		return m, nil

	case ecsContainersMsg:
		if !m.ecsExec.active || msg.seq != m.ecsExec.seq {
			return m, nil
		}
		if msg.err != nil {
			m = m.cancelECSExec()
			return m.setStatus(fmt.Sprintf("list containers: %v", msg.err))
		}
		// Web UI の ECSExecLauncher と同じく、ECS Exec が有効なコンテナだけを候補にする。
		candidates := make([]aws.ECSContainerResource, 0, len(msg.containers))
		for _, c := range msg.containers {
			if c.ExecEnabled {
				candidates = append(candidates, c)
			}
		}
		if len(candidates) == 0 {
			m = m.cancelECSExec()
			return m.setStatus("no container with ECS Exec enabled")
		}
		if len(candidates) == 1 {
			return m.startECSExecProcess(candidates[0].Name)
		}
		items := make([]pickerItem, 0, len(candidates))
		for _, c := range candidates {
			items = append(items, pickerItem{Label: c.Name, Detail: c.LastStatus, Value: c.Name})
		}
		p := newPicker(pickerECSContainer, fmt.Sprintf("Select a container in %s", msg.task), items, "")
		m.picker = &p
		m, _ = m.setStatus("")
		return m, nil

	case sessionReadyMsg:
		return m.finishSessionStart(msg)

	case sessionOutputMsg:
		return m.handleSessionOutput(msg)

	case sessionClosedMsg:
		return m.finishSessionClose(msg)

	case sessionCleanupMsg:
		if msg.err != nil {
			return m.setStatus(fmt.Sprintf("session closed with error: %v", msg.err))
		}
		return m, nil

	case statusExpiredMsg:
		if msg.seq != m.statusSeq {
			return m, nil
		}
		return m.setStatus("")

	case tabScopedMsg:
		if msg.instance != m.tabSeq || m.activeTab == nil {
			return m, nil
		}
		next, cmd, intent, handled := m.activeTab.Update(msg.msg)
		if !handled {
			return m, nil
		}
		m.activeTab = next
		return m.applyTabIntent(intent, cmd)

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
	if m.session != nil {
		return m.updateSessionKey(msg)
	}
	if m.picker != nil {
		return m.updatePicker(msg)
	}
	if m.filtering {
		return m.updateFilter(msg)
	}
	if m.sessionStarting {
		switch msg.String() {
		case "/", "p", "R", "S", "r":
			// セッションの開始中に別の画面や再取得を開くと、届いたウィンドウと
			// 選択肢や読み込み表示が重なる。開始が終わるまで待つ。
			return m, nil
		}
	}

	// ctrl+c はタブより先に TUI の終了として扱う。
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	// タブが処理したキーだけを消費し、残りをグローバル操作へ流す。
	if m.activeTab != nil && m.focus == focusDrawer {
		next, cmd, intent, handled := m.activeTab.Update(msg)
		if handled {
			m.activeTab = next
			return m.applyTabIntent(intent, cmd)
		}
	}

	switch msg.String() {
	case "q":
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
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		index := int(msg.String()[0] - '1')
		tabs := m.drawerTabs()
		if index >= 0 && index < len(tabs) {
			return m.switchTab(tabs[index].ID)
		}
		return m, nil
	case "[":
		return m.cycleTab(-1)
	case "]":
		return m.cycleTab(1)
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
			m = m.resetActiveTab()
			m = m.refreshDrawer()
			m, tabCmd := m.ensureActiveTab()
			var tagsCmd tea.Cmd
			m, tagsCmd = m.ensureTags()
			return m, tea.Batch(cmd, tabCmd, tagsCmd)
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
		if kind == pickerECSTask || kind == pickerECSContainer {
			// 取り消しは ECS Exec の開始をやめる。sessionStarting を戻さないと s が効かなくなる。
			m = m.cancelECSExec()
		}
		return m, nil
	}
	switch kind {
	case pickerECSTask:
		return m.listECSContainers(chosen.Value)
	case pickerECSContainer:
		return m.startECSExecProcess(chosen.Value)
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
		// サービスを変えるとタブの集合が変わるため、先頭の Overview に戻す。
		m.tabID = tabOverview
		return m.reloadForNewTarget()
	}
	return m, nil
}

// reloadForNewTarget はプロファイル、リージョン、サービスの切替後に一覧を取り直す。
// 旧い対象の行を先に消し、取得中に s を押しても旧い行 (別の対象のリソース ID) で
// セッションを始めないようにする。r の再取得は同じ対象なので行を残す (startLoad)。
func (m Model) reloadForNewTarget() (Model, tea.Cmd) {
	// 進行中の ECS Exec の接続先の選択は旧い対象のものなのでやめる。届いていない結果は ecsExec が空 (active が偽) なので捨てられる。
	if m.ecsExec.active {
		m = m.cancelECSExec()
	}
	// セッションの開始中に対象が変わったら開始を取り消し、届いた結果を捨てる。
	if m.sessionStartCancel != nil {
		m.sessionStartCancel()
		m.sessionStartCancel = nil
	}
	m.sessionStarting = false
	m.sessionSeq++
	m = m.resetActiveTab()
	m.rows = nil
	m = m.applyFilter()
	return m.startLoad()
}

func (m Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	before := m.selectedRow()
	switch msg.String() {
	case "esc":
		m.filter.SetValue("")
		m.filtering = false
		m.filter.Blur()
		m = m.applyFilter()
		m, tabCmd := m.syncActiveTab(before)
		m, tagsCmd := m.ensureTags()
		return m, tea.Batch(tabCmd, tagsCmd)
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
	// 絞り込みで選択中の行が変わったときだけ、タブを作り直す。同じ行なら
	// サービス固有タブの検索条件や表示位置を保つ。
	m, tabCmd := m.syncActiveTab(before)
	var tagsCmd tea.Cmd
	m, tagsCmd = m.ensureTags()
	return m, tea.Batch(cmd, tabCmd, tagsCmd)
}

// syncActiveTab は絞り込みの前後で選択行が変わったときだけ、タブを作り直す。
func (m Model) syncActiveTab(before Row) (Model, tea.Cmd) {
	if sameRow(before, m.selectedRow()) {
		return m, nil
	}
	m = m.resetActiveTab()
	m = m.refreshDrawer()
	return m.ensureActiveTab()
}

func sameRow(a, b Row) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.ID() == b.ID()
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

// drawerTabs は選択中のサービスのタブを返す。TabProvider を実装しないサービスは
// Web UI と同じく Overview と Tags の 2 タブにする。
func (m Model) drawerTabs() []TabSpec {
	if p, ok := m.service.(TabProvider); ok {
		return p.Tabs()
	}
	return []TabSpec{
		{ID: tabOverview, Title: "Overview"},
		{ID: tabTags, Title: "Tags"},
	}
}

// currentTabSpec は選択中のタブの仕様を返す。
func (m Model) currentTabSpec() (TabSpec, bool) {
	for _, tab := range m.drawerTabs() {
		if tab.ID == m.tabID {
			return tab, true
		}
	}
	return TabSpec{}, false
}

// drawerInnerSize はタブの本文に使える幅と高さを返す。Drawer の枠とタブバーの 1 行を除く。
func (m Model) drawerInnerSize() (width, height int) {
	leftInner := m.tableInnerWidth()
	width = m.width - leftInner - 2*borderLines
	if width < 1 {
		width = 1
	}
	height = m.bodyHeight() - 1
	if height < 1 {
		height = 1
	}
	return width, height
}

// resetActiveTab は現在のタブのサブモデルと、その Cmd を無効にする。
func (m Model) resetActiveTab() Model {
	if m.tabCancel != nil {
		m.tabCancel()
		m.tabCancel = nil
	}
	m.activeTab = nil
	m.tabSeq++
	return m
}

// ensureActiveTab は選択中のタブがサービス固有タブならサブモデルを作り、Init の Cmd を返す。
func (m Model) ensureActiveTab() (Model, tea.Cmd) {
	spec, ok := m.currentTabSpec()
	if !ok || spec.New == nil {
		m.activeTab = nil
		return m, nil
	}
	row := m.selectedRow()
	if row == nil {
		m.activeTab = nil
		return m, nil
	}
	if m.activeTab != nil {
		return m, nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.tabCancel = cancel
	m.tabSeq++
	m.activeTab = spec.New(ctx, m.profile, m.region, row)
	if m.activeTab == nil {
		return m, nil
	}
	width, height := m.drawerInnerSize()
	m.activeTab.SetSize(width, height)
	return m, wrapTabCmd(m.tabSeq, m.activeTab.Init())
}

// switchTab は指定したタブへ切り替える。同じ ID なら何もしない。
func (m Model) switchTab(id string) (Model, tea.Cmd) {
	if id == m.tabID {
		return m, nil
	}
	found := false
	for _, tab := range m.drawerTabs() {
		if tab.ID == id {
			found = true
			break
		}
	}
	if !found {
		return m, nil
	}

	m = m.resetActiveTab()
	m.tabID = id
	m = m.refreshDrawer()
	m, tabCmd := m.ensureActiveTab()
	if id == tabTags {
		m, tagsCmd := m.ensureTags()
		return m, tea.Batch(tabCmd, tagsCmd)
	}
	return m, tabCmd
}

// cycleTab は前後のタブへ移動する。端では止まる。
func (m Model) cycleTab(delta int) (Model, tea.Cmd) {
	tabs := m.drawerTabs()
	if len(tabs) == 0 {
		return m, nil
	}
	index := 0
	for i, tab := range tabs {
		if tab.ID == m.tabID {
			index = i
			break
		}
	}
	index += delta
	if index < 0 {
		index = 0
	}
	if index >= len(tabs) {
		index = len(tabs) - 1
	}
	return m.switchTab(tabs[index].ID)
}

// wrapTabCmd はタブの Cmd の結果にタブの世代を付ける。tea.Batch の内側の Cmd も再帰的に包む。
func wrapTabCmd(instance int, cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			cmds := make([]tea.Cmd, len(batch))
			for i, inner := range batch {
				cmds[i] = wrapTabCmd(instance, inner)
			}
			// tea.Batch ではなく BatchMsg を直接返し、Bubble Tea に内側の Cmd を
			// 実行させる。Cmd を Msg として返すとイベントループが実行しない。
			return tea.BatchMsg(cmds)
		}
		return tabScopedMsg{instance: instance, msg: msg}
	}
}

// applyTabIntent はタブからの要求を Model の操作へ変換する。
func (m Model) applyTabIntent(intent *TabIntent, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	if intent != nil && intent.ECSExec != nil {
		return m.startECSExecTarget(*intent.ECSExec)
	}
	var statusCmd tea.Cmd
	if intent != nil && intent.Status != "" {
		m, statusCmd = m.setStatus(intent.Status)
	}
	return m, tea.Batch(wrapTabCmd(m.tabSeq, cmd), statusCmd)
}

func (m Model) startSession() (tea.Model, tea.Cmd) {
	if m.sessionStarting || m.session != nil {
		return m, nil
	}
	if !m.service.SupportsSession() {
		return m.setStatus(fmt.Sprintf("%s does not support sessions", m.service.Title()))
	}
	if m.service.ID() == ecsServiceID {
		return m.startECSExec()
	}
	if m.deps.OpenEC2Session == nil {
		return m.setStatus("sessions are not available")
	}
	row := m.selectedRow()
	if row == nil {
		return m.setStatus("no row selected")
	}
	areaWidth, areaHeight := m.sessionAreaSize()
	if _, _, ok := sessionWindowSize(areaWidth, areaHeight); !ok {
		return m.setStatus("terminal too small for a session window")
	}

	m.sessionSeq++
	seq := m.sessionSeq
	m.closedSessionSeq = 0
	ctx, cancel := context.WithCancel(m.ctx)
	m.sessionStartCancel = cancel
	m.sessionStarting = true
	m, statusCmd := m.setStatus(fmt.Sprintf("starting session to %s...", row.ID()))
	open, profile, region, target := m.deps.OpenEC2Session, m.profile, m.region, row.ID()
	title := fmt.Sprintf("%s  %s  %s / %s", m.service.Title(), row.ID(), m.profile, m.region)
	slot := m.sessionStartSlot
	return m, tea.Batch(statusCmd, func() tea.Msg {
		session, err := open(ctx, profile, region, target)
		slot.set(session)
		return sessionReadyMsg{seq: seq, title: title, session: session, err: err}
	})
}

// startECSExec は ECS のクラスタの行で s を押したときの入口。タスクの一覧を取り、ecsTasksMsg から
// タスクとコンテナを選ばせて (1 件なら選択肢を出さない)、startECSExecProcess で Deps.OpenECSExecSession を呼ぶ。
// Web UI の ECSExecLauncher と同じ選び方。選択の間は sessionStarting を真にして s の二度押しを止める。
func (m Model) startECSExec() (tea.Model, tea.Cmd) {
	if m.deps.OpenECSExecSession == nil || m.deps.ListECSTasks == nil || m.deps.ListECSContainers == nil {
		return m.setStatus("sessions are not available")
	}
	row := m.selectedRow()
	if row == nil {
		return m.setStatus("no row selected")
	}
	areaWidth, areaHeight := m.sessionAreaSize()
	if _, _, ok := sessionWindowSize(areaWidth, areaHeight); !ok {
		return m.setStatus("terminal too small for a session window")
	}
	// cluster はクラスタ名 (Row.Name())。Web UI と CLI の --cluster が使うのも名前で、ARN ではない。
	cluster := row.Name()
	m.sessionStarting = true
	m.ecsSeq++
	m.ecsExec = ecsExecState{active: true, seq: m.ecsSeq, cluster: cluster}
	m, statusCmd := m.setStatus(fmt.Sprintf("listing tasks of %s...", cluster))
	list, ctx, profile, region, seq := m.deps.ListECSTasks, m.ctx, m.profile, m.region, m.ecsSeq
	return m, tea.Batch(statusCmd, func() tea.Msg {
		tasks, err := list(ctx, profile, region, cluster, "")
		return ecsTasksMsg{seq: seq, tasks: tasks, err: err}
	})
}

// listECSContainers は選んだタスク (タスク ID) のコンテナを取る。結果は ecsContainersMsg で届く。
func (m Model) listECSContainers(task string) (tea.Model, tea.Cmd) {
	m.ecsExec.task = task
	m, statusCmd := m.setStatus(fmt.Sprintf("listing containers of %s...", task))
	list, ctx, profile, region, cluster, seq := m.deps.ListECSContainers, m.ctx, m.profile, m.region, m.ecsExec.cluster, m.ecsExec.seq
	return m, tea.Batch(statusCmd, func() tea.Msg {
		containers, err := list(ctx, profile, region, cluster, task)
		return ecsContainersMsg{seq: seq, task: task, containers: containers, err: err}
	})
}

// startECSExecProcess は選んだコンテナで Deps.OpenECSExecSession を呼ぶ。結果は EC2 と同じ
// sessionReadyMsg で届き、sessionStarting はそこで戻る。接続先の選択はここで終わる (active を倒す)。
func (m Model) startECSExecProcess(container string) (tea.Model, tea.Cmd) {
	st := m.ecsExec
	m.ecsExec = ecsExecState{}
	// 接続先の選択は終わったので、開始中のガードを一度外してから開始する。
	m.sessionStarting = false
	return m.startECSExecTarget(ECSExecTarget{Cluster: st.cluster, Task: st.task, Container: container})
}

// startECSExecTarget はタブや選択肢から渡された接続先で ECS Exec を開始する。
func (m Model) startECSExecTarget(target ECSExecTarget) (tea.Model, tea.Cmd) {
	if m.sessionStarting || m.session != nil {
		return m, nil
	}
	if m.deps.OpenECSExecSession == nil {
		return m.setStatus("sessions are not available")
	}
	areaWidth, areaHeight := m.sessionAreaSize()
	if _, _, ok := sessionWindowSize(areaWidth, areaHeight); !ok {
		return m.setStatus("terminal too small for a session window")
	}
	m.sessionSeq++
	seq := m.sessionSeq
	m.closedSessionSeq = 0
	ctx, cancel := context.WithCancel(m.ctx)
	m.sessionStartCancel = cancel
	m.sessionStarting = true
	m, statusCmd := m.setStatus(fmt.Sprintf("starting session to %s/%s/%s...", target.Cluster, target.Task, target.Container))
	open, profile, region := m.deps.OpenECSExecSession, m.profile, m.region
	title := fmt.Sprintf("ECS  %s / %s / %s  %s / %s", target.Cluster, target.Task, target.Container, m.profile, m.region)
	slot := m.sessionStartSlot
	return m, tea.Batch(statusCmd, func() tea.Msg {
		session, err := open(ctx, profile, region, target.Cluster, target.Task, target.Container, ecsExecDefaultCommand)
		slot.set(session)
		return sessionReadyMsg{seq: seq, title: title, session: session, err: err}
	})
}

// cancelECSExec は ECS Exec の接続先の選択をやめ、s を再び受け付ける。
func (m Model) cancelECSExec() Model {
	m.ecsExec = ecsExecState{}
	m.sessionStarting = false
	return m
}

// finishSessionStart はセッションの開始結果を処理し、成功したらフロートウィンドウを開く。
func (m Model) finishSessionStart(msg sessionReadyMsg) (tea.Model, tea.Cmd) {
	session := m.sessionStartSlot.take()
	if session == nil {
		session = msg.session
	}
	if m.sessionStartCancel != nil {
		m.sessionStartCancel()
		m.sessionStartCancel = nil
	}
	if msg.seq != m.sessionSeq {
		// 対象の切替や終了の後に届いた結果は、AWS 側のセッションを切断して捨てる。
		if session != nil {
			return m, closeSessionSilentlyCmd(session)
		}
		return m, nil
	}
	m.sessionStarting = false
	m.closedSessionSeq = 0

	if msg.err != nil {
		if session != nil {
			m, _ = m.setStatus(fmt.Sprintf("session closed with error: %v", msg.err))
			return m, closeSessionSilentlyCmd(session)
		}
		var startErr *SessionStartError
		if errors.As(msg.err, &startErr) && startErr.Started {
			return m.setStatus(fmt.Sprintf("session closed with error: %v", startErr.Err))
		}
		return m.setStatus(fmt.Sprintf("session: %v", msg.err))
	}
	if session == nil {
		return m.setStatus("session: no session returned")
	}

	areaWidth, areaHeight := m.sessionAreaSize()
	width, height, ok := sessionWindowSize(areaWidth, areaHeight)
	if !ok {
		m, statusCmd := m.setStatus("terminal too small for a session window")
		return m, tea.Batch(statusCmd, cleanupSessionCmd(session))
	}

	runtime := newSessionRuntime(m.ctx, session)
	m.session = newSessionWindow(msg.seq, msg.title, width, height, runtime)
	m, statusCmd := m.setStatus("")
	return m, tea.Batch(statusCmd, m.sessionReadCmd())
}

// handleSessionOutput はセッションの出力を端末画面に書き、次の読み取りを続ける。
func (m Model) handleSessionOutput(msg sessionOutputMsg) (tea.Model, tea.Cmd) {
	w := m.session
	if w == nil || msg.seq != w.seq {
		return m, nil
	}

	if len(msg.output.Data) > 0 {
		w.write(msg.output.Data)
	}
	if msg.err != nil {
		cause := msg.err
		if err := w.runtime.lastErr(); err != nil {
			cause = err
		}
		w.closed = true
		w.closeErr = cause
		w.runtime.stop()
		return m, closeSessionCmd(w.seq, w.runtime, cause)
	}
	if msg.output.Closed {
		w.closed = true
		w.closeMessage = msg.output.Message
		w.runtime.stop()
		return m, closeSessionCmd(w.seq, w.runtime, nil)
	}
	return m, m.sessionReadCmd()
}

// finishSessionClose はセッションの終了結果を通知行に反映する。リモートの終了ではウィンドウを
// 残し、ユーザーが Ctrl-] で閉じたときはウィンドウが既に無い。
func (m Model) finishSessionClose(msg sessionClosedMsg) (tea.Model, tea.Cmd) {
	if w := m.session; w != nil {
		if msg.seq != w.seq {
			// 新しいセッションが開いているときに、古いセッションの終了結果が届いた場合は捨てる。
			return m, nil
		}
		w.closed = true
		if msg.err != nil {
			w.closeErr = msg.err
		}
	} else {
		// ユーザーが Ctrl-] で閉じたセッションの終了結果だけを通知する。
		// その後に開始したセッションの終了結果が先に届いた場合などは捨てる。
		if msg.seq != m.closedSessionSeq {
			return m, nil
		}
		m.closedSessionSeq = 0
	}

	if msg.err != nil {
		return m.setStatus(fmt.Sprintf("session closed with error: %v", msg.err))
	}
	return m.setStatus("session closed")
}

// sessionReadCmd はセッションから 1 回読み取る Cmd を返す。
func (m Model) sessionReadCmd() tea.Cmd {
	w := m.session
	if w == nil {
		return nil
	}
	seq := w.seq
	ctx := w.runtime.ctx
	return func() tea.Msg {
		output, err := w.runtime.session.Read(ctx)
		return sessionOutputMsg{seq: seq, output: output, err: err}
	}
}

// updateSessionKey はフロートウィンドウが開いているときのキーを処理する。
func (m Model) updateSessionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	w := m.session
	if w == nil {
		return m, nil
	}

	// Ctrl-] だけがウィンドウを閉じる。閉じた後のウィンドウでは他のキーを無視する。
	if msg.Type == tea.KeyCtrlCloseBracket {
		if w.closed {
			m.session = nil
			// closeSessionCmd の結果がまだ届いていない場合に、終了失敗の通知を捨てない。
			m.closedSessionSeq = w.seq
			return m, nil
		}
		m.session = nil
		m.closedSessionSeq = w.seq
		w.runtime.stop()
		return m, closeSessionCmd(w.seq, w.runtime, nil)
	}
	if w.closed {
		return m, nil
	}

	data := encodeKey(msg)
	if len(data) == 0 {
		return m, nil
	}
	if !w.runtime.enqueueInput(data) {
		return m.setStatus("session input buffer is full")
	}
	return m, nil
}

// sessionAreaSize はフロートウィンドウを置く本文の大きさを返す。通知行は一時的に出るため、
// サイズの計算から除く。バナーは本文の一部として扱う。
func (m Model) sessionAreaSize() (width, height int) {
	height = m.bodyHeight()
	if m.status != "" {
		height++
	}
	return m.width, height
}

// shutdownSession は TUI の終了時に有効なセッションを切断する。開始処理が作ったが
// まだ Model が受け取っていないセッションも slot から取り出して切断する。
func (m Model) shutdownSession() {
	m.sessionStartSlot.shutdown()
	if m.session == nil {
		return
	}
	m.session.runtime.stop()
	ctx, cancel := context.WithTimeout(context.Background(), aws.TerminateSessionGracePeriod)
	defer cancel()
	// TUI は終了するため、切断の失敗を表示する先が無い。終了処理はベストエフォートにする。
	_ = m.session.runtime.closeSession(ctx)
}

// closeSessionCmd はセッションを閉じ、終了結果と読み取りの失敗を Model へ返す。
func closeSessionCmd(seq int, runtime *sessionRuntime, cause error) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), aws.TerminateSessionGracePeriod)
		defer cancel()
		return sessionClosedMsg{seq: seq, err: errors.Join(cause, runtime.closeSession(ctx))}
	}
}

// cleanupSessionCmd は開始後にウィンドウを開けなかったセッションを閉じ、失敗だけを通知する。
func cleanupSessionCmd(session InteractiveSession) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), aws.TerminateSessionGracePeriod)
		defer cancel()
		return sessionCleanupMsg{err: session.Close(ctx)}
	}
}

// closeSessionSilentlyCmd は捨てるセッションを通知せずに閉じる。
func closeSessionSilentlyCmd(session InteractiveSession) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), aws.TerminateSessionGracePeriod)
		defer cancel()
		// 対象の切替で捨てるセッションのため、切断の失敗は通知しない。
		_ = session.Close(ctx)
		return nil
	}
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
	if m.activeTab != nil {
		return m
	}
	row := m.selectedRow()
	if row == nil {
		m.drawer.SetContent(mutedStyle.Render("No row selected"))
		m.drawer.GotoTop()
		return m
	}

	var b strings.Builder
	switch m.tabID {
	case tabTerminal:
		b.WriteString(headerStyle.Render("Terminal"))
		b.WriteString("\n")
		if m.service.ID() == ecsServiceID {
			b.WriteString(wrap("Press s to select a running task and an ECS Exec enabled container, then open a floating terminal.", m.drawer.Width))
		} else {
			b.WriteString(wrap("Press s to open a Session Manager terminal to the selected instance in a floating window.", m.drawer.Width))
		}
		b.WriteString("\n\n")
		b.WriteString(mutedStyle.Render("Ctrl-] closes the floating window. Ctrl-C and q are sent to the remote session."))
	case tabTags:
		tags := row.Tags()
		// タグの取得に失敗した行は、Web UI の DrawerTags と同じく本文の先頭に見出しを出し、タグがあればその下に並べ、"No tags" は出さない。
		failed := false
		reason := ""
		if f, ok := row.(TagsFetchFailer); ok && f.TagsFetchFailed() {
			failed = true
		}
		// 行を選んだときに取る行 (TagFetcher) は、保持した結果を出す。結果が届くまでは取得中の表示。
		if _, ok := row.(TagFetcher); ok {
			res, done := m.tags[row.ID()]
			if !done {
				b.WriteString(mutedStyle.Render("Loading tags..."))
				break
			}
			tags = res.tags
			if res.err != nil {
				failed = true
				reason = res.err.Error()
			}
		}
		if failed {
			b.WriteString(statusStyle.Render("Tags (fetch failed)"))
			b.WriteString("\n")
			if reason != "" {
				b.WriteString(mutedStyle.Render(reason))
				b.WriteString("\n")
			}
		}
		if len(tags) == 0 {
			if !failed {
				b.WriteString(mutedStyle.Render("No tags"))
			}
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
	if m.activeTab != nil {
		width, height := m.drawerInnerSize()
		m.activeTab.SetSize(width, height)
	}
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
	} else if m.session != nil {
		body := m.bodyView()
		// bodyView のペインは外側に上下 1 行ずつの枠を持つ。重ね合わせの基準も枠を含めた
		// 実際の描画行数に合わせる。inner の bodyHeight だけを使うと下の枠が切り落とされる。
		b.WriteString(overlayCenter(body, m.width, m.bodyHeight()+borderLines, m.session.View(), m.session.width, m.session.height))
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

	tabSpecs := m.drawerTabs()
	tabs := make([]string, 0, len(tabSpecs))
	plainLabels := make([]string, 0, len(tabSpecs))
	activeLabel := ""
	for i, tab := range tabSpecs {
		label := fmt.Sprintf("%d %s", i+1, tab.Title)
		plainLabels = append(plainLabels, label)
		if tab.ID == m.tabID {
			activeLabel = label
			tabs = append(tabs, selectedStyle.Render(label))
		} else {
			tabs = append(tabs, mutedStyle.Render(label))
		}
	}
	rightTitle := strings.Join(tabs, mutedStyle.Render(" | "))
	if lipgloss.Width(rightTitle) > rightInner {
		// 先頭から詰め、現在のタブが見えなくなる場合だけ … | <active> に置き換える。
		plain := strings.Join(plainLabels, " | ")
		truncatedPlain := ansi.Truncate(plain, rightInner, "…")
		if !strings.Contains(truncatedPlain, activeLabel) {
			prefix := mutedStyle.Render("… | ")
			available := rightInner - lipgloss.Width(prefix)
			if available < 1 {
				available = 1
			}
			rightTitle = prefix + ansi.Truncate(activeLabel, available, "…")
		} else {
			rightTitle = ansi.Truncate(rightTitle, rightInner, "…")
		}
	}

	rightBody := m.drawer.View()
	if m.activeTab != nil {
		rightBody = m.activeTab.View(rightInner, bodyHeight-1)
	}

	leftStyle, rightStyle := paneStyle, paneStyle
	if m.focus == focusTable {
		leftStyle = paneFocusedStyle
	} else {
		rightStyle = paneFocusedStyle
	}
	left := leftStyle.Width(leftInner).Height(bodyHeight).Render(leftTitle + "\n" + leftBody)
	right := rightStyle.Width(rightInner).Height(bodyHeight).Render(rightTitle + "\n" + rightBody)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m Model) footerView() string {
	if m.session != nil {
		parts := []string{
			keyStyle.Render("ctrl+]") + " " + mutedStyle.Render("close"),
			keyStyle.Render("ctrl+c / q") + " " + mutedStyle.Render("send to session"),
		}
		return lipgloss.NewStyle().MaxWidth(m.width).Render(strings.Join(parts, "  "))
	}

	tabHint := fmt.Sprintf("1-%d", len(m.drawerTabs()))
	keys := []struct{ key, desc string }{
		{"/", "filter"}, {"j/k", "move"}, {"tab", "focus"}, {tabHint, "tabs"}, {"[ ]", "tabs"},
		{"s", "session"}, {"r", "reload"}, {"p", "profile"}, {"R", "region"},
	}
	if len(m.deps.Services) > 1 {
		keys = append(keys, struct{ key, desc string }{"S", "service"})
	}
	keys = append(keys, struct{ key, desc string }{"q", "quit"})
	if m.width < 100 {
		// 狭い端末ではタブ以外の案内を削って、タブの番号と [ ] を残す。
		keys = []struct{ key, desc string }{
			{"/", "filter"}, {"j/k", "move"}, {"tab", "focus"}, {tabHint, "tabs"}, {"[ ]", "tabs"},
			{"s", "session"}, {"r", "reload"}, {"q", "quit"},
		}
	}
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
	out := make([]string, n)
	for i := 0; i < n && i < len(cells); i++ {
		out[i] = sanitizeCell(cells[i])
	}
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

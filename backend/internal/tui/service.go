package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

// Drawer の組み込みタブの ID。サービス固有タブは各サービスが ID を決める。
const (
	tabOverview = "overview"
	tabTags     = "tags"
	tabTerminal = "terminal"
)

// KV は Drawer の Overview の 1 行 (項目名と値)。
type KV struct {
	Key   string
	Value string
}

// Align はタブの表のセルの寄せ。
type Align int

const (
	// AlignLeft は左寄せ。
	AlignLeft Align = iota
	// AlignRight は右寄せ。
	AlignRight
)

// Column は一覧の列。幅は表示中の行の内容から決めるため、ここには持たない。
type Column struct {
	Title string
	// Align はタブの表を描くときの寄せ。既存の一覧を描く bubbles/table は参照しない。
	Align Align
}

// TabTable はタブに表示する表。
type TabTable struct {
	Columns []Column
	Rows    [][]string
}

// TabBlock はタブ本文の 1 ブロック。見出し、キーと値、表、テキストのいずれかを持つ。
type TabBlock struct {
	Title string
	KV    []KV
	Table *TabTable
	Text  string
}

// TabContent はタブ本文のブロック列。
type TabContent struct {
	Blocks []TabBlock
}

// ECSExecTarget はタブから開始する ECS Exec の接続先。
type ECSExecTarget struct {
	Cluster   string
	Task      string
	Container string
}

// TabIntent はタブから Model への要求。
type TabIntent struct {
	// Status は通知行に表示する文字列。
	Status string
	// ECSExec は ECS Exec を開始する接続先。nil なら開始しない。
	ECSExec *ECSExecTarget
}

// Tab は Drawer の中で動く 1 タブ。Update の handled は、そのキーまたはメッセージを
// タブが消費したかどうかを返す。消費しなかったキーだけが Model のグローバル操作へ流れる。
type Tab interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (Tab, tea.Cmd, *TabIntent, bool)
	View(width, height int) string
	SetSize(width, height int)
}

// TabSpec はタブの生成仕様。New が nil のタブは Model の組み込みタブ (Overview / Tags / Terminal)。
type TabSpec struct {
	ID    string
	Title string
	New   func(ctx context.Context, profile, region string, row Row) Tab
}

// TabProvider は Drawer のタブを持つ Service が実装する。Tabs はそのサービスで現在実装済みの
// タブを Web UI の DRAWER_TABS の相対順序で返す。
type TabProvider interface {
	Tabs() []TabSpec
}

// Row は一覧の 1 行。Drawer の Overview と Tags も行から導く。
type Row interface {
	// ID はリソースの識別子 (インスタンス ID など)。対話セッションの接続先にも使う。
	ID() string
	// Name は表示名。
	Name() string
	// State は Web UI の StatusBadge が表示する値 (資源の State フィールドの生の値)。
	// 色は aws.NormalizeState で正規化してから選ぶ。
	State() string
	// Cells は Service.Columns と同じ順の表示文字列。
	Cells() []string
	// Overview は Drawer の Overview タブに並べる項目。
	Overview() []KV
	// Tags は Drawer の Tags タブに並べるタグ。
	Tags() map[string]string
}

// TagsFetchFailer は、タグの取得に失敗したことを Drawer に示す Row が任意に実装する。
// refreshDrawer は Row をこの型に型アサーションし、真なら Tags タブの先頭に "Tags (fetch failed)" を出す
// (Web UI の DrawerTags の見出しと同じ英語の文字列)。取得失敗を持たないサービスは実装しない。
type TagsFetchFailer interface {
	TagsFetchFailed() bool
}

// TagFetcher は、行を選んだときに追加の API でタグを取る Row が任意に実装する (CloudFormation の DescribeStacks など)。
// 実装する Row の Tags() は nil を返し、Model が FetchTags の結果を一覧の読み込みごとに行の ID() で保持して
// Tags タブに出す。取得中は "Loading tags..."、失敗は "Tags (fetch failed)" の見出しになる。
type TagFetcher interface {
	// FetchTags は行のタグを取得して返す。失敗はエラーで返し、Drawer が見出し Tags (fetch failed) と理由を出す。
	FetchTags(ctx context.Context) (map[string]string, error)
}

// Service は TUI が表示する 1 サービス。Web UI の ServicePanel に相当する。
type Service interface {
	// ID はサービスの識別子 (ec2 など)。
	ID() string
	// Title は画面に出す名前。
	Title() string
	// Columns は一覧の列。
	Columns() []Column
	// Load は一覧を取得する。
	Load(ctx context.Context, profile, region string) ([]Row, error)
	// SupportsSession は行へ対話セッションを張れるかを返す。
	SupportsSession() bool
}

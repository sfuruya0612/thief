package tui

import "context"

// KV は Drawer の Overview の 1 行 (項目名と値)。
type KV struct {
	Key   string
	Value string
}

// Column は一覧の列。幅は表示中の行の内容から決めるため、ここには持たない。
type Column struct {
	Title string
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

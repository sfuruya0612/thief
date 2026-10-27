package tui

import (
	"context"

	"github.com/charmbracelet/lipgloss"
)

// プロバイダーの識別子。Deps.Providers の並びと Target.Provider に使う。
const (
	ProviderAWS     = "aws"
	ProviderGCP     = "gcp"
	ProviderDatadog = "datadog"
	ProviderTiDB    = "tidb"
)

// プロバイダーのイメージカラー。ANSI 256 色の番号で書き、端末のパレットに依存しない。
// フォーカス中の枠線とタイトルには Accent を、フォーカスしていない枠線には、同じ色味を
// 暗くした MutedAccent を使う。どちらもプロバイダーごとに固定する。
var (
	awsAccent          = lipgloss.Color("208") // AWS のオレンジ (#ff9900 相当)
	awsMutedAccent     = lipgloss.Color("94")  // AWS のオレンジを暗くした色
	gcpAccent          = lipgloss.Color("69")  // Google Cloud のブルー (#4285f4 相当)
	gcpMutedAccent     = lipgloss.Color("25")  // Google Cloud のブルーを暗くした色
	datadogAccent      = lipgloss.Color("97")  // Datadog のパープル (#632ca6 相当)
	datadogMutedAccent = lipgloss.Color("55")  // Datadog のパープルを暗くした色
	tidbAccent         = lipgloss.Color("160") // TiDB のレッド (#dc150b 相当)
	tidbMutedAccent    = lipgloss.Color("88")  // TiDB のレッドを暗くした色
)

// Scope はプロバイダーが一覧を取得する対象の 1 つ。AWS のプロファイル、Google Cloud の
// プロジェクト、Datadog の組織など、プロバイダーごとに意味が異なる。
type Scope struct {
	// Label は画面に出す名前。
	Label string
	// Detail はピッカーに並べる補足 (アカウント ID、SSO の状態、組織の ID など)。
	Detail string
	// Value は Service.Load に渡す値。
	Value string
}

// Provider は TUI が一覧を表示するクラウド / SaaS。プロバイダーを切り替えると、
// そのプロバイダーのスコープ、サービス、イメージカラーが画面全体に切り替わる。
type Provider interface {
	// ID はプロバイダーの識別子 (aws、gcp、datadog、tidb)。
	ID() string
	// Title は画面に出す名前。
	Title() string
	// Accent はフォーカス中の枠線とタイトルに使う色。
	Accent() lipgloss.Color
	// MutedAccent はフォーカスしていない枠線に使う色。
	MutedAccent() lipgloss.Color
	// ScopeLabel は主スコープの表示名 (profile、project、organization)。空なら主スコープを持たない。
	ScopeLabel() string
	// RegionLabel は副スコープの表示名 (region)。空なら副スコープを持たない。
	RegionLabel() string
	// RequiresScope は主スコープが空のときに一覧を取得できないことを示す。
	RequiresScope() bool
	// DefaultScope は主スコープの初期値。
	DefaultScope() string
	// DefaultRegion は副スコープの初期値。
	DefaultRegion() string
	// Services はこのプロバイダーのサービス。先頭を初期表示にする。
	Services() []Service
	// LoadScopes は主スコープの選択肢を返す。主スコープを持たないプロバイダーは空を返す。
	LoadScopes(ctx context.Context) ([]Scope, error)
	// LoadRegions は主スコープに対する副スコープの選択肢を返す。副スコープを持たない
	// プロバイダーは空を返す。
	LoadRegions(ctx context.Context, scope string) ([]Scope, error)
}

// Target は TUI の起動時に選ぶプロバイダーとスコープ。Provider が空なら Deps.Providers の
// 先頭を使う。Scope が空で、プロバイダーが主スコープを必須とする場合は、プロバイダーの
// 既定値 (Google Cloud の設定済みプロジェクトなど) を使う。Region が空なら同じく既定値を使う。
type Target struct {
	Provider string
	Scope    string
	Region   string
}

// provider は Provider の汎用実装。プロバイダーごとの違いはフィールドに閉じ込める。
type provider struct {
	id            string
	title         string
	accent        lipgloss.Color
	mutedAccent   lipgloss.Color
	scopeLabel    string
	regionLabel   string
	requiresScope bool
	defaultScope  string
	defaultRegion string
	services      []Service
	loadScopes    func(ctx context.Context) ([]Scope, error)
	loadRegions   func(ctx context.Context, scope string) ([]Scope, error)
}

func (p *provider) ID() string                  { return p.id }
func (p *provider) Title() string               { return p.title }
func (p *provider) Accent() lipgloss.Color      { return p.accent }
func (p *provider) MutedAccent() lipgloss.Color { return p.mutedAccent }
func (p *provider) ScopeLabel() string          { return p.scopeLabel }
func (p *provider) RegionLabel() string         { return p.regionLabel }
func (p *provider) RequiresScope() bool         { return p.requiresScope }
func (p *provider) DefaultScope() string        { return p.defaultScope }
func (p *provider) DefaultRegion() string       { return p.defaultRegion }
func (p *provider) Services() []Service         { return p.services }
func (p *provider) LoadScopes(ctx context.Context) ([]Scope, error) {
	if p.loadScopes == nil {
		return nil, nil
	}
	return p.loadScopes(ctx)
}
func (p *provider) LoadRegions(ctx context.Context, scope string) ([]Scope, error) {
	if p.loadRegions == nil {
		return nil, nil
	}
	return p.loadRegions(ctx, scope)
}

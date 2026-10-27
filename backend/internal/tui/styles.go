package tui

import "github.com/charmbracelet/lipgloss"

// accentColor はアクセント色を渡さないピッカーの既定色。ペインの枠線、タイトル、
// 選択行、ピッカーのタイトルと選択行は、プロバイダーごとのイメージカラー
// (provider.go の Accent / MutedAccent) を使う。状態色は Web UI のバッジに合わせる。
// ANSI 256 色の番号で書き、端末のパレットに依存しない。
var (
	accentColor  = lipgloss.Color("99")
	mutedColor   = lipgloss.Color("244")
	runningColor = lipgloss.Color("42")
	stoppedColor = lipgloss.Color("245")
	errorColor   = lipgloss.Color("203")
	warnColor    = lipgloss.Color("214")

	headerStyle = lipgloss.NewStyle().Bold(true)
	mutedStyle  = lipgloss.NewStyle().Foreground(mutedColor)
	errorStyle  = lipgloss.NewStyle().Foreground(errorColor)
	bannerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(warnColor).Padding(0, 1)
	statusStyle = lipgloss.NewStyle().Foreground(warnColor)
)

// accentStyle は選択中のプロバイダーのイメージカラーで太字にする。
func (m Model) accentStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(m.provider.Accent())
}

// selectedAccentStyle は選択中のタブをプロバイダーのイメージカラーで強調する。
func (m Model) selectedAccentStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(m.provider.Accent()).Bold(true)
}

// paneStyleFor はフォーカスの有無に応じたペインの枠線を返す。どちらも選択中の
// プロバイダーのイメージカラーを使い、フォーカスしていない側は暗い色にする。
func (m Model) paneStyleFor(focused bool) lipgloss.Style {
	color := m.provider.MutedAccent()
	if focused {
		color = m.provider.Accent()
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(color)
}

// stateStyle は正規化した状態 (aws.NormalizeState) に対応する色を返す。
func stateStyle(state string) lipgloss.Style {
	switch state {
	case "running":
		return lipgloss.NewStyle().Foreground(runningColor)
	case "stopped":
		return lipgloss.NewStyle().Foreground(stoppedColor)
	case "error":
		return lipgloss.NewStyle().Foreground(errorColor)
	case "updating":
		return lipgloss.NewStyle().Foreground(warnColor)
	default:
		return lipgloss.NewStyle()
	}
}

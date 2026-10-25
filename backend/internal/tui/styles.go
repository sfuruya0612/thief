package tui

import "github.com/charmbracelet/lipgloss"

// 色は Web UI の既定のアクセント (紫) と状態色に寄せる。ANSI 256 色の番号で書き、
// 端末のパレットに依存しない。
var (
	accentColor  = lipgloss.Color("99")
	mutedColor   = lipgloss.Color("244")
	runningColor = lipgloss.Color("42")
	stoppedColor = lipgloss.Color("245")
	errorColor   = lipgloss.Color("203")
	warnColor    = lipgloss.Color("214")

	headerStyle   = lipgloss.NewStyle().Bold(true)
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	mutedStyle    = lipgloss.NewStyle().Foreground(mutedColor)
	selectedStyle = lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	errorStyle    = lipgloss.NewStyle().Foreground(errorColor)
	bannerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(warnColor).Padding(0, 1)
	keyStyle      = lipgloss.NewStyle().Foreground(accentColor)
	statusStyle   = lipgloss.NewStyle().Foreground(warnColor)

	paneStyle        = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(mutedColor)
	paneFocusedStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accentColor)
)

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

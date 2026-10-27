package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// pickerKind はピッカーが何を選ぶかを表す。選択後の処理の分岐に使う。
type pickerKind int

const (
	// pickerScope はプロバイダーの主スコープ (AWS のプロファイル、Google Cloud の
	// プロジェクト、Datadog の組織) の選択。pickerRegion は副スコープ (AWS のリージョン)。
	pickerScope pickerKind = iota
	pickerRegion
	pickerService
	pickerProvider
	// pickerECSTask と pickerECSContainer は ECS Exec の接続先 (タスク、コンテナ) の選択。
	pickerECSTask
	pickerECSContainer
)

// pickerItem はピッカーの 1 行。Value が選択結果になる。
type pickerItem struct {
	Label  string
	Detail string
	Value  string
}

// picker は一覧から 1 件選ぶオーバーレイ。文字を打つと Label と Detail で絞り込む。
type picker struct {
	kind   pickerKind
	title  string
	items  []pickerItem
	input  textinput.Model
	cursor int
	// accent はタイトルと選択行の色。呼び出し側が選択中のプロバイダーのイメージカラーを渡す。
	accent lipgloss.Color
}

// newPicker は selected に一致する項目へカーソルを置いたピッカーを返す。accent を省略すると
// 既定のアクセント色を使う (既存のテスト用)。
func newPicker(kind pickerKind, title string, items []pickerItem, selected string, accent ...lipgloss.Color) picker {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Placeholder = "type to filter"
	ti.CharLimit = 128
	// Width が 0 だと textinput はプレースホルダーの先頭 1 文字しか描かない。
	ti.Width = 40
	ti.Focus()

	color := accentColor
	if len(accent) > 0 {
		color = accent[0]
	}
	p := picker{kind: kind, title: title, items: items, input: ti, accent: color}
	for i, it := range items {
		if it.Value == selected {
			p.cursor = i
			break
		}
	}
	return p
}

// filtered は入力で絞り込んだ項目を返す。入力が空ならすべて返す。
func (p picker) filtered() []pickerItem {
	query := strings.ToLower(strings.TrimSpace(p.input.Value()))
	if query == "" {
		return p.items
	}
	out := make([]pickerItem, 0, len(p.items))
	for _, it := range p.items {
		if strings.Contains(strings.ToLower(it.Label+" "+it.Detail), query) {
			out = append(out, it)
		}
	}
	return out
}

// update はキー入力を処理する。done が真なら閉じる。chosen は決定した項目で、取り消しなら nil。
// 文字は絞り込みの入力に渡すため、移動は矢印キーと ctrl+n / ctrl+p で行う。ctrl+c は呼び出し側が
// アプリの終了として先に処理する。
func (p picker) update(msg tea.KeyMsg) (next picker, done bool, chosen *pickerItem, cmd tea.Cmd) {
	items := p.filtered()
	switch msg.String() {
	case "esc":
		// 絞り込み中の esc は入力を消すだけで閉じない。
		if p.input.Value() != "" {
			p.input.SetValue("")
			p.cursor = 0
			return p, false, nil, nil
		}
		return p, true, nil, nil
	case "up", "ctrl+p":
		if p.cursor > 0 {
			p.cursor--
		}
		return p, false, nil, nil
	case "down", "ctrl+n":
		if p.cursor < len(items)-1 {
			p.cursor++
		}
		return p, false, nil, nil
	case "home":
		p.cursor = 0
		return p, false, nil, nil
	case "end":
		if len(items) > 0 {
			p.cursor = len(items) - 1
		}
		return p, false, nil, nil
	case "enter":
		if len(items) == 0 {
			return p, false, nil, nil
		}
		it := items[p.cursor]
		return p, true, &it, nil
	}

	before := p.input.Value()
	p.input, cmd = p.input.Update(msg)
	if p.input.Value() != before {
		// 絞り込みが変わると項目の並びも変わるため、先頭に戻す。
		p.cursor = 0
	}
	return p, false, nil, cmd
}

// view はピッカーを描画する。height は本文に使える行数で、カーソルが見えるように窓を動かす。
func (p picker) view(height int) string {
	var b strings.Builder
	items := p.filtered()
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(p.accent).Render(p.title))
	b.WriteString(mutedStyle.Render(fmt.Sprintf("  (%d/%d)", len(items), len(p.items))))
	b.WriteString("\n")
	b.WriteString(p.input.View())
	b.WriteString("\n")
	if len(items) == 0 {
		if len(p.items) == 0 {
			b.WriteString(mutedStyle.Render("(no items)"))
			b.WriteString("\n")
			b.WriteString(mutedStyle.Render("esc cancel"))
			return b.String()
		}
		b.WriteString(mutedStyle.Render("(no matches)"))
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render("esc clear filter"))
		return b.String()
	}

	labelWidth := 0
	for _, it := range items {
		if w := len(it.Label); w > labelWidth {
			labelWidth = w
		}
	}

	// タイトル、入力、ヘルプの 3 行を除いた残りが項目に使える。
	visible := height - 3
	if visible < 1 {
		visible = 1
	}
	start := 0
	if p.cursor >= visible {
		start = p.cursor - visible + 1
	}
	end := start + visible
	if end > len(items) {
		end = len(items)
	}

	for i := start; i < end; i++ {
		it := items[i]
		line := fmt.Sprintf("%-*s  %s", labelWidth, it.Label, it.Detail)
		if i == p.cursor {
			b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(p.accent).Render("> " + line))
		} else {
			b.WriteString("  " + line)
		}
		b.WriteString("\n")
	}
	b.WriteString(mutedStyle.Render("type to filter  up/down move  enter select  esc cancel"))
	return b.String()
}

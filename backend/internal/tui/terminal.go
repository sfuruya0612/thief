package tui

import (
	"strings"

	"github.com/charmbracelet/x/cellbuf"
	"github.com/charmbracelet/x/vt"
)

// TerminalScreen は対話セッションの端末画面を表す。Model はこのインターフェースだけを使い、
// VT エミュレータの実装を terminal.go に閉じ込める。
type TerminalScreen interface {
	// Write はセッションから届いた端末出力を書き込む。
	Write(p []byte)
	// Resize は端末画面の大きさを変える。
	Resize(width, height int)
	// Render は現在の画面を ANSI 付きの文字列で返す。
	Render() string
	// Cursor はカーソル位置と非表示かどうかを返す。
	Cursor() (x, y int, hidden bool)
}

// vtScreen は github.com/charmbracelet/x/vt の Emulator を TerminalScreen として使う実装。
type vtScreen struct {
	emu    *vt.Emulator
	width  int
	height int
}

// newVTScreen は指定した大きさの端末画面を作る。
func newVTScreen(width, height int) *vtScreen {
	return &vtScreen{
		emu:    vt.NewEmulator(width, height),
		width:  width,
		height: height,
	}
}

// Write は端末出力をエミュレータへ渡す。Emulator.Write は閉じた後だけエラーを返すが、
// 画面の寿命は sessionWindow が管理するため、ここでは戻り値を使わない。
func (s *vtScreen) Write(p []byte) {
	_, _ = s.emu.Write(p)
}

// Resize は端末画面の大きさを変える。
func (s *vtScreen) Resize(width, height int) {
	s.width, s.height = width, height
	s.emu.Resize(width, height)
}

// Render は画面のセルを ANSI 付きの文字列にする。カーソルが表示されているときは、
// カーソル位置のセルを反転してブロックカーソルとして描く。
func (s *vtScreen) Render() string {
	buf := cellbuf.NewBuffer(s.width, s.height)
	cellbuf.SetContent(buf, s.emu.Render())

	if x, y, hidden := s.Cursor(); !hidden && x >= 0 && y >= 0 && x < s.width && y < s.height {
		if cell := buf.Cell(x, y); cell != nil {
			cursor := cell.Clone()
			cursor.Style.Attrs |= cellbuf.ReverseAttr
			buf.SetCell(x, y, cursor)
		}
	}

	return strings.ReplaceAll(cellbuf.Render(buf), "\r\n", "\n")
}

// Cursor はカーソル位置と非表示かどうかを返す。
func (s *vtScreen) Cursor() (x, y int, hidden bool) {
	pos := s.emu.CursorPosition()
	return pos.X, pos.Y, s.emu.CursorHidden()
}

// overlayCenter は base の中央に box を重ねた文字列を返す。base と box は ANSI を含んでよい。
// base の大きさは baseWidth x baseHeight、box の大きさは boxWidth x boxHeight とする。
func overlayCenter(base string, baseWidth, baseHeight int, box string, boxWidth, boxHeight int) string {
	if baseWidth <= 0 || baseHeight <= 0 {
		return base
	}
	buf := cellbuf.NewBuffer(baseWidth, baseHeight)
	cellbuf.SetContent(buf, base)

	x := (baseWidth - boxWidth) / 2
	y := (baseHeight - boxHeight) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	cellbuf.SetContentRect(buf, box, cellbuf.Rect(x, y, boxWidth, boxHeight))
	return strings.ReplaceAll(cellbuf.Render(buf), "\r\n", "\n")
}

package tui

import tea "github.com/charmbracelet/bubbletea"

// encodeKey は bubbletea のキー入力を、対話セッションの端末へ送るバイト列に変換する。
// 変換できないキーは nil を返す。
func encodeKey(msg tea.KeyMsg) []byte {
	if msg.Type == tea.KeyRunes {
		data := []byte(string(msg.Runes))
		if msg.Alt {
			return append([]byte{0x1b}, data...)
		}
		return data
	}

	var data []byte
	switch msg.Type {
	case tea.KeySpace:
		data = []byte(" ")
	case tea.KeyEnter:
		data = []byte("\r")
	case tea.KeyTab:
		data = []byte("\t")
	case tea.KeyBackspace:
		data = []byte{0x7f}
	case tea.KeyEsc:
		data = []byte{0x1b}
	case tea.KeyShiftTab:
		data = []byte("\x1b[Z")
	case tea.KeyUp:
		data = []byte("\x1b[A")
	case tea.KeyDown:
		data = []byte("\x1b[B")
	case tea.KeyRight:
		data = []byte("\x1b[C")
	case tea.KeyLeft:
		data = []byte("\x1b[D")
	case tea.KeyHome:
		data = []byte("\x1b[H")
	case tea.KeyEnd:
		data = []byte("\x1b[F")
	case tea.KeyPgUp:
		data = []byte("\x1b[5~")
	case tea.KeyPgDown:
		data = []byte("\x1b[6~")
	case tea.KeyInsert:
		data = []byte("\x1b[2~")
	case tea.KeyDelete:
		data = []byte("\x1b[3~")
	case tea.KeyCtrlUp:
		data = []byte("\x1b[1;5A")
	case tea.KeyCtrlDown:
		data = []byte("\x1b[1;5B")
	case tea.KeyCtrlRight:
		data = []byte("\x1b[1;5C")
	case tea.KeyCtrlLeft:
		data = []byte("\x1b[1;5D")
	case tea.KeyCtrlHome:
		data = []byte("\x1b[1;5H")
	case tea.KeyCtrlEnd:
		data = []byte("\x1b[1;5F")
	case tea.KeyShiftUp:
		data = []byte("\x1b[1;2A")
	case tea.KeyShiftDown:
		data = []byte("\x1b[1;2B")
	case tea.KeyShiftRight:
		data = []byte("\x1b[1;2C")
	case tea.KeyShiftLeft:
		data = []byte("\x1b[1;2D")
	case tea.KeyShiftHome:
		data = []byte("\x1b[1;2H")
	case tea.KeyShiftEnd:
		data = []byte("\x1b[1;2F")
	case tea.KeyF1:
		data = []byte("\x1bOP")
	case tea.KeyF2:
		data = []byte("\x1bOQ")
	case tea.KeyF3:
		data = []byte("\x1bOR")
	case tea.KeyF4:
		data = []byte("\x1bOS")
	case tea.KeyF5:
		data = []byte("\x1b[15~")
	case tea.KeyF6:
		data = []byte("\x1b[17~")
	case tea.KeyF7:
		data = []byte("\x1b[18~")
	case tea.KeyF8:
		data = []byte("\x1b[19~")
	case tea.KeyF9:
		data = []byte("\x1b[20~")
	case tea.KeyF10:
		data = []byte("\x1b[21~")
	case tea.KeyF11:
		data = []byte("\x1b[23~")
	case tea.KeyF12:
		data = []byte("\x1b[24~")
	default:
		// C0 制御文字 (Ctrl-A から Ctrl-Z、Ctrl-@、Ctrl-[、Ctrl-\、Ctrl-]、
		// Ctrl-^、Ctrl-_、Ctrl-?) は KeyType の値がそのまま端末へ送るバイトになる。
		if msg.Type >= 0 && msg.Type <= 0x1f {
			return []byte{byte(msg.Type)}
		}
		if msg.Type == 0x7f {
			return []byte{0x7f}
		}
		return nil
	}

	if msg.Alt {
		// 特殊キーの Alt 付きは端末ごとにシーケンスが異なるため、ESC を前置しない。
		// 通常文字の Alt 付きだけを encodeKey の先頭で扱う。
		return data
	}
	return data
}

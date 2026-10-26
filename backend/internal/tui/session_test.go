package tui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

type fakeTerminalScreen struct {
	rendered string
	resizes  [][2]int
}

func (s *fakeTerminalScreen) Write([]byte) {}
func (s *fakeTerminalScreen) Resize(width, height int) {
	s.resizes = append(s.resizes, [2]int{width, height})
}
func (s *fakeTerminalScreen) Render() string           { return s.rendered }
func (s *fakeTerminalScreen) Cursor() (int, int, bool) { return 0, 0, true }

func TestEncodeKey(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.KeyMsg
		want []byte
	}{
		{name: "empty runes", msg: tea.KeyMsg{Type: tea.KeyRunes}, want: []byte{}},
		{name: "runes", msg: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("abc")}, want: []byte("abc")},
		{name: "japanese runes", msg: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("日本語")}, want: []byte("日本語")},
		{name: "alt runes", msg: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x"), Alt: true}, want: []byte{0x1b, 'x'}},
		{name: "space", msg: tea.KeyMsg{Type: tea.KeySpace}, want: []byte(" ")},
		{name: "enter", msg: tea.KeyMsg{Type: tea.KeyEnter}, want: []byte("\r")},
		{name: "tab", msg: tea.KeyMsg{Type: tea.KeyTab}, want: []byte("\t")},
		{name: "backspace", msg: tea.KeyMsg{Type: tea.KeyBackspace}, want: []byte{0x7f}},
		{name: "escape", msg: tea.KeyMsg{Type: tea.KeyEsc}, want: []byte{0x1b}},
		{name: "ctrl c", msg: tea.KeyMsg{Type: tea.KeyCtrlC}, want: []byte{0x03}},
		{name: "ctrl close bracket", msg: tea.KeyMsg{Type: tea.KeyCtrlCloseBracket}, want: []byte{0x1d}},
		{name: "up", msg: tea.KeyMsg{Type: tea.KeyUp}, want: []byte("\x1b[A")},
		{name: "down", msg: tea.KeyMsg{Type: tea.KeyDown}, want: []byte("\x1b[B")},
		{name: "right", msg: tea.KeyMsg{Type: tea.KeyRight}, want: []byte("\x1b[C")},
		{name: "left", msg: tea.KeyMsg{Type: tea.KeyLeft}, want: []byte("\x1b[D")},
		{name: "home", msg: tea.KeyMsg{Type: tea.KeyHome}, want: []byte("\x1b[H")},
		{name: "end", msg: tea.KeyMsg{Type: tea.KeyEnd}, want: []byte("\x1b[F")},
		{name: "page up", msg: tea.KeyMsg{Type: tea.KeyPgUp}, want: []byte("\x1b[5~")},
		{name: "page down", msg: tea.KeyMsg{Type: tea.KeyPgDown}, want: []byte("\x1b[6~")},
		{name: "insert", msg: tea.KeyMsg{Type: tea.KeyInsert}, want: []byte("\x1b[2~")},
		{name: "delete", msg: tea.KeyMsg{Type: tea.KeyDelete}, want: []byte("\x1b[3~")},
		{name: "shift tab", msg: tea.KeyMsg{Type: tea.KeyShiftTab}, want: []byte("\x1b[Z")},
		{name: "ctrl up", msg: tea.KeyMsg{Type: tea.KeyCtrlUp}, want: []byte("\x1b[1;5A")},
		{name: "f1", msg: tea.KeyMsg{Type: tea.KeyF1}, want: []byte("\x1bOP")},
		{name: "f2", msg: tea.KeyMsg{Type: tea.KeyF2}, want: []byte("\x1bOQ")},
		{name: "f3", msg: tea.KeyMsg{Type: tea.KeyF3}, want: []byte("\x1bOR")},
		{name: "f4", msg: tea.KeyMsg{Type: tea.KeyF4}, want: []byte("\x1bOS")},
		{name: "f5", msg: tea.KeyMsg{Type: tea.KeyF5}, want: []byte("\x1b[15~")},
		{name: "f6", msg: tea.KeyMsg{Type: tea.KeyF6}, want: []byte("\x1b[17~")},
		{name: "f7", msg: tea.KeyMsg{Type: tea.KeyF7}, want: []byte("\x1b[18~")},
		{name: "f8", msg: tea.KeyMsg{Type: tea.KeyF8}, want: []byte("\x1b[19~")},
		{name: "f9", msg: tea.KeyMsg{Type: tea.KeyF9}, want: []byte("\x1b[20~")},
		{name: "f10", msg: tea.KeyMsg{Type: tea.KeyF10}, want: []byte("\x1b[21~")},
		{name: "f11", msg: tea.KeyMsg{Type: tea.KeyF11}, want: []byte("\x1b[23~")},
		{name: "f12", msg: tea.KeyMsg{Type: tea.KeyF12}, want: []byte("\x1b[24~")},
		{name: "unknown", msg: tea.KeyMsg{Type: tea.KeyType(-9999)}, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := encodeKey(tt.msg); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("encodeKey(%v) = %q, want %q", tt.msg, got, tt.want)
			}
		})
	}
}

func TestSessionWindowSize(t *testing.T) {
	tests := []struct {
		name                  string
		width, height         int
		wantWidth, wantHeight int
		ok                    bool
	}{
		{name: "too narrow", width: 49, height: 30, ok: false},
		{name: "narrow and short", width: 49, height: 18, ok: false},
		{name: "too short", width: 50, height: 17, ok: false},
		{name: "minimum", width: 50, height: 18, wantWidth: 40, wantHeight: 12, ok: true},
		{name: "standard", width: 120, height: 30, wantWidth: 96, wantHeight: 21, ok: true},
		{name: "large", width: 200, height: 100, wantWidth: 160, wantHeight: 70, ok: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			width, height, ok := sessionWindowSize(tt.width, tt.height)
			if ok != tt.ok || width != tt.wantWidth || height != tt.wantHeight {
				t.Errorf("sessionWindowSize(%d, %d) = %d, %d, %v; want %d, %d, %v",
					tt.width, tt.height, width, height, ok, tt.wantWidth, tt.wantHeight, tt.ok)
			}
		})
	}
}

func TestVTScreenRendersOutput(t *testing.T) {
	screen := newVTScreen(20, 5)
	screen.Write([]byte("hello"))
	if got := screen.Render(); !strings.Contains(got, "hello") {
		t.Errorf("Render() should contain hello:\n%s", got)
	}
	screen.Resize(30, 6)
	screen.Write([]byte("\x1b[2Jworld"))
	if got := screen.Render(); !strings.Contains(got, "world") {
		t.Errorf("Render() should contain world after resize:\n%s", got)
	}
}

func TestOverlayCenter(t *testing.T) {
	base := strings.Join([]string{"aaaaaa", "bbbbbb", "cccccc", "dddddd"}, "\n")
	box := strings.Join([]string{"XX", "YY"}, "\n")
	got := overlayCenter(base, 6, 4, box, 2, 2)
	if !strings.Contains(got, "XX") || !strings.Contains(got, "YY") {
		t.Errorf("overlay should contain the box:\n%s", got)
	}
	if !strings.Contains(got, "aaaaaa") || !strings.Contains(got, "dddddd") {
		t.Errorf("overlay should keep the base:\n%s", got)
	}
}

func TestModel_SessionSendsControlKeys(t *testing.T) {
	session := newFakeInteractiveSession()
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{
		OpenEC2Session: func(context.Context, string, string, string) (InteractiveSession, error) {
			return session, nil
		},
	})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m, cmd := updateCmd(t, m, keyMsg("s"))
	m = apply(t, m, cmd)
	if m.session == nil {
		t.Fatal("session window should be open")
	}

	for _, key := range []string{"ctrl+c", "q"} {
		var cmd tea.Cmd
		m, cmd = updateCmd(t, m, keyMsg(key))
		if cmd != nil {
			if _, quit := cmd().(tea.QuitMsg); quit {
				t.Fatalf("%s in a session must not quit the TUI", key)
			}
		}
	}
	eventually(t, "control keys", func() bool {
		inputs := session.inputBytes()
		return len(inputs) >= 2 &&
			string(inputs[len(inputs)-2]) == "\x03" &&
			string(inputs[len(inputs)-1]) == "q"
	})
}

func TestModel_SessionStartingBlocksOtherScreens(t *testing.T) {
	session := newFakeInteractiveSession()
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{
		OpenEC2Session: func(context.Context, string, string, string) (InteractiveSession, error) {
			return session, nil
		},
	})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m, _ = updateCmd(t, m, keyMsg("s"))
	if !m.sessionStarting {
		t.Fatal("session should be starting")
	}
	m = keys(t, m, "S", "p", "R", "/", "r")
	if m.picker != nil || m.filtering {
		t.Errorf("other screens must not open while starting: picker=%v filtering=%v", m.picker, m.filtering)
	}
}

func TestModel_SessionCtrlCloseTerminates(t *testing.T) {
	session := newFakeInteractiveSession()
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{
		OpenEC2Session: func(context.Context, string, string, string) (InteractiveSession, error) {
			return session, nil
		},
	})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m, cmd := updateCmd(t, m, keyMsg("s"))
	m = apply(t, m, cmd)
	if m.session == nil {
		t.Fatal("session window should be open")
	}

	m, cmd = updateCmd(t, m, keyMsg("ctrl+]"))
	if m.session != nil {
		t.Error("ctrl+] should close the window")
	}
	m = apply(t, m, cmd)
	if session.closeCount() != 1 {
		t.Errorf("Close calls = %d, want 1", session.closeCount())
	}
	if m.status != "session closed" {
		t.Errorf("status = %q", m.status)
	}
}

func TestModel_SessionClosedIgnoresKeysExceptCtrlClose(t *testing.T) {
	session := newFakeInteractiveSession()
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{
		OpenEC2Session: func(context.Context, string, string, string) (InteractiveSession, error) {
			return session, nil
		},
	})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m, cmd := updateCmd(t, m, keyMsg("s"))
	m = apply(t, m, cmd)
	if m.session == nil {
		t.Fatal("session window should be open")
	}
	m, cmd = updateCmd(t, m, sessionOutputMsg{seq: m.session.seq, output: SessionOutput{Closed: true}})
	m = apply(t, m, cmd)

	m = keys(t, m, "q", "ctrl+c")
	if got := session.inputBytes(); len(got) != 0 {
		t.Errorf("keys after remote close must be ignored, got %q", got)
	}
	if m.session == nil {
		t.Fatal("closed window should remain until ctrl+]")
	}
	m = keys(t, m, "ctrl+]")
	if m.session != nil {
		t.Error("ctrl+] should dismiss the closed window")
	}
}

func TestModel_SessionResizeSendsSizeAndDoesNotResizeOnStatus(t *testing.T) {
	session := newFakeInteractiveSession()
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{
		OpenEC2Session: func(context.Context, string, string, string) (InteractiveSession, error) {
			return session, nil
		},
	})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m, cmd := updateCmd(t, m, keyMsg("s"))
	m = apply(t, m, cmd)
	if m.session == nil {
		t.Fatal("session window should be open")
	}
	eventually(t, "initial SendSize", func() bool { return len(session.sizeCalls()) > 0 })
	before := len(session.sizeCalls())

	screen := &fakeTerminalScreen{}
	m.session.screen = screen
	m = update(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	eventually(t, "resize", func() bool { return len(session.sizeCalls()) > before })
	last := session.sizeCalls()[len(session.sizeCalls())-1]
	if last != [2]uint32{126, 21} {
		t.Errorf("resize size = %v, want [126 21]", last)
	}
	if len(screen.resizes) == 0 || screen.resizes[len(screen.resizes)-1] != [2]int{126, 21} {
		t.Errorf("TerminalScreen.Resize = %v, want [126 21]", screen.resizes)
	}

	afterResize := len(session.sizeCalls())
	afterScreenResize := len(screen.resizes)
	m, _ = m.setStatus("notice")
	m, _ = m.setStatus("")
	if got := len(session.sizeCalls()); got != afterResize {
		t.Errorf("status changes must not resize the session: %d -> %d", afterResize, got)
	}
	if got := len(screen.resizes); got != afterScreenResize {
		t.Errorf("status changes must not call TerminalScreen.Resize: %d -> %d", afterScreenResize, got)
	}
}

func TestModel_SessionSendSizeFailureClosesWithCause(t *testing.T) {
	session := newFakeInteractiveSession()
	session.sizeErr = errors.New("size failed")
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{
		OpenEC2Session: func(context.Context, string, string, string) (InteractiveSession, error) {
			return session, nil
		},
	})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m, cmd := updateCmd(t, m, keyMsg("s"))
	for _, msg := range drain(t, cmd) {
		if ready, ok := msg.(sessionReadyMsg); ok {
			m, cmd = updateCmd(t, m, ready)
			break
		}
	}
	if m.session == nil {
		t.Fatal("session window should be open")
	}
	for _, msg := range drain(t, cmd) {
		var next tea.Cmd
		m, next = updateCmd(t, m, msg)
		if next != nil {
			m = apply(t, m, next)
		}
	}
	if !strings.Contains(m.status, "session closed with error: send size: size failed") {
		t.Errorf("status = %q", m.status)
	}
}

func TestModel_SessionSendInputFailureClosesWithCause(t *testing.T) {
	session := newFakeInteractiveSession()
	session.inputErr = errors.New("input failed")
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{
		OpenEC2Session: func(context.Context, string, string, string) (InteractiveSession, error) {
			return session, nil
		},
	})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m, cmd := updateCmd(t, m, keyMsg("s"))
	for _, msg := range drain(t, cmd) {
		if ready, ok := msg.(sessionReadyMsg); ok {
			m, cmd = updateCmd(t, m, ready)
			break
		}
	}
	if m.session == nil {
		t.Fatal("session window should be open")
	}
	m = keys(t, m, "a")
	for _, msg := range drain(t, cmd) {
		var next tea.Cmd
		m, next = updateCmd(t, m, msg)
		if next != nil {
			m = apply(t, m, next)
		}
	}
	if !strings.Contains(m.status, "session closed with error: send input: input failed") {
		t.Errorf("status = %q", m.status)
	}
}

func TestModel_SessionShrinkBelowMinimumKeepsWindow(t *testing.T) {
	session := newFakeInteractiveSession()
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{
		OpenEC2Session: func(context.Context, string, string, string) (InteractiveSession, error) {
			return session, nil
		},
	})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m, cmd := updateCmd(t, m, keyMsg("s"))
	m = apply(t, m, cmd)
	if m.session == nil {
		t.Fatal("session window should be open")
	}
	beforeWidth, beforeHeight := m.session.width, m.session.height
	beforeSizes := len(session.sizeCalls())

	m = update(t, m, tea.WindowSizeMsg{Width: 40, Height: 20})
	if m.session == nil {
		t.Fatal("shrinking below the minimum must keep the window")
	}
	if m.session.width != beforeWidth || m.session.height != beforeHeight {
		t.Errorf("window = %dx%d, want %dx%d", m.session.width, m.session.height, beforeWidth, beforeHeight)
	}
	if got := len(session.sizeCalls()); got != beforeSizes {
		t.Errorf("SendSize calls = %d, want %d", got, beforeSizes)
	}
}

func TestModel_SessionReadErrorIsReported(t *testing.T) {
	session := newFakeInteractiveSession()
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{
		OpenEC2Session: func(context.Context, string, string, string) (InteractiveSession, error) {
			return session, nil
		},
	})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m, cmd := updateCmd(t, m, keyMsg("s"))
	m = apply(t, m, cmd)
	if m.session == nil {
		t.Fatal("session window should be open")
	}

	m, cmd = updateCmd(t, m, sessionOutputMsg{seq: m.session.seq, err: errors.New("read failed")})
	m = apply(t, m, cmd)
	if !strings.Contains(m.status, "session closed with error: read failed") {
		t.Errorf("status = %q", m.status)
	}
}

func TestModel_StaleSessionReadyIsClosed(t *testing.T) {
	session := newFakeInteractiveSession()
	m := newTestModel(t, &fakeService{id: "ec2"}, Deps{})
	m.sessionSeq = 2
	m, cmd := updateCmd(t, m, sessionReadyMsg{seq: 1, title: "old", session: session})
	m = apply(t, m, cmd)
	if m.session != nil {
		t.Error("stale session ready must not open a window")
	}
	eventually(t, "stale session close", func() bool { return session.closeCount() == 1 })
}

func TestModel_StaleCloseForOpenWindowIgnored(t *testing.T) {
	session := newFakeInteractiveSession()
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{
		OpenEC2Session: func(context.Context, string, string, string) (InteractiveSession, error) {
			return session, nil
		},
	})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m, cmd := updateCmd(t, m, keyMsg("s"))
	m = apply(t, m, cmd)
	if m.session == nil {
		t.Fatal("session window should be open")
	}
	m, _ = m.setStatus("")
	m = update(t, m, sessionClosedMsg{seq: m.session.seq + 1, err: errors.New("old")})
	if m.status != "" {
		t.Errorf("stale close for an open window must be ignored, status = %q", m.status)
	}
}

func TestModel_SessionStaleCloseNotificationIgnored(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2"}, Deps{})
	m.closedSessionSeq = 7
	m, _ = m.setStatus("")
	m = update(t, m, sessionClosedMsg{seq: 6, err: errors.New("old")})
	if m.status != "" {
		t.Errorf("stale close must be ignored, status = %q", m.status)
	}
	m = update(t, m, sessionClosedMsg{seq: 7, err: nil})
	if m.status != "session closed" {
		t.Errorf("current close status = %q", m.status)
	}
}

func TestCleanupSessionCmdReportsFailure(t *testing.T) {
	session := newFakeInteractiveSession()
	session.closeErr = errors.New("cleanup failed")
	m := newTestModel(t, &fakeService{id: "ec2"}, Deps{})
	m = apply(t, m, cleanupSessionCmd(session))
	if !strings.Contains(m.status, "session closed with error: cleanup failed") {
		t.Errorf("status = %q", m.status)
	}
}

func TestModel_SessionStaleOutputIgnored(t *testing.T) {
	session := newFakeInteractiveSession()
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{
		OpenEC2Session: func(context.Context, string, string, string) (InteractiveSession, error) {
			return session, nil
		},
	})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m, cmd := updateCmd(t, m, keyMsg("s"))
	m = apply(t, m, cmd)
	if m.session == nil {
		t.Fatal("session window should be open")
	}

	m = update(t, m, sessionOutputMsg{seq: m.session.seq + 1, output: SessionOutput{Data: []byte("stale")}})
	if view := m.View(); strings.Contains(view, "stale") {
		t.Errorf("stale output must not be rendered\n%s", view)
	}
}

func TestModel_SessionStartBoundary(t *testing.T) {
	tests := []struct {
		name       string
		width      int
		height     int
		wantOpened int
	}{
		{name: "too narrow", width: 49, height: 30, wantOpened: 0},
		{name: "too short", width: 50, height: 21, wantOpened: 0},
		{name: "exact minimum", width: 50, height: 22, wantOpened: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := newFakeInteractiveSession()
			var opened int
			m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{
				OpenEC2Session: func(context.Context, string, string, string) (InteractiveSession, error) {
					opened++
					return session, nil
				},
			})
			m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
			m = update(t, m, tea.WindowSizeMsg{Width: tt.width, Height: tt.height})
			m, cmd := updateCmd(t, m, keyMsg("s"))
			if tt.wantOpened == 0 {
				if opened != 0 || m.session != nil {
					t.Errorf("opened=%d session=%v, want no start", opened, m.session)
				}
				if m.status != "terminal too small for a session window" {
					t.Errorf("status = %q", m.status)
				}
				return
			}
			m = apply(t, m, cmd)
			if opened != 1 || m.session == nil {
				t.Errorf("opened=%d session=%v, want a session", opened, m.session)
			}
		})
	}
}

func TestModel_ECSExecTooSmallDoesNotListTasks(t *testing.T) {
	rows := []Row{ecsRow{region: "ap-northeast-1", res: aws.ECSResource{ID: "arn:a", Name: "demo", State: "active"}}}
	var listed int
	m := newTestModel(t, &fakeService{id: "ecs", rows: rows}, Deps{
		ListECSTasks: func(context.Context, string, string, string, string) ([]aws.ECSTaskResource, error) {
			listed++
			return nil, nil
		},
		ListECSContainers: func(context.Context, string, string, string, string) ([]aws.ECSContainerResource, error) {
			return nil, nil
		},
		OpenECSExecSession: func(context.Context, string, string, string, string, string, string) (InteractiveSession, error) {
			t.Fatal("OpenECSExecSession must not run on a too-small terminal")
			return nil, nil
		},
	})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: rows})
	m = update(t, m, tea.WindowSizeMsg{Width: 49, Height: 30})
	m = keys(t, m, "s")
	if listed != 0 || m.sessionStarting {
		t.Errorf("listed=%d starting=%v, want no task listing", listed, m.sessionStarting)
	}
	if m.status != "terminal too small for a session window" {
		t.Errorf("status = %q", m.status)
	}
}

func TestModel_ShutdownSessionCloses(t *testing.T) {
	session := newFakeInteractiveSession()
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{
		OpenEC2Session: func(context.Context, string, string, string) (InteractiveSession, error) {
			return session, nil
		},
	})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m, cmd := updateCmd(t, m, keyMsg("s"))
	m = apply(t, m, cmd)
	if m.session == nil {
		t.Fatal("session window should be open")
	}
	m.shutdownSession()
	if session.closeCount() != 1 {
		t.Errorf("Close calls = %d, want 1", session.closeCount())
	}
}

func TestSessionStartSlotTakeAndShutdown(t *testing.T) {
	slot := newSessionStartSlot()
	session := newFakeInteractiveSession()
	slot.set(session)
	if got := slot.take(); got != session {
		t.Fatalf("take() = %v, want the session", got)
	}
	if got := slot.take(); got != nil {
		t.Errorf("second take() = %v, want nil", got)
	}

	slot.set(session)
	slot.shutdown()
	if session.closeCount() != 1 {
		t.Errorf("shutdown Close calls = %d, want 1", session.closeCount())
	}

	// shutdown 後に開始処理がセッションを置いた場合も、その場で閉じる。
	late := newFakeInteractiveSession()
	slot.set(late)
	if late.closeCount() != 1 {
		t.Errorf("late session Close calls = %d, want 1", late.closeCount())
	}
}

func TestSessionStartErrorUnwrap(t *testing.T) {
	cause := errors.New("boom")
	err := &SessionStartError{Started: true, Err: cause}
	if !errors.Is(err, cause) {
		t.Error("SessionStartError must unwrap to its cause")
	}
	if err.Error() != "boom" {
		t.Errorf("Error() = %q", err.Error())
	}
}

package tui

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

// SessionOutput はセッションのデータチャネルから届いた 1 回分の出力を表す。
type SessionOutput struct {
	// Data は端末に書き出すバイト列。
	Data []byte
	// Closed はリモートのエージェントがセッションを閉じたことを示す。
	Closed bool
	// Message は Closed のときの終了メッセージ。
	Message string
}

// InteractiveSession は TUI のフロートウィンドウが使う対話セッションを表す。
// backend/internal/cli/tui.go のアダプタが backend/internal/session の DataChannel を実装する。
type InteractiveSession interface {
	// Read はセッションの出力を 1 回読む。
	Read(ctx context.Context) (SessionOutput, error)
	// SendInput は端末の入力を送る。
	SendInput(ctx context.Context, data []byte) error
	// SendSize は端末の大きさを送る。
	SendSize(ctx context.Context, cols, rows uint32) error
	// Close はデータチャネルを閉じ、AWS 側のセッションを終了する。二重に呼んでもよい。
	Close(ctx context.Context) error
}

// SessionStartError はセッションの開始に失敗したことを表す。Started は、AWS 側のセッションを
// 開始した後に失敗したかどうかを示す。Model は開始前の失敗と開始後の後始末の失敗で通知を分ける。
type SessionStartError struct {
	Started bool
	Err     error
}

// Error は SessionStartError のエラー文字列を返す。
func (e *SessionStartError) Error() string {
	if e == nil || e.Err == nil {
		return "session start failed"
	}
	return e.Err.Error()
}

// Unwrap は内側のエラーを返す。
func (e *SessionStartError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// sessionStartSlot は開始処理が作ったセッションを、Update が受け取るまでの間だけ保持する。
// TUI が開始処理の途中で終了した場合も、開いたセッションをここから取り出して閉じる。
type sessionStartSlot struct {
	mu      sync.Mutex
	session InteractiveSession
	closed  bool
}

// newSessionStartSlot は空の slot を返す。
func newSessionStartSlot() *sessionStartSlot {
	return &sessionStartSlot{}
}

// set は開始処理が作ったセッションを slot に置く。すでに TUI が終了している場合は、
// セッションをその場で閉じる。
func (s *sessionStartSlot) set(session InteractiveSession) {
	if session == nil {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), aws.TerminateSessionGracePeriod)
		defer cancel()
		_ = session.Close(ctx)
		return
	}
	s.session = session
	s.mu.Unlock()
}

// take は slot のセッションを取り出す。取り出した後は slot は空になる。
func (s *sessionStartSlot) take() InteractiveSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.session
	s.session = nil
	return session
}

// shutdown は TUI の終了時に、開始処理が置いたセッションを閉じる。
func (s *sessionStartSlot) shutdown() {
	s.mu.Lock()
	s.closed = true
	session := s.session
	s.session = nil
	s.mu.Unlock()
	if session == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), aws.TerminateSessionGracePeriod)
	defer cancel()
	_ = session.Close(ctx)
}

// sessionRuntime はセッションの入力とリサイズを 1 本の goroutine で直列に送る。
type sessionRuntime struct {
	session InteractiveSession
	ctx     context.Context
	cancel  context.CancelFunc
	input   chan []byte
	size    chan [2]uint32

	errMu sync.Mutex
	err   error

	closeOnce sync.Once
	closeErr  error
}

// newSessionRuntime はセッションの writer goroutine を開始する。
func newSessionRuntime(parent context.Context, session InteractiveSession) *sessionRuntime {
	ctx, cancel := context.WithCancel(parent)
	r := &sessionRuntime{
		session: session,
		ctx:     ctx,
		cancel:  cancel,
		input:   make(chan []byte, 256),
		size:    make(chan [2]uint32, 8),
	}
	go r.run(ctx)
	return r
}

// run は入力をセッションへ順番に送る。ctx が終了するか送信に失敗したら終わる。
func (r *sessionRuntime) run(ctx context.Context) {
	for {
		select {
		case data := <-r.input:
			if err := r.session.SendInput(ctx, data); err != nil {
				r.setErr(fmt.Errorf("send input: %w", err))
				return
			}
		case size := <-r.size:
			if err := r.session.SendSize(ctx, size[0], size[1]); err != nil {
				r.setErr(fmt.Errorf("send size: %w", err))
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// enqueueInput は入力を writer goroutine のキューへ入れる。キューが埋まっているときは捨てる。
func (r *sessionRuntime) enqueueInput(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	select {
	case <-r.ctx.Done():
		return false
	case r.input <- data:
		return true
	default:
		return false
	}
}

// enqueueSize は端末サイズを writer goroutine のキューへ入れる。最新のサイズを優先するため、
// キューが埋まっているときは古いサイズを捨てて入れ替える。
func (r *sessionRuntime) enqueueSize(cols, rows uint32) {
	size := [2]uint32{cols, rows}
	select {
	case <-r.ctx.Done():
		return
	case r.size <- size:
		return
	default:
		select {
		case <-r.size:
		default:
		}
		select {
		case r.size <- size:
		default:
		}
	}
}

// setErr は writer goroutine の最初の失敗を保持し、読み取りを止める。
func (r *sessionRuntime) setErr(err error) {
	r.errMu.Lock()
	if r.err == nil {
		r.err = err
	}
	r.errMu.Unlock()
	r.cancel()
}

// lastErr は writer goroutine の最初の失敗を返す。
func (r *sessionRuntime) lastErr() error {
	r.errMu.Lock()
	defer r.errMu.Unlock()
	return r.err
}

// stop は writer goroutine を止める。Close は別に呼ぶ。
func (r *sessionRuntime) stop() {
	r.cancel()
}

// closeSession は DataChannel の Close と AWS 側のセッションの終了を 1 回だけ行う。
func (r *sessionRuntime) closeSession(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	r.closeOnce.Do(func() {
		r.stop()
		r.closeErr = r.session.Close(ctx)
	})
	return r.closeErr
}

// sessionWindow は TUI の本文に重ねるフロートウィンドウの状態。
type sessionWindow struct {
	seq     int
	title   string
	screen  TerminalScreen
	runtime *sessionRuntime

	width  int
	height int

	closed       bool
	closeMessage string
	closeErr     error
}

// sessionWindowSize は本文の領域からフロートウィンドウの大きさを決める。本文が
// 50 桁 x 18 行未満のときは ok=false を返す。
func sessionWindowSize(areaWidth, areaHeight int) (width, height int, ok bool) {
	if areaWidth < 50 || areaHeight < 18 {
		return 0, 0, false
	}
	width = areaWidth * 4 / 5
	if width < 40 {
		width = 40
	}
	if width > areaWidth {
		width = areaWidth
	}
	height = areaHeight * 7 / 10
	if height < 12 {
		height = 12
	}
	if height > areaHeight {
		height = areaHeight
	}
	return width, height, true
}

// newSessionWindow はフロートウィンドウを作る。
func newSessionWindow(seq int, title string, width, height int, runtime *sessionRuntime) *sessionWindow {
	w := &sessionWindow{
		seq:     seq,
		title:   title,
		runtime: runtime,
		width:   width,
		height:  height,
	}
	w.screen = newVTScreen(width-2, height-4)
	runtime.enqueueSize(uint32(width-2), uint32(height-4))
	return w
}

// resize は端末のリサイズに追従する。
func (w *sessionWindow) resize(width, height int) {
	w.width, w.height = width, height
	innerWidth := width - 2
	innerHeight := height - 4
	if innerWidth < 1 {
		innerWidth = 1
	}
	if innerHeight < 1 {
		innerHeight = 1
	}
	w.screen.Resize(innerWidth, innerHeight)
	w.runtime.enqueueSize(uint32(innerWidth), uint32(innerHeight))
}

// write はセッションの出力を端末画面へ書き込む。
func (w *sessionWindow) write(data []byte) {
	w.screen.Write(data)
}

// View はフロートウィンドウを ANSI 付きの文字列で返す。
func (w *sessionWindow) View() string {
	innerWidth := w.width - 2
	if innerWidth < 1 {
		innerWidth = 1
	}
	title := fmt.Sprintf("Session  %s", w.title)
	title = ansi.Truncate(title, innerWidth, "…")

	footer := "Ctrl-] close"
	if w.closed {
		footer = "Session closed  Ctrl-] close"
		if w.closeMessage != "" {
			footer = fmt.Sprintf("Session closed: %s  Ctrl-] close", w.closeMessage)
		}
		if w.closeErr != nil {
			footer = fmt.Sprintf("Session closed with error: %v  Ctrl-] close", w.closeErr)
		}
	}
	footer = ansi.Truncate(footer, innerWidth, "…")

	content := strings.Join([]string{
		padRight(title, innerWidth),
		w.screen.Render(),
		padRight(footer, innerWidth),
	}, "\n")

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(awsAccent).
		Width(innerWidth).
		Height(w.height - 2)
	return style.Render(content)
}

package util

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Mock Item implementation for testing
type TestItem struct {
	title string
	id    string
}

func (t TestItem) Title() string {
	return t.title
}

func (t TestItem) ID() string {
	return t.id
}

func TestSelect_EmptyItems(t *testing.T) {
	items := []Item{}
	prompt := "Select an item:"

	_, err := Select(context.Background(), items, prompt)

	if err == nil {
		t.Error("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no items to select") {
		t.Errorf("expected error to contain 'no items to select', got %q", err.Error())
	}
}

// fakeSelectRunnerError は run bubble tea program の失敗を模す。selectWith が %w で
// ラップすることを errors.As で検証するための独自型。
type fakeSelectRunnerError struct{}

func (fakeSelectRunnerError) Error() string { return "fake tea program error" }

func TestSelectWith_RunFailureIsWrapped(t *testing.T) {
	items := []Item{TestItem{title: "Item 1", id: "1"}}

	_, err := selectWith(context.Background(), items, "Select an item:", func(context.Context, tea.Model) (tea.Model, error) {
		return nil, fakeSelectRunnerError{}
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var target fakeSelectRunnerError
	if !errors.As(err, &target) {
		t.Errorf("errors.As(fakeSelectRunnerError) = false, want true; the chain is severed (run bubble tea program: %%w is not wrapping): %v", err)
	}
}

func TestSelectWith_NoSelectionReturnsError(t *testing.T) {
	items := []Item{TestItem{title: "Item 1", id: "1"}}

	_, err := selectWith(context.Background(), items, "Select an item:", func(_ context.Context, m tea.Model) (tea.Model, error) {
		// ユーザーが q で終了した場合、model.selected は nil のまま Quit する。
		return m, nil
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no item selected") {
		t.Errorf("expected error to contain 'no item selected', got %q", err.Error())
	}
	if errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(err, context.Canceled) = true, want false; q is a deliberate no-selection exit, not an interruption: %v", err)
	}
}

func TestSelectWith_ReturnsSelectedItem(t *testing.T) {
	items := []Item{
		TestItem{title: "Item 1", id: "1"},
		TestItem{title: "Item 2", id: "2"},
	}

	got, err := selectWith(context.Background(), items, "Select an item:", func(_ context.Context, m tea.Model) (tea.Model, error) {
		finalModel := m.(model)
		finalModel.selected = items[1]
		return finalModel, nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != items[1] {
		t.Errorf("selected = %v, want %v", got, items[1])
	}
}

// TestSelectWith_CtrlCIsTreatedAsInterruption は、選択画面で ctrl+c が押された場合
// (tea.ErrInterrupted を含むエラーで run が戻る場合) に、selectWith が
// errors.Is(err, context.Canceled) が真になるエラーへ変換することを検証する。
// これにより cli.Run の中断判定にそのまま乗り、SIGINT / SIGTERM による中断と同じ
// interrupted / exit 130 になる (issue 0138 の案 A)。
func TestSelectWith_CtrlCIsTreatedAsInterruption(t *testing.T) {
	items := []Item{TestItem{title: "Item 1", id: "1"}}

	// bubbletea の Program.Run は ctrl+c (InterruptMsg) で戻るとき
	// fmt.Errorf("%w: %w", ErrProgramKilled, ErrInterrupted) を返す。
	runErr := fmt.Errorf("%w: %w", tea.ErrProgramKilled, tea.ErrInterrupted)

	_, err := selectWith(context.Background(), items, "Select an item:", func(context.Context, tea.Model) (tea.Model, error) {
		return nil, runErr
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(err, context.Canceled) = false, want true: %v", err)
	}
}

// TestSelectWith_ContextCancellationIsDetected は、run が
// fmt.Errorf("%w: %w", ErrProgramKilled, ctx.Err()) 形 (bubbletea の Program.Run が
// WithContext に渡した context のキャンセルで戻るときの実際の形。この形自体は
// TestBubbleTeaProgramWithContextReturnsContextCanceled が実際の tea.NewProgram で
// 検証している) のエラーを返した場合に、selectWith がそれをそのまま素通りさせ
// errors.Is(err, context.Canceled) まで伝わることを検証する。また ctx をそのまま
// run へ渡していることも併せて確認する。
func TestSelectWith_ContextCancellationIsDetected(t *testing.T) {
	items := []Item{TestItem{title: "Item 1", id: "1"}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runErr := fmt.Errorf("%w: %w", tea.ErrProgramKilled, ctx.Err())

	var gotCtx context.Context
	_, err := selectWith(ctx, items, "Select an item:", func(c context.Context, _ tea.Model) (tea.Model, error) {
		gotCtx = c
		return nil, runErr
	})

	if gotCtx != ctx {
		t.Error("selectWith did not pass the given context to run")
	}
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(err, context.Canceled) = false, want true: %v", err)
	}
}

// TestBubbleTeaProgramWithContextReturnsContextCanceled は、selectWith の変換ロジックが
// 前提とする bubbletea の実際の挙動 ("tea.WithContext に渡した context が (呼び出し前に)
// キャンセル済みだと、Program.Run は errors.Is で context.Canceled まで辿れるエラーで
// 戻る") を、モックではなく実際の tea.NewProgram / Run で検証する。他のテストは run を
// モックしてこの前提の形のエラーを手で組み立てて注入しているだけなので、vendor した
// bubbletea のバージョンでこの前提が実際に成り立つかはここでしか検証していない。
// 入力を無効化し (tea.WithInput(nil))、レンダラを無効化する (tea.WithoutRenderer) ことで、
// 実際の端末や標準入力に依存せず決定的に実行できるようにする。
func TestBubbleTeaProgramWithContextReturnsContextCanceled(t *testing.T) {
	items := []Item{TestItem{title: "Item 1", id: "1"}}
	initialModel := model{items: items, cursor: 0, prompt: "Select an item:"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() {
		_, err := tea.NewProgram(initialModel,
			tea.WithContext(ctx),
			tea.WithInput(nil),
			tea.WithOutput(io.Discard),
			tea.WithoutRenderer(),
			tea.WithoutSignalHandler(),
		).Run()
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("errors.Is(err, context.Canceled) = false, want true: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Program.Run did not return within 5s after the context was already canceled")
	}
}

// fakeTeaModel は tea.Model を実装するが util.model ではない型。selectWith の型アサーションが
// panic せずエラーを返すことを検証するために使う。
type fakeTeaModel struct{}

func (fakeTeaModel) Init() tea.Cmd                       { return nil }
func (fakeTeaModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return fakeTeaModel{}, nil }
func (fakeTeaModel) View() string                        { return "" }

func TestSelectWith_UnexpectedResultTypeReturnsErrorWithoutPanic(t *testing.T) {
	items := []Item{TestItem{title: "Item 1", id: "1"}}

	_, err := selectWith(context.Background(), items, "Select an item:", func(context.Context, tea.Model) (tea.Model, error) {
		return fakeTeaModel{}, nil
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected select program result type") {
		t.Errorf("expected error to contain 'unexpected select program result type', got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "fakeTeaModel") {
		t.Errorf("expected error to contain the actual type name 'fakeTeaModel', got %q", err.Error())
	}
}

func TestModel_Init(t *testing.T) {
	items := []Item{
		TestItem{title: "Item 1", id: "1"},
		TestItem{title: "Item 2", id: "2"},
	}

	m := model{
		items:  items,
		cursor: 0,
		prompt: "Select an item:",
	}

	cmd := m.Init()

	if cmd != nil {
		t.Errorf("expected nil cmd, got %v", cmd)
	}
}

func TestModel_Update_Navigation(t *testing.T) {
	items := []Item{
		TestItem{title: "Item 1", id: "1"},
		TestItem{title: "Item 2", id: "2"},
		TestItem{title: "Item 3", id: "3"},
	}

	m := model{
		items:  items,
		cursor: 1,
		prompt: "Select an item:",
	}

	// Test moving up
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if cmd != nil {
		t.Errorf("expected nil cmd, got %v", cmd)
	}
	if newModel.(model).cursor != 0 {
		t.Errorf("expected cursor 0, got %d", newModel.(model).cursor)
	}

	// Test moving down from initial position
	newModel, cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Errorf("expected nil cmd, got %v", cmd)
	}
	if newModel.(model).cursor != 2 {
		t.Errorf("expected cursor 2, got %d", newModel.(model).cursor)
	}

	// Test boundaries (already at top)
	m.cursor = 0
	newModel, cmd = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if cmd != nil {
		t.Errorf("expected nil cmd, got %v", cmd)
	}
	if newModel.(model).cursor != 0 {
		t.Errorf("expected cursor 0, got %d", newModel.(model).cursor)
	}

	// Test boundaries (already at bottom)
	m.cursor = 2
	newModel, cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Errorf("expected nil cmd, got %v", cmd)
	}
	if newModel.(model).cursor != 2 {
		t.Errorf("expected cursor 2, got %d", newModel.(model).cursor)
	}
}

func TestModel_Update_Select(t *testing.T) {
	items := []Item{
		TestItem{title: "Item 1", id: "1"},
		TestItem{title: "Item 2", id: "2"},
	}

	m := model{
		items:  items,
		cursor: 1,
		prompt: "Select an item:",
	}

	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if newModel.(model).selected != items[1] {
		t.Errorf("expected selected %v, got %v", items[1], newModel.(model).selected)
	}
	if cmd == nil {
		t.Error("expected non-nil cmd, got nil")
	}
	if cmd() != tea.Quit() {
		t.Errorf("expected Quit command")
	}
}

// TestModel_Update_CtrlCReturnsInterrupt は、ctrl+c が tea.Quit ではなく tea.Interrupt を
// 返すことを検証する (issue 0138 の案 A)。selectWith 側の errors.Is(err, context.Canceled)
// への変換は TestSelectWith_CtrlCIsTreatedAsInterruption で検証済みで、ここでは
// Update がその入力となる Cmd を正しく選ぶことだけを見る。
func TestModel_Update_CtrlCReturnsInterrupt(t *testing.T) {
	items := []Item{TestItem{title: "Item 1", id: "1"}}
	m := model{items: items, cursor: 0, prompt: "Select an item:"}

	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	if newModel.(model).selected != nil {
		t.Errorf("expected no selection, got %v", newModel.(model).selected)
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd, got nil")
	}
	if cmd() != tea.Interrupt() {
		t.Errorf("expected Interrupt command")
	}
}

// TestModel_Update_QReturnsQuit は、q が ctrl+c と異なり tea.Quit のまま (中断ではなく
// 「選択せずに終了」という利用者の意思決定) であることを検証する。
func TestModel_Update_QReturnsQuit(t *testing.T) {
	items := []Item{TestItem{title: "Item 1", id: "1"}}
	m := model{items: items, cursor: 0, prompt: "Select an item:"}

	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})

	if newModel.(model).selected != nil {
		t.Errorf("expected no selection, got %v", newModel.(model).selected)
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd, got nil")
	}
	if cmd() != tea.Quit() {
		t.Errorf("expected Quit command")
	}
}

func TestModel_View(t *testing.T) {
	items := []Item{
		TestItem{title: "Item 1", id: "1"},
		TestItem{title: "Item 2", id: "2"},
	}

	m := model{
		items:  items,
		cursor: 0,
		prompt: "Select an item:",
	}

	output := m.View()

	if !strings.Contains(output, "Select an item:") {
		t.Errorf("expected output to contain 'Select an item:', got %q", output)
	}
	if !strings.Contains(output, "> Item 1") {
		t.Errorf("expected output to contain '> Item 1', got %q", output)
	}
	if !strings.Contains(output, "  Item 2") {
		t.Errorf("expected output to contain '  Item 2', got %q", output)
	}
	if !strings.Contains(output, "Press q to quit.") {
		t.Errorf("expected output to contain 'Press q to quit.', got %q", output)
	}
}

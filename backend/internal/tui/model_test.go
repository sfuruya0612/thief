package tui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	smithy "github.com/aws/smithy-go"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

// fakeService は Load の呼び出しを記録し、固定の結果を返す Service。
type fakeService struct {
	id        string
	rows      []Row
	err       error
	calls     []string
	columns   []Column
	noSession bool
}

func (f *fakeService) ID() string    { return f.id }
func (f *fakeService) Title() string { return strings.ToUpper(f.id) }
func (f *fakeService) Columns() []Column {
	if f.columns != nil {
		return f.columns
	}
	return (&ec2Service{}).Columns()
}
func (f *fakeService) Load(_ context.Context, profile, region string) ([]Row, error) {
	f.calls = append(f.calls, profile+"/"+region)
	return f.rows, f.err
}
func (f *fakeService) SupportsSession() bool { return !f.noSession }

func testRows() []Row {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	return []Row{
		ec2Row{region: "ap-northeast-1", now: now, res: aws.EC2Resource{
			ID: "i-web", Name: "web-1", State: "running", InstanceType: "t3.micro", AZ: "ap-northeast-1a",
			PrivateIP: "10.0.0.1", VpcID: "vpc-1", Tags: map[string]string{"Env": "prod"},
			LaunchTime: now.Add(-26 * time.Hour),
		}},
		ec2Row{region: "ap-northeast-1", now: now, res: aws.EC2Resource{
			ID: "i-db", Name: "db-1", State: "stopped", InstanceType: "r6g.large", AZ: "ap-northeast-1c",
			PrivateIP: "10.0.0.2", VpcID: "vpc-1",
		}},
	}
}

func newTestModel(t *testing.T, svc Service, deps Deps) Model {
	t.Helper()
	deps.Services = []Service{svc}
	m, err := New(context.Background(), deps, "first", "ap-northeast-1")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// drain が tea.Tick の Cmd を実行するため、既定の数秒を待たないよう短くする。
	m.statusTTL = time.Millisecond
	return update(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
}

// update は 1 メッセージを適用した Model を返す。返り値の Cmd は捨てる。
func update(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(Model)
}

// updateCmd は 1 メッセージを適用した Model と Cmd を返す。
func updateCmd(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

// drain は Cmd を実行し、Batch を再帰的に展開してメッセージを集める。
// spinner の TickMsg はテストに関係しないため除く。
func drain(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	msg := cmd()
	switch msg := msg.(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, drain(t, c)...)
		}
		return out
	case spinner.TickMsg, statusExpiredMsg:
		// 通知の自動消去は専用のテストで statusExpiredMsg を直接送って確かめる。
		return nil
	default:
		return []tea.Msg{msg}
	}
}

// apply は Cmd を実行し、得たメッセージをすべて Model に適用する。
func apply(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for _, msg := range drain(t, cmd) {
		m = update(t, m, msg)
	}
	return m
}

func keys(t *testing.T, m Model, ks ...string) Model {
	t.Helper()
	for _, k := range ks {
		m = update(t, m, keyMsg(k))
	}
	return m
}

func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "home":
		return tea.KeyMsg{Type: tea.KeyHome}
	case "end":
		return tea.KeyMsg{Type: tea.KeyEnd}
	case "ctrl+n":
		return tea.KeyMsg{Type: tea.KeyCtrlN}
	case "ctrl+p":
		return tea.KeyMsg{Type: tea.KeyCtrlP}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

func TestNew_RequiresService(t *testing.T) {
	if _, err := New(context.Background(), Deps{}, "p", "r"); err == nil {
		t.Fatal("New with no services should fail")
	}
}

func TestModel_InitialLoadPopulatesTableAndDrawer(t *testing.T) {
	svc := &fakeService{id: "ec2", rows: testRows()}
	m := newTestModel(t, svc, Deps{ListProfiles: func() ([]aws.Profile, error) {
		return []aws.Profile{{Name: "first"}}, nil
	}})
	if !m.loading {
		t.Fatal("model should start in loading state")
	}

	m = apply(t, m, m.Init())

	if m.loading {
		t.Error("loading should be false after the initial load")
	}
	if len(m.visible) != 2 {
		t.Fatalf("visible = %d, want 2", len(m.visible))
	}
	if got := svc.calls; len(got) != 1 || got[0] != "first/ap-northeast-1" {
		t.Errorf("Load calls = %v, want [first/ap-northeast-1]", got)
	}
	if len(m.profiles) != 1 {
		t.Errorf("profiles = %d, want 1", len(m.profiles))
	}
	view := m.View()
	for _, want := range []string{"web-1", "db-1", "Resource ID", "i-web", "profile: first", "region: ap-northeast-1"} {
		if !strings.Contains(view, want) {
			t.Errorf("View should contain %q\n%s", want, view)
		}
	}
}

func TestModel_StaleLoadResultIsIgnored(t *testing.T) {
	svc := &fakeService{id: "ec2", rows: testRows()}
	m := newTestModel(t, svc, Deps{})

	// r で取得をやり直すと seq が 1 になる。seq 0 の結果は捨てる。
	m, _ = updateCmd(t, m, keyMsg("r"))
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	if !m.loading || len(m.visible) != 0 {
		t.Fatalf("stale result should be ignored: loading=%v visible=%d", m.loading, len(m.visible))
	}
	m = update(t, m, rowsLoadedMsg{seq: 1, rows: testRows()})
	if m.loading || len(m.visible) != 2 {
		t.Fatalf("current result should be applied: loading=%v visible=%d", m.loading, len(m.visible))
	}
}

func TestModel_FilterNarrowsVisibleRows(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})

	m = keys(t, m, "/", "d", "b")
	if !m.filtering {
		t.Fatal("filtering should be active after /")
	}
	if len(m.visible) != 1 || m.visible[0].ID() != "i-db" {
		t.Fatalf("visible = %v, want only i-db", ids(m.visible))
	}
	if !strings.Contains(m.View(), "i-db") {
		t.Error("drawer should follow the filtered selection")
	}

	// enter で確定しても絞り込みは残る。q はフィルタ中は入力であり終了ではない。
	m = keys(t, m, "enter")
	if m.filtering || len(m.visible) != 1 {
		t.Fatalf("filter should stay after enter: filtering=%v visible=%d", m.filtering, len(m.visible))
	}
	if !strings.Contains(m.View(), "(1/2)") {
		t.Errorf("pane title should show filtered/total count\n%s", m.View())
	}

	// esc で解除する。
	m = keys(t, m, "/", "esc")
	if m.filtering || len(m.visible) != 2 {
		t.Fatalf("esc should clear the filter: filtering=%v visible=%d", m.filtering, len(m.visible))
	}
}

func TestModel_FilterTypingQDoesNotQuit(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{})
	m = update(t, m, keyMsg("/"))
	_, cmd := updateCmd(t, m, keyMsg("q"))
	for _, msg := range drain(t, cmd) {
		if _, ok := msg.(tea.QuitMsg); ok {
			t.Fatal("typing q into the filter must not quit")
		}
	}
}

func TestModel_QuitKeys(t *testing.T) {
	for _, k := range []string{"q", "ctrl+c"} {
		m := newTestModel(t, &fakeService{id: "ec2"}, Deps{})
		_, cmd := updateCmd(t, m, keyMsg(k))
		if cmd == nil {
			t.Fatalf("%s: no cmd", k)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("%s: cmd should produce QuitMsg", k)
		}
	}
}

func TestModel_NavigationUpdatesDrawer(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})

	if sel := m.selectedRow(); sel == nil || sel.ID() != "i-web" {
		t.Fatalf("initial selection = %v, want i-web", sel)
	}
	m = keys(t, m, "j")
	if sel := m.selectedRow(); sel == nil || sel.ID() != "i-db" {
		t.Fatalf("selection after j = %v, want i-db", sel)
	}
	if !strings.Contains(m.View(), "r6g.large") {
		t.Errorf("drawer should show the newly selected row\n%s", m.View())
	}
	m = keys(t, m, "up")
	if sel := m.selectedRow(); sel == nil || sel.ID() != "i-web" {
		t.Fatalf("selection after up = %v, want i-web", sel)
	}
}

func TestModel_DrawerTabsShowOverviewAndTags(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})

	view := m.View()
	for _, want := range []string{"Resource ID", "Launched", "2026-10-22", "1d 2h", "vpc-1"} {
		if !strings.Contains(view, want) {
			t.Errorf("overview should contain %q\n%s", want, view)
		}
	}

	m = keys(t, m, "2")
	view = m.View()
	if !strings.Contains(view, "Env") || !strings.Contains(view, "prod") {
		t.Errorf("tags tab should list tags\n%s", view)
	}

	m = keys(t, m, "j")
	if !strings.Contains(m.View(), "No tags") {
		t.Errorf("row without tags should say so\n%s", m.View())
	}

	m = keys(t, m, "1")
	if !strings.Contains(m.View(), "Resource ID") {
		t.Error("1 should switch back to the overview tab")
	}
}

func TestModel_TabTogglesFocus(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2"}, Deps{})
	if m.focus != focusTable {
		t.Fatal("initial focus should be the table")
	}
	m = keys(t, m, "tab")
	if m.focus != focusDrawer {
		t.Error("tab should move focus to the drawer")
	}
	m = keys(t, m, "tab")
	if m.focus != focusTable {
		t.Error("tab again should move focus back to the table")
	}
}

func TestModel_SSOExpiredShowsBannerUntilNextSuccess(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2"}, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, err: fmt.Errorf("list: %w", aws.ErrSSOTokenExpired)})

	if !m.ssoExpired {
		t.Fatal("ssoExpired should be set")
	}
	if !strings.Contains(m.View(), "SSO token expired for profile first") {
		t.Errorf("banner missing\n%s", m.View())
	}

	m, _ = updateCmd(t, m, keyMsg("r"))
	m = update(t, m, rowsLoadedMsg{seq: 1, rows: testRows()})
	if m.ssoExpired || m.err != nil {
		t.Errorf("successful load should clear the banner: ssoExpired=%v err=%v", m.ssoExpired, m.err)
	}
}

func TestModel_OtherErrorsAreShownInPane(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2"}, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, err: errors.New("AccessDenied: boom")})
	if m.ssoExpired {
		t.Error("a non-SSO error must not raise the SSO banner")
	}
	if !strings.Contains(m.View(), "AccessDenied: boom") {
		t.Errorf("error should be visible\n%s", m.View())
	}
}

func TestModel_ProfilePickerReloadsWithChosenProfile(t *testing.T) {
	svc := &fakeService{id: "ec2", rows: testRows()}
	m := newTestModel(t, svc, Deps{})
	m = update(t, m, profilesLoadedMsg{profiles: []aws.Profile{
		{Name: "first", AccountID: "111111111111", AuthType: aws.AuthTypeSSO, SSORoleName: "Admin", SSOStatus: aws.SSOStatusValid},
		{Name: "second", AccountID: "222222222222", AuthType: aws.AuthTypeAccessKey},
	}})

	m = keys(t, m, "p")
	if m.picker == nil || m.picker.kind != pickerProfile {
		t.Fatal("p should open the profile picker")
	}
	if !strings.Contains(m.View(), "Select a profile") || !strings.Contains(m.View(), "222222222222") {
		t.Errorf("picker view missing entries\n%s", m.View())
	}

	m = keys(t, m, "down")
	m, cmd := updateCmd(t, m, keyMsg("enter"))
	if m.picker != nil {
		t.Fatal("enter should close the picker")
	}
	if m.profile != "second" || !m.loading {
		t.Fatalf("profile=%q loading=%v, want second/true", m.profile, m.loading)
	}
	m = apply(t, m, cmd)
	if got := svc.calls; len(got) != 1 || got[0] != "second/ap-northeast-1" {
		t.Errorf("Load calls = %v, want [second/ap-northeast-1]", got)
	}
	if m.loading {
		t.Error("loading should finish after the reload")
	}
}

func TestModel_ProfilePickerCancelKeepsProfile(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2"}, Deps{})
	m = update(t, m, profilesLoadedMsg{profiles: []aws.Profile{{Name: "first"}, {Name: "second"}}})
	m = keys(t, m, "p", "down", "esc")
	if m.picker != nil || m.profile != "first" {
		t.Errorf("esc should cancel: picker=%v profile=%q", m.picker, m.profile)
	}
}

func TestModel_RegionPickerLoadsRegionsThenReloads(t *testing.T) {
	svc := &fakeService{id: "ec2"}
	var askedProfile string
	m := newTestModel(t, svc, Deps{ListRegions: func(_ context.Context, profile string) ([]aws.RegionResource, error) {
		askedProfile = profile
		return []aws.RegionResource{{Code: "ap-northeast-1", Name: "Tokyo"}, {Code: "us-east-1", Name: "N. Virginia"}}, nil
	}})

	m, cmd := updateCmd(t, m, keyMsg("R"))
	m = apply(t, m, cmd)
	if askedProfile != "first" {
		t.Errorf("ListRegions profile = %q, want first", askedProfile)
	}
	if m.picker == nil || m.picker.kind != pickerRegion {
		t.Fatal("regions result should open the region picker")
	}
	if m.picker.cursor != 0 {
		t.Errorf("cursor should start on the current region, got %d", m.picker.cursor)
	}

	m = keys(t, m, "down")
	m, cmd = updateCmd(t, m, keyMsg("enter"))
	if m.region != "us-east-1" {
		t.Fatalf("region = %q, want us-east-1", m.region)
	}
	apply(t, m, cmd)
	if got := svc.calls; len(got) != 1 || got[0] != "first/us-east-1" {
		t.Errorf("Load calls = %v, want [first/us-east-1]", got)
	}
}

func TestModel_RegionListFailureSetsStatus(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2"}, Deps{ListRegions: func(context.Context, string) ([]aws.RegionResource, error) {
		return nil, errors.New("boom")
	}})
	m, cmd := updateCmd(t, m, keyMsg("R"))
	m = apply(t, m, cmd)
	if m.picker != nil {
		t.Error("picker must not open when regions fail to load")
	}
	if !strings.Contains(m.status, "list regions: boom") {
		t.Errorf("status = %q", m.status)
	}
}

func TestModel_ServicePickerSwitchesService(t *testing.T) {
	ec2 := &fakeService{id: "ec2", rows: testRows()}
	rds := &fakeService{id: "rds"}
	m, err := New(context.Background(), Deps{Services: []Service{ec2, rds}}, "first", "ap-northeast-1")
	if err != nil {
		t.Fatal(err)
	}
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})

	if !strings.Contains(m.View(), "S service") {
		t.Error("footer should advertise the service picker when more than one service exists")
	}
	m = keys(t, m, "S", "down")
	m, cmd := updateCmd(t, m, keyMsg("enter"))
	if m.service.ID() != "rds" {
		t.Fatalf("service = %q, want rds", m.service.ID())
	}
	if len(m.visible) != 0 {
		t.Error("rows of the previous service must be cleared")
	}
	apply(t, m, cmd)
	if len(rds.calls) != 1 {
		t.Errorf("rds Load calls = %v, want 1", rds.calls)
	}
}

func TestModel_SessionWithoutSelectionSetsStatus(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2"}, Deps{StartSession: func(context.Context, string, string, string) (*exec.Cmd, func() error, error) {
		t.Fatal("StartSession must not be called without a selection")
		return nil, nil, nil
	}})
	m, cmd := updateCmd(t, m, keyMsg("s"))
	if msgs := drain(t, cmd); len(msgs) != 0 {
		t.Errorf("only the status expiry tick is expected, got %v", msgs)
	}
	if m.status != "no row selected" {
		t.Errorf("status = %q", m.status)
	}
}

func TestModel_SessionLifecycleRunsFinalizeAfterExit(t *testing.T) {
	var finalized int
	var startedTarget string
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{
		StartSession: func(_ context.Context, profile, region, target string) (*exec.Cmd, func() error, error) {
			startedTarget = profile + "/" + region + "/" + target
			return exec.Command("true"), func() error { finalized++; return nil }, nil
		},
	})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})

	m, cmd := updateCmd(t, m, keyMsg("s"))
	if !strings.Contains(m.status, "starting session to i-web") {
		t.Errorf("status = %q", m.status)
	}
	msgs := drain(t, cmd)
	if len(msgs) != 1 {
		t.Fatalf("msgs = %v, want one sessionReadyMsg", msgs)
	}
	ready, ok := msgs[0].(sessionReadyMsg)
	if !ok || ready.err != nil {
		t.Fatalf("msg = %#v, want sessionReadyMsg without error", msgs[0])
	}
	if startedTarget != "first/ap-northeast-1/i-web" {
		t.Errorf("StartSession args = %q", startedTarget)
	}

	m, cmd = updateCmd(t, m, ready)
	if !m.inSession || cmd == nil {
		t.Fatal("sessionReadyMsg should enter the session and return the exec cmd")
	}
	if finalized != 0 {
		t.Fatal("finalize must not run before the process exits")
	}

	m, cmd = updateCmd(t, m, sessionExitMsg{})
	if m.inSession {
		t.Error("sessionExitMsg should leave the session")
	}
	m = apply(t, m, cmd)
	if finalized != 1 {
		t.Errorf("finalize ran %d times, want 1", finalized)
	}
	if m.status != "session closed" {
		t.Errorf("status = %q", m.status)
	}
	view := m.View()
	if !strings.Contains(view, "session closed") || !strings.Contains(view, "q quit") {
		t.Errorf("status must not hide the key hints\n%s", view)
	}
}

func TestModel_SessionExitErrorAndFinalizeErrorAreBothReported(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m = update(t, m, sessionReadyMsg{cmd: exec.Command("true"), finalize: func() error { return errors.New("terminate failed") }})

	m, cmd := updateCmd(t, m, sessionExitMsg{err: errors.New("exit status 1")})
	m = apply(t, m, cmd)
	for _, want := range []string{"exit status 1", "terminate failed"} {
		if !strings.Contains(m.status, want) {
			t.Errorf("status %q should mention %q", m.status, want)
		}
	}
}

func TestModel_SessionStartErrorSetsStatus(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m = update(t, m, sessionReadyMsg{err: errors.New("session-manager-plugin not found in PATH")})
	if m.inSession {
		t.Error("a failed start must not enter the session")
	}
	if !strings.Contains(m.status, "session-manager-plugin not found") {
		t.Errorf("status = %q", m.status)
	}
}

func TestModel_SmallTerminalDoesNotPanic(t *testing.T) {
	m, err := New(context.Background(), Deps{Services: []Service{&fakeService{id: "ec2", rows: testRows()}}}, "first", "ap-northeast-1")
	if err != nil {
		t.Fatal(err)
	}
	m = update(t, m, tea.WindowSizeMsg{Width: 10, Height: 3})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	if got := m.View(); got != "terminal too small" {
		t.Errorf("View = %q", got)
	}
}

func ids(rows []Row) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ID())
	}
	return out
}

func TestModel_RowsWithMismatchedCellCountDoNotPanic(t *testing.T) {
	svc := &fakeService{id: "odd", rows: testRows(), columns: []Column{{Title: "Name"}}}
	m := newTestModel(t, svc, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	if !strings.Contains(m.View(), "web-1") {
		t.Errorf("first cell should still render\n%s", m.View())
	}
}

func TestFitCells(t *testing.T) {
	tests := []struct {
		name  string
		cells []string
		n     int
		want  []string
	}{
		{name: "same", cells: []string{"a", "b"}, n: 2, want: []string{"a", "b"}},
		{name: "truncate", cells: []string{"a", "b", "c"}, n: 2, want: []string{"a", "b"}},
		{name: "pad", cells: []string{"a"}, n: 3, want: []string{"a", "", ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fitCells(tt.cells, tt.n); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("fitCells = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShrinkToFit(t *testing.T) {
	tests := []struct {
		name   string
		widths []int
		budget int
		want   []int
	}{
		{name: "fits untouched", widths: []int{10, 5}, budget: 20, want: []int{10, 5}},
		{name: "shrinks widest first", widths: []int{10, 5}, budget: 12, want: []int{7, 5}},
		{name: "spreads across columns", widths: []int{10, 10}, budget: 12, want: []int{6, 6}},
		{name: "stops at minimum", widths: []int{5, 5}, budget: 2, want: []int{4, 4}},
		{name: "empty", widths: nil, budget: 0, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shrinkToFit(tt.widths, tt.budget)
			if !reflect.DeepEqual(tt.widths, tt.want) {
				t.Errorf("widths = %v, want %v", tt.widths, tt.want)
			}
		})
	}
}

func TestModel_TableColumnsFollowContentAndBudget(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2"}, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})

	cols := m.tableColumns(200)
	if cols[0].Width != len("web-1") || cols[3].Width != len("ap-northeast-1") || cols[5].Width != len("Private IP") {
		t.Errorf("natural widths = %+v", cols)
	}

	// 8 列 x (下限 4 + 余白 2) = 48 は収まる予算。
	narrow := m.tableColumns(60)
	total := 0
	for i, c := range narrow {
		total += c.Width + cellPadding
		floor := min(cols[i].Width, minColumnWidth)
		if c.Width < floor {
			t.Errorf("column %q narrower than its floor: %d < %d", c.Title, c.Width, floor)
		}
	}
	if total > 60 {
		t.Errorf("columns exceed the budget: %d > 60 (%+v)", total, narrow)
	}

	// 下限まで縮めても収まらない予算では、下限で止める。
	for _, c := range m.tableColumns(20) {
		if c.Width < min(minColumnWidth, lipgloss.Width(c.Title)) {
			t.Errorf("column %q went below the minimum: %d", c.Title, c.Width)
		}
	}
}

func TestModel_PickerFiltersByTyping(t *testing.T) {
	svc := &fakeService{id: "ec2"}
	m := newTestModel(t, svc, Deps{})
	m = update(t, m, profilesLoadedMsg{profiles: []aws.Profile{
		{Name: "prod-admin", AccountID: "111111111111"},
		{Name: "stg-admin", AccountID: "222222222222"},
		{Name: "dev-read", AccountID: "333333333333"},
	}})
	m = keys(t, m, "p", "d", "e", "v")
	if m.picker == nil {
		t.Fatal("typing must not close the picker")
	}
	view := m.View()
	if !strings.Contains(view, "dev-read") || strings.Contains(view, "prod-admin") {
		t.Errorf("picker should show only matching profiles\n%s", view)
	}
	if !strings.Contains(view, "(1/3)") {
		t.Errorf("picker should show the match count\n%s", view)
	}
	m, cmd := updateCmd(t, m, keyMsg("enter"))
	if m.profile != "dev-read" {
		t.Fatalf("profile = %q, want dev-read", m.profile)
	}
	m = apply(t, m, cmd)
	if got := svc.calls; len(got) != 1 || got[0] != "dev-read/ap-northeast-1" {
		t.Errorf("Load calls = %v", got)
	}
}

func TestModel_PickerQKeyFiltersInsteadOfQuitting(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2"}, Deps{})
	m = update(t, m, profilesLoadedMsg{profiles: []aws.Profile{{Name: "first"}, {Name: "qa"}}})
	m, cmd := updateCmd(t, m, keyMsg("p"))
	if cmd != nil {
		t.Error("opening the picker needs no cmd")
	}
	m, cmd = updateCmd(t, m, keyMsg("q"))
	if m.picker == nil {
		t.Fatal("q must be typed into the filter, not quit")
	}
	for _, msg := range drain(t, cmd) {
		if _, ok := msg.(tea.QuitMsg); ok {
			t.Fatal("q in the picker must not quit")
		}
	}
	if !strings.Contains(m.View(), "qa") || strings.Contains(m.View(), "  first") {
		t.Errorf("filter should keep only qa\n%s", m.View())
	}
}

func TestModel_StatusExpiresAndKeysClearIt(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	before := m.table.Height()

	m, cmd := updateCmd(t, m, sessionClosedMsg{})
	if m.status != "session closed" {
		t.Fatalf("status = %q", m.status)
	}
	if m.table.Height() != before-1 {
		t.Errorf("table height = %d, want %d (status line takes one row)", m.table.Height(), before-1)
	}
	if cmd == nil {
		t.Fatal("setting a status should schedule its expiry")
	}
	seq := m.statusSeq

	// 古いタイマーは無視する。
	m = update(t, m, statusExpiredMsg{seq: seq - 1})
	if m.status == "" {
		t.Fatal("a stale expiry must not clear a newer status")
	}
	m = update(t, m, statusExpiredMsg{seq: seq})
	if m.status != "" {
		t.Errorf("status should expire, got %q", m.status)
	}
	if m.table.Height() != before {
		t.Errorf("table height = %d, want %d after the status line disappears", m.table.Height(), before)
	}

	// キーを押しても消える。
	m = update(t, m, sessionClosedMsg{})
	m = keys(t, m, "j")
	if m.status != "" {
		t.Errorf("a key press should clear the status, got %q", m.status)
	}

	// キーが新しい通知を出す場合はそちらが残る。
	m = update(t, m, sessionClosedMsg{err: errors.New("boom")})
	m = keys(t, m, "s")
	if !strings.Contains(m.status, "sessions are not available") {
		t.Errorf("status = %q", m.status)
	}
}

func TestModel_SSOBannerShrinksPanes(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2"}, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0})
	before := m.table.Height()
	m = update(t, m, rowsLoadedMsg{seq: 0, err: fmt.Errorf("list: %w", aws.ErrSSOTokenExpired)})
	if !m.ssoExpired {
		t.Fatal("ssoExpired should be set")
	}
	if m.table.Height() != before-1 {
		t.Errorf("table height = %d, want %d while the banner is shown", m.table.Height(), before-1)
	}
	m = update(t, m, rowsLoadedMsg{seq: 0})
	if m.table.Height() != before {
		t.Errorf("table height = %d, want %d after the banner clears", m.table.Height(), before)
	}
}

func TestModel_CtrlCQuitsFromPicker(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2"}, Deps{})
	m = update(t, m, profilesLoadedMsg{profiles: []aws.Profile{{Name: "first"}}})
	m = keys(t, m, "p")
	_, cmd := updateCmd(t, m, keyMsg("ctrl+c"))
	msgs := drain(t, cmd)
	if len(msgs) != 1 {
		t.Fatalf("msgs = %v, want tea.QuitMsg", msgs)
	}
	if _, ok := msgs[0].(tea.QuitMsg); !ok {
		t.Errorf("msg = %#v, want tea.QuitMsg", msgs[0])
	}
}

// TestModel_StatusWithNewlinesRendersAsOneLine は errors.Join でまとめた失敗のように改行を含む通知が
// 1 行で描画され、キー操作のヒントの行が最後の行に残ることを確かめる。
func TestModel_StatusWithNewlinesRendersAsOneLine(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m = update(t, m, sessionReadyMsg{cmd: exec.Command("true"), finalize: func() error { return errors.New("terminate failed") }})
	m, cmd := updateCmd(t, m, sessionExitMsg{err: errors.New("exit status 1")})
	m = apply(t, m, cmd)

	if strings.Contains(m.status, "\n") {
		t.Fatalf("status should not contain a newline, got %q", m.status)
	}
	lines := strings.Split(strings.TrimRight(m.View(), "\n"), "\n")
	last := lines[len(lines)-1]
	if !strings.Contains(last, "q quit") {
		t.Errorf("the last line should be the key hints, got %q", last)
	}
	var statusLines []string
	for _, l := range lines {
		if strings.Contains(l, "exit status 1") || strings.Contains(l, "terminate failed") {
			statusLines = append(statusLines, l)
		}
	}
	if len(statusLines) != 1 {
		t.Fatalf("status should take exactly 1 line, got %d\n%s", len(statusLines), m.View())
	}
	if !strings.Contains(statusLines[0], "exit status 1") || !strings.Contains(statusLines[0], "terminate failed") {
		t.Errorf("status line should mention both errors, got %q", statusLines[0])
	}
	if statusLines[0] != lines[len(lines)-2] {
		t.Errorf("status should be right above the key hints\n%s", m.View())
	}
}

// TestModel_DrawerShowsRawState は Drawer の State が Row.State() の生の値 (Web UI の StatusBadge と同じ)
// のまま表示されることを確かめる。色は aws.NormalizeState で選ぶが、文字列は正規化しない。
func TestModel_DrawerShowsRawState(t *testing.T) {
	rows := []Row{ec2Row{region: "ap-northeast-1", res: aws.EC2Resource{ID: "i-1", Name: "web", State: "available"}}}
	m := newTestModel(t, &fakeService{id: "ec2", rows: rows}, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: rows})

	// 一覧の State のセルにも同じ文字列が出るため、Drawer の描画だけを見る。
	drawer := m.drawer.View()
	var stateLine string
	for _, l := range strings.Split(drawer, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "State") {
			stateLine = l
			break
		}
	}
	if stateLine == "" {
		t.Fatalf("drawer should have a State line\n%s", drawer)
	}
	if !strings.Contains(stateLine, "available") {
		t.Errorf("drawer State should be the raw value available, got %q", stateLine)
	}
	if strings.Contains(stateLine, aws.NormalizeState("available")) {
		t.Errorf("drawer State should not be the normalized %q, got %q", aws.NormalizeState("available"), stateLine)
	}
}

// TestModel_StatusReplacesCRLFAndCR は CRLF と CR を含む通知も 1 行になり、CR が残らないことを確かめる。
func TestModel_StatusReplacesCRLFAndCR(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{})
	tests := []struct{ name, in, want string }{
		{name: "crlf", in: "a\r\nb", want: "a b"},
		{name: "cr", in: "a\rb", want: "a b"},
		{name: "lf", in: "a\nb", want: "a b"},
		{name: "mixed", in: "a\r\nb\nc\rd", want: "a b c d"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := m.setStatus(tt.in)
			if got.status != tt.want {
				t.Errorf("status = %q, want %q", got.status, tt.want)
			}
		})
	}
}

// TestModel_AccessDeniedDoesNotRaiseSSOBanner は IAM の権限不足 (メッセージに "not authorized" を含む) を
// Web UI の writeAWSError と同じく SSO の期限切れとして扱わず、一覧の領域に失敗として出すことを確かめる。
func TestModel_AccessDeniedDoesNotRaiseSSOBanner(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2"}, Deps{})
	denied := &smithy.GenericAPIError{Code: "UnauthorizedOperation", Message: "You are not authorized to perform this operation."}
	if !aws.IsSSOTokenExpired(denied) {
		t.Fatal("precondition: the loose SSO check must match this message, otherwise the test proves nothing")
	}
	m = update(t, m, rowsLoadedMsg{seq: 0, err: fmt.Errorf("list: %w", denied)})
	if m.ssoExpired {
		t.Error("an AccessDenied error must not raise the SSO banner")
	}
	if !strings.Contains(m.View(), "not authorized") {
		t.Errorf("the failure should be shown in the pane\n%s", m.View())
	}
}

// collect は drain と同じく Cmd を展開するが、spinner の TickMsg も捨てずに集める。
func collect(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, collect(t, c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func TestModel_InitTicksSpinnerOnce(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2"}, Deps{ListProfiles: func() ([]aws.Profile, error) { return nil, nil }})
	var ticks, loads, profiles int
	for _, msg := range collect(t, m.Init()) {
		switch msg.(type) {
		case spinner.TickMsg:
			ticks++
		case rowsLoadedMsg:
			loads++
		case profilesLoadedMsg:
			profiles++
		default:
			t.Errorf("unexpected msg %#v", msg)
		}
	}
	if ticks != 1 || loads != 1 || profiles != 1 {
		t.Errorf("ticks=%d loads=%d profiles=%d, want 1/1/1", ticks, loads, profiles)
	}
}

func TestModel_ProfileSwitchClearsRowsUntilReload(t *testing.T) {
	svc := &fakeService{id: "ec2", rows: testRows()}
	var started int
	m := newTestModel(t, svc, Deps{StartSession: func(context.Context, string, string, string) (*exec.Cmd, func() error, error) {
		started++
		return nil, nil, nil
	}})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m = update(t, m, profilesLoadedMsg{profiles: []aws.Profile{{Name: "first"}, {Name: "second"}}})

	m = keys(t, m, "p", "down")
	m, cmd := updateCmd(t, m, keyMsg("enter"))
	if m.profile != "second" || !m.loading {
		t.Fatalf("profile=%q loading=%v, want second/true", m.profile, m.loading)
	}
	if len(m.rows) != 0 || strings.Contains(m.View(), "web-1") {
		t.Errorf("rows of the old profile must be cleared while loading\n%s", m.View())
	}

	// 取得中の s は旧い行で始めてはならない。行が無いので "no row selected" になる。
	m = keys(t, m, "s")
	if started != 0 || m.status != "no row selected" {
		t.Errorf("s during reload: started=%d status=%q", started, m.status)
	}

	m = apply(t, m, cmd)
	if len(m.rows) != 2 || !strings.Contains(m.View(), "web-1") {
		t.Errorf("rows should be back after the reload\n%s", m.View())
	}
}

func TestModel_RegionSwitchClearsRowsUntilReload(t *testing.T) {
	svc := &fakeService{id: "ec2", rows: testRows()}
	m := newTestModel(t, svc, Deps{ListRegions: func(context.Context, string) ([]aws.RegionResource, error) {
		return []aws.RegionResource{{Code: "ap-northeast-1", Name: "Tokyo"}, {Code: "us-east-1", Name: "N. Virginia"}}, nil
	}})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})

	m, cmd := updateCmd(t, m, keyMsg("R"))
	m = apply(t, m, cmd)
	m = keys(t, m, "down")
	m, cmd = updateCmd(t, m, keyMsg("enter"))
	if m.region != "us-east-1" || !m.loading {
		t.Fatalf("region=%q loading=%v, want us-east-1/true", m.region, m.loading)
	}
	if len(m.rows) != 0 || strings.Contains(m.View(), "web-1") {
		t.Errorf("rows of the old region must be cleared while loading\n%s", m.View())
	}
	m = apply(t, m, cmd)
	if len(m.rows) != 2 {
		t.Errorf("rows = %d after reload, want 2", len(m.rows))
	}
}

func TestModel_ReloadKeepsRowsWhileLoading(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m = keys(t, m, "r")
	if !m.loading || len(m.rows) != 2 {
		t.Errorf("r reloads the same target, so rows stay: loading=%v rows=%d", m.loading, len(m.rows))
	}
}

func TestModel_SessionKeyIgnoredWhileStarting(t *testing.T) {
	var started int
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{
		StartSession: func(context.Context, string, string, string) (*exec.Cmd, func() error, error) {
			started++
			return nil, nil, errors.New("plugin missing")
		},
	})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})

	m, first := updateCmd(t, m, keyMsg("s"))
	if !m.sessionStarting {
		t.Fatal("s should mark the session as starting")
	}
	m, second := updateCmd(t, m, keyMsg("s"))
	if second != nil {
		t.Error("the second s must be ignored while the first start is in flight")
	}
	m = apply(t, m, first)
	if started != 1 {
		t.Fatalf("StartSession ran %d times, want 1", started)
	}
	if m.sessionStarting || !strings.Contains(m.status, "plugin missing") {
		t.Errorf("a failed start must clear the flag and report: starting=%v status=%q", m.sessionStarting, m.status)
	}

	// 失敗の後は再び始められる。
	m, cmd := updateCmd(t, m, keyMsg("s"))
	apply(t, m, cmd)
	if started != 2 {
		t.Errorf("StartSession ran %d times after retry, want 2", started)
	}
}

func TestModel_SessionKeyIgnoredDuringSession(t *testing.T) {
	var started int
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{
		StartSession: func(context.Context, string, string, string) (*exec.Cmd, func() error, error) {
			started++
			return exec.Command("true"), nil, nil
		},
	})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m, cmd := updateCmd(t, m, keyMsg("s"))
	m = apply(t, m, cmd)
	if !m.inSession || m.sessionStarting {
		t.Fatalf("inSession=%v starting=%v, want true/false", m.inSession, m.sessionStarting)
	}
	m, cmd = updateCmd(t, m, keyMsg("s"))
	if cmd != nil || started != 1 {
		t.Errorf("s during a session must do nothing: cmd=%v started=%d", cmd, started)
	}
	m = update(t, m, sessionExitMsg{})
	m, cmd = updateCmd(t, m, keyMsg("s"))
	apply(t, m, cmd)
	if started != 2 {
		t.Errorf("after the session ends, s should start again: started=%d", started)
	}
}

func TestModel_SessionKeyOnServiceWithoutSessions(t *testing.T) {
	var started int
	m := newTestModel(t, &fakeService{id: "ecs", rows: testRows(), noSession: true}, Deps{
		StartSession: func(context.Context, string, string, string) (*exec.Cmd, func() error, error) {
			started++
			return nil, nil, nil
		},
	})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m = keys(t, m, "s")
	if started != 0 || m.status != "ECS does not support sessions" {
		t.Errorf("started=%d status=%q", started, m.status)
	}
}

func TestModel_ProfilesLoadFailureSetsStatus(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2"}, Deps{})
	m = update(t, m, profilesLoadedMsg{err: errors.New("boom")})
	if m.status != "list profiles: boom" || len(m.profiles) != 0 {
		t.Errorf("status=%q profiles=%v", m.status, m.profiles)
	}
}

func TestModel_ProfileKeyBeforeProfilesLoadedWaitsForList(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2"}, Deps{ListProfiles: func() ([]aws.Profile, error) {
		return []aws.Profile{{Name: "first"}, {Name: "second"}}, nil
	}})

	m, cmd := updateCmd(t, m, keyMsg("p"))
	if m.picker != nil {
		t.Fatal("p must not open an empty picker before the profiles arrive")
	}
	if m.status != "loading profiles..." {
		t.Errorf("status = %q", m.status)
	}
	m = apply(t, m, cmd)
	if m.picker == nil || m.picker.kind != pickerProfile {
		t.Fatal("the picker should open once the profiles arrive")
	}
	if m.status != "" {
		t.Errorf("status = %q, want cleared when the picker opens", m.status)
	}
	if !strings.Contains(m.View(), "second") {
		t.Errorf("picker should list the loaded profiles\n%s", m.View())
	}
}

func TestModel_ProfileKeyBeforeProfilesLoadedReportsFailure(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2"}, Deps{ListProfiles: func() ([]aws.Profile, error) {
		return nil, errors.New("boom")
	}})
	m, cmd := updateCmd(t, m, keyMsg("p"))
	m = apply(t, m, cmd)
	if m.picker != nil || m.profilePickerPending {
		t.Errorf("a failed list must not open the picker or keep it pending: picker=%v pending=%v", m.picker, m.profilePickerPending)
	}
	if m.status != "list profiles: boom" {
		t.Errorf("status = %q", m.status)
	}
}

func TestModel_LongStatusFitsNarrowWidth(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{})
	m = update(t, m, tea.WindowSizeMsg{Width: 40, Height: 20})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	without := strings.Split(m.View(), "\n")

	m = update(t, m, sessionClosedMsg{err: errors.New(strings.Repeat("x", 200))})
	with := strings.Split(m.View(), "\n")
	if len(with) != len(without) {
		t.Errorf("a long status must take exactly one line: %d lines, want %d", len(with), len(without))
	}
	for i, line := range with {
		if w := lipgloss.Width(line); w > 40 {
			t.Errorf("line %d is %d cells wide, want <= 40: %q", i, w, line)
		}
	}
	if last := with[len(with)-1]; !strings.Contains(last, "filter") {
		t.Errorf("the key hints must stay on the last line: %q", last)
	}
}

// TestModel_ServiceSwitchToFewerColumnsSelectsFirstRow は、列数の少ないサービスへ切り替えても
// panic せず (bubbles の table は SetColumns でも旧い行を描き直し、行のセルの数が列の数を超えると
// panic する)、取り直した一覧の先頭の行が選ばれて Drawer に Overview が出ることを確かめる。
// issue 0219 の floci の確認で、Lambda (6 列) から ECR (5 列) への切替が panic し、切替の後の
// Drawer が "No row selected" のままになる (行が無いときの SetCursor(0) が -1 に丸められる) ことを
// 見つけた。
func TestModel_ServiceSwitchToFewerColumnsSelectsFirstRow(t *testing.T) {
	wide := &fakeService{id: "wide", rows: testRows()}
	narrow := &fakeService{id: "narrow", rows: testRows(), columns: []Column{{Title: "A"}, {Title: "B"}, {Title: "C"}}}
	m, err := New(context.Background(), Deps{Services: []Service{wide, narrow}}, "first", "ap-northeast-1")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m.statusTTL = time.Millisecond
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	if m.selectedRow() == nil {
		t.Fatal("the first row should be selected after the initial load")
	}

	m = keys(t, m, "S", "down")
	m, cmd := updateCmd(t, m, keyMsg("enter"))
	if m.service.ID() != "narrow" || !m.loading {
		t.Fatalf("service=%q loading=%v, want narrow/true", m.service.ID(), m.loading)
	}
	if strings.Contains(m.View(), "web-1") {
		t.Errorf("rows of the old service must be cleared while loading\n%s", m.View())
	}

	m = apply(t, m, cmd)
	if m.selectedRow() == nil || m.table.Cursor() != 0 {
		t.Fatalf("the first row should be selected after the reload: cursor=%d\n%s", m.table.Cursor(), m.View())
	}
	if v := m.View(); strings.Contains(v, "No row selected") || !strings.Contains(v, "Resource ID") {
		t.Errorf("Drawer should show the Overview of the first row\n%s", v)
	}
}

// TestModel_RowsAfterEmptyListSelectFirstRow は、一覧が空のとき (取得の失敗や 0 件) を経てから
// 行が届いても先頭の行が選ばれることを確かめる。
func TestModel_RowsAfterEmptyListSelectFirstRow(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2"}, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: nil})
	if m.selectedRow() != nil || !strings.Contains(m.View(), "No row selected") {
		t.Fatalf("no row should be selected while the list is empty\n%s", m.View())
	}

	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	if m.selectedRow() == nil || m.table.Cursor() != 0 {
		t.Fatalf("the first row should be selected once rows arrive: cursor=%d", m.table.Cursor())
	}
	if v := m.View(); strings.Contains(v, "No row selected") || !strings.Contains(v, "Resource ID") {
		t.Errorf("Drawer should show the Overview of the first row\n%s", v)
	}
}

// TestModel_ReloadKeepsCursorOnSelectedRow は、2 行目を選んだまま r で取り直しても、行が届いた後の
// カーソルと選択が 2 行目のまま保たれることを確かめる (applyFilter が列を入れ替える前に行を空にする
// ようにした修正で、選択が先頭に戻らないことの回帰テスト)。
func TestModel_ReloadKeepsCursorOnSelectedRow(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m = keys(t, m, "down")
	if r := m.selectedRow(); r == nil || r.ID() != "i-db" {
		t.Fatalf("down should select the second row, got %v", r)
	}

	m, cmd := updateCmd(t, m, keyMsg("r"))
	if !m.loading {
		t.Fatal("r should start a reload")
	}
	m = apply(t, m, cmd)
	if m.loading {
		t.Fatal("the reload should have finished")
	}
	if r := m.selectedRow(); m.table.Cursor() != 1 || r == nil || r.ID() != "i-db" {
		t.Fatalf("the second row should stay selected after the reload: cursor=%d row=%v", m.table.Cursor(), r)
	}
	if v := m.View(); !strings.Contains(v, "i-db") {
		t.Errorf("Drawer should show the Overview of the second row\n%s", v)
	}
}

// TestModel_RowsAfterLoadErrorSelectFirstRow は、取得の失敗の後に r で行が届いたとき、先頭の行が
// 選ばれて Drawer に Overview が出ることを確かめる (失敗で行が空になった後の SetCursor(0) が -1 に
// 丸められる経路の回帰テスト)。
func TestModel_RowsAfterLoadErrorSelectFirstRow(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m = update(t, m, rowsLoadedMsg{seq: 0, err: errors.New("boom")})
	if m.err == nil || m.selectedRow() != nil || !strings.Contains(m.View(), "No row selected") {
		t.Fatalf("no row should be selected after a failed load\n%s", m.View())
	}

	m, cmd := updateCmd(t, m, keyMsg("r"))
	m = apply(t, m, cmd)
	if r := m.selectedRow(); m.err != nil || m.table.Cursor() != 0 || r == nil || r.ID() != "i-web" {
		t.Fatalf("the first row should be selected once rows arrive: err=%v cursor=%d row=%v", m.err, m.table.Cursor(), r)
	}
	if v := m.View(); strings.Contains(v, "No row selected") || !strings.Contains(v, "Resource ID") {
		t.Errorf("Drawer should show the Overview of the first row\n%s", v)
	}
}

// TestModel_TagsTabShowsFetchFailedHeading は TagsFetchFailed() が真の行を選ぶと Drawer の Tags タブの先頭に
// "Tags (fetch failed)" が出て、タグがあればその下に並ぶことを確かめる。
func TestModel_TagsTabShowsFetchFailedHeading(t *testing.T) {
	rows := []Row{dynamoRow{region: "ap-northeast-1", res: aws.DynamoResource{
		ID: "t", Name: "t", State: "active", TagsFetchFailed: true, Tags: map[string]string{"Env": "prod"},
	}}}
	m := newTestModel(t, &fakeService{id: "dynamo", rows: rows, columns: (&dynamoService{}).Columns()}, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: rows})
	m = keys(t, m, "2")

	drawer := m.drawer.View()
	if !strings.Contains(drawer, "Tags (fetch failed)") {
		t.Errorf("Tags tab should show the fetch failed heading\n%s", drawer)
	}
	if !strings.Contains(drawer, "Env") || !strings.Contains(drawer, "prod") {
		t.Errorf("Tags tab should still list the tags\n%s", drawer)
	}
	// 見出しは本文の先頭で、タグより前に出る。
	if strings.Index(drawer, "Tags (fetch failed)") > strings.Index(drawer, "Env") {
		t.Errorf("the fetch failed heading must precede the tags\n%s", drawer)
	}
	if strings.Contains(drawer, "No tags") {
		t.Errorf("Tags tab must not say No tags when tags exist\n%s", drawer)
	}
}

// TestModel_TagsTabFetchFailedWithoutTagsOmitsNoTags は TagsFetchFailed() が真でタグが空の行では、
// 見出しだけが出て "No tags" が出ないことを確かめる (Web UI の DrawerTags は取得失敗の見出しだけを出す)。
// Tags は backend の fetchDynamoTableTags が失敗時に返すのと同じ空の map にする。
func TestModel_TagsTabFetchFailedWithoutTagsOmitsNoTags(t *testing.T) {
	rows := []Row{dynamoRow{region: "ap-northeast-1", res: aws.DynamoResource{ID: "t", Name: "t", TagsFetchFailed: true, Tags: map[string]string{}}}}
	m := newTestModel(t, &fakeService{id: "dynamo", rows: rows, columns: (&dynamoService{}).Columns()}, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: rows})
	m = keys(t, m, "2")

	drawer := m.drawer.View()
	if !strings.Contains(drawer, "Tags (fetch failed)") {
		t.Errorf("Tags tab should show the fetch failed heading\n%s", drawer)
	}
	if strings.Contains(drawer, "No tags") {
		t.Errorf("Tags tab must not say No tags when the fetch failed\n%s", drawer)
	}
}

// TestModel_TagsTabFetchFailedHeadingForWAFRow は WAF の行 (backend の wafACLDetail が取得失敗のときに返す空の非 nil map と
// TagsFetchFailed が真) を Drawer に通し、見出しだけが出て "No tags" が出ないことを確かめる。
func TestModel_TagsTabFetchFailedHeadingForWAFRow(t *testing.T) {
	rows := []Row{wafRow{region: "ap-northeast-1", res: aws.WAFResource{ID: "acl-1", Name: "web-acl", State: "active", Scope: "REGIONAL", Tags: map[string]string{}, TagsFetchFailed: true}}}
	m := newTestModel(t, &fakeService{id: "waf", rows: rows, columns: (&wafService{}).Columns()}, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: rows})
	m = keys(t, m, "2")

	drawer := m.drawer.View()
	if !strings.Contains(drawer, "Tags (fetch failed)") {
		t.Errorf("Tags tab should show the fetch failed heading\n%s", drawer)
	}
	if strings.Contains(drawer, "No tags") {
		t.Errorf("Tags tab must not say No tags when the fetch failed\n%s", drawer)
	}
}

// TestModel_TagsTabWithoutFailureShowsNoTags は TagsFetchFailed() が偽でタグが空の行では "No tags" だけが出て、
// 取得失敗の見出しが出ないことを確かめる。TagsFetchFailer を実装しない行 (S3) も同じ。
func TestModel_TagsTabWithoutFailureShowsNoTags(t *testing.T) {
	tests := []struct {
		name string
		row  Row
		cols []Column
	}{
		{name: "dynamo not failed", row: dynamoRow{region: "ap-northeast-1", res: aws.DynamoResource{ID: "t", Name: "t", Tags: map[string]string{}}}, cols: (&dynamoService{}).Columns()},
		{name: "s3 without TagsFetchFailer", row: s3Row{region: "ap-northeast-1", res: aws.S3Resource{ID: "b", Name: "b"}}, cols: (&s3Service{}).Columns()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := []Row{tt.row}
			m := newTestModel(t, &fakeService{id: "x", rows: rows, columns: tt.cols}, Deps{})
			m = update(t, m, rowsLoadedMsg{seq: 0, rows: rows})
			m = keys(t, m, "2")

			drawer := m.drawer.View()
			if !strings.Contains(drawer, "No tags") {
				t.Errorf("Tags tab should say No tags\n%s", drawer)
			}
			if strings.Contains(drawer, "fetch failed") {
				t.Errorf("Tags tab must not show the fetch failed heading\n%s", drawer)
			}
		})
	}
}

// cfnTestModel は偽の detail 関数を持つ CloudFormation の Service で Model を組み、一覧を読み込ませた
// 直後の Model と、その読み込みが返した Cmd (選択中の行のタグの取得) を返す。
// calls はスタック名ごとの detail の呼び出し回数。
func cfnTestModel(t *testing.T, tags map[string]map[string]string, detailErr error) (Model, tea.Cmd, map[string]int, []Row) {
	t.Helper()
	calls := map[string]int{}
	detail := func(_ context.Context, _, _, name string) (*aws.CFNStackDetail, error) {
		calls[name]++
		if detailErr != nil {
			return nil, detailErr
		}
		return &aws.CFNStackDetail{StackName: name, Tags: tags[name]}, nil
	}
	list := func(context.Context, string, string) ([]aws.CFNStackResource, error) {
		return []aws.CFNStackResource{
			{ID: "arn:a", Name: "app", State: "CREATE_COMPLETE", DriftStatus: "IN_SYNC"},
			{ID: "arn:b", Name: "batch", State: "UPDATE_COMPLETE", DriftStatus: "NOT_CHECKED"},
		}, nil
	}
	svc := NewCFNService(list, detail)
	rows, err := svc.Load(context.Background(), "p", "ap-northeast-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	m := newTestModel(t, &fakeService{id: "cfn", rows: rows, columns: svc.Columns()}, Deps{})
	m, cmd := updateCmd(t, m, rowsLoadedMsg{seq: 0, rows: rows})
	return m, cmd, calls, rows
}

// TestModel_TagFetcherFetchesOnLoadAndShowsLoading は TagFetcher を実装する行が選ばれると FetchTags が呼ばれ、
// 結果が届くまで Tags タブに "Loading tags..." が出て、届いたらそのタグが出ることを確かめる。
func TestModel_TagFetcherFetchesOnLoadAndShowsLoading(t *testing.T) {
	m, cmd, calls, _ := cfnTestModel(t, map[string]map[string]string{"app": {"Env": "prod", "Name": "app"}}, nil)
	if cmd == nil {
		t.Fatal("loading the rows must return the Cmd that fetches the selected row's tags")
	}
	if calls["app"] != 0 {
		t.Fatalf("FetchTags must run inside the Cmd, not synchronously: calls=%v", calls)
	}
	m = keys(t, m, "2")
	if drawer := m.drawer.View(); !strings.Contains(drawer, "Loading tags...") {
		t.Errorf("Tags tab should say Loading tags... until the result arrives\n%s", drawer)
	}

	msgs := drain(t, cmd)
	if len(msgs) != 1 {
		t.Fatalf("msgs = %v, want one tagsFetchedMsg", msgs)
	}
	got, ok := msgs[0].(tagsFetchedMsg)
	if !ok || got.seq != 0 || got.id != "arn:a" || got.err != nil {
		t.Fatalf("msg = %+v, want tagsFetchedMsg for arn:a of generation 0", msgs[0])
	}
	if calls["app"] != 1 || calls["batch"] != 0 {
		t.Errorf("calls = %v, want app once and batch never (only the selected row is fetched)", calls)
	}
	m = update(t, m, got)
	drawer := m.drawer.View()
	for _, want := range []string{"Env", "prod", "Name", "app"} {
		if !strings.Contains(drawer, want) {
			t.Errorf("Tags tab should show %q after the result\n%s", want, drawer)
		}
	}
	for _, bad := range []string{"Loading tags...", "No tags", "fetch failed"} {
		if strings.Contains(drawer, bad) {
			t.Errorf("Tags tab must not show %q after a successful fetch\n%s", bad, drawer)
		}
	}
}

// TestModel_TagFetcherEmptyAndFailedResults は空のタグで "No tags"、失敗で "Tags (fetch failed)" とその理由が出ることを確かめる。
func TestModel_TagFetcherEmptyAndFailedResults(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		m, cmd, _, _ := cfnTestModel(t, map[string]map[string]string{"app": {}}, nil)
		m = apply(t, keys(t, m, "2"), cmd)
		drawer := m.drawer.View()
		if !strings.Contains(drawer, "No tags") || strings.Contains(drawer, "Loading") || strings.Contains(drawer, "fetch failed") {
			t.Errorf("Tags tab should say No tags for an empty result\n%s", drawer)
		}
	})
	t.Run("failed", func(t *testing.T) {
		m, cmd, _, _ := cfnTestModel(t, nil, errors.New("boom"))
		m = apply(t, keys(t, m, "2"), cmd)
		drawer := m.drawer.View()
		if !strings.Contains(drawer, "Tags (fetch failed)") || !strings.Contains(drawer, "boom") {
			t.Errorf("Tags tab should show the fetch failed heading and the reason\n%s", drawer)
		}
		if strings.Contains(drawer, "No tags") || strings.Contains(drawer, "Loading") {
			t.Errorf("Tags tab must not say No tags or Loading after a failure\n%s", drawer)
		}
	})
}

// TestModel_TagFetcherDropsStaleGenerationAndRefetchesAfterReload は r の再取得より前の世代の結果が表示も保持も
// されず、再取得の後は同じ行でも FetchTags を呼び直すことを確かめる。
func TestModel_TagFetcherDropsStaleGenerationAndRefetchesAfterReload(t *testing.T) {
	m, oldCmd, calls, rows := cfnTestModel(t, map[string]map[string]string{"app": {"Env": "old"}}, nil)
	m = keys(t, m, "2")
	m, reload := updateCmd(t, m, keyMsg("r"))
	if m.loadSeq != 1 || !m.loading {
		t.Fatalf("r must start generation 1: seq=%d loading=%v", m.loadSeq, m.loading)
	}
	_ = reload

	// 旧い世代の結果が届いても捨てる。
	m = apply(t, m, oldCmd)
	if calls["app"] != 1 {
		t.Fatalf("calls = %v, want the old Cmd to have run once", calls)
	}
	if _, kept := m.tags["arn:a"]; kept {
		t.Error("a result of the old generation must not be kept")
	}
	if drawer := m.drawer.View(); strings.Contains(drawer, "old") {
		t.Errorf("a result of the old generation must not be shown\n%s", drawer)
	}

	// 新しい世代の一覧が届くと、同じ行でも取り直す。
	m, cmd := updateCmd(t, m, rowsLoadedMsg{seq: 1, rows: rows})
	if drawer := m.drawer.View(); !strings.Contains(drawer, "Loading tags...") {
		t.Errorf("Tags tab should be loading again after the reload\n%s", drawer)
	}
	m = apply(t, m, cmd)
	if calls["app"] != 2 {
		t.Errorf("calls = %v, want app fetched again after r", calls)
	}
	if drawer := m.drawer.View(); !strings.Contains(drawer, "old") {
		t.Errorf("Tags tab should show the new generation's result\n%s", drawer)
	}
}

// TestModel_TagFetcherReloadClearsShownTags は Tags タブに取得済みのタグが出ている状態で r を押すと、
// 一覧が届く前でも旧いタグが消えて取得中の表示に戻ることを確かめる。
func TestModel_TagFetcherReloadClearsShownTags(t *testing.T) {
	m, cmd, _, _ := cfnTestModel(t, map[string]map[string]string{"app": {"Env": "old"}}, nil)
	m = apply(t, keys(t, m, "2"), cmd)
	if drawer := m.drawer.View(); !strings.Contains(drawer, "old") {
		t.Fatalf("Tags tab should show the fetched tags before r\n%s", drawer)
	}
	m = keys(t, m, "r")
	drawer := m.drawer.View()
	if strings.Contains(drawer, "old") {
		t.Errorf("Tags tab must not keep showing the dropped generation's tags right after r\n%s", drawer)
	}
	if !strings.Contains(drawer, "Loading tags...") {
		t.Errorf("Tags tab should be back to the loading state right after r\n%s", drawer)
	}
}

// TestModel_TagFetcherFilterStartsFetch は絞り込みの入力で選択中の行が変わったときと、esc で絞り込みを解除して
// 選択中の行が戻ったときに、その行のタグの取得が始まることを確かめる。
func TestModel_TagFetcherFilterStartsFetch(t *testing.T) {
	tags := map[string]map[string]string{"app": {"Env": "prod"}, "batch": {"Env": "nightly"}}
	m, _, calls, rows := cfnTestModel(t, tags, nil)
	m = keys(t, m, "2")

	// "/" "b" で batch だけが見え、選択が batch に変わるので batch の取得が始まる。
	m = keys(t, m, "/")
	m, cmd := updateCmd(t, m, keyMsg("b"))
	if len(m.visible) != 1 || m.visible[0].ID() != "arn:b" {
		t.Fatalf("visible = %v, want only arn:b", ids(m.visible))
	}
	m = apply(t, m, cmd)
	if calls["batch"] != 1 {
		t.Fatalf("calls = %v, want batch fetched by the filter input", calls)
	}
	if drawer := m.drawer.View(); !strings.Contains(drawer, "nightly") {
		t.Errorf("Tags tab should show batch's tags\n%s", drawer)
	}

	// enter で絞り込みを残したまま r を押すと、届いた一覧でも batch だけが見え、batch を取り直す。
	m = keys(t, m, "enter", "r")
	m, cmd = updateCmd(t, m, rowsLoadedMsg{seq: 1, rows: rows})
	m = apply(t, m, cmd)
	if calls["batch"] != 2 || len(m.visible) != 1 {
		t.Fatalf("calls = %v visible=%d, want batch fetched again with the filter kept", calls, len(m.visible))
	}

	// "/" "esc" で絞り込みを解除すると選択が app に戻り、この世代では未取得なので app の取得が始まる。
	m = keys(t, m, "/")
	m, cmd = updateCmd(t, m, keyMsg("esc"))
	if m.filtering || len(m.visible) != 2 || m.table.Cursor() != 0 {
		t.Fatalf("esc should clear the filter and select app: filtering=%v visible=%d cursor=%d", m.filtering, len(m.visible), m.table.Cursor())
	}
	if drawer := m.drawer.View(); !strings.Contains(drawer, "Loading tags...") {
		t.Errorf("Tags tab should be loading app's tags after esc\n%s", drawer)
	}
	m = apply(t, m, cmd)
	if calls["app"] != 1 {
		t.Errorf("calls = %v, want app fetched once by esc", calls)
	}
	if drawer := m.drawer.View(); !strings.Contains(drawer, "prod") {
		t.Errorf("Tags tab should show app's tags after esc\n%s", drawer)
	}
}

// TestModel_TagFetcherFailedResultIsNotRefetchedUntilReload は失敗した結果も世代の中で保持し、同じ行を選び直しても
// 取り直さず、r の後に取り直すことを確かめる。
func TestModel_TagFetcherFailedResultIsNotRefetchedUntilReload(t *testing.T) {
	m, cmd, calls, rows := cfnTestModel(t, nil, errors.New("boom"))
	m = apply(t, keys(t, m, "2"), cmd)
	if drawer := m.drawer.View(); !strings.Contains(drawer, "Tags (fetch failed)") {
		t.Fatalf("Tags tab should show the failure\n%s", drawer)
	}

	m = keys(t, m, "down")
	m, cmd = updateCmd(t, m, keyMsg("up"))
	if msgs := drain(t, cmd); len(msgs) != 0 || calls["app"] != 1 {
		t.Errorf("reselecting app must not fetch again after a failure: msgs=%v calls=%v", msgs, calls)
	}
	if drawer := m.drawer.View(); !strings.Contains(drawer, "Tags (fetch failed)") {
		t.Errorf("Tags tab should still show the kept failure\n%s", drawer)
	}

	m = keys(t, m, "r")
	m, cmd = updateCmd(t, m, rowsLoadedMsg{seq: 1, rows: rows})
	m = apply(t, m, cmd)
	if calls["app"] != 2 {
		t.Errorf("calls = %v, want app fetched again after r", calls)
	}
}

// TestModel_TagFetcherOtherRowResultKeepsDrawerScroll は別の行の結果が届いても、表示中の Drawer の
// スクロール位置が先頭に戻らないことと、選択中の行の結果では組み直されることを確かめる。
func TestModel_TagFetcherOtherRowResultKeepsDrawerScroll(t *testing.T) {
	tags := map[string]map[string]string{"app": {"Env": "prod"}, "batch": {"Env": "nightly"}}
	m, cmdA, _, _ := cfnTestModel(t, tags, nil)
	m, cmdB := updateCmd(t, m, keyMsg("down"))
	// Overview が Drawer に収まらない高さにしてから 1 行スクロールする。
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 8})
	m.drawer.SetYOffset(1)
	if m.drawer.YOffset != 1 {
		t.Fatalf("YOffset = %d, want 1 (the Overview must be taller than the drawer)", m.drawer.YOffset)
	}

	m = apply(t, m, cmdA)
	if m.drawer.YOffset != 1 {
		t.Errorf("app's result must not reset batch's drawer scroll: YOffset = %d", m.drawer.YOffset)
	}
	m = apply(t, keys(t, m, "2"), cmdB)
	if drawer := m.drawer.View(); !strings.Contains(drawer, "nightly") {
		t.Errorf("the selected row's result must redraw the drawer\n%s", drawer)
	}
}

// TestModel_TagFetcherKeepsResultForOtherRowAndDoesNotRefetch は別の行に移ってから届いた結果は表示されないが
// 保持され、その行に戻ったとき FetchTags を呼び直さずに出ること、同じ行の 2 度目の選択でも呼び直さないことを確かめる。
func TestModel_TagFetcherKeepsResultForOtherRowAndDoesNotRefetch(t *testing.T) {
	tags := map[string]map[string]string{"app": {"Env": "prod"}, "batch": {"Env": "batch"}}
	m, cmdA, calls, _ := cfnTestModel(t, tags, nil)
	m = keys(t, m, "2")

	// 結果が届く前に別の行へ移る。移った行の取得が始まる。
	m, cmdB := updateCmd(t, m, keyMsg("down"))
	if m.table.Cursor() != 1 || cmdB == nil {
		t.Fatalf("moving to batch must start its fetch: cursor=%d cmd=%v", m.table.Cursor(), cmdB)
	}
	m = apply(t, m, cmdA)
	drawer := m.drawer.View()
	if strings.Contains(drawer, "prod") || !strings.Contains(drawer, "Loading tags...") {
		t.Errorf("app's result must not be shown while batch is selected\n%s", drawer)
	}
	if res, kept := m.tags["arn:a"]; !kept || res.tags["Env"] != "prod" {
		t.Errorf("app's result must be kept: %+v", m.tags)
	}
	m = apply(t, m, cmdB)
	if drawer := m.drawer.View(); !strings.Contains(drawer, "batch") {
		t.Errorf("batch's result should be shown\n%s", drawer)
	}

	// 戻ると保持した結果が出て、FetchTags は呼ばれない。
	m, cmd := updateCmd(t, m, keyMsg("up"))
	if msgs := drain(t, cmd); len(msgs) != 0 {
		t.Errorf("returning to app must not fetch again: msgs=%v", msgs)
	}
	if drawer := m.drawer.View(); !strings.Contains(drawer, "prod") {
		t.Errorf("app's kept result should be shown\n%s", drawer)
	}
	m, cmd = updateCmd(t, m, keyMsg("down"))
	if msgs := drain(t, cmd); len(msgs) != 0 {
		t.Errorf("selecting batch again must not fetch again: msgs=%v", msgs)
	}
	if calls["app"] != 1 || calls["batch"] != 1 {
		t.Errorf("calls = %v, want each stack fetched once", calls)
	}
}

// TestModel_TagFetcherTagsTabSwitchStartsFetch は Overview タブのまま行を選んでから Tags タブへ切り替えたときにも
// 取得が始まることを確かめる。実行中は行を選んだ時点で取得が始まるため、取得中なら 2 度目は呼ばず取得中の表示を出し、
// 取得が始まっていない行 (取得中の記録を消して再現する) では切り替えが取得を始める。
func TestModel_TagFetcherTagsTabSwitchStartsFetch(t *testing.T) {
	m, _, calls, _ := cfnTestModel(t, map[string]map[string]string{"app": {"Env": "prod"}}, nil)
	if drawer := m.drawer.View(); strings.Contains(drawer, "Loading tags...") {
		t.Errorf("Overview tab must not show the tags' loading state\n%s", drawer)
	}
	m, cmd := updateCmd(t, m, keyMsg("2"))
	if msgs := drain(t, cmd); len(msgs) != 0 {
		t.Errorf("switching tabs while the fetch is pending must not fetch again: msgs=%v", msgs)
	}
	if drawer := m.drawer.View(); !strings.Contains(drawer, "Loading tags...") {
		t.Errorf("Tags tab should show the loading state\n%s", drawer)
	}

	m = keys(t, m, "1")
	m.tagsPending = map[string]bool{}
	m, cmd = updateCmd(t, m, keyMsg("2"))
	m = apply(t, m, cmd)
	if calls["app"] != 1 {
		t.Errorf("calls = %v, want the tab switch to fetch app", calls)
	}
	if drawer := m.drawer.View(); !strings.Contains(drawer, "prod") {
		t.Errorf("Tags tab should show the fetched tags\n%s", drawer)
	}
}

// TestModel_TagFetcherFetchesRowSelectedDuringReload は読み込み中に選んだ行のタグを、一覧が届いたときに取ることを確かめる。
func TestModel_TagFetcherFetchesRowSelectedDuringReload(t *testing.T) {
	m, cmdA, calls, rows := cfnTestModel(t, map[string]map[string]string{"batch": {"Env": "batch"}}, nil)
	m = apply(t, m, cmdA)
	m = keys(t, m, "2", "r")
	m, cmd := updateCmd(t, m, keyMsg("down"))
	if msgs := drain(t, cmd); len(msgs) != 0 {
		t.Errorf("moving while loading must not fetch: msgs=%v", msgs)
	}
	m, cmd = updateCmd(t, m, rowsLoadedMsg{seq: 1, rows: rows})
	m = apply(t, m, cmd)
	if calls["batch"] != 1 || calls["app"] != 1 {
		t.Errorf("calls = %v, want batch fetched once the reload finished", calls)
	}
	if drawer := m.drawer.View(); !strings.Contains(drawer, "batch") {
		t.Errorf("Tags tab should show batch's tags\n%s", drawer)
	}
}

package tui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

type fakeTab struct {
	initCalls int
	lastMsg   tea.Msg
	view      string
}

func (t *fakeTab) Init() tea.Cmd { t.initCalls++; return nil }

func (t *fakeTab) Update(msg tea.Msg) (Tab, tea.Cmd, *TabIntent, bool) {
	t.lastMsg = msg
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "x" {
		return t, nil, &TabIntent{Status: "tab x"}, true
	}
	return t, nil, nil, false
}

func (t *fakeTab) View(int, int) string { return t.view }
func (t *fakeTab) SetSize(int, int)     {}

type fakeTabService struct {
	*fakeService
	tabs []TabSpec
}

func (s *fakeTabService) Tabs() []TabSpec { return s.tabs }

func TestModel_TabProviderSwitchesTabsAndDelegatesKeys(t *testing.T) {
	tab := &fakeTab{view: "custom tab"}
	svc := &fakeTabService{
		fakeService: &fakeService{id: "ec2", rows: testRows()},
		tabs: []TabSpec{
			{ID: tabOverview, Title: "Overview"},
			{ID: "first", Title: "First", New: func(context.Context, string, string, Row) Tab { return tab }},
			{ID: tabTags, Title: "Tags"},
		},
	}
	m := newTestModel(t, svc, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})

	if got := m.drawerTabs(); len(got) != 3 || got[1].Title != "First" {
		t.Fatalf("tabs = %+v", got)
	}
	m, cmd := updateCmd(t, m, keyMsg("2"))
	if m.tabID != "first" || m.activeTab == nil {
		t.Fatalf("tabID=%q activeTab=%v, want first", m.tabID, m.activeTab)
	}
	if cmd != nil {
		// Init が nil のため Cmd も nil。
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if view := m.View(); !strings.Contains(view, "custom tab") {
		t.Errorf("View should contain the custom tab\n%s", view)
	}

	m = keys(t, m, "tab")
	m, _ = updateCmd(t, m, keyMsg("x"))
	if m.status != "tab x" {
		t.Errorf("status = %q, want the tab intent status", m.status)
	}

	m, cmd = updateCmd(t, m, keyMsg("1"))
	if m.tabID != tabOverview || m.activeTab != nil || cmd != nil {
		t.Errorf("1 should switch to Overview: tabID=%q activeTab=%v cmd=%v", m.tabID, m.activeTab, cmd)
	}
}

func TestWrapTabCmdScopesBatches(t *testing.T) {
	if cmd := wrapTabCmd(7, nil); cmd != nil {
		t.Error("wrapTabCmd(nil) must return nil")
	}
	if msgs := drain(t, wrapTabCmd(7, tea.Batch())); len(msgs) != 0 {
		t.Errorf("empty batch msgs = %v, want none", msgs)
	}
	single := wrapTabCmd(7, func() tea.Msg { return contentTabLoadedMsg{} })
	msgs := drain(t, single)
	if len(msgs) != 1 {
		t.Fatalf("single msgs = %v, want 1", msgs)
	}
	if scoped, ok := msgs[0].(tabScopedMsg); !ok || scoped.instance != 7 {
		t.Errorf("single msg = %#v, want tabScopedMsg instance 7", msgs[0])
	}

	cmd := wrapTabCmd(7, tea.Batch(
		func() tea.Msg { return contentTabLoadedMsg{} },
		func() tea.Msg { return contentTabLoadedMsg{} },
	))
	msgs = drain(t, cmd)
	if len(msgs) != 2 {
		t.Fatalf("msgs = %v, want 2", msgs)
	}
	for _, msg := range msgs {
		scoped, ok := msg.(tabScopedMsg)
		if !ok || scoped.instance != 7 {
			t.Errorf("msg = %#v, want tabScopedMsg instance 7", msg)
		}
	}

	nested := wrapTabCmd(9, tea.Batch(
		tea.Batch(func() tea.Msg { return contentTabLoadedMsg{} }),
		func() tea.Msg { return contentTabLoadedMsg{} },
	))
	msgs = drain(t, nested)
	if len(msgs) != 2 {
		t.Fatalf("nested msgs = %v, want 2", msgs)
	}
	for _, msg := range msgs {
		scoped, ok := msg.(tabScopedMsg)
		if !ok || scoped.instance != 9 {
			t.Errorf("nested msg = %#v, want tabScopedMsg instance 9", msg)
		}
	}
}

func TestModel_StaleTabMessageIgnored(t *testing.T) {
	tab := &fakeTab{view: "custom"}
	svc := &fakeTabService{
		fakeService: &fakeService{id: "ec2", rows: testRows()},
		tabs:        []TabSpec{{ID: tabOverview, Title: "Overview"}, {ID: "first", Title: "First", New: func(context.Context, string, string, Row) Tab { return tab }}},
	}
	m := newTestModel(t, svc, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m, _ = updateCmd(t, m, keyMsg("2"))
	before := tab.lastMsg
	m = update(t, m, tabScopedMsg{instance: m.tabSeq + 1, msg: "stale"})
	if tab.lastMsg != before {
		t.Error("stale tab message must not reach the active tab")
	}
}

func TestRenderTabTableDropsRightmostColumns(t *testing.T) {
	table := TabTable{
		Columns: []Column{{Title: "One"}, {Title: "Two"}, {Title: "Three"}, {Title: "Four"}, {Title: "Five"}, {Title: "Six"}},
		Rows:    [][]string{{"value-one", "value-two", "value-three", "value-four", "value-five", "value-six"}},
	}
	view := renderTabTable(table, 30)
	if !strings.Contains(view, "…") {
		t.Errorf("truncated table must show an ellipsis:\n%s", view)
	}
	if strings.Contains(view, "Six") || strings.Contains(view, "value-six") {
		t.Errorf("rightmost column must be dropped:\n%s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > 30 {
			t.Errorf("line width = %d, want <= 30: %q", w, line)
		}
	}
}

func TestNewTabListDropsRightmostColumns(t *testing.T) {
	columns := []Column{{Title: "One"}, {Title: "Two"}, {Title: "Three"}, {Title: "Four"}, {Title: "Five"}, {Title: "Six"}}
	rows := []listRow{{cells: []string{"value-one", "value-two", "value-three", "value-four", "value-five", "value-six"}}}
	table := newTabList(columns, rows, 30, 10)
	if len(table.Columns()) >= len(columns) {
		t.Errorf("columns = %d, want fewer than %d", len(table.Columns()), len(columns))
	}
	if len(table.Columns()) == 0 || table.Columns()[len(table.Columns())-1].Title != "…" {
		t.Errorf("last column = %+v, want …", table.Columns())
	}
}

func TestContentTabLoading(t *testing.T) {
	tab := newContentTab(context.Background(), func(context.Context) (TabContent, error) {
		time.Sleep(10 * time.Millisecond)
		return TabContent{}, nil
	})
	tab.SetSize(40, 10)
	if view := tab.View(40, 10); !strings.Contains(view, "Loading...") {
		t.Errorf("loading view = %s", view)
	}
}

func TestContentTabEmptyTableShowsNoData(t *testing.T) {
	tab := newContentTab(context.Background(), func(context.Context) (TabContent, error) {
		return TabContent{Blocks: []TabBlock{{
			Title: "Images (0)",
			Table: &TabTable{Columns: []Column{{Title: "Tag"}, {Title: "Digest"}}},
		}}}, nil
	})
	tab.SetSize(40, 10)
	tab = loadTab(t, tab).(*contentTab)
	view := tab.View(40, 10)
	if !strings.Contains(view, "Images (0)") || !strings.Contains(view, "No data") {
		t.Errorf("empty table view = %s", view)
	}
}

func TestModel_TabBarShowsNumbersAndClamps(t *testing.T) {
	m := newTestModel(t, &fakeService{id: "ec2", rows: testRows()}, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	view := m.View()
	if !strings.Contains(view, "1 Overview") || !strings.Contains(view, "2 Tags") {
		t.Errorf("tab bar should show numbers\n%s", view)
	}
	m = keys(t, m, "9")
	if m.tabID != tabOverview {
		t.Errorf("out-of-range tab key changed tabID to %q", m.tabID)
	}
	m = keys(t, m, "2")
	m = keys(t, m, "]")
	if m.tabID != tabTags {
		t.Errorf("] at the last tab should stay at Tags, got %q", m.tabID)
	}
	m = keys(t, m, "1")
	m = keys(t, m, "]")
	if m.tabID != tabTags {
		t.Errorf("] should move to the next tab, got %q", m.tabID)
	}
	m = keys(t, m, "[")
	if m.tabID != tabOverview {
		t.Errorf("[ should move to the previous tab, got %q", m.tabID)
	}
	m = keys(t, m, "[")
	if m.tabID != tabOverview {
		t.Errorf("[ at the first tab should stay at Overview, got %q", m.tabID)
	}
}

func TestModel_TabBarFitsNarrowTerminal(t *testing.T) {
	svc := NewECSService(nil, func(context.Context, string, string, string) ([]aws.ECSServiceResource, error) { return nil, nil })
	rows := []Row{ecsRow{res: aws.ECSResource{ID: "arn:cluster", Name: "cluster", State: "active"}}}
	m := newTestModel(t, svc, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: rows})
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	view := m.View()
	if got := strings.Count(view, "\n") + 1; got != 24 {
		t.Errorf("View height = %d, want 24\n%s", got, view)
	}
	if !strings.Contains(view, "1 Overview") {
		t.Errorf("narrow tab bar should keep the active tab visible\n%s", view)
	}
	if !strings.Contains(view, "1-4 tabs") || !strings.Contains(view, "[ ] tabs") || !strings.Contains(view, "q quit") {
		t.Errorf("narrow footer must keep the tab hints\n%s", view)
	}
}

func TestModel_TabBarAndFooterAt120x30(t *testing.T) {
	svc := NewECSInteractiveService(nil,
		func(context.Context, string, string, string) ([]aws.ECSServiceResource, error) { return nil, nil },
		func(context.Context, string, string, string, string) ([]aws.ECSTaskResource, error) { return nil, nil },
		func(context.Context, string, string, string) ([]aws.ECSContainerInstanceResource, error) {
			return nil, nil
		},
	)
	rows := []Row{ecsRow{res: aws.ECSResource{ID: "arn:cluster", Name: "cluster", State: "active"}}}
	m := newTestModel(t, svc, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: rows})
	view := m.View()
	if got := strings.Count(view, "\n") + 1; got != 30 {
		t.Errorf("View height = %d, want 30\n%s", got, view)
	}
	for _, want := range []string{"1 Overview", "1-6 tabs", "[ ] tabs"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q\n%s", want, view)
		}
	}
}

func TestModel_TabECRHasNoTags(t *testing.T) {
	svc := NewECRService(nil, func(context.Context, string, string, string) ([]aws.ECRImageResource, error) {
		return nil, nil
	})
	rows := []Row{ecrRow{res: aws.ECRRepoResource{ID: "arn:repo", Name: "repo"}}}
	m := newTestModel(t, svc, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: rows})
	view := m.View()
	if !strings.Contains(view, "1 Overview") || !strings.Contains(view, "2 Images") {
		t.Errorf("ECR tab bar should show Overview and Images\n%s", view)
	}
	if strings.Contains(view, "Tags") {
		t.Errorf("ECR tab bar must not show Tags\n%s", view)
	}
}

func TestModel_TabSelectionRecreatesOnRowChange(t *testing.T) {
	tab := &fakeTab{view: "custom"}
	svc := &fakeTabService{
		fakeService: &fakeService{id: "ec2", rows: testRows()},
		tabs: []TabSpec{
			{ID: tabOverview, Title: "Overview"},
			{ID: "first", Title: "First", New: func(context.Context, string, string, Row) Tab { return tab }},
		},
	}
	m := newTestModel(t, svc, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m, _ = updateCmd(t, m, keyMsg("2"))
	if tab.initCalls != 1 {
		t.Fatalf("Init calls = %d, want 1", tab.initCalls)
	}
	m, cmd := updateCmd(t, m, keyMsg("down"))
	if tab.initCalls != 2 {
		t.Errorf("Init calls after row change = %d, want 2", tab.initCalls)
	}
	_ = cmd
	if m.tabID != "first" {
		t.Errorf("tabID = %q, want first", m.tabID)
	}
}

func TestModel_TabSelectionResetsOnServiceChange(t *testing.T) {
	first := &fakeTabService{
		fakeService: &fakeService{id: "first", rows: testRows()},
		tabs: []TabSpec{
			{ID: tabOverview, Title: "Overview"},
			{ID: "first-tab", Title: "First", New: func(context.Context, string, string, Row) Tab { return &fakeTab{} }},
		},
	}
	second := &fakeTabService{
		fakeService: &fakeService{id: "second", rows: testRows()},
		tabs:        []TabSpec{{ID: tabOverview, Title: "Overview"}, {ID: "second-tab", Title: "Second"}},
	}
	m, err := New(context.Background(), Deps{Services: []Service{first, second}}, Target{Scope: "profile", Region: "region"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m, _ = updateCmd(t, m, keyMsg("2"))
	if m.tabID != "first-tab" {
		t.Fatalf("tabID = %q, want first-tab", m.tabID)
	}
	p := newPicker(pickerService, "Select a service", []pickerItem{
		{Label: "first", Value: "first"},
		{Label: "second", Value: "second"},
	}, "first")
	p.cursor = 1
	m.picker = &p
	m = update(t, m, keyMsg("enter"))
	if m.service.ID() != "second" {
		t.Errorf("service = %q, want second", m.service.ID())
	}
	if m.tabID != tabOverview {
		t.Errorf("tabID = %q, want Overview after a service change", m.tabID)
	}
}

func TestModel_ECSTasksDetailFits80x24(t *testing.T) {
	task := aws.ECSTaskResource{
		ARN:                  "arn:aws:ecs:ap-northeast-1:123456789012:task/demo/task-a",
		Group:                "service:web",
		LastStatus:           "RUNNING",
		ContainerInstanceArn: "arn:instance",
		Containers: []aws.ECSTaskContainerDetail{
			{Name: "app", LastStatus: "RUNNING", RuntimeID: "rt-1", ExecEnabled: true},
		},
	}
	svc := NewECSInteractiveService(nil,
		func(context.Context, string, string, string) ([]aws.ECSServiceResource, error) { return nil, nil },
		func(context.Context, string, string, string, string) ([]aws.ECSTaskResource, error) {
			return []aws.ECSTaskResource{task}, nil
		},
		func(context.Context, string, string, string) ([]aws.ECSContainerInstanceResource, error) {
			return nil, nil
		},
	)
	m := newTestModel(t, svc, Deps{})
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	rows := []Row{ecsRow{res: aws.ECSResource{ID: "arn:cluster", Name: "demo", State: "active"}}}
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: rows})
	m, cmd := updateCmd(t, m, keyMsg("3"))
	m = apply(t, m, cmd)
	m = keys(t, m, "tab", "enter")
	view := m.View()
	if got := strings.Count(view, "\n") + 1; got != 24 {
		t.Errorf("View height = %d, want 24\n%s", got, view)
	}
	if !strings.Contains(view, "app") {
		t.Errorf("container row must be visible at 80x24\n%s", view)
	}
}

func TestModel_FilterKeepsTabOnlyWhenSelectionIsUnchanged(t *testing.T) {
	created := 0
	svc := &fakeTabService{
		fakeService: &fakeService{id: "ec2", rows: testRows()},
		tabs: []TabSpec{
			{ID: tabOverview, Title: "Overview"},
			{ID: "first", Title: "First", New: func(context.Context, string, string, Row) Tab {
				created++
				return &fakeTab{view: "custom"}
			}},
			{ID: tabTags, Title: "Tags"},
		},
	}
	m := newTestModel(t, svc, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m, _ = updateCmd(t, m, keyMsg("2"))
	if created != 1 {
		t.Fatalf("created = %d, want 1", created)
	}

	// web-1 が選ばれたままなら、絞り込みの入力でタブを作り直さない。
	m = keys(t, m, "/", "w", "e", "b")
	if created != 1 {
		t.Errorf("same row should keep the tab: created = %d", created)
	}
	if m.activeTab == nil {
		t.Fatal("tab must stay active while the same row is selected")
	}
}

func TestModel_FilterRecreatesTabWhenSelectionChanges(t *testing.T) {
	created := 0
	svc := &fakeTabService{
		fakeService: &fakeService{id: "ec2", rows: testRows()},
		tabs: []TabSpec{
			{ID: tabOverview, Title: "Overview"},
			{ID: "first", Title: "First", New: func(context.Context, string, string, Row) Tab {
				created++
				return &fakeTab{view: "custom"}
			}},
			{ID: tabTags, Title: "Tags"},
		},
	}
	m := newTestModel(t, svc, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m, _ = updateCmd(t, m, keyMsg("2"))
	if created != 1 {
		t.Fatalf("created = %d, want 1", created)
	}

	// 入力するたびに絞り込み、選択行が db-1 に変わった時点でタブを作り直す。
	m = keys(t, m, "/", "d", "b")
	if created != 2 {
		t.Errorf("selection change should recreate the tab: created = %d", created)
	}
	if m.activeTab == nil || m.selectedRow() == nil || m.selectedRow().ID() != "i-db" {
		t.Errorf("selected row = %v, want i-db", m.selectedRow())
	}

	// esc で絞り込みを解除すると web-1 に戻り、タブを作り直す。
	m = keys(t, m, "esc")
	if created != 3 {
		t.Errorf("clearing the filter should recreate the tab: created = %d", created)
	}
	if m.activeTab == nil || m.selectedRow() == nil || m.selectedRow().ID() != "i-web" {
		t.Errorf("selected row = %v, want i-web", m.selectedRow())
	}
}

type ctrlCConsumingTab struct {
	calls int
}

func (t *ctrlCConsumingTab) Init() tea.Cmd { return nil }
func (t *ctrlCConsumingTab) Update(msg tea.Msg) (Tab, tea.Cmd, *TabIntent, bool) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
		t.calls++
		return t, nil, nil, true
	}
	return t, nil, nil, false
}
func (t *ctrlCConsumingTab) View(int, int) string { return "custom" }
func (t *ctrlCConsumingTab) SetSize(int, int)     {}

func TestModel_CtrlCQuitsBeforeTab(t *testing.T) {
	tab := &ctrlCConsumingTab{}
	svc := &fakeTabService{
		fakeService: &fakeService{id: "ec2", rows: testRows()},
		tabs:        []TabSpec{{ID: tabOverview, Title: "Overview"}, {ID: "first", Title: "First", New: func(context.Context, string, string, Row) Tab { return tab }}},
	}
	m := newTestModel(t, svc, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})
	m, _ = updateCmd(t, m, keyMsg("2"))
	m = keys(t, m, "tab")
	_, cmd := updateCmd(t, m, keyMsg("ctrl+c"))
	msgs := drain(t, cmd)
	if len(msgs) != 1 {
		t.Fatalf("msgs = %v, want QuitMsg", msgs)
	}
	if _, ok := msgs[0].(tea.QuitMsg); !ok {
		t.Errorf("msg = %#v, want QuitMsg", msgs[0])
	}
	if tab.calls != 0 {
		t.Errorf("ctrl+c must be handled before the tab: tab calls = %d", tab.calls)
	}
}

func TestServiceTabsMatchWebUI(t *testing.T) {
	ecr := NewECRService(nil, func(context.Context, string, string, string) ([]aws.ECRImageResource, error) { return nil, nil }).(TabProvider)
	if got := tabTitles(ecr.Tabs()); !reflect.DeepEqual(got, []string{"Overview", "Images"}) {
		t.Errorf("ECR tabs = %v", got)
	}
	ecs := NewECSService(nil, func(context.Context, string, string, string) ([]aws.ECSServiceResource, error) { return nil, nil }).(TabProvider)
	if got := tabTitles(ecs.Tabs()); !reflect.DeepEqual(got, []string{"Overview", "Services", "Terminal", "Tags"}) {
		t.Errorf("ECS tabs = %v", got)
	}
	rds := NewRDSService(nil, func(context.Context, string, string, string) (aws.RDSClusterParameterGroup, error) {
		return aws.RDSClusterParameterGroup{}, nil
	}).(TabProvider)
	if got := tabTitles(rds.Tabs()); !reflect.DeepEqual(got, []string{"Overview", "Cluster Parameters", "Tags"}) {
		t.Errorf("RDS tabs = %v", got)
	}
	cache := NewCacheService(nil, func(context.Context, string, string, string) ([]aws.ElastiCacheParameter, error) { return nil, nil }).(TabProvider)
	if got := tabTitles(cache.Tabs()); !reflect.DeepEqual(got, []string{"Overview", "Parameters", "Tags"}) {
		t.Errorf("Cache tabs = %v", got)
	}
	cfn := NewCFNServiceWithTabs(nil, nil,
		func(context.Context, string, string, string) ([]aws.CFNStackEvent, error) { return nil, nil },
		func(context.Context, string, string, string) ([]aws.CFNStackResourceSummary, error) { return nil, nil },
	).(TabProvider)
	if got := tabTitles(cfn.Tabs()); !reflect.DeepEqual(got, []string{"Overview", "Events", "Resources", "Tags"}) {
		t.Errorf("CFN tabs = %v", got)
	}
	cf := NewCloudFrontService(nil).(TabProvider)
	if got := tabTitles(cf.Tabs()); !reflect.DeepEqual(got, []string{"Overview", "Behaviors", "Tags"}) {
		t.Errorf("CloudFront tabs = %v", got)
	}
	ec2 := NewEC2Service(nil).(TabProvider)
	if got := tabTitles(ec2.Tabs()); !reflect.DeepEqual(got, []string{"Overview", "Terminal", "Tags"}) {
		t.Errorf("EC2 tabs = %v", got)
	}

	ecsInteractive := NewECSInteractiveService(nil,
		func(context.Context, string, string, string) ([]aws.ECSServiceResource, error) { return nil, nil },
		func(context.Context, string, string, string, string) ([]aws.ECSTaskResource, error) { return nil, nil },
		func(context.Context, string, string, string) ([]aws.ECSContainerInstanceResource, error) {
			return nil, nil
		},
	).(TabProvider)
	if got := tabTitles(ecsInteractive.Tabs()); !reflect.DeepEqual(got, []string{"Overview", "Services", "Tasks", "Instances", "Terminal", "Tags"}) {
		t.Errorf("ECS interactive tabs = %v", got)
	}
	rdsInteractive := NewRDSInteractiveService(nil, func(context.Context, string, string, string) ([]aws.RDSParameter, error) { return nil, nil }, nil).(TabProvider)
	if got := tabTitles(rdsInteractive.Tabs()); !reflect.DeepEqual(got, []string{"Overview", "Instance Parameters", "Tags"}) {
		t.Errorf("RDS interactive tabs = %v", got)
	}
	elbInteractive := NewELBServiceWithTabs(nil, nil, nil, nil, nil).(TabProvider)
	if got := tabTitles(elbInteractive.Tabs()); !reflect.DeepEqual(got, []string{"Overview", "Tags"}) {
		t.Errorf("ELB tabs without loaders = %v", got)
	}
	wafInteractive := NewWAFServiceWithRules(nil, func(context.Context, string, string, string, string, string) ([]aws.WAFRule, error) { return nil, nil }).(TabProvider)
	if got := tabTitles(wafInteractive.Tabs()); !reflect.DeepEqual(got, []string{"Overview", "Rules", "Tags"}) {
		t.Errorf("WAF interactive tabs = %v", got)
	}
}

func TestServiceTabLoaders(t *testing.T) {
	t.Run("ECR Images", func(t *testing.T) {
		var got string
		svc := NewECRService(nil, func(_ context.Context, profile, region, repo string) ([]aws.ECRImageResource, error) {
			got = profile + "/" + region + "/" + repo
			return []aws.ECRImageResource{{ImageTag: "v1", ImageDigest: "sha256:0123456789012345678901234567890123456789", ImageSizeBytes: 1024, PushedAt: "now"}}, nil
		}).(TabProvider)
		tab := newTabForTest(t, svc.Tabs(), "images", ecrRow{res: aws.ECRRepoResource{Name: "repo"}})
		content := loadContentTab(t, tab)
		if got != "prof/region/repo" {
			t.Errorf("loader args = %q", got)
		}
		if view := content.View(80, 20); !strings.Contains(view, "v1") {
			t.Errorf("view = %s", view)
		}
	})
	t.Run("ECS Services", func(t *testing.T) {
		var got string
		svc := NewECSService(nil, func(_ context.Context, profile, region, cluster string) ([]aws.ECSServiceResource, error) {
			got = profile + "/" + region + "/" + cluster
			return []aws.ECSServiceResource{{Name: "web", Status: "ACTIVE", DesiredCount: 2, RunningCount: 1}}, nil
		}).(TabProvider)
		tab := newTabForTest(t, svc.Tabs(), "services", ecsRow{res: aws.ECSResource{Name: "cluster"}})
		content := loadContentTab(t, tab)
		if got != "prof/region/cluster" {
			t.Errorf("loader args = %q", got)
		}
		if view := content.View(80, 20); !strings.Contains(view, "web") {
			t.Errorf("view = %s", view)
		}
	})
	t.Run("RDS Cluster Parameters", func(t *testing.T) {
		called := false
		svc := NewRDSService(nil, func(_ context.Context, _, _, clusterID string) (aws.RDSClusterParameterGroup, error) {
			called = true
			return aws.RDSClusterParameterGroup{GroupName: "group", Parameters: []aws.RDSParameter{{Name: "max_connections", Value: "100"}}}, nil
		}).(TabProvider)
		tab := newTabForTest(t, svc.Tabs(), "cluster-parameters", rdsRow{res: aws.RDSResource{Name: "db", ClusterID: "cluster"}})
		content := loadContentTab(t, tab)
		if !called || !strings.Contains(content.View(80, 20), "max_connections") {
			t.Errorf("called=%v view=%s", called, content.View(80, 20))
		}
		emptyCalled := false
		svc = NewRDSService(nil, func(context.Context, string, string, string) (aws.RDSClusterParameterGroup, error) {
			emptyCalled = true
			return aws.RDSClusterParameterGroup{}, nil
		}).(TabProvider)
		tab = newTabForTest(t, svc.Tabs(), "cluster-parameters", rdsRow{res: aws.RDSResource{Name: "db"}})
		content = loadContentTab(t, tab)
		if emptyCalled || !strings.Contains(content.View(80, 20), "Not part of a DB cluster.") {
			t.Errorf("emptyCalled=%v view=%s", emptyCalled, content.View(80, 20))
		}
	})
	t.Run("Cache Parameters", func(t *testing.T) {
		called := false
		svc := NewCacheService(nil, func(_ context.Context, _, region, group string) ([]aws.ElastiCacheParameter, error) {
			called = true
			return []aws.ElastiCacheParameter{{Name: "timeout", Value: "30"}}, nil
		}).(TabProvider)
		tab := newTabForTest(t, svc.Tabs(), "parameters", cacheRow{res: aws.ElastiCacheResource{Name: "cache", ParameterGroup: "group"}})
		content := loadContentTab(t, tab)
		if !called || !strings.Contains(content.View(80, 20), "timeout") {
			t.Errorf("called=%v view=%s", called, content.View(80, 20))
		}

		emptyCalled := false
		empty := NewCacheService(nil, func(context.Context, string, string, string) ([]aws.ElastiCacheParameter, error) {
			emptyCalled = true
			return nil, nil
		}).(TabProvider)
		emptyTab := newTabForTest(t, empty.Tabs(), "parameters", cacheRow{res: aws.ElastiCacheResource{Name: "cache"}})
		emptyContent := loadContentTab(t, emptyTab)
		if emptyCalled || !strings.Contains(emptyContent.View(80, 20), "No parameter group.") {
			t.Errorf("emptyCalled=%v view=%s", emptyCalled, emptyContent.View(80, 20))
		}
	})
	t.Run("CFN Events and Resources", func(t *testing.T) {
		svc := NewCFNServiceWithTabs(nil, nil,
			func(_ context.Context, _, _, stack string) ([]aws.CFNStackEvent, error) {
				return []aws.CFNStackEvent{{LogicalResourceID: "Bucket", ResourceStatus: "CREATE_COMPLETE"}}, nil
			},
			func(_ context.Context, _, _, stack string) ([]aws.CFNStackResourceSummary, error) {
				return []aws.CFNStackResourceSummary{{LogicalResourceID: "Bucket", ResourceStatus: "CREATE_COMPLETE"}}, nil
			},
		).(TabProvider)
		events := loadContentTab(t, newTabForTest(t, svc.Tabs(), "events", cfnRow{res: aws.CFNStackResource{Name: "stack"}}))
		if !strings.Contains(events.View(80, 20), "Bucket") {
			t.Errorf("events view = %s", events.View(80, 20))
		}
		resources := loadContentTab(t, newTabForTest(t, svc.Tabs(), "resources", cfnRow{res: aws.CFNStackResource{Name: "stack"}}))
		if !strings.Contains(resources.View(80, 20), "Bucket") {
			t.Errorf("resources view = %s", resources.View(80, 20))
		}
	})
	t.Run("CloudFront Behaviors", func(t *testing.T) {
		svc := NewCloudFrontService(nil).(TabProvider)
		tab := newTabForTest(t, svc.Tabs(), "behaviors", cloudfrontRow{res: aws.CloudFrontResource{Behaviors: []aws.CloudFrontBehavior{{PathPattern: "/api/*", TargetOriginID: "origin"}}}})
		content := loadContentTab(t, tab)
		if !strings.Contains(content.View(80, 20), "/api/*") {
			t.Errorf("view = %s", content.View(80, 20))
		}
		empty := newTabForTest(t, svc.Tabs(), "behaviors", cloudfrontRow{res: aws.CloudFrontResource{}})
		emptyContent := loadContentTab(t, empty)
		if view := emptyContent.View(80, 20); !strings.Contains(view, "Behaviors (0)") || !strings.Contains(view, "No data") {
			t.Errorf("empty view = %s", view)
		}
	})
}

func tabTitles(tabs []TabSpec) []string {
	out := make([]string, 0, len(tabs))
	for _, tab := range tabs {
		out = append(out, tab.Title)
	}
	return out
}

func newTabForTest(t *testing.T, tabs []TabSpec, id string, row Row) Tab {
	t.Helper()
	for _, spec := range tabs {
		if spec.ID == id {
			tab := spec.New(context.Background(), "prof", "region", row)
			if tab == nil {
				t.Fatalf("tab %q returned nil", id)
			}
			return tab
		}
	}
	t.Fatalf("tab %q not found in %v", id, tabTitles(tabs))
	return nil
}

func loadContentTab(t *testing.T, tab Tab) *contentTab {
	t.Helper()
	cmd := tab.Init()
	if cmd == nil {
		t.Fatal("content tab Init returned nil")
	}
	for _, msg := range drain(t, cmd) {
		next, _, _, handled := tab.Update(msg)
		if !handled {
			t.Fatalf("content tab did not handle %#v", msg)
		}
		tab = next
	}
	content, ok := tab.(*contentTab)
	if !ok {
		t.Fatalf("tab = %T, want *contentTab", tab)
	}
	content.SetSize(80, 20)
	return content
}

func TestContentTabErrorAndEmpty(t *testing.T) {
	tab := newContentTab(context.Background(), func(context.Context) (TabContent, error) {
		return TabContent{}, errors.New("boom")
	})
	cmd := tab.Init()
	for _, msg := range drain(t, cmd) {
		next, _, _, _ := tab.Update(msg)
		tab = next.(*contentTab)
	}
	if view := tab.View(80, 20); !strings.Contains(view, "boom") {
		t.Errorf("error view = %s", view)
	}

	tab = newContentTab(context.Background(), func(context.Context) (TabContent, error) {
		return TabContent{}, nil
	})
	cmd = tab.Init()
	for _, msg := range drain(t, cmd) {
		next, _, _, _ := tab.Update(msg)
		tab = next.(*contentTab)
	}
	if view := tab.View(80, 20); !strings.Contains(view, "No data") {
		t.Errorf("empty view = %s", view)
	}
}

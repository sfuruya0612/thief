package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestListDetailTabImmediateDetailAndIntent(t *testing.T) {
	tab := newListDetailTab(context.Background(),
		[]Column{{Title: "Name"}},
		func(context.Context) ([]listRow, error) {
			return []listRow{{cells: []string{"task-a"}, value: "task-a"}}, nil
		},
		func(row listRow) *TabDetail {
			return &TabDetail{
				Title:      "Detail",
				KVs:        []KV{{Key: "Name", Value: row.value.(string)}},
				Table:      &TabTable{Columns: []Column{{Title: "Container"}}, Rows: [][]string{{"app"}}},
				Selectable: true,
				Intent: func(index int) *TabIntent {
					return &TabIntent{Status: "selected app"}
				},
			}
		},
		nil,
	)
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*listDetailTab)
	if view := tab.View(80, 20); !strings.Contains(view, "task-a") {
		t.Errorf("list view = %s", view)
	}

	next, _, _, handled := tab.Update(keyMsg("enter"))
	if !handled {
		t.Fatal("enter should open the detail")
	}
	tab = next.(*listDetailTab)
	if tab.focus != listDetailFocusDetail || tab.detail == nil {
		t.Fatalf("focus=%v detail=%v, want detail focus", tab.focus, tab.detail)
	}
	_, _, intent, handled := tab.Update(keyMsg("s"))
	if !handled || intent == nil || intent.Status != "selected app" {
		t.Errorf("s = handled %v intent %+v, want selected app", handled, intent)
	}

	next, _, _, handled = tab.Update(keyMsg("esc"))
	if !handled {
		t.Fatal("esc should close the detail")
	}
	tab = next.(*listDetailTab)
	if tab.focus != listDetailFocusList || tab.detail != nil {
		t.Errorf("focus=%v detail=%v, want list", tab.focus, tab.detail)
	}
}

func TestListDetailTabAsyncDetailAndStaleResult(t *testing.T) {
	var calls int
	tab := newListDetailTab(context.Background(),
		[]Column{{Title: "Name"}},
		func(context.Context) ([]listRow, error) {
			return []listRow{{cells: []string{"listener"}, value: "arn:listener"}}, nil
		},
		nil,
		func(_ context.Context, row listRow) (*TabDetail, error) {
			calls++
			return &TabDetail{Title: "Rules", Text: "rule for " + row.value.(string)}, nil
		},
	)
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*listDetailTab)
	next, cmd, _, handled := tab.Update(keyMsg("enter"))
	if !handled || cmd == nil {
		t.Fatal("enter should start the detail load")
	}
	tab = next.(*listDetailTab)
	if !tab.detailLoad {
		t.Fatal("detail should be loading")
	}
	msgs := drain(t, cmd)
	if len(msgs) != 1 {
		t.Fatalf("msgs = %v", msgs)
	}
	fetched, ok := msgs[0].(listDetailFetchedMsg)
	if !ok {
		t.Fatalf("msg = %T", msgs[0])
	}
	next, _, _, _ = tab.Update(fetched)
	tab = next.(*listDetailTab)
	if calls != 1 || !strings.Contains(tab.View(80, 20), "rule for arn:listener") {
		t.Errorf("calls=%d view=%s", calls, tab.View(80, 20))
	}

	// 詳細を閉じた後に届いた古い結果は捨てる。
	tab.Update(keyMsg("esc"))
	tab.Update(listDetailFetchedMsg{seq: fetched.seq, detail: &TabDetail{Text: "stale"}})
	if strings.Contains(tab.View(80, 20), "stale") {
		t.Error("stale detail must not be rendered")
	}
}

func TestListDetailTabEmptyText(t *testing.T) {
	tab := newListDetailTab(context.Background(),
		[]Column{{Title: "Name"}},
		func(context.Context) ([]listRow, error) { return nil, nil },
		nil, nil,
	)
	tab.empty = "No listeners"
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*listDetailTab)
	if view := tab.View(80, 20); !strings.Contains(view, "No listeners") {
		t.Errorf("empty view = %s", view)
	}
}

func TestListDetailTabListError(t *testing.T) {
	tab := newListDetailTab(context.Background(),
		[]Column{{Title: "Name"}},
		func(context.Context) ([]listRow, error) { return nil, errors.New("boom") },
		nil, nil,
	)
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*listDetailTab)
	if view := tab.View(80, 20); !strings.Contains(view, "boom") {
		t.Errorf("error view = %s", view)
	}
}

func TestECSInteractiveTabs(t *testing.T) {
	svc := NewECSInteractiveService(nil,
		func(context.Context, string, string, string) ([]aws.ECSServiceResource, error) { return nil, nil },
		func(context.Context, string, string, string, string) ([]aws.ECSTaskResource, error) {
			return []aws.ECSTaskResource{{
				ARN:                  "arn:aws:ecs:ap-northeast-1:123456789012:task/demo/task-a",
				Group:                "service:web",
				LastStatus:           "RUNNING",
				ContainerInstanceArn: "arn:instance",
				Containers:           []aws.ECSTaskContainerDetail{{Name: "app", LastStatus: "RUNNING", RuntimeID: "rt-1", ExecEnabled: true}},
			}}, nil
		},
		func(context.Context, string, string, string) ([]aws.ECSContainerInstanceResource, error) {
			return []aws.ECSContainerInstanceResource{{ARN: "arn:instance", EC2InstanceID: "i-1", Status: "ACTIVE"}}, nil
		},
	).(TabProvider)
	if got := tabTitles(svc.Tabs()); !strings.HasSuffix(strings.Join(got, ","), "Tasks,Instances,Terminal,Tags") {
		t.Fatalf("tabs = %v", got)
	}

	cluster := ecsRow{res: aws.ECSResource{ID: "arn:cluster", Name: "demo"}}
	tasksTab := newTabForTest(t, svc.Tabs(), "tasks", cluster).(*listDetailTab)
	tasksTab.SetSize(80, 20)
	tasksTab = loadTab(t, tasksTab).(*listDetailTab)
	if view := tasksTab.View(80, 20); !strings.Contains(view, "app") {
		t.Errorf("tasks list = %s", view)
	}
	next, _, _, _ := tasksTab.Update(keyMsg("enter"))
	tasksTab = next.(*listDetailTab)
	if tasksTab.detail == nil || !strings.Contains(tasksTab.View(80, 20), "app") {
		t.Errorf("task detail = %s", tasksTab.View(80, 20))
	}
	_, _, intent, handled := tasksTab.Update(keyMsg("s"))
	if !handled || intent == nil || intent.ECSExec == nil {
		t.Fatalf("s intent = %+v handled=%v", intent, handled)
	}
	if intent.ECSExec.Task != "task-a" || intent.ECSExec.Container != "app" || intent.ECSExec.Cluster != "demo" {
		t.Errorf("intent = %+v", intent.ECSExec)
	}

	instancesTab := newTabForTest(t, svc.Tabs(), "instances", cluster).(*listDetailTab)
	instancesTab.SetSize(80, 20)
	instancesTab = loadTab(t, instancesTab).(*listDetailTab)
	if view := instancesTab.View(80, 20); !strings.Contains(view, "i-1") {
		t.Errorf("instances list = %s", view)
	}
	next, _, _, _ = instancesTab.Update(keyMsg("enter"))
	instancesTab = next.(*listDetailTab)
	if instancesTab.detail == nil || !strings.Contains(instancesTab.View(80, 20), "RUNNING") {
		t.Errorf("instance detail = %s", instancesTab.View(80, 20))
	}
}

func TestELBWAFAndRDSTabs(t *testing.T) {
	elb := NewELBServiceWithTabs(nil,
		func(context.Context, string, string, string) ([]aws.ELBListenerResource, error) {
			return []aws.ELBListenerResource{{ARN: "arn:listener", Protocol: "HTTP", Port: 80}}, nil
		},
		func(context.Context, string, string, string) ([]aws.ELBRuleResource, error) {
			return []aws.ELBRuleResource{{ARN: "arn:rule", Priority: "1", ActionType: "forward"}}, nil
		},
		func(context.Context, string, string, string) ([]aws.ELBTargetGroupResource, error) {
			return []aws.ELBTargetGroupResource{{ARN: "arn:tg", Name: "tg", Protocol: "HTTP", Port: 80}}, nil
		},
		func(context.Context, string, string, string) ([]aws.ELBTargetHealthResource, error) {
			return []aws.ELBTargetHealthResource{{TargetID: "i-1", Port: 80, State: "healthy"}}, nil
		},
	).(TabProvider)
	lb := elbRow{res: aws.ELBResource{ID: "arn:lb", Name: "lb"}}
	listeners := newTabForTest(t, elb.Tabs(), "listeners", lb).(*listDetailTab)
	listeners.SetSize(80, 20)
	listeners = loadTab(t, listeners).(*listDetailTab)
	next, cmd, _, _ := listeners.Update(keyMsg("enter"))
	listeners = next.(*listDetailTab)
	listeners = applyTabCmd(t, listeners, cmd).(*listDetailTab)
	if !strings.Contains(listeners.View(80, 20), "forward") {
		t.Errorf("listener rules = %s", listeners.View(80, 20))
	}

	waf := NewWAFServiceWithRules(nil, func(context.Context, string, string, string, string, string) ([]aws.WAFRule, error) {
		return []aws.WAFRule{{Name: "rule", Priority: 1, RuleJSON: `{"Name":"rule"}`}}, nil
	}).(TabProvider)
	wafTab := newTabForTest(t, waf.Tabs(), "rules", wafRow{res: aws.WAFResource{ID: "id", Name: "acl", Scope: "REGIONAL"}}).(*listDetailTab)
	wafTab.SetSize(80, 20)
	wafTab = loadTab(t, wafTab).(*listDetailTab)
	next, _, _, _ = wafTab.Update(keyMsg("enter"))
	wafTab = next.(*listDetailTab)
	if !strings.Contains(wafTab.View(80, 20), "rule") {
		t.Errorf("waf detail = %s", wafTab.View(80, 20))
	}

	rds := NewRDSInteractiveService(nil,
		func(context.Context, string, string, string) ([]aws.RDSParameter, error) {
			return []aws.RDSParameter{{Name: "max_connections", Value: "100"}}, nil
		},
		nil,
	).(TabProvider)
	rdsTab := newTabForTest(t, rds.Tabs(), "instance-parameters", rdsRow{res: aws.RDSResource{Name: "db", ParameterGroups: []string{"group"}}}).(*rdsInstanceParametersTab)
	rdsTab.SetSize(80, 20)
	rdsTab = loadTab(t, rdsTab).(*rdsInstanceParametersTab)
	if view := rdsTab.View(80, 20); !strings.Contains(view, "max_connections") || !strings.Contains(view, "Group: group") {
		t.Errorf("rds params = %s", view)
	}
}

func applyTabCmd(t *testing.T, tab Tab, cmd tea.Cmd) Tab {
	t.Helper()
	if cmd == nil {
		return tab
	}
	for _, msg := range drain(t, cmd) {
		next, nextCmd, _, _ := tab.Update(msg)
		tab = next
		if nextCmd != nil {
			tab = applyTabCmd(t, tab, nextCmd)
		}
	}
	return tab
}

func loadTab(t *testing.T, tab Tab) Tab {
	t.Helper()
	cmd := tab.Init()
	if cmd == nil {
		return tab
	}
	for _, msg := range drain(t, cmd) {
		next, nextCmd, _, _ := tab.Update(msg)
		tab = next
		if nextCmd != nil {
			tab = loadTab(t, tab)
		}
	}
	return tab
}

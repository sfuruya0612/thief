package tui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestECSTasksTabContainerDisplayAndExecIntent(t *testing.T) {
	exit := int32(1)
	tasks := []aws.ECSTaskResource{
		{
			ARN:                  "arn:aws:ecs:ap-northeast-1:123456789012:task/demo/task-a",
			Group:                "service:web",
			LastStatus:           "RUNNING",
			ContainerInstanceArn: "arn:instance",
			Containers: []aws.ECSTaskContainerDetail{
				{Name: "plain", CPU: "0", Memory: "2048", MemoryReservation: "1024", ExecEnabled: false},
				{Name: "exec", CPU: "512", Memory: "", MemoryReservation: "512", ExitCode: &exit, Reason: "oom", ExecEnabled: true},
			},
		},
		{
			ARN:        "arn:aws:ecs:ap-northeast-1:123456789012:task/demo/task-b",
			Group:      "service:empty",
			LastStatus: "RUNNING",
		},
	}
	svc := NewECSInteractiveService(nil,
		func(context.Context, string, string, string) ([]aws.ECSServiceResource, error) { return nil, nil },
		func(context.Context, string, string, string, string) ([]aws.ECSTaskResource, error) {
			return tasks, nil
		},
		func(context.Context, string, string, string) ([]aws.ECSContainerInstanceResource, error) {
			return nil, nil
		},
	).(TabProvider)
	cluster := ecsRow{res: aws.ECSResource{ID: "arn:cluster", Name: "demo"}}
	tab := newTabForTest(t, svc.Tabs(), "tasks", cluster).(*listDetailTab)
	tab.SetSize(200, 40)
	tab = loadTab(t, tab).(*listDetailTab)

	next, _, _, _ := tab.Update(keyMsg("enter"))
	tab = next.(*listDetailTab)
	view := tab.View(200, 40)
	for _, want := range []string{"2048 / 1024", "512", "oom"} {
		if !strings.Contains(view, want) {
			t.Errorf("container view missing %q:\n%s", want, view)
		}
	}
	if len(tab.detail.Table.Rows) != 2 {
		t.Fatalf("container rows = %d, want 2", len(tab.detail.Table.Rows))
	}
	first := tab.detail.Table.Rows[0]
	if first[4] != dash || first[5] != "2048 / 1024" || first[6] != dash || first[7] != dash {
		t.Errorf("first container row = %v, want CPU - / Memory 2048 / 1024 / Exit - / Reason -", first)
	}
	second := tab.detail.Table.Rows[1]
	if second[4] != "512" || second[5] != "512" || second[6] != "1" || second[7] != "oom" {
		t.Errorf("second container row = %v", second)
	}
	// 先頭の Exec 無効のコンテナでは接続せず、通知用の status を返す。
	_, _, intent, handled := tab.Update(keyMsg("s"))
	if !handled || intent == nil || intent.Status != "container plain does not support ECS Exec" {
		t.Fatalf("s on a disabled container = handled %v intent %+v", handled, intent)
	}
	next, _, _, _ = tab.Update(keyMsg("down"))
	tab = next.(*listDetailTab)
	_, _, intent, handled = tab.Update(keyMsg("enter"))
	if !handled || intent == nil || intent.ECSExec == nil {
		t.Fatalf("enter on an enabled container = handled %v intent %+v", handled, intent)
	}
	if intent.ECSExec.Cluster != "demo" || intent.ECSExec.Task != "task-a" || intent.ECSExec.Container != "exec" {
		t.Errorf("intent = %+v", intent.ECSExec)
	}
	_, _, intent, handled = tab.Update(keyMsg("s"))
	if !handled || intent == nil || intent.ECSExec == nil {
		t.Fatalf("s on an enabled container = handled %v intent %+v", handled, intent)
	}
	if intent.ECSExec.Cluster != "demo" || intent.ECSExec.Task != "task-a" || intent.ECSExec.Container != "exec" {
		t.Errorf("intent = %+v", intent.ECSExec)
	}

	// コンテナが 0 件のタスクでは No containers を表示する。
	next, _, _, _ = tab.Update(keyMsg("esc"))
	tab = next.(*listDetailTab)
	next, _, _, _ = tab.Update(keyMsg("down"))
	tab = next.(*listDetailTab)
	next, _, _, _ = tab.Update(keyMsg("enter"))
	tab = next.(*listDetailTab)
	if view := tab.View(200, 40); !strings.Contains(view, "No containers") {
		t.Errorf("empty containers = %s", view)
	}
}

func TestECSInstancesTabGroupsTasksAndUnknown(t *testing.T) {
	instances := []aws.ECSContainerInstanceResource{
		{ARN: "arn:instance-a", EC2InstanceID: "i-1", Status: "ACTIVE", AgentConnected: true},
		{ARN: "arn:instance-b", EC2InstanceID: "i-2", Status: "ACTIVE", AgentConnected: true},
	}
	tasks := []aws.ECSTaskResource{
		{ARN: "arn:task-a", Group: "service:web", LastStatus: "RUNNING", ContainerInstanceArn: "arn:instance-b"},
		{ARN: "arn:fargate", Group: "service:fargate", LastStatus: "RUNNING"},
		{ARN: "arn:task-unknown", Group: "service:unknown", LastStatus: "RUNNING", ContainerInstanceArn: "arn:gone"},
	}
	svc := NewECSInteractiveService(nil,
		func(context.Context, string, string, string) ([]aws.ECSServiceResource, error) { return nil, nil },
		func(context.Context, string, string, string, string) ([]aws.ECSTaskResource, error) {
			return tasks, nil
		},
		func(context.Context, string, string, string) ([]aws.ECSContainerInstanceResource, error) {
			return instances, nil
		},
	).(TabProvider)
	cluster := ecsRow{res: aws.ECSResource{ID: "arn:cluster", Name: "demo"}}
	tab := newTabForTest(t, svc.Tabs(), "instances", cluster).(*listDetailTab)
	tab.SetSize(200, 40)
	tab = loadTab(t, tab).(*listDetailTab)
	view := tab.View(200, 40)
	if !strings.Contains(view, "i-1") || !strings.Contains(view, "i-2") || !strings.Contains(view, "Unknown container instance") {
		t.Errorf("instances view = %s", view)
	}

	// i-1 にはタスクが無いので No tasks。
	next, _, _, _ := tab.Update(keyMsg("enter"))
	tab = next.(*listDetailTab)
	if view := tab.View(200, 40); !strings.Contains(view, "No tasks") {
		t.Errorf("i-1 detail = %s", view)
	}
	// Instances の詳細では Exec を開始しない。s はタブが消費し、Intent を返さない。
	_, _, intent, handled := tab.Update(keyMsg("s"))
	if !handled || intent != nil {
		t.Errorf("s in an instance detail: handled=%v intent=%+v, want consumed without an intent", handled, intent)
	}

	// i-2 には task-a だけが表示され、Fargate と不明なインスタンスのタスクは含まれない。
	next, _, _, _ = tab.Update(keyMsg("esc"))
	tab = next.(*listDetailTab)
	next, _, _, _ = tab.Update(keyMsg("down"))
	tab = next.(*listDetailTab)
	next, _, _, _ = tab.Update(keyMsg("enter"))
	tab = next.(*listDetailTab)
	detail := tab.View(200, 40)
	if !strings.Contains(detail, "service:web") {
		t.Errorf("i-2 detail = %s", detail)
	}
	if strings.Contains(detail, "service:fargate") {
		t.Errorf("Fargate task must not be shown in an instance detail: %s", detail)
	}

	// Unknown container instance の行にも不明なタスクがまとまる。
	next, _, _, _ = tab.Update(keyMsg("esc"))
	tab = next.(*listDetailTab)
	next, _, _, _ = tab.Update(keyMsg("down"))
	tab = next.(*listDetailTab)
	next, _, _, _ = tab.Update(keyMsg("enter"))
	tab = next.(*listDetailTab)
	if view := tab.View(200, 40); !strings.Contains(view, "service:unknown") {
		t.Errorf("unknown group detail = %s", view)
	}
}

func TestModel_ECSTasksTabStartsExecOnce(t *testing.T) {
	var starts []string
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
	m := newTestModel(t, svc, Deps{
		OpenECSExecSession: func(_ context.Context, profile, region, cluster, taskID, container, command string) (InteractiveSession, error) {
			starts = append(starts, strings.Join([]string{profile, region, cluster, taskID, container, command}, "/"))
			return newFakeInteractiveSession(), nil
		},
	})
	rows := []Row{ecsRow{res: aws.ECSResource{ID: "arn:cluster", Name: "demo", State: "active"}}}
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: rows})
	m, cmd := updateCmd(t, m, keyMsg("3"))
	m = apply(t, m, cmd)
	m = keys(t, m, "tab", "enter")

	m, first := updateCmd(t, m, keyMsg("s"))
	if first == nil || !m.sessionStarting {
		t.Fatalf("first s should start a session: cmd=%v starting=%v", first, m.sessionStarting)
	}
	m, second := updateCmd(t, m, keyMsg("s"))
	if second != nil {
		t.Error("second s must not start another session")
	}
	m = apply(t, m, first)
	if want := []string{"first/ap-northeast-1/demo/task-a/app//bin/sh"}; !reflect.DeepEqual(starts, want) {
		t.Errorf("starts = %v, want %v", starts, want)
	}
	if m.session == nil {
		t.Error("session must be open after applying the start command")
	}
}

func TestELBEmptyStatesAndDetailError(t *testing.T) {
	newELB := func(listeners []aws.ELBListenerResource, groups []aws.ELBTargetGroupResource) TabProvider {
		return NewELBServiceWithTabs(nil,
			func(context.Context, string, string, string) ([]aws.ELBListenerResource, error) {
				return listeners, nil
			},
			func(context.Context, string, string, string) ([]aws.ELBRuleResource, error) {
				return nil, errors.New("rules boom")
			},
			func(context.Context, string, string, string) ([]aws.ELBTargetGroupResource, error) {
				return groups, nil
			},
			func(context.Context, string, string, string) ([]aws.ELBTargetHealthResource, error) {
				return nil, errors.New("health boom")
			},
		).(TabProvider)
	}
	lb := elbRow{res: aws.ELBResource{ID: "arn:lb", Name: "lb"}}

	empty := newELB(nil, nil)
	listeners := newTabForTest(t, empty.Tabs(), "listeners", lb).(*listDetailTab)
	listeners.SetSize(120, 30)
	listeners = loadTab(t, listeners).(*listDetailTab)
	if view := listeners.View(120, 30); !strings.Contains(view, "No listeners") {
		t.Errorf("empty listeners = %s", view)
	}
	targets := newTabForTest(t, empty.Tabs(), "targets", lb).(*listDetailTab)
	targets.SetSize(120, 30)
	targets = loadTab(t, targets).(*listDetailTab)
	if view := targets.View(120, 30); !strings.Contains(view, "No target groups") {
		t.Errorf("empty target groups = %s", view)
	}

	withItems := newELB([]aws.ELBListenerResource{{ARN: "arn:listener"}}, []aws.ELBTargetGroupResource{{ARN: "arn:tg", Name: "tg"}})
	emptyDetail := NewELBServiceWithTabs(nil,
		func(context.Context, string, string, string) ([]aws.ELBListenerResource, error) {
			return []aws.ELBListenerResource{{ARN: "arn:listener"}}, nil
		},
		func(context.Context, string, string, string) ([]aws.ELBRuleResource, error) { return nil, nil },
		func(context.Context, string, string, string) ([]aws.ELBTargetGroupResource, error) {
			return []aws.ELBTargetGroupResource{{ARN: "arn:tg", Name: "tg"}}, nil
		},
		func(context.Context, string, string, string) ([]aws.ELBTargetHealthResource, error) { return nil, nil },
	).(TabProvider)
	listenerEmpty := newTabForTest(t, emptyDetail.Tabs(), "listeners", lb).(*listDetailTab)
	listenerEmpty.SetSize(120, 30)
	listenerEmpty = loadTab(t, listenerEmpty).(*listDetailTab)
	next, cmd, _, _ := listenerEmpty.Update(keyMsg("enter"))
	listenerEmpty = next.(*listDetailTab)
	listenerEmpty = applyTabCmd(t, listenerEmpty, cmd).(*listDetailTab)
	if view := listenerEmpty.View(120, 30); !strings.Contains(view, "No rules") {
		t.Errorf("empty rules = %s", view)
	}
	targetEmpty := newTabForTest(t, emptyDetail.Tabs(), "targets", lb).(*listDetailTab)
	targetEmpty.SetSize(120, 30)
	targetEmpty = loadTab(t, targetEmpty).(*listDetailTab)
	next, cmd, _, _ = targetEmpty.Update(keyMsg("enter"))
	targetEmpty = next.(*listDetailTab)
	targetEmpty = applyTabCmd(t, targetEmpty, cmd).(*listDetailTab)
	if view := targetEmpty.View(120, 30); !strings.Contains(view, "No targets") {
		t.Errorf("empty targets = %s", view)
	}

	failed := newTabForTest(t, withItems.Tabs(), "listeners", lb).(*listDetailTab)
	failed.SetSize(120, 30)
	failed = loadTab(t, failed).(*listDetailTab)
	next, cmd, _, _ = failed.Update(keyMsg("enter"))
	failed = next.(*listDetailTab)
	failed = applyTabCmd(t, failed, cmd).(*listDetailTab)
	if view := failed.View(120, 30); !strings.Contains(view, "rules boom") || !strings.Contains(view, "arn:listener") {
		t.Errorf("listener detail error must keep the list: %s", view)
	}

	targetFailed := newTabForTest(t, withItems.Tabs(), "targets", lb).(*listDetailTab)
	targetFailed.SetSize(120, 30)
	targetFailed = loadTab(t, targetFailed).(*listDetailTab)
	next, cmd, _, _ = targetFailed.Update(keyMsg("enter"))
	targetFailed = next.(*listDetailTab)
	targetFailed = applyTabCmd(t, targetFailed, cmd).(*listDetailTab)
	if view := targetFailed.View(120, 30); !strings.Contains(view, "health boom") || !strings.Contains(view, "tg") {
		t.Errorf("target detail error must keep the list: %s", view)
	}
}

func TestWAFRulesTabJSONFormatting(t *testing.T) {
	rules := []aws.WAFRule{
		{Name: "valid", RuleJSON: `{"Name":"valid","Statement":{"ByteMatchStatement":{"SearchString":"x"}}}`},
		{Name: "invalid", RuleJSON: `{not json`},
		{Name: "empty"},
	}
	svc := NewWAFServiceWithRules(nil, func(context.Context, string, string, string, string, string) ([]aws.WAFRule, error) {
		return rules, nil
	}).(TabProvider)
	row := wafRow{res: aws.WAFResource{ID: "id", Name: "acl", Scope: "REGIONAL"}}
	tab := newTabForTest(t, svc.Tabs(), "rules", row).(*listDetailTab)
	tab.SetSize(160, 40)
	tab = loadTab(t, tab).(*listDetailTab)

	next, _, _, _ := tab.Update(keyMsg("enter"))
	tab = next.(*listDetailTab)
	view := tab.View(160, 40)
	if !strings.Contains(view, `"Name": "valid"`) || !strings.Contains(view, "\n  ") {
		t.Errorf("valid JSON should be indented: %s", view)
	}
	next, _, _, _ = tab.Update(keyMsg("esc"))
	tab = next.(*listDetailTab)
	next, _, _, _ = tab.Update(keyMsg("down"))
	tab = next.(*listDetailTab)
	next, _, _, _ = tab.Update(keyMsg("enter"))
	tab = next.(*listDetailTab)
	if view := tab.View(160, 40); !strings.Contains(view, "{not json") {
		t.Errorf("invalid JSON should be shown as is: %s", view)
	}
	next, _, _, _ = tab.Update(keyMsg("esc"))
	tab = next.(*listDetailTab)
	next, _, _, _ = tab.Update(keyMsg("down"))
	tab = next.(*listDetailTab)
	next, _, _, _ = tab.Update(keyMsg("enter"))
	tab = next.(*listDetailTab)
	if tab.detail == nil || tab.detail.Text != dash {
		t.Errorf("empty RuleJSON detail = %+v, want —", tab.detail)
	}
}

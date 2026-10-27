package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRDSInstanceParametersTabSingleGroup(t *testing.T) {
	var gotGroups []string
	tab := newRDSInstanceParametersTab(context.Background(), "prof", "region", []string{"default.mysql8"},
		func(_ context.Context, profile, region, group string) ([][]string, error) {
			if profile != "prof" || region != "region" {
				t.Errorf("profile=%q region=%q", profile, region)
			}
			gotGroups = append(gotGroups, group)
			return [][]string{
				{"max_connections", "100", "dynamic", "integer", check, "engine-default"},
				{"empty", dash, "static", "string", dash, "user"},
			}, nil
		})
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*rdsInstanceParametersTab)
	view := tab.View(80, 20)
	if !strings.Contains(view, "Group: default.mysql8") || strings.Contains(view, "(1/1)") {
		t.Errorf("header = %s", view)
	}
	if !strings.Contains(view, "max_connections") || !strings.Contains(view, "100") {
		t.Errorf("rows = %s", view)
	}
	if got := strings.Join(gotGroups, ","); got != "default.mysql8" {
		t.Errorf("groups = %s", got)
	}
}

func TestRDSInstanceParametersTabSwitchingStopsAtEnds(t *testing.T) {
	loads := map[string]int{}
	tab := newRDSInstanceParametersTab(context.Background(), "prof", "region", []string{"g1", "g2"},
		func(_ context.Context, _, _, group string) ([][]string, error) {
			loads[group]++
			return [][]string{{group, "1", "dynamic", "integer", check, "user"}}, nil
		})
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*rdsInstanceParametersTab)
	if view := tab.View(80, 20); !strings.Contains(view, "Group: g1 (1/2)") || !strings.Contains(view, "g1") {
		t.Errorf("initial view = %s", view)
	}

	// 先頭で left を押しても止まる。
	next, cmd, _, handled := tab.Update(keyMsg("left"))
	tab = next.(*rdsInstanceParametersTab)
	if !handled || cmd != nil || tab.index != 0 {
		t.Errorf("left at the first group: handled=%v cmd=%v index=%d", handled, cmd, tab.index)
	}

	next, cmd, _, _ = tab.Update(keyMsg("right"))
	tab = next.(*rdsInstanceParametersTab)
	tab = applyTabCmd(t, tab, cmd).(*rdsInstanceParametersTab)
	if tab.index != 1 {
		t.Fatalf("index = %d, want 1", tab.index)
	}
	if view := tab.View(80, 20); !strings.Contains(view, "Group: g2 (2/2)") || !strings.Contains(view, "g2") {
		t.Errorf("second view = %s", view)
	}

	// 末尾で right を押しても止まる。
	next, cmd, _, _ = tab.Update(keyMsg("right"))
	tab = next.(*rdsInstanceParametersTab)
	if cmd != nil || tab.index != 1 {
		t.Errorf("right at the last group: cmd=%v index=%d", cmd, tab.index)
	}

	next, cmd, _, _ = tab.Update(keyMsg("left"))
	tab = next.(*rdsInstanceParametersTab)
	tab = applyTabCmd(t, tab, cmd).(*rdsInstanceParametersTab)
	if tab.index != 0 || loads["g1"] != 2 {
		t.Errorf("index=%d loads=%v, want back to g1 with a reload", tab.index, loads)
	}

	// r は選択中のグループのまま取り直す。
	next, cmd, _, _ = tab.Update(keyMsg("r"))
	tab = next.(*rdsInstanceParametersTab)
	tab = applyTabCmd(t, tab, cmd).(*rdsInstanceParametersTab)
	if tab.index != 0 || loads["g1"] != 3 {
		t.Errorf("r changed the group: index=%d loads=%v", tab.index, loads)
	}
}

func TestRDSInstanceParametersTabNoGroups(t *testing.T) {
	called := false
	tab := newRDSInstanceParametersTab(context.Background(), "prof", "region", nil,
		func(context.Context, string, string, string) ([][]string, error) {
			called = true
			return nil, nil
		})
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*rdsInstanceParametersTab)
	if called {
		t.Error("load must not be called without parameter groups")
	}
	if cmd := tab.Init(); cmd != nil {
		t.Error("Init must not start a load without parameter groups")
	}
	if view := tab.View(80, 20); !strings.Contains(view, "No parameter groups.") {
		t.Errorf("view = %s", view)
	}
}

func TestRDSInstanceParametersTabEmptyAndError(t *testing.T) {
	empty := newRDSInstanceParametersTab(context.Background(), "prof", "region", []string{"g"},
		func(context.Context, string, string, string) ([][]string, error) { return nil, nil })
	empty.SetSize(80, 20)
	empty = loadTab(t, empty).(*rdsInstanceParametersTab)
	if view := empty.View(80, 20); !strings.Contains(view, "No parameters.") {
		t.Errorf("empty view = %s", view)
	}

	failed := newRDSInstanceParametersTab(context.Background(), "prof", "region", []string{"g"},
		func(context.Context, string, string, string) ([][]string, error) {
			return nil, errors.New("params boom")
		})
	failed.SetSize(80, 20)
	failed = loadTab(t, failed).(*rdsInstanceParametersTab)
	if view := failed.View(80, 20); !strings.Contains(view, "params boom") || !strings.Contains(view, "Group: g") {
		t.Errorf("error view = %s", view)
	}
}

func TestRDSInstanceParametersTabStaleResult(t *testing.T) {
	tab := newRDSInstanceParametersTab(context.Background(), "prof", "region", []string{"g1", "g2"},
		func(_ context.Context, _, _, group string) ([][]string, error) {
			return [][]string{{"param", group, "", "", "", ""}}, nil
		})
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*rdsInstanceParametersTab)
	next, cmd, _, _ := tab.Update(keyMsg("right"))
	tab = next.(*rdsInstanceParametersTab)
	oldSeq := tab.loadSeq
	tab = applyTabCmd(t, tab, cmd).(*rdsInstanceParametersTab)
	// 切り替え前に開始した取得の結果は捨てる。
	tab.Update(rdsParamsLoadedMsg{seq: oldSeq - 1, params: [][]string{{"stale", "old", "", "", "", ""}}})
	if strings.Contains(tab.View(80, 20), "stale") {
		t.Error("stale parameters must be ignored")
	}
}

package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestDynamoItemsTabDefaultSearchAndDetail(t *testing.T) {
	var reqs []aws.DynamoItemQuery
	describe := func(context.Context, string, string, string) (aws.DynamoTableSchema, error) {
		return aws.DynamoTableSchema{
			Table: aws.DynamoIndexSchema{
				PartitionKey: aws.DynamoKeyAttribute{Name: "pk", Type: "S"},
				SortKey:      &aws.DynamoKeyAttribute{Name: "sk", Type: "S"},
			},
		}, nil
	}
	query := func(_ context.Context, _, _, _ string, req aws.DynamoItemQuery) ([]map[string]any, error) {
		reqs = append(reqs, req)
		return []map[string]any{{"pk": "user#1", "sk": "profile", "name": "Alice", "count": 3, "binary": []byte("hi")}}, nil
	}
	tab := newDynamoItemsTab(context.Background(), "prof", "region", "table", describe, query)
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*dynamoItemsTab)
	if len(reqs) != 1 || reqs[0].Limit != 10 {
		t.Fatalf("default requests = %+v", reqs)
	}
	if view := tab.View(80, 20); !strings.Contains(view, "Alice") {
		t.Errorf("view = %s", view)
	}
	if view := tab.View(80, 20); !strings.Contains(view, "Limit: 10") {
		t.Errorf("summary = %s", view)
	}
	if strings.Contains(tab.View(80, 20), `"aGk="`) {
		t.Errorf("[]byte should be unquoted base64: %s", tab.View(80, 20))
	}

	next, _, _, _ := tab.Update(keyMsg("enter"))
	tab = next.(*dynamoItemsTab)
	if !tab.detail || !strings.Contains(tab.View(80, 20), "Alice") {
		t.Errorf("detail = %s", tab.View(80, 20))
	}
	next, _, _, _ = tab.Update(keyMsg("esc"))
	tab = next.(*dynamoItemsTab)
	if tab.detail {
		t.Error("esc should close the item detail")
	}

	next, _, _, _ = tab.Update(keyMsg("f"))
	tab = next.(*dynamoItemsTab)
	if !tab.form {
		t.Fatal("f should open the query form")
	}
	next, _, _, _ = tab.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("user#1")})
	tab = next.(*dynamoItemsTab)
	next, _, _, _ = tab.Update(keyMsg("tab"))
	tab = next.(*dynamoItemsTab)
	next, _, _, _ = tab.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("profile")})
	tab = next.(*dynamoItemsTab)
	next, cmd, _, _ := tab.Update(keyMsg("enter"))
	tab = next.(*dynamoItemsTab)
	tab = applyTabCmd(t, tab, cmd).(*dynamoItemsTab)
	if !tab.form || len(reqs) < 2 {
		t.Fatalf("form=%v requests=%+v", tab.form, reqs)
	}
	last := reqs[len(reqs)-1]
	if last.PKValue != "user#1" || last.SKValue != "profile" || last.Limit != 10 {
		t.Errorf("query = %+v", last)
	}
	// 検索が成功してもフォームは esc で閉じるまで残る。
	if view := tab.View(80, 20); !strings.Contains(view, "Query items") || !strings.Contains(view, "Alice") {
		t.Errorf("form and previous list must remain: %s", view)
	}
	next, _, _, _ = tab.Update(keyMsg("esc"))
	tab = next.(*dynamoItemsTab)
	if tab.form {
		t.Fatal("esc should close the form")
	}

	next, cmd, _, _ = tab.Update(keyMsg("l"))
	tab = next.(*dynamoItemsTab)
	tab = applyTabCmd(t, tab, cmd).(*dynamoItemsTab)
	if tab.submitted.limit != 50 || reqs[len(reqs)-1].Limit != 50 {
		t.Errorf("l should switch limit to 50: submitted=%d req=%+v", tab.submitted.limit, reqs[len(reqs)-1])
	}
}

func TestDynamoItemsTabSchemaFailureDisablesSK(t *testing.T) {
	var reqs []aws.DynamoItemQuery
	tab := newDynamoItemsTab(context.Background(), "prof", "region", "table",
		func(context.Context, string, string, string) (aws.DynamoTableSchema, error) {
			return aws.DynamoTableSchema{}, errors.New("schema boom")
		},
		func(_ context.Context, _, _, _ string, req aws.DynamoItemQuery) ([]map[string]any, error) {
			reqs = append(reqs, req)
			return []map[string]any{{"pk": "user#1"}}, nil
		},
	)
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*dynamoItemsTab)
	if !strings.Contains(tab.View(80, 20), "schema boom") {
		t.Errorf("schema error missing: %s", tab.View(80, 20))
	}
	if !strings.Contains(tab.View(80, 20), "user#1") {
		t.Errorf("items should still load: %s", tab.View(80, 20))
	}

	next, _, _, _ := tab.Update(keyMsg("f"))
	tab = next.(*dynamoItemsTab)
	if view := tab.View(80, 20); !strings.Contains(view, "SK: disabled (schema unavailable)") {
		t.Errorf("SK must be disabled: %s", view)
	}
	next, _, _, _ = tab.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("user#1")})
	tab = next.(*dynamoItemsTab)
	next, _, _, _ = tab.Update(keyMsg("tab"))
	tab = next.(*dynamoItemsTab)
	if tab.focus != dynamoFieldAttrName {
		t.Errorf("focus = %d, want the attribute field (SK is disabled)", tab.focus)
	}
	tab.fields[dynamoFieldSK].SetValue("sk")
	next, cmd, _, _ := tab.Update(keyMsg("enter"))
	tab = next.(*dynamoItemsTab)
	_ = applyTabCmd(t, tab, cmd)
	last := reqs[len(reqs)-1]
	if last.PKValue != "user#1" || last.SKValue != "" {
		t.Errorf("query = %+v, want no SKValue", last)
	}
	if tab.formErr != "" {
		t.Errorf("formErr = %q", tab.formErr)
	}
}

func TestDynamoItemsTabAttrNameOnlyAndNilValues(t *testing.T) {
	var got aws.DynamoItemQuery
	tab := newDynamoItemsTab(context.Background(), "prof", "region", "table",
		func(context.Context, string, string, string) (aws.DynamoTableSchema, error) {
			return aws.DynamoTableSchema{Table: aws.DynamoIndexSchema{PartitionKey: aws.DynamoKeyAttribute{Name: "pk"}}}, nil
		},
		func(_ context.Context, _, _, _ string, req aws.DynamoItemQuery) ([]map[string]any, error) {
			got = req
			return []map[string]any{{"a": nil, "b": map[string]any{"x": "<y>"}}}, nil
		},
	)
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*dynamoItemsTab)
	view := tab.View(80, 20)
	if !strings.Contains(view, "null") || !strings.Contains(view, `"<y>"`) || strings.Contains(view, `\u003c`) {
		t.Errorf("value rendering = %s", view)
	}

	next, _, _, _ := tab.Update(keyMsg("f"))
	tab = next.(*dynamoItemsTab)
	if view := tab.View(80, 20); !strings.Contains(view, "SK: disabled (no sort key)") {
		t.Errorf("SK must be disabled when the table has no sort key: %s", view)
	}
	// ソートキーが無いので、PK の次は属性名へ移る。
	next, _, _, _ = tab.Update(keyMsg("tab"))
	tab = next.(*dynamoItemsTab)
	if tab.focus != dynamoFieldAttrName {
		t.Errorf("focus = %d, want the attribute field", tab.focus)
	}
	next, _, _, _ = tab.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("name")})
	tab = next.(*dynamoItemsTab)
	next, cmd, _, _ := tab.Update(keyMsg("enter"))
	tab = next.(*dynamoItemsTab)
	_ = applyTabCmd(t, tab, cmd)
	if got.AttrName != "name" || got.AttrValue != "" {
		t.Errorf("attr query = %+v", got)
	}
}

func TestDynamoItemsTabPKEmptyDoesNotSendSK(t *testing.T) {
	var reqs []aws.DynamoItemQuery
	tab := newDynamoItemsTab(context.Background(), "prof", "region", "table",
		func(context.Context, string, string, string) (aws.DynamoTableSchema, error) {
			return aws.DynamoTableSchema{Table: aws.DynamoIndexSchema{
				PartitionKey: aws.DynamoKeyAttribute{Name: "pk"},
				SortKey:      &aws.DynamoKeyAttribute{Name: "sk"},
			}}, nil
		},
		func(_ context.Context, _, _, _ string, req aws.DynamoItemQuery) ([]map[string]any, error) {
			reqs = append(reqs, req)
			return nil, nil
		},
	)
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*dynamoItemsTab)
	next, _, _, _ := tab.Update(keyMsg("f"))
	tab = next.(*dynamoItemsTab)
	next, _, _, _ = tab.Update(keyMsg("tab"))
	tab = next.(*dynamoItemsTab)
	next, _, _, _ = tab.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("sk-only")})
	tab = next.(*dynamoItemsTab)
	next, cmd, _, _ := tab.Update(keyMsg("enter"))
	tab = next.(*dynamoItemsTab)
	_ = applyTabCmd(t, tab, cmd)
	last := reqs[len(reqs)-1]
	if last.PKValue != "" || last.SKValue != "" {
		t.Errorf("query = %+v, want no PK and no SK", last)
	}
}

func TestDynamoItemsTabLimitFieldSwitching(t *testing.T) {
	tab := newDynamoItemsTab(context.Background(), "prof", "region", "table",
		func(context.Context, string, string, string) (aws.DynamoTableSchema, error) {
			return aws.DynamoTableSchema{Table: aws.DynamoIndexSchema{
				PartitionKey: aws.DynamoKeyAttribute{Name: "pk"},
				SortKey:      &aws.DynamoKeyAttribute{Name: "sk"},
			}}, nil
		},
		func(context.Context, string, string, string, aws.DynamoItemQuery) ([]map[string]any, error) {
			return nil, nil
		},
	)
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*dynamoItemsTab)
	next, _, _, _ := tab.Update(keyMsg("f"))
	tab = next.(*dynamoItemsTab)
	for i := 0; i < 4; i++ {
		next, _, _, _ = tab.Update(keyMsg("tab"))
		tab = next.(*dynamoItemsTab)
	}
	if tab.focus != dynamoFieldLimit {
		t.Fatalf("focus = %d, want the limit field", tab.focus)
	}
	if got := tab.fields[dynamoFieldLimit].Value(); got != "10" {
		t.Fatalf("limit = %q", got)
	}
	next, _, _, _ = tab.Update(keyMsg("left"))
	tab = next.(*dynamoItemsTab)
	if got := tab.fields[dynamoFieldLimit].Value(); got != "10" {
		t.Errorf("left at 10 = %q, want 10", got)
	}
	next, _, _, _ = tab.Update(keyMsg("right"))
	tab = next.(*dynamoItemsTab)
	if got := tab.fields[dynamoFieldLimit].Value(); got != "50" {
		t.Errorf("right = %q, want 50", got)
	}
	next, _, _, _ = tab.Update(keyMsg("l"))
	tab = next.(*dynamoItemsTab)
	if got := tab.fields[dynamoFieldLimit].Value(); got != "100" {
		t.Errorf("l = %q, want 100", got)
	}
	next, _, _, _ = tab.Update(keyMsg("right"))
	tab = next.(*dynamoItemsTab)
	if got := tab.fields[dynamoFieldLimit].Value(); got != "100" {
		t.Errorf("right at 100 = %q, want 100", got)
	}
	next, _, _, _ = tab.Update(keyMsg("left"))
	tab = next.(*dynamoItemsTab)
	if got := tab.fields[dynamoFieldLimit].Value(); got != "50" {
		t.Errorf("left = %q, want 50", got)
	}
}

func TestDynamoItemsTabFormInputIsolation(t *testing.T) {
	tab := newDynamoItemsTab(context.Background(), "prof", "region", "table",
		func(context.Context, string, string, string) (aws.DynamoTableSchema, error) {
			return aws.DynamoTableSchema{}, nil
		},
		func(context.Context, string, string, string, aws.DynamoItemQuery) ([]map[string]any, error) {
			return nil, nil
		},
	)
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*dynamoItemsTab)
	next, _, _, _ := tab.Update(keyMsg("f"))
	tab = next.(*dynamoItemsTab)
	for _, key := range []string{"l", "c", "f", "r", "q", "j", "k", "1", "2", "3"} {
		next, _, _, handled := tab.Update(keyMsg(key))
		if !handled {
			t.Fatalf("%q should be handled by the form", key)
		}
		tab = next.(*dynamoItemsTab)
	}
	if got := tab.fields[dynamoFieldPK].Value(); got != "lcfrqjk123" {
		t.Errorf("PK input = %q", got)
	}
}

func TestDynamoItemsTabFailureKeepsFormAndPreviousItems(t *testing.T) {
	fail := false
	tab := newDynamoItemsTab(context.Background(), "prof", "region", "table",
		func(context.Context, string, string, string) (aws.DynamoTableSchema, error) {
			return aws.DynamoTableSchema{Table: aws.DynamoIndexSchema{PartitionKey: aws.DynamoKeyAttribute{Name: "pk"}}}, nil
		},
		func(context.Context, string, string, string, aws.DynamoItemQuery) ([]map[string]any, error) {
			if fail {
				return nil, errors.New("query boom")
			}
			return []map[string]any{{"pk": "user#1"}}, nil
		},
	)
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*dynamoItemsTab)
	fail = true
	next, _, _, _ := tab.Update(keyMsg("f"))
	tab = next.(*dynamoItemsTab)
	next, cmd, _, _ := tab.Update(keyMsg("enter"))
	tab = next.(*dynamoItemsTab)
	tab = applyTabCmd(t, tab, cmd).(*dynamoItemsTab)
	if !tab.form {
		t.Error("the form must remain after a failed search")
	}
	view := tab.View(80, 20)
	if !strings.Contains(view, "query boom") || !strings.Contains(view, "user#1") {
		t.Errorf("failure must keep the previous list and show the reason: %s", view)
	}
}

func TestDynamoItemsTabInvalidLimit(t *testing.T) {
	tab := newDynamoItemsTab(context.Background(), "prof", "region", "table",
		func(context.Context, string, string, string) (aws.DynamoTableSchema, error) {
			return aws.DynamoTableSchema{}, nil
		},
		func(context.Context, string, string, string, aws.DynamoItemQuery) ([]map[string]any, error) {
			return nil, nil
		},
	)
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*dynamoItemsTab)
	next, _, _, _ := tab.Update(keyMsg("f"))
	tab = next.(*dynamoItemsTab)
	tab.fields[dynamoFieldLimit].SetValue("25")
	next, cmd, _, _ := tab.Update(keyMsg("enter"))
	tab = next.(*dynamoItemsTab)
	if cmd != nil {
		t.Error("invalid limit must not start a query")
	}
	if !strings.Contains(tab.View(80, 20), "limit must be 10, 50, or 100") {
		t.Errorf("form error missing: %s", tab.View(80, 20))
	}
}

func TestModel_DynamoFormConsumesGlobalKeys(t *testing.T) {
	queries := 0
	svc := NewDynamoServiceWithTabs(nil,
		func(context.Context, string, string, string) (aws.DynamoTableSchema, error) {
			return aws.DynamoTableSchema{Table: aws.DynamoIndexSchema{PartitionKey: aws.DynamoKeyAttribute{Name: "pk"}}}, nil
		},
		func(context.Context, string, string, string, aws.DynamoItemQuery) ([]map[string]any, error) {
			queries++
			return nil, nil
		},
	)
	m := newTestModel(t, svc, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: []Row{dynamoRow{res: aws.DynamoResource{ID: "table", Name: "table"}}}})
	m, cmd := updateCmd(t, m, keyMsg("2"))
	m = apply(t, m, cmd)
	m = keys(t, m, "tab")

	before := queries
	m = keys(t, m, "f", "r", "q", "l", "1", "c")
	tab, ok := m.activeTab.(*dynamoItemsTab)
	if !ok || !tab.form {
		t.Fatalf("activeTab = %#v, want an open form", m.activeTab)
	}
	if got := tab.fields[dynamoFieldPK].Value(); got != "rql1c" {
		t.Errorf("PK input = %q, want rql1c", got)
	}
	if queries != before {
		t.Errorf("queries = %d, want no reload while typing (before %d)", queries, before)
	}
}

func TestDynamoItemsTabRKeepsQuery(t *testing.T) {
	var reqs []aws.DynamoItemQuery
	tab := newDynamoItemsTab(context.Background(), "prof", "region", "table",
		func(context.Context, string, string, string) (aws.DynamoTableSchema, error) {
			return aws.DynamoTableSchema{Table: aws.DynamoIndexSchema{
				PartitionKey: aws.DynamoKeyAttribute{Name: "pk"},
				SortKey:      &aws.DynamoKeyAttribute{Name: "sk"},
			}}, nil
		},
		func(_ context.Context, _, _, _ string, req aws.DynamoItemQuery) ([]map[string]any, error) {
			reqs = append(reqs, req)
			return nil, nil
		},
	)
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*dynamoItemsTab)
	next, _, _, _ := tab.Update(keyMsg("f"))
	tab = next.(*dynamoItemsTab)
	next, _, _, _ = tab.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("user#1")})
	tab = next.(*dynamoItemsTab)
	next, _, _, _ = tab.Update(keyMsg("tab"))
	tab = next.(*dynamoItemsTab)
	next, _, _, _ = tab.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("profile")})
	tab = next.(*dynamoItemsTab)
	next, cmd, _, _ := tab.Update(keyMsg("enter"))
	tab = next.(*dynamoItemsTab)
	tab = applyTabCmd(t, tab, cmd).(*dynamoItemsTab)
	next, _, _, _ = tab.Update(keyMsg("esc"))
	tab = next.(*dynamoItemsTab)

	before := len(reqs)
	next, cmd, _, handled := tab.Update(keyMsg("r"))
	tab = next.(*dynamoItemsTab)
	if !handled || cmd == nil {
		t.Fatal("r should reload with the current query")
	}
	tab = applyTabCmd(t, tab, cmd).(*dynamoItemsTab)
	if len(reqs) != before+1 {
		t.Fatalf("requests = %d, want %d", len(reqs), before+1)
	}
	last := reqs[len(reqs)-1]
	if last.PKValue != "user#1" || last.SKValue != "profile" || last.Limit != 10 {
		t.Errorf("query after r = %+v", last)
	}
	if tab.submitted.pk != "user#1" || tab.submitted.sk != "profile" {
		t.Errorf("submitted = %+v", tab.submitted)
	}
}

func TestDynamoItemsTabColumnsUnionAndValues(t *testing.T) {
	tab := newDynamoItemsTab(context.Background(), "prof", "region", "table",
		func(context.Context, string, string, string) (aws.DynamoTableSchema, error) {
			return aws.DynamoTableSchema{}, nil
		},
		func(context.Context, string, string, string, aws.DynamoItemQuery) ([]map[string]any, error) {
			return []map[string]any{
				{"b": "two", "a": nil},
				{"a": "one", "c": "three", "bin": []byte("hi")},
			}, nil
		},
	)
	tab.SetSize(200, 20)
	tab = loadTab(t, tab).(*dynamoItemsTab)
	want := []string{"a", "b", "bin", "c"}
	if len(tab.columns) != len(want) {
		t.Fatalf("columns = %v, want %v", tab.columns, want)
	}
	for i := range want {
		if tab.columns[i] != want[i] {
			t.Errorf("columns = %v, want %v", tab.columns, want)
			break
		}
	}
	view := tab.View(200, 20)
	for _, s := range []string{"null", "one", "two", "three", "aGk="} {
		if !strings.Contains(view, s) {
			t.Errorf("view missing %q\n%s", s, view)
		}
	}
	if strings.Contains(view, `"aGk="`) {
		t.Errorf("[]byte must be unquoted base64: %s", view)
	}
}

func TestDynamoItemsTabSchemaFailureAfterSuccess(t *testing.T) {
	fail := false
	tab := newDynamoItemsTab(context.Background(), "prof", "region", "table",
		func(context.Context, string, string, string) (aws.DynamoTableSchema, error) {
			if fail {
				return aws.DynamoTableSchema{}, errors.New("schema boom")
			}
			return aws.DynamoTableSchema{Table: aws.DynamoIndexSchema{
				PartitionKey: aws.DynamoKeyAttribute{Name: "pk"},
				SortKey:      &aws.DynamoKeyAttribute{Name: "sk"},
			}}, nil
		},
		func(context.Context, string, string, string, aws.DynamoItemQuery) ([]map[string]any, error) {
			return nil, nil
		},
	)
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*dynamoItemsTab)
	fail = true
	next, cmd, _, _ := tab.Update(keyMsg("r"))
	tab = next.(*dynamoItemsTab)
	tab = applyTabCmd(t, tab, cmd).(*dynamoItemsTab)
	next, _, _, _ = tab.Update(keyMsg("f"))
	tab = next.(*dynamoItemsTab)
	view := tab.View(80, 20)
	if !strings.Contains(view, "SK: disabled (schema unavailable)") {
		t.Errorf("SK should be disabled after a later schema failure:\n%s", view)
	}
	if !strings.Contains(view, "PK: ") || strings.Contains(view, "pk: ") {
		t.Errorf("PK label should fall back to PK:\n%s", view)
	}
}

func TestDynamoItemsTabReopenKeepsSubmittedValues(t *testing.T) {
	tab := newDynamoItemsTab(context.Background(), "prof", "region", "table",
		func(context.Context, string, string, string) (aws.DynamoTableSchema, error) {
			return aws.DynamoTableSchema{Table: aws.DynamoIndexSchema{PartitionKey: aws.DynamoKeyAttribute{Name: "pk"}}}, nil
		},
		func(context.Context, string, string, string, aws.DynamoItemQuery) ([]map[string]any, error) {
			return nil, nil
		},
	)
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*dynamoItemsTab)
	next, _, _, _ := tab.Update(keyMsg("f"))
	tab = next.(*dynamoItemsTab)
	next, _, _, _ = tab.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("user#1")})
	tab = next.(*dynamoItemsTab)
	next, cmd, _, _ := tab.Update(keyMsg("enter"))
	tab = next.(*dynamoItemsTab)
	tab = applyTabCmd(t, tab, cmd).(*dynamoItemsTab)
	next, _, _, _ = tab.Update(keyMsg("esc"))
	tab = next.(*dynamoItemsTab)
	next, _, _, _ = tab.Update(keyMsg("f"))
	tab = next.(*dynamoItemsTab)
	if got := tab.fields[dynamoFieldPK].Value(); got != "user#1" {
		t.Errorf("reopened PK = %q, want user#1", got)
	}
}

func TestDynamoItemsTabEmptyAndStale(t *testing.T) {
	tab := newDynamoItemsTab(context.Background(), "prof", "region", "table",
		func(context.Context, string, string, string) (aws.DynamoTableSchema, error) {
			return aws.DynamoTableSchema{}, nil
		},
		func(context.Context, string, string, string, aws.DynamoItemQuery) ([]map[string]any, error) {
			return nil, nil
		},
	)
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*dynamoItemsTab)
	if view := tab.View(80, 20); !strings.Contains(view, "No items") {
		t.Errorf("empty view = %s", view)
	}
	tab.Update(dynamoLoadedMsg{seq: tab.loadSeq + 1, items: []map[string]any{{"stale": "value"}}})
	if strings.Contains(tab.View(80, 20), "stale") {
		t.Error("stale items must be ignored")
	}
}

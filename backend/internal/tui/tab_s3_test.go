package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestS3ObjectsTabFolderNavigationSearchAndMode(t *testing.T) {
	var calls []string
	list := func(_ context.Context, _, _, _, prefix, delimiter string) ([]aws.S3ObjectResource, []string, bool, error) {
		calls = append(calls, prefix+"|"+delimiter)
		switch prefix {
		case "":
			return []aws.S3ObjectResource{{Key: "notes/sample.txt", Size: 12}}, []string{"reports/", "config/"}, false, nil
		case "reports/":
			return []aws.S3ObjectResource{{Key: "reports/sample.csv", Size: 1024}}, nil, true, nil
		default:
			return nil, nil, false, nil
		}
	}
	tab := newS3ObjectsTab(context.Background(), "prof", "region", "bucket", "bucket-region", list)
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*s3ObjectsTab)
	if calls[0] != "|/" {
		t.Errorf("initial call = %q, want |/", calls[0])
	}
	if view := tab.View(80, 20); !strings.Contains(view, "reports/") {
		t.Errorf("initial view = %s", view)
	}

	// config/ が先頭なので、reports/ まで下へ移動する。
	next, _, _, _ := tab.Update(keyMsg("down"))
	tab = next.(*s3ObjectsTab)
	next, cmd, _, _ := tab.Update(keyMsg("enter"))
	tab = next.(*s3ObjectsTab)
	tab = applyTabCmd(t, tab, cmd).(*s3ObjectsTab)
	if tab.currentPrefix != "reports/" {
		t.Errorf("currentPrefix = %q, want reports/", tab.currentPrefix)
	}
	if view := tab.View(80, 20); !strings.Contains(view, "Showing the first 1000") {
		t.Errorf("truncated note missing: %s", view)
	}

	next, cmd, _, _ = tab.Update(keyMsg("esc"))
	tab = next.(*s3ObjectsTab)
	tab = applyTabCmd(t, tab, cmd).(*s3ObjectsTab)
	if tab.currentPrefix != "" {
		t.Errorf("esc should go to the parent, currentPrefix = %q", tab.currentPrefix)
	}

	// 検索入力中の r はグローバル再取得ではなく入力として扱う。
	next, _, _, _ = tab.Update(keyMsg("f"))
	tab = next.(*s3ObjectsTab)
	for _, key := range []string{"r", "e", "p", "o", "r", "t", "s"} {
		next, _, _, handled := tab.Update(keyMsg(key))
		if !handled {
			t.Fatalf("%q should be handled by the search input", key)
		}
		tab = next.(*s3ObjectsTab)
	}
	next, cmd, _, _ = tab.Update(keyMsg("enter"))
	tab = next.(*s3ObjectsTab)
	tab = applyTabCmd(t, tab, cmd).(*s3ObjectsTab)
	if tab.searchPrefix != "reports" {
		t.Errorf("searchPrefix = %q, want reports", tab.searchPrefix)
	}
	if !containsCall(calls, "reports|/") {
		t.Errorf("search calls = %v, want reports|/", calls)
	}

	// f で開き直すと確定済みの prefix が初期値になる。検索入力中の tab と q は
	// タブ切替や終了ではなく入力欄が消費する。
	next, _, _, _ = tab.Update(keyMsg("f"))
	tab = next.(*s3ObjectsTab)
	if got := tab.searchInput.Value(); got != "reports" {
		t.Errorf("reopened search value = %q, want reports", got)
	}
	before := len(calls)
	next, _, _, handled := tab.Update(keyMsg("tab"))
	tab = next.(*s3ObjectsTab)
	if !handled || len(calls) != before {
		t.Errorf("tab in search: handled=%v calls=%v", handled, calls)
	}
	next, _, _, _ = tab.Update(keyMsg("q"))
	tab = next.(*s3ObjectsTab)
	if got := tab.searchInput.Value(); got != "reportsq" {
		t.Errorf("q in search = %q, want reportsq", got)
	}
	next, _, _, _ = tab.Update(keyMsg("esc"))
	tab = next.(*s3ObjectsTab)
	if tab.searching || tab.searchPrefix != "reports" {
		t.Errorf("esc after reopening: searching=%v prefix=%q", tab.searching, tab.searchPrefix)
	}

	next, cmd, _, _ = tab.Update(keyMsg("m"))
	tab = next.(*s3ObjectsTab)
	tab = applyTabCmd(t, tab, cmd).(*s3ObjectsTab)
	if !tab.flat || !containsCall(calls, "reports|") {
		t.Errorf("flat=%v calls=%v, want flat mode with empty delimiter", tab.flat, calls)
	}

	// r は現在の prefix と表示モードのまま取り直す。
	next, cmd, _, _ = tab.Update(keyMsg("r"))
	tab = next.(*s3ObjectsTab)
	tab = applyTabCmd(t, tab, cmd).(*s3ObjectsTab)
	if tab.searchPrefix != "reports" || !tab.flat {
		t.Errorf("r changed the search state: prefix=%q flat=%v", tab.searchPrefix, tab.flat)
	}
}

func TestS3ObjectsTabSearchEscKeepsPrefix(t *testing.T) {
	tab := newS3ObjectsTab(context.Background(), "prof", "region", "bucket", "region", func(context.Context, string, string, string, string, string) ([]aws.S3ObjectResource, []string, bool, error) {
		return nil, nil, false, nil
	})
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*s3ObjectsTab)
	next, _, _, _ := tab.Update(keyMsg("f"))
	tab = next.(*s3ObjectsTab)
	next, _, _, _ = tab.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("draft")})
	tab = next.(*s3ObjectsTab)
	next, _, _, _ = tab.Update(keyMsg("esc"))
	tab = next.(*s3ObjectsTab)
	if tab.searching || tab.searchPrefix != "" {
		t.Errorf("esc should discard the search input: searching=%v prefix=%q", tab.searching, tab.searchPrefix)
	}
}

func TestS3ObjectsTabRootBackspaceDoesNothing(t *testing.T) {
	calls := 0
	tab := newS3ObjectsTab(context.Background(), "prof", "region", "bucket", "region", func(context.Context, string, string, string, string, string) ([]aws.S3ObjectResource, []string, bool, error) {
		calls++
		return nil, nil, false, nil
	})
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*s3ObjectsTab)
	before := calls
	next, cmd, _, handled := tab.Update(keyMsg("backspace"))
	tab = next.(*s3ObjectsTab)
	if !handled || cmd != nil || calls != before || tab.currentPrefix != "" {
		t.Errorf("backspace at root: handled=%v cmd=%v calls=%d prefix=%q", handled, cmd, calls, tab.currentPrefix)
	}
	next, cmd, _, handled = tab.Update(keyMsg("esc"))
	tab = next.(*s3ObjectsTab)
	if !handled || cmd != nil || calls != before || tab.currentPrefix != "" {
		t.Errorf("esc at root: handled=%v cmd=%v calls=%d prefix=%q", handled, cmd, calls, tab.currentPrefix)
	}
}

func TestS3ObjectsTabEmptyState(t *testing.T) {
	tab := newS3ObjectsTab(context.Background(), "prof", "region", "bucket", "region", func(context.Context, string, string, string, string, string) ([]aws.S3ObjectResource, []string, bool, error) {
		return nil, nil, false, nil
	})
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*s3ObjectsTab)
	if view := tab.View(80, 20); !strings.Contains(view, "No objects") {
		t.Errorf("empty view = %s", view)
	}
}

func TestS3ServiceObjectsTabUsesBucketRegion(t *testing.T) {
	var gotProfile, gotRegion, gotBucket string
	svc := NewS3Service(nil, func(_ context.Context, profile, region, bucket, _, _ string) ([]aws.S3ObjectResource, []string, bool, error) {
		gotProfile, gotRegion, gotBucket = profile, region, bucket
		return nil, nil, false, nil
	}).(TabProvider)
	row := s3Row{
		res:    aws.S3Resource{ID: "bucket", Name: "bucket", Region: "us-east-1"},
		region: "ap-northeast-1",
	}
	tab := newTabForTest(t, svc.Tabs(), "objects", row).(*s3ObjectsTab)
	tab.SetSize(80, 20)
	_ = loadTab(t, tab)
	if gotProfile != "prof" || gotRegion != "us-east-1" || gotBucket != "bucket" {
		t.Errorf("args = profile %q region %q bucket %q", gotProfile, gotRegion, gotBucket)
	}
}

func TestS3ObjectsTabParentUsesCombinedPrefix(t *testing.T) {
	var calls []string
	tab := newS3ObjectsTab(context.Background(), "prof", "region", "bucket", "region", func(_ context.Context, _, _, _, prefix, delimiter string) ([]aws.S3ObjectResource, []string, bool, error) {
		calls = append(calls, prefix+"|"+delimiter)
		return nil, nil, false, nil
	})
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*s3ObjectsTab)
	tab.currentPrefix = "reports/"
	tab.searchPrefix = "2026/"
	next, cmd, _, handled := tab.Update(keyMsg("esc"))
	tab = next.(*s3ObjectsTab)
	if !handled || cmd == nil {
		t.Fatal("esc should move to the parent")
	}
	tab = applyTabCmd(t, tab, cmd).(*s3ObjectsTab)
	if tab.currentPrefix != "reports/" || tab.searchPrefix != "" {
		t.Errorf("prefix = %q search = %q, want reports/ and empty", tab.currentPrefix, tab.searchPrefix)
	}
	if !containsCall(calls, "reports/|/") {
		t.Errorf("calls = %v, want reports/|/", calls)
	}
}

func TestS3ObjectsTabSubfolderEmptyAndDotDot(t *testing.T) {
	tab := newS3ObjectsTab(context.Background(), "prof", "region", "bucket", "region", func(_ context.Context, _, _, _, prefix, _ string) ([]aws.S3ObjectResource, []string, bool, error) {
		if prefix == "" {
			return nil, []string{"empty/"}, false, nil
		}
		return nil, nil, false, nil
	})
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*s3ObjectsTab)
	if strings.Contains(tab.View(80, 20), "..") {
		t.Errorf("root must not show ..:\n%s", tab.View(80, 20))
	}
	next, cmd, _, _ := tab.Update(keyMsg("enter"))
	tab = next.(*s3ObjectsTab)
	tab = applyTabCmd(t, tab, cmd).(*s3ObjectsTab)
	view := tab.View(80, 20)
	if !strings.Contains(view, "..") || !strings.Contains(view, "No objects") {
		t.Errorf("empty subfolder view = %s", view)
	}
	// 先頭の .. 行でルートへ戻る。
	next, cmd, _, _ = tab.Update(keyMsg("enter"))
	tab = next.(*s3ObjectsTab)
	tab = applyTabCmd(t, tab, cmd).(*s3ObjectsTab)
	if tab.currentPrefix != "" {
		t.Errorf(".. row should go back to the root, prefix = %q", tab.currentPrefix)
	}
	// もう一度入って backspace でも親へ戻る。
	next, cmd, _, _ = tab.Update(keyMsg("enter"))
	tab = next.(*s3ObjectsTab)
	tab = applyTabCmd(t, tab, cmd).(*s3ObjectsTab)
	next, cmd, _, _ = tab.Update(keyMsg("backspace"))
	tab = next.(*s3ObjectsTab)
	tab = applyTabCmd(t, tab, cmd).(*s3ObjectsTab)
	if tab.currentPrefix != "" {
		t.Errorf("backspace should go back to the root, prefix = %q", tab.currentPrefix)
	}
}

func TestS3ObjectsTabSearchInputConsumesModeAndBackspace(t *testing.T) {
	tab := newS3ObjectsTab(context.Background(), "prof", "region", "bucket", "region", func(context.Context, string, string, string, string, string) ([]aws.S3ObjectResource, []string, bool, error) {
		return nil, nil, false, nil
	})
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*s3ObjectsTab)
	tab.currentPrefix = "a/"
	next, _, _, _ := tab.Update(keyMsg("f"))
	tab = next.(*s3ObjectsTab)
	next, _, _, _ = tab.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("xy")})
	tab = next.(*s3ObjectsTab)
	next, _, _, _ = tab.Update(keyMsg("m"))
	tab = next.(*s3ObjectsTab)
	if tab.flat {
		t.Error("m in the search input must not toggle the mode")
	}
	if tab.searchInput.Value() != "xym" {
		t.Errorf("search value = %q, want xym", tab.searchInput.Value())
	}
	next, _, _, _ = tab.Update(keyMsg("backspace"))
	tab = next.(*s3ObjectsTab)
	if tab.currentPrefix != "a/" || tab.searchInput.Value() != "xy" {
		t.Errorf("backspace in search: prefix=%q value=%q", tab.currentPrefix, tab.searchInput.Value())
	}
}

func TestS3PrefixHelpers(t *testing.T) {
	if got := normalizeS3Prefix("  //reports/2026  "); got != "reports/2026" {
		t.Errorf("normalizeS3Prefix = %q", got)
	}
	tests := []struct{ in, want string }{
		{"", ""},
		{"a/", ""},
		{"a/b/", "a/"},
		{"a/b", "a/"},
		{"a", ""},
	}
	for _, tt := range tests {
		if got := parentS3Prefix(tt.in); got != tt.want {
			t.Errorf("parentS3Prefix(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestModel_S3SearchConsumesGlobalKeys(t *testing.T) {
	listCalls := 0
	svc := NewS3Service(nil, func(context.Context, string, string, string, string, string) ([]aws.S3ObjectResource, []string, bool, error) {
		listCalls++
		return []aws.S3ObjectResource{{Key: "a.txt"}}, nil, false, nil
	})
	m := newTestModel(t, svc, Deps{})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: []Row{s3Row{res: aws.S3Resource{ID: "bucket", Name: "bucket"}}}})
	m, cmd := updateCmd(t, m, keyMsg("2"))
	m = apply(t, m, cmd)
	m = keys(t, m, "tab")

	before := listCalls
	m = keys(t, m, "f", "r", "q", "s", "p", "R", "S", "[", "]", "/", "tab")
	if m.tabID != "objects" {
		t.Errorf("tabID = %q, want objects", m.tabID)
	}
	tab, ok := m.activeTab.(*s3ObjectsTab)
	if !ok || !tab.searching || tab.searchInput.Value() != "rqspRS[]/" {
		t.Fatalf("activeTab = %#v, want an open search with rqspRS[]/", m.activeTab)
	}
	if listCalls != before {
		t.Errorf("list calls = %d, want no reload while typing (before %d)", listCalls, before)
	}

	_, quit := updateCmd(t, m, keyMsg("ctrl+c"))
	msgs := drain(t, quit)
	if len(msgs) != 1 {
		t.Fatalf("msgs = %v, want QuitMsg", msgs)
	}
	if _, ok := msgs[0].(tea.QuitMsg); !ok {
		t.Errorf("msg = %#v, want QuitMsg", msgs[0])
	}
}

func TestS3ObjectsTabErrorAndStale(t *testing.T) {
	tab := newS3ObjectsTab(context.Background(), "prof", "region", "bucket", "region", func(context.Context, string, string, string, string, string) ([]aws.S3ObjectResource, []string, bool, error) {
		return nil, nil, false, errors.New("boom")
	})
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*s3ObjectsTab)
	if view := tab.View(80, 20); !strings.Contains(view, "boom") {
		t.Errorf("error view = %s", view)
	}

	tab = newS3ObjectsTab(context.Background(), "prof", "region", "bucket", "region", func(context.Context, string, string, string, string, string) ([]aws.S3ObjectResource, []string, bool, error) {
		return []aws.S3ObjectResource{{Key: "a"}}, nil, false, nil
	})
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*s3ObjectsTab)
	tab.Update(s3LoadedMsg{seq: tab.loadSeq + 1, objects: []aws.S3ObjectResource{{Key: "stale"}}})
	if strings.Contains(tab.View(80, 20), "stale") {
		t.Error("stale S3 result must be ignored")
	}
}

func containsCall(calls []string, want string) bool {
	for _, call := range calls {
		if call == want {
			return true
		}
	}
	return false
}

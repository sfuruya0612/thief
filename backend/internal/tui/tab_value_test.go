package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

func TestValueTabMasksAndReveals(t *testing.T) {
	var loads int
	tab := newValueTab(context.Background(),
		[]KV{{Key: "Name", Value: "secret"}},
		true,
		func(context.Context) (string, error) {
			loads++
			return "super-secret", nil
		},
	)
	tab.SetSize(80, 20)
	if cmd := tab.Init(); cmd != nil {
		t.Error("masked value must not load on Init")
	}
	if view := tab.View(80, 20); !strings.Contains(view, "Press v to reveal") {
		t.Errorf("initial view = %s", view)
	}

	next, cmd, _, handled := tab.Update(keyMsg("v"))
	if !handled || cmd == nil {
		t.Fatal("v should start loading")
	}
	tab = applyTabCmd(t, next, cmd).(*valueTab)
	if loads != 1 || !strings.Contains(tab.View(80, 20), "super-secret") {
		t.Errorf("loads=%d view=%s", loads, tab.View(80, 20))
	}
	next, _, _, _ = tab.Update(keyMsg("v"))
	tab = next.(*valueTab)
	if strings.Contains(tab.View(80, 20), "super-secret") || !strings.Contains(tab.View(80, 20), "Hidden; press v to reveal") {
		t.Errorf("hidden view = %s", tab.View(80, 20))
	}
	next, _, _, _ = tab.Update(keyMsg("v"))
	tab = next.(*valueTab)
	if loads != 1 || !strings.Contains(tab.View(80, 20), "super-secret") {
		t.Errorf("second reveal loads=%d view=%s", loads, tab.View(80, 20))
	}
}

func TestValueTabStringLoadsOnInitAndError(t *testing.T) {
	var loads int
	tab := newValueTab(context.Background(), []KV{{Key: "Type", Value: "String"}}, false, func(context.Context) (string, error) {
		loads++
		return "plain", nil
	})
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*valueTab)
	if loads != 1 || !strings.Contains(tab.View(80, 20), "plain") {
		t.Errorf("loads=%d view=%s", loads, tab.View(80, 20))
	}
	next, _, _, handled := tab.Update(keyMsg("v"))
	if !handled {
		t.Error("v on String should be handled as a no-op")
	}
	tab = next.(*valueTab)
	if !strings.Contains(tab.View(80, 20), "plain") {
		t.Errorf("v changed String view = %s", tab.View(80, 20))
	}

	failed := newValueTab(context.Background(), nil, false, func(context.Context) (string, error) {
		return "", errors.New("boom")
	})
	failed.SetSize(80, 20)
	failed = loadTab(t, failed).(*valueTab)
	if view := failed.View(80, 20); !strings.Contains(view, "boom") {
		t.Errorf("error view = %s", view)
	}
}

func TestValueTabEmptyAndScroll(t *testing.T) {
	empty := newValueTab(context.Background(), nil, false, func(context.Context) (string, error) { return "", nil })
	empty.SetSize(80, 20)
	empty = loadTab(t, empty).(*valueTab)
	if view := empty.View(80, 20); !strings.Contains(view, "Value is empty") {
		t.Errorf("empty view = %s", view)
	}

	long := newValueTab(context.Background(), nil, false, func(context.Context) (string, error) {
		return strings.Join([]string{"line1", "line2", "line3", "line4", "line5", "line6"}, "\n"), nil
	})
	long.SetSize(20, 4)
	long = loadTab(t, long).(*valueTab)
	before := long.viewport.YOffset
	next, _, _, _ := long.Update(keyMsg("down"))
	long = next.(*valueTab)
	if long.viewport.YOffset <= before {
		t.Errorf("scroll offset = %d, want greater than %d", long.viewport.YOffset, before)
	}
}

func TestValueTabStaleAndErrorDoNotKeepValue(t *testing.T) {
	tab := newValueTab(context.Background(), nil, false, func(context.Context) (string, error) {
		return "first", nil
	})
	tab.SetSize(80, 20)
	tab = loadTab(t, tab).(*valueTab)
	tab.Update(valueLoadedMsg{seq: tab.loadSeq + 1, value: "stale"})
	if view := tab.View(80, 20); strings.Contains(view, "stale") || !strings.Contains(view, "first") {
		t.Errorf("stale view = %s", view)
	}

	failed := newValueTab(context.Background(), nil, true, func(context.Context) (string, error) {
		return "leaked-secret", errors.New("boom")
	})
	failed.SetSize(80, 20)
	next, cmd, _, _ := failed.Update(keyMsg("v"))
	failed = applyTabCmd(t, next, cmd).(*valueTab)
	if failed.value != "" {
		t.Errorf("value must be discarded on error: %q", failed.value)
	}
	if view := failed.View(80, 20); !strings.Contains(view, "boom") || strings.Contains(view, "leaked-secret") {
		t.Errorf("error view = %s", view)
	}
}

func TestValueTabViewFitsHeight(t *testing.T) {
	tab := newValueTab(context.Background(),
		[]KV{{Key: "Name", Value: "secret"}, {Key: "Description", Value: "demo"}},
		true,
		func(context.Context) (string, error) { return "super-secret", nil },
	)
	tab.SetSize(20, 10)
	next, cmd, _, _ := tab.Update(keyMsg("v"))
	tab = applyTabCmd(t, next, cmd).(*valueTab)
	view := tab.View(20, 10)
	if got := strings.Count(view, "\n") + 1; got > 10 {
		t.Errorf("view height = %d, want <= 10\n%s", got, view)
	}
	if !strings.Contains(view, "super-secret") || !strings.Contains(view, "Press v to hide") {
		t.Errorf("revealed view = %s", view)
	}
}

func TestValueTabLongLineWraps(t *testing.T) {
	value := strings.Repeat("x", 100)
	tab := newValueTab(context.Background(), nil, false, func(context.Context) (string, error) { return value, nil })
	tab.SetSize(20, 10)
	tab = loadTab(t, tab).(*valueTab)
	if got := tab.viewport.TotalLineCount(); got < 2 {
		t.Errorf("long value should wrap: total lines = %d", got)
	}
}

func TestValueTabMetadataAndStringDecrypt(t *testing.T) {
	var gotName string
	var gotDecrypt bool
	ssm := NewSSMService(nil, func(_ context.Context, _, _, name string, decrypt bool) (string, error) {
		gotName, gotDecrypt = name, decrypt
		return "plain", nil
	}).(TabProvider)
	plain := newTabForTest(t, ssm.Tabs(), "value", ssmRow{res: aws.SSMParameterResource{Name: "/app/name", Type: "String", Tier: "", Version: 3}}).(*valueTab)
	plain.SetSize(60, 20)
	plain = loadTab(t, plain).(*valueTab)
	if gotName != "/app/name" || !gotDecrypt {
		t.Errorf("GetSSMParameter name=%q decrypt=%v, want /app/name and true", gotName, gotDecrypt)
	}
	view := plain.View(60, 20)
	for _, want := range []string{"/app/name", "String", "Tier", "—", "3", "plain"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q\n%s", want, view)
		}
	}

	var listLoads int
	listSvc := NewSSMService(nil, func(context.Context, string, string, string, bool) (string, error) {
		listLoads++
		return "a,b", nil
	}).(TabProvider)
	list := newTabForTest(t, listSvc.Tabs(), "value", ssmRow{res: aws.SSMParameterResource{Name: "/app/list", Type: "StringList"}}).(*valueTab)
	list.SetSize(60, 20)
	list = loadTab(t, list).(*valueTab)
	if listLoads != 1 || !strings.Contains(list.View(60, 20), "a,b") {
		t.Errorf("StringList loads=%d view=%s", listLoads, list.View(60, 20))
	}

	var secureLoads int
	secure := newTabForTest(t, ssm.Tabs(), "value", ssmRow{res: aws.SSMParameterResource{Name: "/app/password", Type: "securestring", Tier: "Standard"}}).(*valueTab)
	secure.load = func(context.Context) (string, error) {
		secureLoads++
		return "secret", nil
	}
	secure.SetSize(60, 20)
	if cmd := secure.Init(); cmd != nil {
		t.Error("SecureString must not load on Init")
	}
	if secureLoads != 0 {
		t.Errorf("SecureString loads = %d, want 0 before v", secureLoads)
	}
	next, cmd, _, _ := secure.Update(keyMsg("v"))
	secure = applyTabCmd(t, next, cmd).(*valueTab)
	if secureLoads != 1 || !strings.Contains(secure.View(60, 20), "secret") {
		t.Errorf("SecureString loads=%d view=%s", secureLoads, secure.View(60, 20))
	}
}

func TestValueTabServiceTabs(t *testing.T) {
	ssm := NewSSMService(nil, func(context.Context, string, string, string, bool) (string, error) { return "", nil }).(TabProvider)
	if got := tabTitles(ssm.Tabs()); !equalStrings(got, []string{"Overview", "Value", "Tags"}) {
		t.Errorf("SSM tabs = %v", got)
	}
	secure := newTabForTest(t, ssm.Tabs(), "value", ssmRow{res: aws.SSMParameterResource{Name: "p", Type: "SecureString"}}).(*valueTab)
	secure.SetSize(80, 20)
	if view := secure.View(80, 20); !strings.Contains(view, "Press v to reveal") {
		t.Errorf("SecureString view = %s", view)
	}
	plain := newTabForTest(t, ssm.Tabs(), "value", ssmRow{res: aws.SSMParameterResource{Name: "p", Type: "String"}}).(*valueTab)
	plain.SetSize(80, 20)
	if view := plain.View(80, 20); strings.Contains(view, "Press v to reveal") {
		t.Errorf("String view must not be masked: %s", view)
	}

	secrets := NewSecretsService(nil, func(context.Context, string, string, string) (string, error) { return "", nil }).(TabProvider)
	if got := tabTitles(secrets.Tabs()); !equalStrings(got, []string{"Overview", "Value", "Tags"}) {
		t.Errorf("Secrets tabs = %v", got)
	}
	secret := newTabForTest(t, secrets.Tabs(), "value", secretsRow{res: aws.SecretResource{Name: "s"}}).(*valueTab)
	secret.SetSize(80, 20)
	if view := secret.View(80, 20); !strings.Contains(view, "Press v to reveal") {
		t.Errorf("Secret view = %s", view)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

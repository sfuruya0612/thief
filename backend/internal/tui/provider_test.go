package tui

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/datadog"
	"github.com/sfuruya0612/thief/backend/internal/gcp"
	"github.com/sfuruya0612/thief/backend/internal/tidb"
)

// TestProviderBrandColors はプロバイダーのイメージカラーを固定する。枠線とタイトルは
// この色を使うため、値を変えると画面の見た目が変わる。
func TestProviderBrandColors(t *testing.T) {
	tests := []struct {
		name string
		got  lipgloss.Color
		want lipgloss.Color
	}{
		{name: "aws accent", got: awsAccent, want: lipgloss.Color("208")},
		{name: "aws muted", got: awsMutedAccent, want: lipgloss.Color("94")},
		{name: "gcp accent", got: gcpAccent, want: lipgloss.Color("69")},
		{name: "gcp muted", got: gcpMutedAccent, want: lipgloss.Color("25")},
		{name: "datadog accent", got: datadogAccent, want: lipgloss.Color("97")},
		{name: "datadog muted", got: datadogMutedAccent, want: lipgloss.Color("55")},
		{name: "tidb accent", got: tidbAccent, want: lipgloss.Color("160")},
		{name: "tidb muted", got: tidbMutedAccent, want: lipgloss.Color("88")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("color = %q, want %q", tt.got, tt.want)
			}
		})
	}

	// 4 プロバイダーの色が互いに区別できることを確かめる。
	accents := []lipgloss.Color{awsAccent, gcpAccent, datadogAccent, tidbAccent}
	seen := map[lipgloss.Color]bool{}
	for _, c := range accents {
		if seen[c] {
			t.Errorf("accent %q is used by more than one provider", c)
		}
		seen[c] = true
	}
}

func TestAWSProvider_MapsProfilesAndRegions(t *testing.T) {
	p := NewAWSProvider(
		func() ([]aws.Profile, error) {
			return []aws.Profile{
				{Name: "prod", AccountID: "111111111111", AuthType: aws.AuthTypeSSO, SSORoleName: "Admin", SSOStatus: aws.SSOStatusValid},
				{Name: "dev", AccountID: "222222222222", AuthType: aws.AuthTypeAccessKey},
			}, nil
		},
		func(_ context.Context, profile string) ([]aws.RegionResource, error) {
			if profile != "prod" {
				t.Errorf("LoadRegions profile = %q, want prod", profile)
			}
			return []aws.RegionResource{{Code: "ap-northeast-1", Name: "Tokyo"}}, nil
		},
		[]Service{&fakeService{id: "ec2"}},
	)

	if p.ID() != ProviderAWS || p.Title() != "AWS" {
		t.Errorf("provider = %s/%s", p.ID(), p.Title())
	}
	if p.ScopeLabel() != "profile" || p.RegionLabel() != "region" || p.RequiresScope() {
		t.Errorf("scope labels = %q/%q requires=%v", p.ScopeLabel(), p.RegionLabel(), p.RequiresScope())
	}

	scopes, err := p.LoadScopes(context.Background())
	if err != nil {
		t.Fatalf("LoadScopes: %v", err)
	}
	want := []Scope{
		{Label: "prod", Detail: "111111111111  Admin  sso:valid", Value: "prod"},
		{Label: "dev", Detail: "222222222222", Value: "dev"},
	}
	if !reflect.DeepEqual(scopes, want) {
		t.Errorf("scopes = %#v, want %#v", scopes, want)
	}

	regions, err := p.LoadRegions(context.Background(), "prod")
	if err != nil {
		t.Fatalf("LoadRegions: %v", err)
	}
	if want := []Scope{{Label: "ap-northeast-1", Detail: "Tokyo", Value: "ap-northeast-1"}}; !reflect.DeepEqual(regions, want) {
		t.Errorf("regions = %#v, want %#v", regions, want)
	}
}

func TestGCPProvider_MergesDefaultProjectAndKeepsListError(t *testing.T) {
	tests := []struct {
		name     string
		projects []gcp.ProjectInfo
		listErr  error
		want     []Scope
	}{
		{
			name: "default project is added to the list",
			projects: []gcp.ProjectInfo{
				{ProjectID: "p2", Name: "Second", State: "ACTIVE"},
			},
			want: []Scope{
				{Label: "p1", Detail: "default project", Value: "p1"},
				{Label: "p2", Detail: "Second  ACTIVE", Value: "p2"},
			},
		},
		{
			name:    "default project is kept when the list fails",
			listErr: errors.New("permission denied"),
			want:    []Scope{{Label: "p1", Detail: "default project", Value: "p1"}},
		},
		{
			name: "default project is not duplicated",
			projects: []gcp.ProjectInfo{
				{ProjectID: "p1", Name: "First", State: "ACTIVE"},
			},
			want: []Scope{{Label: "p1", Detail: "First  ACTIVE", Value: "p1"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewGCPProvider(func(context.Context) ([]gcp.ProjectInfo, error) {
				return tt.projects, tt.listErr
			}, "p1", []Service{&fakeService{id: "cloudrun"}})
			if !p.RequiresScope() || p.ScopeLabel() != "project" || p.RegionLabel() != "" {
				t.Errorf("scope = %q/%q requires=%v", p.ScopeLabel(), p.RegionLabel(), p.RequiresScope())
			}
			got, err := p.LoadScopes(context.Background())
			if !errors.Is(err, tt.listErr) {
				t.Errorf("err = %v, want %v", err, tt.listErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("scopes = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestDatadogProvider_PutsParentFirst(t *testing.T) {
	p := newDatadogProvider(func(context.Context) ([]datadog.OrgInfo, error) {
		return []datadog.OrgInfo{
			{ID: "sub-b", Name: "Beta"},
			{ID: "self", Name: "Acme", IsSelf: true},
			{ID: "sub-a", Name: "Alpha"},
		}, nil
	}, []Service{&fakeService{id: "datadog-dashboards"}})

	if p.ScopeLabel() != "organization" || p.RegionLabel() != "" || p.RequiresScope() {
		t.Errorf("scope = %q/%q requires=%v", p.ScopeLabel(), p.RegionLabel(), p.RequiresScope())
	}
	scopes, err := p.LoadScopes(context.Background())
	if err != nil {
		t.Fatalf("LoadScopes: %v", err)
	}
	want := []Scope{
		{Label: "Acme", Detail: "parent organization", Value: ""},
		{Label: "Alpha", Detail: "sub organization  sub-a", Value: "sub-a"},
		{Label: "Beta", Detail: "sub organization  sub-b", Value: "sub-b"},
	}
	if !reflect.DeepEqual(scopes, want) {
		t.Errorf("scopes = %#v, want %#v", scopes, want)
	}
}

func TestDatadogProvider_AddsParentWhenMissing(t *testing.T) {
	p := newDatadogProvider(func(context.Context) ([]datadog.OrgInfo, error) {
		return []datadog.OrgInfo{{ID: "sub", Name: "Sub"}}, nil
	}, []Service{&fakeService{id: "datadog-dashboards"}})
	scopes, err := p.LoadScopes(context.Background())
	if err != nil {
		t.Fatalf("LoadScopes: %v", err)
	}
	if len(scopes) != 2 || scopes[0].Value != "" {
		t.Errorf("scopes = %#v, want a parent fallback first", scopes)
	}
}

func TestTiDBProvider_HasNoScope(t *testing.T) {
	p := NewTiDBProvider(tidb.NewClient("", ""), "")
	if p.ScopeLabel() != "" || p.RegionLabel() != "" || p.RequiresScope() {
		t.Errorf("scope = %q/%q requires=%v", p.ScopeLabel(), p.RegionLabel(), p.RequiresScope())
	}
	var ids []string
	for _, svc := range p.Services() {
		ids = append(ids, svc.ID())
	}
	if want := []string{"tidb-projects", "tidb-cost"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("services = %v, want %v", ids, want)
	}
}

func TestModel_SwitchProviderLoadsScopesAndReloads(t *testing.T) {
	awsService := &fakeService{id: "ec2", rows: testRows()}
	gcpService := &fakeService{id: "cloudrun"}
	awsProvider := NewAWSProvider(nil, nil, []Service{awsService})
	gcpProvider := NewGCPProvider(func(context.Context) ([]gcp.ProjectInfo, error) {
		return []gcp.ProjectInfo{{ProjectID: "proj", Name: "Project", State: "ACTIVE"}}, nil
	}, "", []Service{gcpService})

	m, err := New(context.Background(), Deps{Providers: []Provider{awsProvider, gcpProvider}}, Target{
		Provider: ProviderAWS, Scope: "first", Region: "ap-northeast-1",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m.statusTTL = time.Millisecond
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(t, m, rowsLoadedMsg{seq: 0, rows: testRows()})

	m = keys(t, m, "P")
	if m.picker == nil || m.picker.kind != pickerProvider {
		t.Fatal("P should open the provider picker")
	}
	if !strings.Contains(m.View(), "Google Cloud") {
		t.Errorf("provider picker should list Google Cloud\n%s", m.View())
	}
	m = keys(t, m, "down")
	m, cmd := updateCmd(t, m, keyMsg("enter"))
	if m.provider.ID() != ProviderGCP {
		t.Fatalf("provider = %q, want gcp", m.provider.ID())
	}
	if m.profile != "" || m.loading {
		t.Fatalf("scope=%q loading=%v, want an empty project waiting for the picker", m.profile, m.loading)
	}
	m = apply(t, m, cmd)
	if m.picker == nil || m.picker.kind != pickerScope {
		t.Fatalf("the project picker should open after the list arrives: %+v", m.picker)
	}
	if !strings.Contains(m.View(), "Select a project") || !strings.Contains(m.View(), "proj") {
		t.Errorf("project picker view missing entries\n%s", m.View())
	}

	m, cmd = updateCmd(t, m, keyMsg("enter"))
	if m.profile != "proj" || !m.loading {
		t.Fatalf("scope=%q loading=%v, want proj/true", m.profile, m.loading)
	}
	m = apply(t, m, cmd)
	if got := gcpService.calls; len(got) != 1 || got[0] != "proj/" {
		t.Errorf("Load calls = %v, want [proj/]", got)
	}

	// 別のプロバイダーへ行って戻ると、AWS のスコープとサービスが復元される。
	m = keys(t, m, "P", "home")
	m, cmd = updateCmd(t, m, keyMsg("enter"))
	m = apply(t, m, cmd)
	if m.provider.ID() != ProviderAWS || m.profile != "first" || m.region != "ap-northeast-1" {
		t.Errorf("restored provider = %s/%q/%q", m.provider.ID(), m.profile, m.region)
	}
	if m.service.ID() != "ec2" {
		t.Errorf("restored service = %q, want ec2", m.service.ID())
	}
}

func TestModel_SwitchProviderDiscardsInFlightRows(t *testing.T) {
	awsService := &fakeService{id: "ec2", rows: testRows()}
	gcpService := &fakeService{id: "cloudrun"}
	awsProvider := NewAWSProvider(nil, nil, []Service{awsService})
	gcpProvider := NewGCPProvider(func(context.Context) ([]gcp.ProjectInfo, error) {
		return []gcp.ProjectInfo{{ProjectID: "proj", Name: "Project"}}, nil
	}, "", []Service{gcpService})

	m, err := New(context.Background(), Deps{Providers: []Provider{awsProvider, gcpProvider}}, Target{
		Provider: ProviderAWS, Scope: "first", Region: "ap-northeast-1",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m.statusTTL = time.Millisecond
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})

	// AWS の取得中 (seq 0) に、主スコープが未選択の Google Cloud へ切り替える。
	oldSeq := m.loadSeq
	m = keys(t, m, "P", "down")
	m, _ = updateCmd(t, m, keyMsg("enter"))
	if m.provider.ID() != ProviderGCP {
		t.Fatalf("provider = %q, want gcp", m.provider.ID())
	}
	m = update(t, m, rowsLoadedMsg{seq: oldSeq, rows: testRows()})
	if len(m.visible) != 0 {
		t.Errorf("rows from the previous provider must be discarded: %v", ids(m.visible))
	}
}

func TestModel_NonAWSProviderDoesNotShowSSOBanner(t *testing.T) {
	svc := &fakeService{id: "cloudrun"}
	provider := NewGCPProvider(nil, "proj", []Service{svc})
	m, err := New(context.Background(), Deps{Providers: []Provider{provider}}, Target{Provider: ProviderGCP})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m.statusTTL = time.Millisecond
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(t, m, rowsLoadedMsg{seq: m.loadSeq, err: fmt.Errorf("not authorized to list Cloud Run")})
	if m.ssoExpired {
		t.Error("a non-AWS error must not raise the SSO banner")
	}
	if strings.Contains(m.View(), "SSO token expired") {
		t.Errorf("the SSO banner must not be shown for Google Cloud\n%s", m.View())
	}
	if !strings.Contains(m.View(), "not authorized") {
		t.Errorf("the failure should be shown in the pane\n%s", m.View())
	}
}

func TestModel_ScopeDisplayUsesScopeLabel(t *testing.T) {
	p := newDatadogProvider(nil, []Service{&fakeService{id: "datadog-dashboards"}})
	m, err := New(context.Background(), Deps{Providers: []Provider{p}}, Target{Provider: ProviderDatadog})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := m.scopeDisplay(); got != "parent" {
		t.Errorf("scopeDisplay before loading = %q, want parent", got)
	}
	m.scopes = []Scope{{Label: "Acme", Detail: "parent organization", Value: ""}}
	if got := m.scopeDisplay(); got != "Acme" {
		t.Errorf("scopeDisplay = %q, want Acme", got)
	}
}

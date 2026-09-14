package datadog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestOrgsAPI は、指定した httptest.Server を向いた OrganizationsV2API を返す。
func newTestOrgsAPI(t *testing.T, srv *httptest.Server) (*OrganizationsV2API, context.Context) {
	t.Helper()
	t.Cleanup(srv.Close)

	host := strings.TrimPrefix(srv.URL, "http://")
	cfg := NewConfiguration("datadoghq.com")
	cfg.Host = host
	cfg.Scheme = "http"

	ctx := NewContext(context.Background(), "public", "private")
	return NewOrganizationsV2API(cfg), ctx
}

// managedOrgsBody は GET /api/v2/org のレスポンスボディ (JSON:API 形式) を組み立てる。
// selfID は current_org の id、managed は管理下の組織 id の順序付きリスト
// (実際の API と同様に自組織自身も含む)、included は id から表示名への対応
// (対応が無い id は included に一致するリソースが無い状態を表す)。
func managedOrgsBody(selfID string, managed []string, included map[string]string) string {
	var managedRefs strings.Builder
	for i, id := range managed {
		if i > 0 {
			managedRefs.WriteString(",")
		}
		managedRefs.WriteString(`{"id":"` + id + `","type":"orgs"}`)
	}

	var includedResources strings.Builder
	i := 0
	for id, name := range included {
		if i > 0 {
			includedResources.WriteString(",")
		}
		// OrgAttributes は created_at/description/disabled/modified_at/name/public_id/
		// sharing/url を全て必須とし (SDK の UnmarshalJSON が検証する)、これらが欠けると
		// この OrgData 自体が UnparsedObject に落ちて GetId() がゼロ値になる。
		includedResources.WriteString(
			`{"id":"` + id + `","type":"orgs","attributes":{` +
				`"created_at":"2020-01-01T00:00:00Z","description":"","disabled":false,` +
				`"modified_at":"2020-01-01T00:00:00Z","name":"` + name + `","public_id":"` + id + `",` +
				`"sharing":"organization","url":""}}`,
		)
		i++
	}

	return `{"data":{"id":"` + selfID + `","type":"managed_orgs","relationships":{` +
		`"current_org":{"data":{"id":"` + selfID + `","type":"orgs"}},` +
		`"managed_orgs":{"data":[` + managedRefs.String() + `]}` +
		`}},"included":[` + includedResources.String() + `]}`
}

func TestListOrgs(t *testing.T) {
	const (
		selfID = "4c4f231c-00cc-11ea-a77b-17122b83a2a2"
		subID  = "5d5f342d-11dd-22fb-b88c-28233c94b3b3"
	)

	tests := []struct {
		name   string
		status int
		body   string
		want   []OrgInfo
		// wantErr が真なら ListOrgs がエラーを返すこと。
		wantErr bool
	}{
		{
			name:   "the current org is marked IsSelf and ids are lower-cased",
			status: http.StatusOK,
			body: managedOrgsBody(strings.ToUpper(selfID), []string{strings.ToUpper(selfID), strings.ToUpper(subID)}, map[string]string{
				strings.ToUpper(selfID): "Parent Org",
				strings.ToUpper(subID):  "Sub Org",
			}),
			want: []OrgInfo{
				{ID: selfID, Name: "Parent Org", IsSelf: true},
				{ID: subID, Name: "Sub Org", IsSelf: false},
			},
		},
		{
			name:   "an organization missing from included falls back to its id as the name",
			status: http.StatusOK,
			body:   managedOrgsBody(selfID, []string{selfID, subID}, map[string]string{selfID: "Parent Org"}),
			want: []OrgInfo{
				{ID: selfID, Name: "Parent Org", IsSelf: true},
				{ID: subID, Name: subID, IsSelf: false},
			},
		},
		{
			name:   "only the current org managed yields a single self entry",
			status: http.StatusOK,
			body:   managedOrgsBody(selfID, []string{selfID}, map[string]string{selfID: "Parent Org"}),
			want:   []OrgInfo{{ID: selfID, Name: "Parent Org", IsSelf: true}},
		},
		{
			name:   "an empty managed orgs list yields an empty slice",
			status: http.StatusOK,
			body:   managedOrgsBody(selfID, nil, nil),
			want:   []OrgInfo{},
		},
		{
			name:    "an api error is wrapped",
			status:  http.StatusForbidden,
			body:    `{"errors":["insufficient_scope"]}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				w.Write([]byte(tt.body))
			}))
			api, ctx := newTestOrgsAPI(t, srv)

			orgs, err := ListOrgs(ctx, api)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ListOrgs() error = nil, want an error")
				}
				if !strings.Contains(err.Error(), "list datadog orgs") {
					t.Errorf("ListOrgs() error = %v, want it to be wrapped with the operation name", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ListOrgs() error = %v", err)
			}
			if gotPath != "/api/v2/org" {
				t.Errorf("requested path = %q, want /api/v2/org", gotPath)
			}
			if orgs == nil {
				t.Fatal("ListOrgs() returned a nil slice, want an empty slice")
			}
			if len(orgs) != len(tt.want) {
				t.Fatalf("orgs = %+v, want %+v", orgs, tt.want)
			}
			for i, want := range tt.want {
				if orgs[i] != want {
					t.Errorf("orgs[%d] = %+v, want %+v", i, orgs[i], want)
				}
			}
		})
	}
}

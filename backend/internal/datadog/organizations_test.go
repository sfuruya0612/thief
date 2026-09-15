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

// includedOrg は GET /api/v2/org の included[] に入れる組織リソース 1 件分のフィクスチャ。
// Type と Attributes は JSON の断片をそのまま埋め込む (Attributes が空文字なら
// attributes キー自体を出さない)。
type includedOrg struct {
	ID         string
	Type       string
	Attributes string
}

// fullOrgAttributes は SDK (v2.65.0) の OrgAttributes が必須とする 8 フィールドを
// すべて満たす attributes の JSON を返す。
func fullOrgAttributes(id, name string) string {
	return `{"created_at":"2020-01-01T00:00:00Z","description":"","disabled":false,` +
		`"modified_at":"2020-01-01T00:00:00Z","name":"` + name + `","public_id":"` + id + `",` +
		`"sharing":"organization","url":""}`
}

// fullOrg は SDK の検証に通る included の組織を返す。
func fullOrg(id, name string) includedOrg {
	return includedOrg{ID: id, Type: "orgs", Attributes: fullOrgAttributes(id, name)}
}

// managedOrgsBody は GET /api/v2/org のレスポンスボディ (JSON:API 形式) を組み立てる。
// selfID は current_org の id、managed は管理下の組織 id の順序付きリスト
// (実際の API と同様に自組織自身も含む)、included は included[] に並べる組織リソース。
func managedOrgsBody(selfID string, managed []string, included []includedOrg) string {
	var managedRefs strings.Builder
	for i, id := range managed {
		if i > 0 {
			managedRefs.WriteString(",")
		}
		managedRefs.WriteString(`{"id":"` + id + `","type":"orgs"}`)
	}

	var includedResources strings.Builder
	for i, o := range included {
		if i > 0 {
			includedResources.WriteString(",")
		}
		includedResources.WriteString(`{"id":"` + o.ID + `","type":"` + o.Type + `"`)
		if o.Attributes != "" {
			includedResources.WriteString(`,"attributes":` + o.Attributes)
		}
		includedResources.WriteString(`}`)
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
			body: managedOrgsBody(strings.ToUpper(selfID), []string{strings.ToUpper(selfID), strings.ToUpper(subID)}, []includedOrg{
				fullOrg(strings.ToUpper(selfID), "Parent Org"),
				fullOrg(strings.ToUpper(subID), "Sub Org"),
			}),
			want: []OrgInfo{
				{ID: selfID, Name: "Parent Org", IsSelf: true},
				{ID: subID, Name: "Sub Org", IsSelf: false},
			},
		},
		{
			name:   "an organization missing from included falls back to its id as the name",
			status: http.StatusOK,
			body:   managedOrgsBody(selfID, []string{selfID, subID}, []includedOrg{fullOrg(selfID, "Parent Org")}),
			want: []OrgInfo{
				{ID: selfID, Name: "Parent Org", IsSelf: true},
				{ID: subID, Name: subID, IsSelf: false},
			},
		},
		{
			// SDK の OrgAttributes は 8 フィールドすべてを必須とするため、この組織は
			// SDK 側では UnparsedObject に退避される。生の JSON から名前を読めること。
			name:   "an included org whose attributes lack fields the sdk requires still resolves its name (issue 0173)",
			status: http.StatusOK,
			body: managedOrgsBody(selfID, []string{selfID, subID}, []includedOrg{
				fullOrg(selfID, "Parent Org"),
				{ID: subID, Type: "orgs", Attributes: `{"name":"Sub Org","public_id":"` + subID + `"}`},
			}),
			want: []OrgInfo{
				{ID: selfID, Name: "Parent Org", IsSelf: true},
				{ID: subID, Name: "Sub Org", IsSelf: false},
			},
		},
		{
			// 必須フィールドが JSON の null でも SDK は欠落と同じく拒否する。
			name:   "an included org with a null description still resolves its name (issue 0173)",
			status: http.StatusOK,
			body: managedOrgsBody(selfID, []string{selfID, subID}, []includedOrg{
				{ID: selfID, Type: "orgs", Attributes: strings.Replace(fullOrgAttributes(selfID, "Parent Org"), `"description":""`, `"description":null`, 1)},
				{ID: subID, Type: "orgs", Attributes: strings.Replace(fullOrgAttributes(subID, "Sub Org"), `"description":""`, `"description":null`, 1)},
			}),
			want: []OrgInfo{
				{ID: selfID, Name: "Parent Org", IsSelf: true},
				{ID: subID, Name: "Sub Org", IsSelf: false},
			},
		},
		{
			name:   "an included org with an unknown resource type still resolves its name (issue 0173)",
			status: http.StatusOK,
			body: managedOrgsBody(selfID, []string{selfID, subID}, []includedOrg{
				fullOrg(selfID, "Parent Org"),
				{ID: subID, Type: "organizations", Attributes: fullOrgAttributes(subID, "Sub Org")},
			}),
			want: []OrgInfo{
				{ID: selfID, Name: "Parent Org", IsSelf: true},
				{ID: subID, Name: "Sub Org", IsSelf: false},
			},
		},
		{
			// included の要素に attributes キー自体が無いと、SDK はその要素だけでなく
			// レスポンス全体を UnparsedObject に退避する。その場合も current_org と
			// managed_orgs を生の JSON から読めること。
			name:   "an included org without attributes makes the sdk reject the whole response, which is still read from the raw json (issue 0173)",
			status: http.StatusOK,
			body: managedOrgsBody(selfID, []string{selfID, subID}, []includedOrg{
				fullOrg(selfID, "Parent Org"),
				{ID: subID, Type: "orgs"},
			}),
			want: []OrgInfo{
				{ID: selfID, Name: "Parent Org", IsSelf: true},
				{ID: subID, Name: subID, IsSelf: false},
			},
		},
		{
			name:   "an included org whose raw attributes have no name falls back to its id",
			status: http.StatusOK,
			body: managedOrgsBody(selfID, []string{selfID, subID}, []includedOrg{
				fullOrg(selfID, "Parent Org"),
				{ID: subID, Type: "orgs", Attributes: `{"public_id":"` + subID + `","name":null}`},
			}),
			want: []OrgInfo{
				{ID: selfID, Name: "Parent Org", IsSelf: true},
				{ID: subID, Name: subID, IsSelf: false},
			},
		},
		{
			name:   "only the current org managed yields a single self entry",
			status: http.StatusOK,
			body:   managedOrgsBody(selfID, []string{selfID}, []includedOrg{fullOrg(selfID, "Parent Org")}),
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

package datadog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestOrgsAPI returns an OrganizationsV1API pointed at the given httptest.Server.
func newTestOrgsAPI(t *testing.T, srv *httptest.Server) (*OrganizationsV1API, context.Context) {
	t.Helper()
	t.Cleanup(srv.Close)

	host := strings.TrimPrefix(srv.URL, "http://")
	cfg := NewConfiguration("datadoghq.com")
	cfg.Host = host
	cfg.Scheme = "http"

	ctx := NewContext(context.Background(), "public", "private")
	return NewOrganizationsV1API(cfg), ctx
}

func TestListOrgs(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   []OrgInfo
		// wantErr が真なら ListOrgs がエラーを返すこと。
		wantErr bool
	}{
		{
			name:   "public ids are lower-cased and names are kept as they are",
			status: http.StatusOK,
			body: `{"orgs":[
			  {"public_id":"ABC123def","name":"Parent Org"},
			  {"public_id":"sub456","name":"Sub Org"}
			]}`,
			want: []OrgInfo{
				{ID: "abc123def", Name: "Parent Org"},
				{ID: "sub456", Name: "Sub Org"},
			},
		},
		{
			name:   "an organization without a public id is skipped",
			status: http.StatusOK,
			body:   `{"orgs":[{"name":"No Public Id"},{"public_id":"sub456","name":"Sub Org"}]}`,
			want:   []OrgInfo{{ID: "sub456", Name: "Sub Org"}},
		},
		{
			name:   "an organization without a name keeps its id",
			status: http.StatusOK,
			body:   `{"orgs":[{"public_id":"sub456"}]}`,
			want:   []OrgInfo{{ID: "sub456", Name: ""}},
		},
		{
			name:   "an empty list yields an empty slice",
			status: http.StatusOK,
			body:   `{"orgs":[]}`,
			want:   []OrgInfo{},
		},
		{
			name:   "a missing orgs field yields an empty slice",
			status: http.StatusOK,
			body:   `{}`,
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
			if gotPath != "/api/v1/org" {
				t.Errorf("requested path = %q, want /api/v1/org", gotPath)
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

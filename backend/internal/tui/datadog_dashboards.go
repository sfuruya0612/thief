package tui

import (
	"context"
	"net/url"

	"github.com/sfuruya0612/thief/backend/internal/datadog"
)

// datadogDashboardsService は Datadog のダッシュボード一覧を提供する Service。
type datadogDashboardsService struct {
	auth DatadogAuth
	api  *datadog.DashboardsV1API
	site string
}

// NewDatadogDashboardsService は Dashboards の Service を返す。auth は選択中の組織の
// 認証コンテキストを組み立て、site はダッシュボードの絶対 URL の組み立てに使う。
func NewDatadogDashboardsService(auth DatadogAuth, api *datadog.DashboardsV1API, site string) Service {
	return &datadogDashboardsService{auth: auth, api: api, site: site}
}

func (s *datadogDashboardsService) ID() string    { return "datadog-dashboards" }
func (s *datadogDashboardsService) Title() string { return "Dashboards" }

// Columns は一覧に出すダッシュボードの項目。Web UI の一覧は select なので列は無いが、
// TUI ではタイトルと説明を並べる。
func (s *datadogDashboardsService) Columns() []Column {
	return []Column{
		{Title: "Title"},
		{Title: "Description"},
	}
}

func (s *datadogDashboardsService) Load(ctx context.Context, org, _ string) ([]Row, error) {
	items, err := s.list(ctx, org)
	if err != nil {
		return nil, err
	}
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, datadogDashboardRow{res: it})
	}
	return rows, nil
}

func (s *datadogDashboardsService) SupportsSession() bool { return false }

// Tabs は Overview と、ダッシュボードのウィジェットを並べる Widgets を返す。
func (s *datadogDashboardsService) Tabs() []TabSpec {
	return []TabSpec{
		{ID: tabOverview, Title: "Overview"},
		contentTabSpec("widgets", "Widgets", func(ctx context.Context, org, _ string, row Row) (TabContent, error) {
			dashboard := row.(datadogDashboardRow)
			detail, err := s.detail(ctx, org, dashboard.res.ID)
			if err != nil {
				return TabContent{}, err
			}
			return datadogWidgetsContent(detail), nil
		}),
	}
}

func (s *datadogDashboardsService) list(ctx context.Context, org string) ([]datadog.DashboardInfo, error) {
	items, err := datadogCall(ctx, s.auth, org, func(ctx context.Context) ([]datadog.DashboardInfo, error) {
		return datadog.ListDashboards(ctx, s.api)
	})
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].URL = datadogAppURL(s.site, items[i].URL)
	}
	return items, nil
}

func (s *datadogDashboardsService) detail(ctx context.Context, org, id string) (datadog.DashboardDetail, error) {
	detail, err := datadogCall(ctx, s.auth, org, func(ctx context.Context) (datadog.DashboardDetail, error) {
		return datadog.GetDashboard(ctx, s.api, id)
	})
	if err != nil {
		return datadog.DashboardDetail{}, err
	}
	detail.URL = datadogAppURL(s.site, detail.URL)
	return detail, nil
}

// datadogAppURL は Datadog が返す相対 URL を、ブラウザで開ける絶対 URL にする。
func datadogAppURL(site, raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.IsAbs() {
		return raw
	}
	return (&url.URL{Scheme: "https", Host: "app." + site, Path: u.Path, RawQuery: u.RawQuery}).String()
}

// datadogWidgetsContent は 1 つのダッシュボードのウィジェットを表にする。
func datadogWidgetsContent(detail datadog.DashboardDetail) TabContent {
	if len(detail.Widgets) == 0 {
		return TabContent{Blocks: []TabBlock{{Text: "This dashboard has no widgets"}}}
	}
	rows := make([][]string, 0, len(detail.Widgets))
	for _, w := range detail.Widgets {
		queries := ""
		for i, q := range w.Queries {
			if i > 0 {
				queries += " | "
			}
			queries += q
		}
		rows = append(rows, []string{
			string(w.Kind),
			w.Type,
			orDash(w.Title),
			orDash(queries),
		})
	}
	return TabContent{Blocks: []TabBlock{{
		Title: "Widgets",
		Table: &TabTable{
			Columns: []Column{{Title: "Kind"}, {Title: "Type"}, {Title: "Title"}, {Title: "Queries"}},
			Rows:    rows,
		},
	}}}
}

// datadogDashboardRow はダッシュボード 1 件の行。ID はダッシュボード ID。
type datadogDashboardRow struct {
	res datadog.DashboardInfo
}

func (r datadogDashboardRow) ID() string    { return r.res.ID }
func (r datadogDashboardRow) Name() string  { return r.res.Title }
func (r datadogDashboardRow) State() string { return "" }

func (r datadogDashboardRow) Cells() []string {
	return []string{
		orDash(r.res.Title),
		orDash(r.res.Description),
	}
}

// Overview は一覧で選んだダッシュボードの詳細を、ウィジェットの取得を待たずに出す。
func (r datadogDashboardRow) Overview() []KV {
	return []KV{
		{Key: "ID", Value: r.res.ID},
		{Key: "Title", Value: orDash(r.res.Title)},
		{Key: "Description", Value: orDash(r.res.Description)},
		{Key: "URL", Value: orDash(r.res.URL)},
	}
}

func (r datadogDashboardRow) Tags() map[string]string { return nil }

var (
	_ Service     = (*datadogDashboardsService)(nil)
	_ TabProvider = (*datadogDashboardsService)(nil)
)

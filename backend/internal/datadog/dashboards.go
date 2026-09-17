package datadog

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
)

// WidgetKind は thief がウィジェットをどう描くかを表す。Datadog のウィジェット種別は
// 多岐にわたるが、thief が再描画するのは時系列グラフと単一値の 2 つだけで、それ以外は
// Datadog へのリンクで代替する。
type WidgetKind string

const (
	// WidgetKindTimeseries は時系列グラフとして描けるウィジェット。
	WidgetKindTimeseries WidgetKind = "timeseries"
	// WidgetKindQueryValue は単一値として描けるウィジェット。
	WidgetKindQueryValue WidgetKind = "query_value"
	// WidgetKindUnsupported は thief では描かず Datadog へのリンクで代替するウィジェット。
	WidgetKindUnsupported WidgetKind = "unsupported"
)

// unknownWidgetType は応答からウィジェットの種別名を読み取れなかったときの値。
// 種別名は利用者に「何のウィジェットが未対応だったか」を示すためだけに使うので、
// 読み取れなくてもエラーにはせずこの値で埋める。
const unknownWidgetType = "unknown"

// DashboardInfo はダッシュボード一覧の 1 件。
type DashboardInfo struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	// URL は Datadog が返すダッシュボードへのパス。先頭が "/" の相対パスなので、
	// 絶対 URL への組み立ては site を知っている呼び出し側が行う。
	URL string `json:"url"`
}

// DashboardDetail は 1 つのダッシュボードと、そこから抽出したウィジェット。
type DashboardDetail struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	URL         string       `json:"url"`
	Widgets     []WidgetInfo `json:"widgets"`
}

// WidgetInfo は 1 つのウィジェット。group ウィジェットは入れ子を展開した結果に置き換わる
// ため、この形では現れない。
type WidgetInfo struct {
	ID   int64      `json:"id"`
	Kind WidgetKind `json:"kind"`
	// Type は Datadog が返す種別名 ("timeseries"、"toplist" など)。Kind が
	// unsupported のとき、何のウィジェットだったかを利用者に示すために使う。
	Type  string `json:"type"`
	Title string `json:"title"`
	// Queries はウィジェットの各リクエストのクエリ文字列 (q)。Kind が
	// unsupported の場合は空になる。
	Queries []string `json:"queries"`
}

// ListDashboards は現在の認証情報から見えるダッシュボードの一覧を返す
// (GET /api/v1/dashboard)。
func ListDashboards(ctx context.Context, api *DashboardsV1API) ([]DashboardInfo, error) {
	resp, _, err := api.api.ListDashboards(ctx)
	if err != nil {
		return nil, fmt.Errorf("list datadog dashboards: %w", err)
	}

	dashboards := []DashboardInfo{}
	for _, d := range resp.GetDashboards() {
		id := d.GetId()
		if id == "" {
			// id が無いダッシュボードは詳細を引けず、一覧に出しても選べない。
			// 落とした事実は残す (ListOrgs が public_id 無しを落とすのと同じ扱い)。
			slog.Warn("datadog returned a dashboard without an id; skipping it", "title", d.GetTitle())
			continue
		}
		dashboards = append(dashboards, DashboardInfo{
			ID:          id,
			Title:       d.GetTitle(),
			Description: d.GetDescription(),
			URL:         d.GetUrl(),
		})
	}
	return dashboards, nil
}

// GetDashboard は 1 つのダッシュボードを、そのウィジェットを thief が描ける範囲へ
// 平坦化して返す (GET /api/v1/dashboard/{id})。
func GetDashboard(ctx context.Context, api *DashboardsV1API, id string) (DashboardDetail, error) {
	resp, _, err := api.api.GetDashboard(ctx, id)
	if err != nil {
		return DashboardDetail{}, fmt.Errorf("get datadog dashboard %q: %w", id, err)
	}

	return DashboardDetail{
		ID:          resp.GetId(),
		Title:       resp.GetTitle(),
		Description: resp.GetDescription(),
		URL:         resp.GetUrl(),
		Widgets:     extractWidgets(resp.GetWidgets()),
	}, nil
}

// extractWidgets はウィジェット列を平坦な WidgetInfo 列へ変換する。
//
// group ウィジェットは他のウィジェットを入れ子にするだけの容れ物なので、自身は結果に
// 含めず、内側のウィジェットを同じ規則で展開する。再帰の深さは応答の入れ子の深さで
// 頭打ちになる (Datadog の group は group を入れ子にできない)。
func extractWidgets(widgets []datadogV1.Widget) []WidgetInfo {
	out := []WidgetInfo{}
	for _, w := range widgets {
		def := w.GetDefinition()
		if g := def.GroupWidgetDefinition; g != nil {
			out = append(out, extractWidgets(g.GetWidgets())...)
			continue
		}
		out = append(out, widgetInfo(w))
	}
	return out
}

// widgetInfo は 1 つのウィジェットを WidgetInfo へ写す。
//
// timeseries と query_value でも、クエリ文字列 (q) を 1 つも持たないもの
// (formulas と queries で組み立てるリクエストだけのもの) は未対応として扱う。
// 再描画には実行できるクエリが要るため、q が無ければ描く材料が無く、空のグラフを
// 出すより Datadog へのリンクを出す方が利用者にとって役に立つ。
func widgetInfo(w datadogV1.Widget) WidgetInfo {
	def := w.GetDefinition()
	widgetType, title := widgetMeta(def)
	info := WidgetInfo{
		ID:      w.GetId(),
		Kind:    WidgetKindUnsupported,
		Type:    widgetType,
		Title:   title,
		Queries: []string{},
	}

	switch {
	case def.TimeseriesWidgetDefinition != nil:
		for _, r := range def.TimeseriesWidgetDefinition.GetRequests() {
			if q := r.GetQ(); q != "" {
				info.Queries = append(info.Queries, q)
			}
		}
		if len(info.Queries) > 0 {
			info.Kind = WidgetKindTimeseries
		}
	case def.QueryValueWidgetDefinition != nil:
		for _, r := range def.QueryValueWidgetDefinition.GetRequests() {
			if q := r.GetQ(); q != "" {
				info.Queries = append(info.Queries, q)
			}
		}
		if len(info.Queries) > 0 {
			info.Kind = WidgetKindQueryValue
		}
	}
	return info
}

// widgetMeta はウィジェット定義の種別名とタイトルを JSON 表現から読み取る。
//
// WidgetDefinition は 35 種を超える oneOf で、種別ごとに Go の型が異なり GetType の
// 戻り値の型も異なるため、共通のインターフェースでは種別名を取り出せない。一方 JSON
// 表現はどの種別でも type と title を同じキーに持ち、SDK がまだ知らない新しい種別
// (UnparsedObject へ落ちたもの) でも同じ形で残る。JSON を経由することで、種別ごとの
// 分岐を持たずに、SDK の版に依存せず種別名を得られる。
func widgetMeta(def datadogV1.WidgetDefinition) (widgetType, title string) {
	raw, err := def.MarshalJSON()
	if err != nil || len(raw) == 0 {
		return unknownWidgetType, ""
	}
	var meta struct {
		Type  string `json:"type"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return unknownWidgetType, ""
	}
	if meta.Type == "" {
		return unknownWidgetType, meta.Title
	}
	return meta.Type, meta.Title
}

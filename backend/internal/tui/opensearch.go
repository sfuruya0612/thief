package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/sfuruya0612/thief/backend/internal/aws"
)

// openSearchService は Amazon OpenSearch Service のドメイン一覧を提供する Service。
type openSearchService struct {
	list func(ctx context.Context, profile, region string) ([]aws.OpenSearchResource, error)
}

// NewOpenSearchService は OpenSearch の Service を返す。list には aws.ListOpenSearchResources を渡す。
func NewOpenSearchService(list func(ctx context.Context, profile, region string) ([]aws.OpenSearchResource, error)) Service {
	return &openSearchService{list: list}
}

func (s *openSearchService) ID() string    { return "opensearch" }
func (s *openSearchService) Title() string { return "OpenSearch" }

// Columns は Web UI の opensearchColumns (frontend/src/components/tables/columns.tsx) と同じ列。
// Web UI に dash を描画するセルは無く、値が無いセルは空文字列にする。
func (s *openSearchService) Columns() []Column {
	return []Column{
		{Title: "Domain"},
		{Title: "State"},
		{Title: "Version"},
		{Title: "Instance"},
		{Title: "Nodes", Align: AlignRight},
		{Title: "Endpoint"},
		{Title: "Region"},
	}
}

func (s *openSearchService) Load(ctx context.Context, profile, region string) ([]Row, error) {
	items, err := s.list(ctx, profile, region)
	if err != nil {
		return nil, fmt.Errorf("list OpenSearch domains: %w", err)
	}
	rows := make([]Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, openSearchRow{res: it, region: region})
	}
	return rows, nil
}

func (s *openSearchService) SupportsSession() bool { return false }

// openSearchRow はドメイン 1 つの行。ID はドメインの ARN。
type openSearchRow struct {
	res    aws.OpenSearchResource
	region string
}

func (r openSearchRow) ID() string    { return r.res.ID }
func (r openSearchRow) Name() string  { return r.res.Name }
func (r openSearchRow) State() string { return r.res.State }

func (r openSearchRow) Cells() []string {
	return []string{
		r.res.Name,
		r.State(),
		r.res.EngineVersion,
		r.res.DataNodeType,
		formatInt32(r.res.DataNodeCount),
		dashIfEmpty(r.res.Endpoint),
		r.region,
	}
}

// Overview は Web UI の opensearchOverviewRows (frontend/src/components/Drawer/overviewRows.tsx) と同じ項目。
func (r openSearchRow) Overview() []KV {
	return []KV{
		{Key: "Resource ID", Value: r.res.ID},
		{Key: "Engine version", Value: r.res.EngineVersion},
		{Key: "Data node type", Value: dashIfEmpty(r.res.DataNodeType)},
		{Key: "Data nodes", Value: formatInt32(r.res.DataNodeCount)},
		{
			Key:   "Dedicated master type",
			Value: openSearchMasterValue(r.res.DedicatedMasterEnabled, r.res.DedicatedMasterType),
		},
		{
			Key:   "Dedicated masters",
			Value: openSearchMasterCount(r.res.DedicatedMasterEnabled, r.res.DedicatedMasterCount),
		},
		{Key: "EBS storage", Value: openSearchEBSStorage(r.res)},
		{Key: "Endpoint", Value: dashIfEmpty(r.res.Endpoint)},
		{Key: "VPC", Value: dashIfEmpty(r.res.VPCID)},
		{Key: "Encryption at rest", Value: enabledLabel(r.res.EncryptionAtRest)},
		{Key: "Node-to-node encryption", Value: enabledLabel(r.res.NodeToNodeEncryption)},
		{Key: "HTTPS enforcement", Value: enabledLabel(r.res.EnforceHTTPS)},
		{Key: "Region", Value: r.region},
	}
}

func (r openSearchRow) Tags() map[string]string { return r.res.Tags }

// TagsFetchFailed は ListOpenSearchResources の内部の ListTags が失敗したかを返す (TagsFetchFailer)。
func (r openSearchRow) TagsFetchFailed() bool { return r.res.TagsFetchFailed }

// openSearchMasterValue は dedicated master が有効なときだけインスタンスタイプを返す。
// 無効なドメインではデータノードが master の役割を兼ねるため "—" にする。
func openSearchMasterValue(enabled bool, nodeType string) string {
	if !enabled {
		return "—"
	}
	return dashIfEmpty(nodeType)
}

// openSearchMasterCount は dedicated master が有効なときだけノード数を返す。
func openSearchMasterCount(enabled bool, count int32) string {
	if !enabled {
		return "—"
	}
	return formatInt32(count)
}

// openSearchEBSStorage は Web UI の Overview と同じ "<size> GiB <type>" を返す。
// サイズが 0 のときはボリュームが無い (インスタンスストア) ため "—" にする。
func openSearchEBSStorage(r aws.OpenSearchResource) string {
	if r.EBSVolumeSizeGB <= 0 {
		return "—"
	}
	s := fmt.Sprintf("%d GiB", r.EBSVolumeSizeGB)
	if r.EBSVolumeType != "" {
		s += " " + r.EBSVolumeType
	}
	return s
}

// dashIfEmpty は Web UI が空の値に "—" を出すセルと Overview の項目で使う。
func dashIfEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

// enabledLabel は Web UI が yes / no ではなく enabled / disabled で表示する真偽値を返す。
func enabledLabel(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}

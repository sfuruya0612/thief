package aws

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	opensearchtypes "github.com/aws/aws-sdk-go-v2/service/opensearch/types"
	"golang.org/x/sync/errgroup"
)

// OpenSearchResource represents an Amazon OpenSearch Service domain.
type OpenSearchResource struct {
	ID                     string            `json:"id"`
	Name                   string            `json:"name"`
	State                  string            `json:"state"`
	EngineVersion          string            `json:"engine_version"`
	DataNodeType           string            `json:"data_node_type"`
	DataNodeCount          int32             `json:"data_node_count"`
	DedicatedMasterEnabled bool              `json:"dedicated_master_enabled"`
	DedicatedMasterType    string            `json:"dedicated_master_type"`
	DedicatedMasterCount   int32             `json:"dedicated_master_count"`
	EBSVolumeSizeGB        int32             `json:"ebs_volume_size_gb"`
	EBSVolumeType          string            `json:"ebs_volume_type"`
	Endpoint               string            `json:"endpoint"`
	VPCID                  string            `json:"vpc_id"`
	EncryptionAtRest       bool              `json:"encryption_at_rest"`
	NodeToNodeEncryption   bool              `json:"node_to_node_encryption"`
	EnforceHTTPS           bool              `json:"enforce_https"`
	Tags                   map[string]string `json:"tags"`
	TagsFetchFailed        bool              `json:"tags_fetch_failed,omitempty"`
	CostMonthly            float64           `json:"cost_monthly"`
}

func (r OpenSearchResource) ResourceID() string    { return r.ID }
func (r OpenSearchResource) ResourceName() string  { return r.Name }
func (r OpenSearchResource) ResourceState() string { return NormalizeState(r.State) }
func (r OpenSearchResource) ServiceName() string   { return "opensearch" }

const (
	// openSearchDescribeBatchSize は DescribeDomains が 1 回で受け付ける最大ドメイン数。
	openSearchDescribeBatchSize = 5
	// openSearchTagConcurrency はドメインごとのタグ取得 (ListTags) を同時実行する上限数。
	// OpenSearch のマネジメント系 API はレート制限が緩くないため、WAF と同じ控えめな値から始める。
	openSearchTagConcurrency = 10
)

// openSearchListClient は一覧取得に必要な OpenSearch API を抽象化する。
// テストではモックを差し込み、実行時は *opensearch.Client がこれを満たす。
type openSearchListClient interface {
	ListDomainNames(ctx context.Context, params *opensearch.ListDomainNamesInput, optFns ...func(*opensearch.Options)) (*opensearch.ListDomainNamesOutput, error)
	DescribeDomains(ctx context.Context, params *opensearch.DescribeDomainsInput, optFns ...func(*opensearch.Options)) (*opensearch.DescribeDomainsOutput, error)
	ListTags(ctx context.Context, params *opensearch.ListTagsInput, optFns ...func(*opensearch.Options)) (*opensearch.ListTagsOutput, error)
}

// ListOpenSearchResources returns all Amazon OpenSearch Service domains for the given profile/region.
func ListOpenSearchResources(ctx context.Context, profile, region string) ([]OpenSearchResource, error) {
	// 各フェーズの所要時間を計測してログに残す (issue 0081 と同じ形式)。
	overallStart := time.Now()

	clientStart := time.Now()
	client, err := newOpenSearchClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	slog.Debug("opensearch client created",
		"profile", profile, "region", region, "duration_ms", time.Since(clientStart).Milliseconds())

	resources, err := listOpenSearchResources(ctx, client, profile, region)
	if err != nil {
		return nil, err
	}

	slog.Debug("opensearch list all done",
		"profile", profile, "region", region,
		"duration_ms", time.Since(overallStart).Milliseconds(), "count", len(resources))
	return resources, nil
}

// listOpenSearchResources は生成済みクライアントでドメイン一覧、詳細、タグを取得するコア。
// DescribeDomainsInput に載せるドメイン名の分割を単体テストで固定できるよう、
// クライアントの生成と分離してある。
func listOpenSearchResources(ctx context.Context, client openSearchListClient, profile, region string) ([]OpenSearchResource, error) {
	listStart := time.Now()
	out, err := client.ListDomainNames(ctx, &opensearch.ListDomainNamesInput{})
	if err != nil {
		return nil, fmt.Errorf("list opensearch domains: %w", err)
	}
	names := openSearchDomainNames(out)
	slog.Debug("opensearch list domains done",
		"profile", profile, "region", region,
		"duration_ms", time.Since(listStart).Milliseconds(), "count", len(names))

	if len(names) == 0 {
		return []OpenSearchResource{}, nil
	}

	// DescribeDomains は 1 回に 5 ドメインまでしか受け付けないため分割して呼ぶ。
	// レスポンスの順序は API に保証が無いので、最後に Name で整列する。
	describeStart := time.Now()
	var statuses []opensearchtypes.DomainStatus
	for start := 0; start < len(names); start += openSearchDescribeBatchSize {
		end := min(start+openSearchDescribeBatchSize, len(names))
		batch := names[start:end]
		described, err := client.DescribeDomains(ctx, &opensearch.DescribeDomainsInput{DomainNames: batch})
		if err != nil {
			return nil, fmt.Errorf("describe opensearch domains %v: %w", batch, err)
		}
		if described != nil {
			statuses = append(statuses, described.DomainStatusList...)
		}
	}
	slog.Debug("opensearch describe domains done",
		"profile", profile, "region", region,
		"duration_ms", time.Since(describeStart).Milliseconds(), "count", len(statuses))

	// ドメインごとのタグ取得は互いに独立しているため並列実行する。各 goroutine は
	// 自分の index にのみ書き込むため結果スライスへの書き込みはロック不要で競合しない
	// (データオーナーシップを goroutine ごとに分離、ListSQSResources と同型)。
	tagStart := time.Now()
	resources := make([]OpenSearchResource, len(statuses))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(openSearchTagConcurrency)
	for i, status := range statuses {
		g.Go(func() error {
			tags, tagsFetchFailed, err := fetchOpenSearchTags(gctx, client, ptrStr(status.ARN))
			if err != nil {
				return err
			}
			resources[i] = openSearchFromStatus(status, tags, tagsFetchFailed)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	slog.Debug("opensearch list tags done",
		"profile", profile, "region", region,
		"duration_ms", time.Since(tagStart).Milliseconds(),
		"concurrency", openSearchTagConcurrency, "count", len(resources))

	sort.Slice(resources, func(i, j int) bool {
		if resources[i].Name == resources[j].Name {
			return resources[i].ID < resources[j].ID
		}
		return resources[i].Name < resources[j].Name
	})
	return resources, nil
}

// openSearchDomainNames は ListDomainNames の応答から空の名前と重複を除いたドメイン名を返す。
// ListDomainNames はページを持たないため 1 回の応答で全件を受け取る。
func openSearchDomainNames(out *opensearch.ListDomainNamesOutput) []string {
	if out == nil {
		return nil
	}
	names := make([]string, 0, len(out.DomainNames))
	seen := make(map[string]struct{}, len(out.DomainNames))
	for _, d := range out.DomainNames {
		name := ptrStr(d.DomainName)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// fetchOpenSearchTags はドメインのタグを取得する。取得に失敗した場合は空 map と
// tagsFetchFailed=true を返す (キャンセル起因は全体エラーとして伝播)。
func fetchOpenSearchTags(ctx context.Context, client openSearchListClient, arn string) (map[string]string, bool, error) {
	tags := map[string]string{}
	if arn == "" {
		return tags, false, nil
	}
	out, tagErr := client.ListTags(ctx, &opensearch.ListTagsInput{ARN: aws.String(arn)})
	if tagErr == nil && out != nil {
		return tagsToMapFunc(out.TagList, func(t opensearchtypes.Tag) (*string, *string) {
			return t.Key, t.Value
		}), false, nil
	}
	degraded, err := handleIgnoredErrFlag(tagErr, "list opensearch domain tags failed (ignored)", "domain_arn", arn)
	if err != nil {
		return nil, false, err
	}
	tagsFetchFailed := degraded || tagErr == nil
	return tags, tagsFetchFailed, nil
}

// openSearchFromStatus は SDK の DomainStatus を OpenSearchResource へ変換する。
// 一覧に直接使わない項目 (VPC、暗号化、HTTPS) は Drawer の Overview 用に保持する。
func openSearchFromStatus(s opensearchtypes.DomainStatus, tags map[string]string, tagsFetchFailed bool) OpenSearchResource {
	r := OpenSearchResource{
		ID:              ptrStr(s.ARN),
		Name:            ptrStr(s.DomainName),
		State:           openSearchState(s),
		EngineVersion:   ptrStr(s.EngineVersion),
		Endpoint:        ptrStr(s.Endpoint),
		Tags:            tags,
		TagsFetchFailed: tagsFetchFailed,
	}
	if s.ClusterConfig != nil {
		r.DataNodeType = string(s.ClusterConfig.InstanceType)
		r.DataNodeCount = ptrInt32(s.ClusterConfig.InstanceCount)
		r.DedicatedMasterEnabled = ptrBool(s.ClusterConfig.DedicatedMasterEnabled)
		r.DedicatedMasterType = string(s.ClusterConfig.DedicatedMasterType)
		r.DedicatedMasterCount = ptrInt32(s.ClusterConfig.DedicatedMasterCount)
	}
	if s.EBSOptions != nil {
		r.EBSVolumeSizeGB = ptrInt32(s.EBSOptions.VolumeSize)
		r.EBSVolumeType = string(s.EBSOptions.VolumeType)
	}
	// VPC ドメインは Endpoint が空で、Endpoints にだけ接続先が入る。dual stack の
	// VPC ドメインは "vpcv2" キーだけを持つため、両方を順に確認する。
	if r.Endpoint == "" {
		r.Endpoint = ptrStr(s.EndpointV2)
	}
	if r.Endpoint == "" {
		r.Endpoint = s.Endpoints["vpc"]
	}
	if r.Endpoint == "" {
		r.Endpoint = s.Endpoints["vpcv2"]
	}
	if s.VPCOptions != nil {
		r.VPCID = ptrStr(s.VPCOptions.VPCId)
	}
	if s.EncryptionAtRestOptions != nil {
		r.EncryptionAtRest = ptrBool(s.EncryptionAtRestOptions.Enabled)
	}
	if s.NodeToNodeEncryptionOptions != nil {
		r.NodeToNodeEncryption = ptrBool(s.NodeToNodeEncryptionOptions.Enabled)
	}
	if s.DomainEndpointOptions != nil {
		r.EnforceHTTPS = ptrBool(s.DomainEndpointOptions.EnforceHTTPS)
	}
	return r
}

// openSearchState は DomainStatus の処理状態を Web UI の StatusBadge が解釈できる値へ変換する。
// DomainProcessingStatus が空 (古い API 応答や floci など) のときは作成・削除・構成変更の
// フラグで判断する。未知の enum 値は DisplayState で小文字・ハイフンに正規化してそのまま返す。
func openSearchState(s opensearchtypes.DomainStatus) string {
	switch s.DomainProcessingStatus {
	case opensearchtypes.DomainProcessingStatusTypeCreating:
		return "creating"
	case opensearchtypes.DomainProcessingStatusTypeActive:
		return "active"
	case opensearchtypes.DomainProcessingStatusTypeModifying,
		opensearchtypes.DomainProcessingStatusTypeUpgrading,
		opensearchtypes.DomainProcessingStatusTypeUpdating:
		return "updating"
	case opensearchtypes.DomainProcessingStatusTypeIsolated:
		return "error"
	case opensearchtypes.DomainProcessingStatusTypeDeleting:
		return "deleting"
	case "":
		switch {
		case ptrBool(s.Deleted):
			return "deleting"
		case s.Created != nil && !ptrBool(s.Created):
			return "creating"
		case ptrBool(s.Processing), ptrBool(s.UpgradeProcessing):
			return "updating"
		default:
			return "active"
		}
	default:
		return DisplayState(string(s.DomainProcessingStatus))
	}
}

// newOpenSearchClient は OpenSearch API クライアントを生成する。
func newOpenSearchClient(ctx context.Context, profile, region string) (*opensearch.Client, error) {
	return NewClient(ctx, profile, region, func(cfg aws.Config) *opensearch.Client {
		return opensearch.NewFromConfig(cfg)
	})
}

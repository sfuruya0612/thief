package aws

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/aws/aws-sdk-go-v2/service/wafv2"
	waftypes "github.com/aws/aws-sdk-go-v2/service/wafv2/types"
	"golang.org/x/sync/errgroup"
)

// WAFResource represents a WAFv2 Web ACL.
type WAFResource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// ARN は CLOUDFRONT スコープの Associated 集計 (cloudFrontACLCounts /
	// applyCloudFrontCounts) でのみ使う内部フィールドで、API レスポンスには含めない。
	ARN                        string            `json:"-"`
	State                      string            `json:"state"`
	Scope                      string            `json:"scope"`
	Description                string            `json:"description"`
	RuleCount                  int               `json:"rule_count"`
	AssociatedCount            int               `json:"associated_count"`
	AssociatedCountFetchFailed bool              `json:"associated_count_fetch_failed,omitempty"`
	Tags                       map[string]string `json:"tags"`
	TagsFetchFailed            bool              `json:"tags_fetch_failed,omitempty"`
	CostMonthly                float64           `json:"cost_monthly"`
}

func (r WAFResource) ResourceID() string    { return r.ID }
func (r WAFResource) ResourceName() string  { return r.Name }
func (r WAFResource) ResourceState() string { return NormalizeState(r.State) }
func (r WAFResource) ServiceName() string   { return "waf" }

// wafACLConcurrency は Web ACL ごとの詳細取得 (GetWebACL / ListResourcesForWebACL /
// ListTagsForResource) を同時実行する上限数。wafv2 のマネジメント系 API は他サービスより
// レート制限が厳しいため控えめに始める (issue 0081 の設計判断)。
const wafACLConcurrency = 10

// ListWAFResources returns all WAFv2 Web ACLs (REGIONAL for the given region and
// CLOUDFRONT scope from us-east-1) for the given profile.
func ListWAFResources(ctx context.Context, profile, region string) ([]WAFResource, error) {
	// 各フェーズの所要時間を計測してログに残す (issue 0081: クライアント生成の区間は
	// issue 0083 の GetSession キャッシュ化調査の入力を兼ねる)。
	overallStart := time.Now()

	clientStart := time.Now()
	regionalClient, err := newWAFClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	// CloudFront スコープは us-east-1 必須
	globalClient, err := newWAFClient(ctx, profile, "us-east-1")
	if err != nil {
		return nil, err
	}
	slog.Info("waf clients created",
		"profile", profile, "region", region, "duration_ms", time.Since(clientStart).Milliseconds())

	// REGIONAL と CLOUDFRONT の 2 スコープは互いに独立しているため並列に取得する。
	// 各 goroutine は自分のスコープの変数にのみ書き込むため競合しない。
	var regionalACLs, cfACLs []WAFResource
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		acls, err := listWAFACLs(gctx, regionalClient, waftypes.ScopeRegional)
		if err != nil {
			return err
		}
		regionalACLs = acls
		return nil
	})
	g.Go(func() error {
		acls, err := listWAFACLs(gctx, globalClient, waftypes.ScopeCloudfront)
		if err != nil {
			return err
		}
		if len(acls) > 0 {
			degraded, err := applyCloudFrontAssociatedCounts(gctx, profile, acls)
			if err != nil {
				return err
			}
			if degraded {
				markCloudFrontAssociatedCountFetchFailed(acls)
			}
		}
		cfACLs = acls
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// 返却順は並列化前と同じ REGIONAL → CLOUDFRONT に保つ
	resources := make([]WAFResource, 0, len(regionalACLs)+len(cfACLs))
	resources = append(resources, regionalACLs...)
	resources = append(resources, cfACLs...)

	slog.Info("waf list all done",
		"profile", profile, "region", region,
		"duration_ms", time.Since(overallStart).Milliseconds(), "count", len(resources))
	return resources, nil
}

func listWAFACLs(ctx context.Context, client *wafv2.Client, scope waftypes.Scope) ([]WAFResource, error) {
	listStart := time.Now()
	var summaries []waftypes.WebACLSummary
	var nextMarker *string
	for {
		out, err := client.ListWebACLs(ctx, &wafv2.ListWebACLsInput{
			Scope:      scope,
			NextMarker: nextMarker,
		})
		if err != nil {
			return nil, fmt.Errorf("list web acls scope=%s: %w", scope, err)
		}
		summaries = append(summaries, out.WebACLs...)
		if out.NextMarker == nil || *out.NextMarker == "" {
			break
		}
		nextMarker = out.NextMarker
	}
	slog.Info("waf list web acls done",
		"scope", string(scope), "duration_ms", time.Since(listStart).Milliseconds(), "count", len(summaries))

	// Web ACL ごとの詳細取得は互いに独立しているため並列実行する。各 goroutine は
	// 自分の index にのみ書き込むため結果スライスへの書き込みはロック不要で競合しない
	// (データオーナーシップを goroutine ごとに分離、ListS3Resources と同型)。
	detailStart := time.Now()
	resources := make([]WAFResource, len(summaries))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(wafACLConcurrency)
	for i, s := range summaries {
		g.Go(func() error {
			acl, err := client.GetWebACL(gctx, &wafv2.GetWebACLInput{
				Id:    s.Id,
				Name:  s.Name,
				Scope: scope,
			})
			if err != nil {
				return fmt.Errorf("get web acl %s: %w", ptrStr(s.Id), err)
			}
			ruleCount := 0
			if acl.WebACL != nil {
				ruleCount = len(acl.WebACL.Rules)
			}
			resources[i], err = wafACLDetail(gctx, client, scope, s, ruleCount)
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	slog.Info("waf get web acl details done",
		"scope", string(scope), "duration_ms", time.Since(detailStart).Milliseconds(),
		"concurrency", wafACLConcurrency, "count", len(resources))
	return resources, nil
}

// wafACLDetailClient は wafACLDetail が必要とする wafv2 API を narrow interface
// として切り出したもの (テストで手書きモックに差し替えるため)。
type wafACLDetailClient interface {
	ListResourcesForWebACL(ctx context.Context, params *wafv2.ListResourcesForWebACLInput, optFns ...func(*wafv2.Options)) (*wafv2.ListResourcesForWebACLOutput, error)
	ListTagsForResource(ctx context.Context, params *wafv2.ListTagsForResourceInput, optFns ...func(*wafv2.Options)) (*wafv2.ListTagsForResourceOutput, error)
}

// wafACLDetail は Web ACL 1 件分の関連リソース件数とタグを取得し WAFResource に
// 変換する。いずれの取得も失敗を無視するため、error はキャンセル起因の失敗
// (handleIgnoredErrFlag が伝播させるもの) に限られる。
func wafACLDetail(ctx context.Context, client wafACLDetailClient, scope waftypes.Scope, s waftypes.WebACLSummary, ruleCount int) (WAFResource, error) {
	// ListResourcesForWebACL は REGIONAL でのみ有効。ResourceType 省略は
	// APPLICATION_LOAD_BALANCER 扱いになり ALB 以外の関連が数えられないため、
	// 既知の全種別についてそれぞれ呼んで合算する。失敗しても ACL 情報は返す
	// (キャンセル起因は全体エラーとして伝播)
	associatedCount := 0
	var associatedCountFetchFailed bool
	if scope == waftypes.ScopeRegional {
		resourceTypes := wafRegionalResourceTypes()
		arnLists := make([][]string, 0, len(resourceTypes))
		for _, rt := range resourceTypes {
			resOut, resErr := client.ListResourcesForWebACL(ctx, &wafv2.ListResourcesForWebACLInput{
				WebACLArn:    s.ARN,
				ResourceType: rt,
			})
			if resErr == nil && resOut != nil {
				arnLists = append(arnLists, resOut.ResourceArns)
				continue
			}
			degraded, err := handleIgnoredErrFlag(resErr, "list resources for web acl failed (ignored)", "web_acl_arn", ptrStr(s.ARN), "resource_type", string(rt))
			if err != nil {
				return WAFResource{}, err
			}
			arnLists = append(arnLists, nil)
			// resErr == nil の項は「エラーは無いが resOut が nil で成功分岐に入れなかった」
			// ケース (SDK が呼び出し自体は成功させつつ nil を返す想定外の応答) を縮退として
			// 扱うためのもの。handleIgnoredErrFlag の degraded だけでは resErr が nil の場合に
			// 常に false を返すため拾えない。
			associatedCountFetchFailed = associatedCountFetchFailed || degraded || resErr == nil
		}
		associatedCount = sumResourceARNs(arnLists)
	}
	// タグ取得も失敗を無視する (キャンセル起因は全体エラーとして伝播)
	tags := map[string]string{}
	var tagsFetchFailed bool
	tagsOut, tagErr := client.ListTagsForResource(ctx, &wafv2.ListTagsForResourceInput{
		ResourceARN: s.ARN,
	})
	if tagErr == nil && tagsOut != nil && tagsOut.TagInfoForResource != nil {
		tags = tagsToMapFunc(tagsOut.TagInfoForResource.TagList, func(t waftypes.Tag) (*string, *string) { return t.Key, t.Value })
	} else {
		degraded, err := handleIgnoredErrFlag(tagErr, "list tags for web acl failed (ignored)", "web_acl", ptrStr(s.Name), "web_acl_arn", ptrStr(s.ARN))
		if err != nil {
			return WAFResource{}, err
		}
		// tagErr == nil の項は「エラーは無いが tagsOut または TagInfoForResource が nil で
		// 成功分岐に入れなかった」ケースを縮退として扱うためのもの。resErr == nil と同じ理由。
		tagsFetchFailed = degraded || tagErr == nil
	}
	return newWAFResource(ptrStr(s.Id), ptrStr(s.Name), ptrStr(s.ARN), scope, ruleCount, associatedCount, associatedCountFetchFailed, tags, tagsFetchFailed, s.Description), nil
}

func newWAFResource(id, name, arn string, scope waftypes.Scope, ruleCount, associatedCount int, associatedCountFetchFailed bool, tags map[string]string, tagsFetchFailed bool, description *string) WAFResource {
	return WAFResource{
		ID:                         id,
		Name:                       name,
		ARN:                        arn,
		State:                      "active",
		Scope:                      string(scope),
		Description:                ptrStr(description),
		RuleCount:                  ruleCount,
		AssociatedCount:            associatedCount,
		AssociatedCountFetchFailed: associatedCountFetchFailed,
		Tags:                       tags,
		TagsFetchFailed:            tagsFetchFailed,
	}
}

// wafRegionalResourceTypes は ListResourcesForWebACL で REGIONAL スコープの関連
// リソースを数える対象の全種別を返す。SDK が既知の種別として持つものをそのまま
// 使うため、SDK 更新で種別が追加されれば実装が自動で追随する。
func wafRegionalResourceTypes() []waftypes.ResourceType {
	return waftypes.ResourceType("").Values()
}

// sumResourceARNs は種別ごとの ListResourcesForWebACL 結果 (ResourceArns) を
// 合算する。取得に失敗した種別は nil 要素として渡され、0 件として扱われる。
func sumResourceARNs(arnLists [][]string) int {
	total := 0
	for _, arns := range arnLists {
		total += len(arns)
	}
	return total
}

// applyCloudFrontAssociatedCounts は CLOUDFRONT スコープの ACL 群に CloudFront
// ディストリビューションの関連付け件数を適用する。ディストリビューション一覧の
// 取得に失敗した場合は部分集計を適用せず、Warn ログと全 ACL 0 表示に劣化させる
// (キャンセル起因は全体エラーとして伝播)。degraded は縮退が発生したかを示す。
func applyCloudFrontAssociatedCounts(ctx context.Context, profile string, acls []WAFResource) (bool, error) {
	client, err := newCloudFrontClient(ctx, profile)
	if err != nil {
		return handleIgnoredErrFlag(err, "create cloudfront client for waf associated count failed (ignored)", "profile", profile)
	}
	return applyCloudFrontAssociatedCountsWithClient(ctx, client, profile, acls)
}

// markCloudFrontAssociatedCountFetchFailed は applyCloudFrontAssociatedCounts が
// degraded を返したときに、CLOUDFRONT スコープの全 ACL の AssociatedCountFetchFailed
// を true にする。
func markCloudFrontAssociatedCountFetchFailed(acls []WAFResource) {
	for i := range acls {
		acls[i].AssociatedCountFetchFailed = true
	}
}

// cloudFrontDistributionLister は applyCloudFrontAssociatedCountsWithClient が
// 必要とする cloudfront API を narrow interface として切り出したもの
// (テストで手書きモックに差し替えるため)。
type cloudFrontDistributionLister interface {
	ListDistributions(ctx context.Context, params *cloudfront.ListDistributionsInput, optFns ...func(*cloudfront.Options)) (*cloudfront.ListDistributionsOutput, error)
}

// applyCloudFrontAssociatedCountsWithClient は applyCloudFrontAssociatedCounts の
// クライアント生成後の本体。degraded はディストリビューション一覧の取得に失敗し、
// 部分集計を適用せず縮退した (Warn ログを出して続行した) かを示す。
func applyCloudFrontAssociatedCountsWithClient(ctx context.Context, client cloudFrontDistributionLister, profile string, acls []WAFResource) (bool, error) {
	var summaries []cftypes.DistributionSummary
	paginator := cloudfront.NewListDistributionsPaginator(client, &cloudfront.ListDistributionsInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return handleIgnoredErrFlag(err, "list cloudfront distributions for waf associated count failed (ignored)", "profile", profile)
		}
		if page.DistributionList == nil {
			continue
		}
		summaries = append(summaries, page.DistributionList.Items...)
	}
	applyCloudFrontCounts(acls, cloudFrontACLCounts(summaries))
	return false, nil
}

// cloudFrontACLCounts は CloudFront ディストリビューション一覧から、Web ACL の
// ARN ごとの関連付け件数を集計する。WebACLId が nil または空文字のディストリ
// ビューションは除外する。
func cloudFrontACLCounts(summaries []cftypes.DistributionSummary) map[string]int {
	counts := make(map[string]int)
	for _, s := range summaries {
		if s.WebACLId == nil || *s.WebACLId == "" {
			continue
		}
		counts[*s.WebACLId]++
	}
	return counts
}

// applyCloudFrontCounts は ACL の ARN で counts を引き、AssociatedCount に
// 適用する。counts に存在しない ARN の ACL は 0 のままになる。
func applyCloudFrontCounts(acls []WAFResource, counts map[string]int) {
	for i := range acls {
		acls[i].AssociatedCount = counts[acls[i].ARN]
	}
}

// WAFRule represents a rule in a WAFv2 Web ACL.
type WAFRule struct {
	Name      string `json:"name"`
	Priority  int32  `json:"priority"`
	Action    string `json:"action"`
	Statement string `json:"statement"`
	// RuleJSON はルール定義全体 (Statement, Action, OverrideAction,
	// VisibilityConfig, RuleLabels, CaptchaConfig, ChallengeConfig 等) を
	// null 値を除去した JSON 文字列にしたもの。wafRuleJSON が生成する。
	RuleJSON string `json:"rule_json"`
}

// ListWAFRules returns the rules of the Web ACL identified by name + id + scope,
// sorted by priority ascending. scope must be REGIONAL or CLOUDFRONT (validated
// by the caller).
func ListWAFRules(ctx context.Context, profile, region, scope, name, id string) ([]WAFRule, error) {
	sc := waftypes.Scope(scope)
	client, err := newWAFClient(ctx, profile, wafClientRegion(sc, region))
	if err != nil {
		return nil, err
	}
	out, err := client.GetWebACL(ctx, &wafv2.GetWebACLInput{
		Id:    aws.String(id),
		Name:  aws.String(name),
		Scope: sc,
	})
	if err != nil {
		return nil, fmt.Errorf("get web acl %s: %w", id, err)
	}
	rules := []WAFRule{}
	if out.WebACL != nil {
		for _, r := range out.WebACL.Rules {
			rules = append(rules, newWAFRule(r))
		}
	}
	sortWAFRules(rules)
	return rules, nil
}

// wafClientRegion は wafv2 クライアントに使うリージョンを返す。
// CLOUDFRONT スコープは us-east-1 必須 (ListWAFResources と同じ扱い)。
func wafClientRegion(scope waftypes.Scope, region string) string {
	if scope == waftypes.ScopeCloudfront {
		return "us-east-1"
	}
	return region
}

// newWAFRule は SDK の Rule を WAFRule へ変換する。
func newWAFRule(r waftypes.Rule) WAFRule {
	return WAFRule{
		Name:      ptrStr(r.Name),
		Priority:  r.Priority,
		Action:    wafRuleAction(r),
		Statement: wafRuleStatement(r.Statement),
		RuleJSON:  wafRuleJSON(r),
	}
}

// wafRuleJSON は Rule 全体を JSON 文字列にする。SDK 構造体をそのまま Marshal すると
// 未設定の nil フィールドが null として大量に出力されるため、一度 map[string]any に
// 落として null 値のみを再帰的に除去してから再度 Marshal する。null 除去の結果空に
// なったオブジェクト (例: Action.Allow の {}) は WAFv2 API の JSON 仕様上の判別子
// (NoneAction, UriPath 等) であるため保持し、配列の要素も取り除かない。
// 数値は json.Decoder の UseNumber() で元の表記のまま保つ。Marshal に失敗した場合は
// 空文字を返す (1 ルールの変換失敗で一覧全体を落とさない)。
func wafRuleJSON(r waftypes.Rule) string {
	b, err := json.Marshal(r)
	if err != nil {
		return ""
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var decoded any
	if err := dec.Decode(&decoded); err != nil {
		return ""
	}
	out, err := json.Marshal(removeJSONNulls(decoded))
	if err != nil {
		return ""
	}
	return string(out)
}

// removeJSONNulls は decode 済みの JSON 値からオブジェクトのキーのうち値が null の
// ものを再帰的に取り除く。null 除去の結果空になったオブジェクトや、元から空の
// オブジェクト・配列はそのまま保持する (WAFv2 API では空オブジェクトであること
// 自体が設定の判別子になっているため)。
func removeJSONNulls(v any) any {
	switch val := v.(type) {
	case map[string]any:
		for k, vv := range val {
			if vv == nil {
				delete(val, k)
				continue
			}
			val[k] = removeJSONNulls(vv)
		}
		return val
	case []any:
		for i, vv := range val {
			val[i] = removeJSONNulls(vv)
		}
		return val
	default:
		return val
	}
}

// wafRuleAction は Rule のアクションを表示用文字列へ要約する。
// Action は非 nil フィールド名、ルールグループ参照の OverrideAction は
// "Override: None" / "Override: Count" とする。SDK 更新で増えたフィールドや
// どちらも nil の場合は空文字に落ちる。
func wafRuleAction(r waftypes.Rule) string {
	if a := r.Action; a != nil {
		switch {
		case a.Allow != nil:
			return "Allow"
		case a.Block != nil:
			return "Block"
		case a.Count != nil:
			return "Count"
		case a.Captcha != nil:
			return "Captcha"
		case a.Challenge != nil:
			return "Challenge"
		case a.Monetize != nil:
			return "Monetize"
		}
		return ""
	}
	if o := r.OverrideAction; o != nil {
		switch {
		case o.None != nil:
			return "Override: None"
		case o.Count != nil:
			return "Override: Count"
		}
	}
	return ""
}

// wafRuleStatement は Statement の非 nil フィールドから種別名を導出する。
// ManagedRuleGroupStatement は VendorName/Name、RuleGroupReferenceStatement は
// ARN の末尾セグメント、And/Or/Not は入れ子を展開せず AND/OR/NOT とする。
// どのフィールドも非 nil でない場合は Unknown を返す (SDK 更新で種別が増えた場合の既定値)。
func wafRuleStatement(st *waftypes.Statement) string {
	if st == nil {
		return "Unknown"
	}
	switch {
	case st.ManagedRuleGroupStatement != nil:
		return ptrStr(st.ManagedRuleGroupStatement.VendorName) + "/" + ptrStr(st.ManagedRuleGroupStatement.Name)
	case st.RuleGroupReferenceStatement != nil:
		return arnLastSegment(ptrStr(st.RuleGroupReferenceStatement.ARN))
	case st.AndStatement != nil:
		return "AND"
	case st.OrStatement != nil:
		return "OR"
	case st.NotStatement != nil:
		return "NOT"
	case st.AsnMatchStatement != nil:
		return "AsnMatch"
	case st.ByteMatchStatement != nil:
		return "ByteMatch"
	case st.GeoMatchStatement != nil:
		return "GeoMatch"
	case st.IPSetReferenceStatement != nil:
		return "IPSetReference"
	case st.LabelMatchStatement != nil:
		return "LabelMatch"
	case st.RateBasedStatement != nil:
		return "RateBased"
	case st.RegexMatchStatement != nil:
		return "RegexMatch"
	case st.RegexPatternSetReferenceStatement != nil:
		return "RegexPatternSetReference"
	case st.SizeConstraintStatement != nil:
		return "SizeConstraint"
	case st.SqliMatchStatement != nil:
		return "SqliMatch"
	case st.XssMatchStatement != nil:
		return "XssMatch"
	}
	return "Unknown"
}

// arnLastSegment は ARN の末尾セグメント (最後の "/" 以降) を返す。
// "/" を含まない場合は入力をそのまま返す。
func arnLastSegment(arn string) string {
	if i := strings.LastIndex(arn, "/"); i >= 0 {
		return arn[i+1:]
	}
	return arn
}

// sortWAFRules は priority 昇順に整列する。GetWebACL のレスポンス配列の順序は
// SDK に文書化されていないため明示的に整列する (Priority は Web ACL 内で一意)。
func sortWAFRules(rules []WAFRule) {
	slices.SortFunc(rules, func(a, b WAFRule) int {
		return int(a.Priority) - int(b.Priority)
	})
}

// newWAFClient は WAFv2 API クライアントを生成する。
func newWAFClient(ctx context.Context, profile, region string) (*wafv2.Client, error) {
	return NewClient(ctx, profile, region, func(cfg aws.Config) *wafv2.Client {
		return wafv2.NewFromConfig(cfg)
	})
}

package aws

import (
	"context"
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

// RegionResource は AWS リージョンのコードと表示名を表す。
type RegionResource struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

func (r RegionResource) ResourceID() string    { return r.Code }
func (r RegionResource) ResourceName() string  { return r.Name }
func (r RegionResource) ResourceState() string { return "" }
func (r RegionResource) ServiceName() string   { return "regions" }

// regionNames は AWS 公式のリージョン表示名 (英語) 静的マップ。
// 未知コードは regionResourceFromCode 側で Name=Code にフォールバックする。
var regionNames = map[string]string{
	"us-east-1":      "US East (N. Virginia)",
	"us-east-2":      "US East (Ohio)",
	"us-west-1":      "US West (N. California)",
	"us-west-2":      "US West (Oregon)",
	"af-south-1":     "Africa (Cape Town)",
	"ap-east-1":      "Asia Pacific (Hong Kong)",
	"ap-south-1":     "Asia Pacific (Mumbai)",
	"ap-south-2":     "Asia Pacific (Hyderabad)",
	"ap-southeast-1": "Asia Pacific (Singapore)",
	"ap-southeast-2": "Asia Pacific (Sydney)",
	"ap-southeast-3": "Asia Pacific (Jakarta)",
	"ap-southeast-4": "Asia Pacific (Melbourne)",
	"ap-southeast-5": "Asia Pacific (Malaysia)",
	"ap-southeast-7": "Asia Pacific (Thailand)",
	"ap-northeast-1": "Asia Pacific (Tokyo)",
	"ap-northeast-2": "Asia Pacific (Seoul)",
	"ap-northeast-3": "Asia Pacific (Osaka)",
	"ca-central-1":   "Canada (Central)",
	"ca-west-1":      "Canada West (Calgary)",
	"eu-central-1":   "Europe (Frankfurt)",
	"eu-central-2":   "Europe (Zurich)",
	"eu-west-1":      "Europe (Ireland)",
	"eu-west-2":      "Europe (London)",
	"eu-west-3":      "Europe (Paris)",
	"eu-south-1":     "Europe (Milan)",
	"eu-south-2":     "Europe (Spain)",
	"eu-north-1":     "Europe (Stockholm)",
	"il-central-1":   "Israel (Tel Aviv)",
	"me-central-1":   "Middle East (UAE)",
	"me-south-1":     "Middle East (Bahrain)",
	"mx-central-1":   "Mexico (Central)",
	"sa-east-1":      "South America (Sao Paulo)",
}

// regionResourceFromCode はリージョンコードから RegionResource を生成する。
// 未知コードは Name=Code にフォールバックする。
func regionResourceFromCode(code string) RegionResource {
	name, ok := regionNames[code]
	if !ok {
		name = code
	}
	return RegionResource{Code: code, Name: name}
}

// ListRegions は有効化済みの AWS リージョン一覧を返す。
// DescribeRegions は us-east-1 固定で呼び出す (どのリージョンでも同結果を返すが、
// プロファイルのデフォルトリージョン未設定でも動く汎用な選択として)。
func ListRegions(ctx context.Context, profile string) ([]RegionResource, error) {
	client, err := NewClient(ctx, profile, "us-east-1", func(cfg aws.Config) *ec2.Client {
		return ec2.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	out, err := client.DescribeRegions(ctx, &ec2.DescribeRegionsInput{})
	if err != nil {
		return nil, fmt.Errorf("describe regions: %w", err)
	}

	resources := make([]RegionResource, 0, len(out.Regions))
	for _, r := range out.Regions {
		code := ptrStr(r.RegionName)
		if code == "" {
			continue
		}
		resources = append(resources, regionResourceFromCode(code))
	}
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].Code < resources[j].Code
	})
	return resources, nil
}

package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// ssmGetParametersBatchSize is the max number of names GetParameters accepts per call.
const ssmGetParametersBatchSize = 10

// SSMParameterResource represents an SSM Parameter Store entry.
//
// Value は一覧レスポンスに復号済みの値をそのまま含める (SecureString も WithDecryption=true で復号する)。
// この方針により機密値が backend の 1 時間キャッシュ (cacheTTL) とフロントの react-query キャッシュに
// 平文で載ることを許容している。slog には Value を渡さないこと。
type SSMParameterResource struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	State        string `json:"state"`
	Type         string `json:"type"`
	Tier         string `json:"tier"`
	Version      int64  `json:"version"`
	LastModified string `json:"last_modified"`
	Value        string `json:"value"`
}

func (r SSMParameterResource) ResourceID() string    { return r.ID }
func (r SSMParameterResource) ResourceName() string  { return r.Name }
func (r SSMParameterResource) ResourceState() string { return "active" }
func (r SSMParameterResource) ServiceName() string   { return "ssm" }

// ListSSMParameters returns all SSM Parameter Store parameters for the given profile/region.
// 一覧には復号済みの値を含める (GetParameters を最大 10 件ずつのバッチで呼び出す)。
func ListSSMParameters(ctx context.Context, profile, region string) ([]SSMParameterResource, error) {
	client, err := NewClient(ctx, profile, region, func(cfg aws.Config) *ssm.Client {
		return ssm.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	var resources []SSMParameterResource
	paginator := ssm.NewDescribeParametersPaginator(client, &ssm.DescribeParametersInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe ssm parameters: %w", err)
		}
		for _, p := range page.Parameters {
			resources = append(resources, ssmFromMeta(p))
		}
	}

	if err := fillSSMValues(ctx, client, resources); err != nil {
		return nil, err
	}
	return resources, nil
}

// fillSSMValues populates Value on each resource by batching GetParameters calls
// (max ssmGetParametersBatchSize names per call), with WithDecryption=true so
// SecureString parameters are returned in plaintext.
func fillSSMValues(ctx context.Context, client *ssm.Client, resources []SSMParameterResource) error {
	byName := make(map[string]int, len(resources))
	for i, r := range resources {
		byName[r.Name] = i
	}

	for start := 0; start < len(resources); start += ssmGetParametersBatchSize {
		end := min(start+ssmGetParametersBatchSize, len(resources))
		names := make([]string, 0, end-start)
		for _, r := range resources[start:end] {
			names = append(names, r.Name)
		}

		out, err := client.GetParameters(ctx, &ssm.GetParametersInput{
			Names:          names,
			WithDecryption: aws.Bool(true),
		})
		if err != nil {
			return fmt.Errorf("get ssm parameters batch: %w", err)
		}
		for _, p := range out.Parameters {
			if idx, ok := byName[ptrStr(p.Name)]; ok {
				resources[idx].Value = ptrStr(p.Value)
			}
		}
	}
	return nil
}

// GetSSMParameter fetches the value of a single SSM parameter.
// Set decrypt=true for SecureString parameters.
func GetSSMParameter(ctx context.Context, profile, region, name string, decrypt bool) (string, error) {
	client, err := NewClient(ctx, profile, region, func(cfg aws.Config) *ssm.Client {
		return ssm.NewFromConfig(cfg)
	})
	if err != nil {
		return "", err
	}
	out, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(name),
		WithDecryption: aws.Bool(decrypt),
	})
	if err != nil {
		return "", fmt.Errorf("get ssm parameter %s: %w", name, err)
	}
	if out.Parameter == nil {
		return "", nil
	}
	return ptrStr(out.Parameter.Value), nil
}

// SSMParameterInfo はレガシー CLI 互換の SSM パラメータ一覧表示用フィールドを保持する。
// SSMParameterResource と異なり値を含まない (DescribeParameters のメタデータのみ)。
type SSMParameterInfo struct {
	Name             string
	Type             string
	LastModifiedDate string
	Version          int64
	DataType         string
}

// ToRow converts SSMParameterInfo to a string slice for table output.
func (p SSMParameterInfo) ToRow() []string {
	return []string{
		p.Name,
		p.Type,
		p.LastModifiedDate,
		fmt.Sprintf("%d", p.Version),
		p.DataType,
	}
}

// SSMParameterValue は GetParameter が返す単一パラメータの値と属性を保持する。
type SSMParameterValue struct {
	Name    string
	Type    string
	Value   string
	Version int64
	ARN     string
}

// ToRow converts SSMParameterValue to a string slice for table output.
func (p SSMParameterValue) ToRow() []string {
	return []string{
		p.Name,
		p.Type,
		p.Value,
		fmt.Sprintf("%d", p.Version),
		p.ARN,
	}
}

// ListSSMParameterInfos は SSM パラメータのメタデータ一覧を返す。
// path が非空のときは名前の前方一致 (BeginsWith) で絞り込む。値は取得しない。
func ListSSMParameterInfos(ctx context.Context, profile, region, path string) ([]SSMParameterInfo, error) {
	client, err := NewClient(ctx, profile, region, func(cfg aws.Config) *ssm.Client {
		return ssm.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	input := &ssm.DescribeParametersInput{}
	if path != "" {
		input.ParameterFilters = []ssmtypes.ParameterStringFilter{
			{
				Key:    aws.String("Name"),
				Option: aws.String("BeginsWith"),
				Values: []string{path},
			},
		}
	}

	var params []SSMParameterInfo
	paginator := ssm.NewDescribeParametersPaginator(client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to describe parameters: %w", err)
		}
		for _, p := range page.Parameters {
			lastModified := ""
			if p.LastModifiedDate != nil {
				lastModified = p.LastModifiedDate.Format("2006-01-02 15:04:05")
			}
			params = append(params, SSMParameterInfo{
				Name:             ptrStr(p.Name),
				Type:             string(p.Type),
				LastModifiedDate: lastModified,
				Version:          p.Version,
				DataType:         ptrStr(p.DataType),
			})
		}
	}
	return params, nil
}

// GetSSMParameterDetail は単一 SSM パラメータの値と属性 (Name/Type/Value/Version/ARN) を返す。
// withDecryption が true のとき SecureString を復号する。
func GetSSMParameterDetail(ctx context.Context, profile, region, name string, withDecryption bool) (*SSMParameterValue, error) {
	client, err := NewClient(ctx, profile, region, func(cfg aws.Config) *ssm.Client {
		return ssm.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	out, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(name),
		WithDecryption: aws.Bool(withDecryption),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter %s: %w", name, err)
	}

	p := out.Parameter
	if p == nil {
		return nil, fmt.Errorf("failed to get parameter %s: empty response", name)
	}

	return &SSMParameterValue{
		Name:    ptrStr(p.Name),
		Type:    string(p.Type),
		Value:   ptrStr(p.Value),
		Version: p.Version,
		ARN:     ptrStr(p.ARN),
	}, nil
}

// ListSSMOnlineInstanceIDs は SSM Session Manager で接続可能な (PingStatus=Online の)
// EC2 インスタンス ID 一覧を返す。
func ListSSMOnlineInstanceIDs(ctx context.Context, profile, region string) ([]string, error) {
	client, err := NewClient(ctx, profile, region, func(cfg aws.Config) *ssm.Client {
		return ssm.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	input := &ssm.DescribeInstanceInformationInput{
		Filters: []ssmtypes.InstanceInformationStringFilter{
			{Key: aws.String("PingStatus"), Values: []string{"Online"}},
			{Key: aws.String("ResourceType"), Values: []string{"EC2Instance"}},
		},
	}

	var ids []string
	paginator := ssm.NewDescribeInstanceInformationPaginator(client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get next page: %w", err)
		}
		for _, inst := range page.InstanceInformationList {
			if inst.InstanceId != nil {
				ids = append(ids, *inst.InstanceId)
			}
		}
	}
	return ids, nil
}

func ssmFromMeta(p ssmtypes.ParameterMetadata) SSMParameterResource {
	lastMod := ""
	if p.LastModifiedDate != nil {
		lastMod = p.LastModifiedDate.Format(time.RFC3339)
	}
	return SSMParameterResource{
		ID:           ptrStr(p.Name),
		Name:         ptrStr(p.Name),
		Type:         string(p.Type),
		Tier:         string(p.Tier),
		Version:      p.Version,
		LastModified: lastMod,
	}
}

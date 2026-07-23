package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// SSMParameterResource represents an SSM Parameter Store entry.
// 値は一覧に含めない。機密値をキャッシュに常時載せず、GetSSMParameter でオンデマンド取得する。
type SSMParameterResource struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	State        string `json:"state"`
	Type         string `json:"type"`
	Tier         string `json:"tier"`
	Version      int64  `json:"version"`
	LastModified string `json:"last_modified"`
}

func (r SSMParameterResource) ResourceID() string    { return r.ID }
func (r SSMParameterResource) ResourceName() string  { return r.Name }
func (r SSMParameterResource) ResourceState() string { return "active" }
func (r SSMParameterResource) ServiceName() string   { return "ssm" }

// ListSSMParameters returns all SSM Parameter Store parameters for the given profile/region.
// 値は含めない (メタデータのみ)。値は GetSSMParameter でオンデマンド取得する。
func ListSSMParameters(ctx context.Context, profile, region string) ([]SSMParameterResource, error) {
	client, err := newSSMClient(ctx, profile, region)
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
	return resources, nil
}

// GetSSMParameter fetches the value of a single SSM parameter.
// Set decrypt=true for SecureString parameters.
func GetSSMParameter(ctx context.Context, profile, region, name string, decrypt bool) (string, error) {
	client, err := newSSMClient(ctx, profile, region)
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
	client, err := newSSMClient(ctx, profile, region)
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
	client, err := newSSMClient(ctx, profile, region)
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
	client, err := newSSMClient(ctx, profile, region)
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

// PutSSMParameter は既存パラメータの値を上書き更新する (Overwrite=true)。
// Type や KMS キー (SecureString の KeyId) を指定しないことで、既存の型・暗号化キーを
// 保持したまま値だけを更新する。呼び出しごとにパラメータのバージョンが 1 つ繰り上がる。
// エラーメッセージや slog に value を含めないこと (機密値のため)。
func PutSSMParameter(ctx context.Context, profile, region, name, value string) error {
	client, err := newSSMClient(ctx, profile, region)
	if err != nil {
		return err
	}
	if _, err := client.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      aws.String(name),
		Value:     aws.String(value),
		Overwrite: aws.Bool(true),
	}); err != nil {
		return fmt.Errorf("put ssm parameter %s: %w", name, err)
	}
	return nil
}

// newSSMClient は SSM API クライアントを生成する。
func newSSMClient(ctx context.Context, profile, region string) (*ssm.Client, error) {
	return NewClient(ctx, profile, region, func(cfg aws.Config) *ssm.Client {
		return ssm.NewFromConfig(cfg)
	})
}

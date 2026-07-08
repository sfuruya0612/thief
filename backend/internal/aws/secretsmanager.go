package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

// SecretResource represents a Secrets Manager secret.
//
// Value は一覧レスポンスに復号済みの値をそのまま含める (ssm.go の SSMParameterResource と同方針)。
// これにより機密値が backend の 1 時間キャッシュ (cacheTTL) とフロントの react-query キャッシュに
// 平文で載ることを許容している。slog には Value を渡さないこと。
type SecretResource struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	State       string `json:"state"`
	Description string `json:"description"`
	LastChanged string `json:"last_changed"`
	Value       string `json:"value"`
}

func (r SecretResource) ResourceID() string    { return r.ID }
func (r SecretResource) ResourceName() string  { return r.Name }
func (r SecretResource) ResourceState() string { return "active" }
func (r SecretResource) ServiceName() string   { return "secrets" }

// ListSecretResources returns all Secrets Manager secrets for the given profile/region.
// 一覧には復号済みの値を含める (シークレットごとに GetSecretValue を呼び出す)。
func ListSecretResources(ctx context.Context, profile, region string) ([]SecretResource, error) {
	client, err := NewClient(ctx, profile, region, func(cfg aws.Config) *secretsmanager.Client {
		return secretsmanager.NewFromConfig(cfg)
	})
	if err != nil {
		return nil, err
	}

	var resources []SecretResource
	paginator := secretsmanager.NewListSecretsPaginator(client, &secretsmanager.ListSecretsInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list secrets: %w", err)
		}
		for _, s := range page.SecretList {
			resources = append(resources, secretFromEntry(s))
		}
	}

	for i := range resources {
		value, err := getSecretValue(ctx, client, resources[i].Name)
		if err != nil {
			return nil, err
		}
		resources[i].Value = value
	}
	return resources, nil
}

// getSecretValue fetches the decrypted value of a single secret.
func getSecretValue(ctx context.Context, client *secretsmanager.Client, name string) (string, error) {
	out, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(name),
	})
	if err != nil {
		return "", fmt.Errorf("get secret value %s: %w", name, err)
	}
	return ptrStr(out.SecretString), nil
}

func secretFromEntry(s smtypes.SecretListEntry) SecretResource {
	lastChanged := ""
	if s.LastChangedDate != nil {
		lastChanged = s.LastChangedDate.Format(time.RFC3339)
	}
	return SecretResource{
		ID:          ptrStr(s.ARN),
		Name:        ptrStr(s.Name),
		Description: ptrStr(s.Description),
		LastChanged: lastChanged,
	}
}

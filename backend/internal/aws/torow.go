package aws

import (
	"fmt"
	"strings"
)

// ToRow implementations for util.Row compatibility.

func (r EC2Resource) ToRow() []string {
	return []string{r.ID, r.Name, r.State, string(r.InstanceType), r.AZ, r.PrivateIP, r.PublicIP, r.VpcID, tagMapStr(r.Tags)}
}

func (r RDSResource) ToRow() []string {
	return []string{r.ID, r.State, r.Engine, r.EngineVersion, r.Class, fmt.Sprintf("%v", r.MultiAZ), r.Endpoint}
}

func (r ElastiCacheResource) ToRow() []string {
	return []string{r.ID, r.State, r.Engine, r.EngineVersion, r.NodeType, fmt.Sprintf("%d", r.NumNodes), r.Endpoint}
}

func (r LambdaResource) ToRow() []string {
	return []string{r.Name, r.State, r.Runtime, fmt.Sprintf("%d", r.MemoryMB), fmt.Sprintf("%d", r.TimeoutSec)}
}

func (r ECSResource) ToRow() []string {
	return []string{r.Name, r.State, fmt.Sprintf("%d", r.ActiveServices), fmt.Sprintf("%d", r.RunningTasks), fmt.Sprintf("%d", r.PendingTasks)}
}

func (r ECRRepoResource) ToRow() []string {
	return []string{r.Name, r.URI, r.CreatedAt, r.ImageTagMutability, fmt.Sprintf("%v", r.ScanOnPush)}
}

func (r ECRImageResource) ToRow() []string {
	return []string{r.RepositoryName, r.ImageTag, r.ImageDigest, r.PushedAt, fmt.Sprintf("%d", r.ImageSizeBytes)}
}

func (r S3Resource) ToRow() []string {
	return []string{r.Name, r.Region, r.CreatedAt, fmt.Sprintf("%v", r.Public), r.Encryption}
}

func (r IAMResource) ToRow() []string {
	return []string{r.Name, r.ARN, fmt.Sprintf("%v", r.MFAEnabled), r.LastActivity, strings.Join(r.Groups, ","), strings.Join(r.Policies, ",")}
}

func (r SSOAccountResource) ToRow() []string {
	return []string{r.ID, r.Name, r.EmailAddress, strings.Join(r.Roles, ",")}
}

func (r SSMParameterResource) ToRow() []string {
	return []string{r.Name, r.Type, r.Tier, fmt.Sprintf("%d", r.Version), r.LastModified, r.Value}
}

func (r SecretResource) ToRow() []string {
	return []string{r.Name, r.Description, r.LastChanged, r.Value}
}

func (r CFNStackResource) ToRow() []string {
	return []string{r.Name, r.State, r.CreationTime, r.LastUpdatedTime, r.DriftStatus}
}

func (r KinesisResource) ToRow() []string {
	return []string{r.Name, r.State, fmt.Sprintf("%d", r.ShardCount), fmt.Sprintf("%d", r.RetentionHours), r.EncryptionType}
}

func (r CloudFrontResource) ToRow() []string {
	return []string{r.ID, r.Name, r.State, r.DomainName, strings.Join(r.Origins, ","), fmt.Sprintf("%v", r.Enabled)}
}

func (r ELBResource) ToRow() []string {
	return []string{r.Name, r.Type, r.State, r.Scheme, r.DNSName, r.VpcID, strings.Join(r.AZs, ",")}
}

func (r CostResource) ToRow() []string {
	return []string{r.TimePeriod, r.Service, fmt.Sprintf("%.4f", r.UnblendedAmount), fmt.Sprintf("%.4f", r.NetAmortizedAmount), r.Unit}
}

func (r ForecastResource) ToRow() []string {
	return []string{r.TimePeriod, fmt.Sprintf("%.4f", r.Amount), r.Unit}
}

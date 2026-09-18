package aws

import (
	"fmt"
	"strings"
	"time"
)

// ToRow implementations for util.Row compatibility.

func (r LambdaResource) ToRow() []string {
	return []string{r.Name, r.State, r.Runtime, fmt.Sprintf("%d", r.MemoryMB), fmt.Sprintf("%d", r.TimeoutSec)}
}

func (r SSOAccountResource) ToRow() []string {
	return []string{r.ID, r.Name, r.EmailAddress, strings.Join(r.Roles, ",")}
}

func (r KinesisResource) ToRow() []string {
	return []string{r.Name, r.State, r.Mode, fmt.Sprintf("%d", r.ShardCount), fmt.Sprintf("%d", r.RetentionHours), r.EncryptionType}
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

func (r DynamoResource) ToRow() []string {
	return []string{
		r.Name,
		r.State,
		r.Mode,
		fmt.Sprintf("%d", r.ItemCount),
		fmt.Sprintf("%d", r.SizeBytes),
		fmt.Sprintf("%d", r.GSICount),
	}
}

func (r APIGatewayResource) ToRow() []string {
	return []string{r.Name, r.State, r.Type, r.Stage, r.Endpoint}
}

func (r NATGatewayResource) ToRow() []string {
	return []string{
		r.Name,
		r.State,
		r.ID,
		r.VpcID,
		r.ElasticIP,
		r.LaunchTime.Format(time.RFC3339),
	}
}

func (r SQSResource) ToRow() []string {
	return []string{
		r.Name,
		r.State,
		r.Type,
		fmt.Sprintf("%d", r.AvailableMessages),
		fmt.Sprintf("%d", r.InFlight),
		fmt.Sprintf("%d", r.RetentionDays),
	}
}

func (r WAFResource) ToRow() []string {
	return []string{
		r.Name,
		r.Description,
		r.State,
		r.Scope,
		fmt.Sprintf("%d", r.RuleCount),
		fmt.Sprintf("%d", r.AssociatedCount),
	}
}

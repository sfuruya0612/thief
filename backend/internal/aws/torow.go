package aws

import (
	"fmt"
	"strings"
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

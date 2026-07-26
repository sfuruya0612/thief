package aws

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
)

func TestCloudfrontFromSummary(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   string
	}{
		{name: "deployed", status: "Deployed", want: "deployed"},
		{name: "in progress", status: "InProgress", want: "in-progress"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := cftypes.DistributionSummary{
				Id:      aws.String("E123"),
				Comment: aws.String("test"),
				Status:  aws.String(tt.status),
			}
			got := cloudfrontFromSummary(d)
			if got.State != tt.want {
				t.Errorf("state = %q, want %q", got.State, tt.want)
			}
		})
	}
}

func TestCloudfrontFromSummaryAliases(t *testing.T) {
	tests := []struct {
		name    string
		aliases *cftypes.Aliases
		want    []string
	}{
		{
			name:    "複数件のエイリアスが写る",
			aliases: &cftypes.Aliases{Items: []string{"example.com", "www.example.com"}},
			want:    []string{"example.com", "www.example.com"},
		},
		{
			name:    "Items が空の非 nil Aliases では nil になる",
			aliases: &cftypes.Aliases{Items: []string{}},
			want:    nil,
		},
		{
			name:    "Aliases が nil では nil になる",
			aliases: nil,
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := cftypes.DistributionSummary{
				Id:      aws.String("E123"),
				Comment: aws.String("test"),
				Status:  aws.String("Deployed"),
				Aliases: tt.aliases,
			}
			got := cloudfrontFromSummary(d)
			if (got.Aliases == nil) != (tt.want == nil) {
				t.Fatalf("aliases nilness = %v, want %v", got.Aliases, tt.want)
			}
			if len(got.Aliases) != len(tt.want) {
				t.Fatalf("aliases = %v, want %v", got.Aliases, tt.want)
			}
			for i := range tt.want {
				if got.Aliases[i] != tt.want[i] {
					t.Errorf("aliases[%d] = %q, want %q", i, got.Aliases[i], tt.want[i])
				}
			}
		})
	}
}

func TestCloudFrontResourceJSONHasNullAliasesWhenEmpty(t *testing.T) {
	r := CloudFrontResource{ID: "E123", Name: "test"}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"aliases":null`) {
		t.Errorf("json = %s, want to contain %q", b, `"aliases":null`)
	}
}

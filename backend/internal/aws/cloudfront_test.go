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

func TestCloudfrontBehaviorsFromSummary(t *testing.T) {
	allowAll := cftypes.AllowedMethods{Items: []cftypes.Method{cftypes.MethodGet, cftypes.MethodHead}}
	tests := []struct {
		name string
		d    cftypes.DistributionSummary
		want []CloudFrontBehavior
	}{
		{
			name: "既定 + 追加複数件が Items 順で先、既定が末尾になる",
			d: cftypes.DistributionSummary{
				CacheBehaviors: &cftypes.CacheBehaviors{Items: []cftypes.CacheBehavior{
					{
						PathPattern:          aws.String("/images/*"),
						TargetOriginId:       aws.String("origin-1"),
						ViewerProtocolPolicy: cftypes.ViewerProtocolPolicyHttpsOnly,
						AllowedMethods:       &allowAll,
						Compress:             aws.Bool(true),
					},
					{
						PathPattern:          aws.String("/api/*"),
						TargetOriginId:       aws.String("origin-2"),
						ViewerProtocolPolicy: cftypes.ViewerProtocolPolicyAllowAll,
					},
				}},
				DefaultCacheBehavior: &cftypes.DefaultCacheBehavior{
					TargetOriginId:       aws.String("origin-default"),
					ViewerProtocolPolicy: cftypes.ViewerProtocolPolicyRedirectToHttps,
				},
			},
			want: []CloudFrontBehavior{
				{PathPattern: "/images/*", TargetOriginID: "origin-1", ViewerProtocolPolicy: "https-only", AllowedMethods: []string{"GET", "HEAD"}, Compress: true},
				{PathPattern: "/api/*", TargetOriginID: "origin-2", ViewerProtocolPolicy: "allow-all", AllowedMethods: []string{}},
				{TargetOriginID: "origin-default", ViewerProtocolPolicy: "redirect-to-https", AllowedMethods: []string{}, IsDefault: true},
			},
		},
		{
			name: "CacheBehaviors が nil のとき既定のみ 1 件になる",
			d: cftypes.DistributionSummary{
				DefaultCacheBehavior: &cftypes.DefaultCacheBehavior{
					TargetOriginId:       aws.String("origin-default"),
					ViewerProtocolPolicy: cftypes.ViewerProtocolPolicyAllowAll,
				},
			},
			want: []CloudFrontBehavior{
				{TargetOriginID: "origin-default", ViewerProtocolPolicy: "allow-all", AllowedMethods: []string{}, IsDefault: true},
			},
		},
		{
			name: "CacheBehaviors が非 nil で Items が空でも既定のみ 1 件になる",
			d: cftypes.DistributionSummary{
				CacheBehaviors: &cftypes.CacheBehaviors{Items: []cftypes.CacheBehavior{}},
				DefaultCacheBehavior: &cftypes.DefaultCacheBehavior{
					TargetOriginId:       aws.String("origin-default"),
					ViewerProtocolPolicy: cftypes.ViewerProtocolPolicyAllowAll,
				},
			},
			want: []CloudFrontBehavior{
				{TargetOriginID: "origin-default", ViewerProtocolPolicy: "allow-all", AllowedMethods: []string{}, IsDefault: true},
			},
		},
		{
			name: "DefaultCacheBehavior が nil のとき追加のみが並ぶ",
			d: cftypes.DistributionSummary{
				CacheBehaviors: &cftypes.CacheBehaviors{Items: []cftypes.CacheBehavior{
					{
						PathPattern:          aws.String("/api/*"),
						TargetOriginId:       aws.String("origin-2"),
						ViewerProtocolPolicy: cftypes.ViewerProtocolPolicyAllowAll,
					},
				}},
			},
			want: []CloudFrontBehavior{
				{PathPattern: "/api/*", TargetOriginID: "origin-2", ViewerProtocolPolicy: "allow-all", AllowedMethods: []string{}},
			},
		},
		{
			name: "どちらも nil のとき nil を返す",
			d:    cftypes.DistributionSummary{},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cloudfrontBehaviorsFromSummary(tt.d)
			if (got == nil) != (tt.want == nil) {
				t.Fatalf("behaviors nilness = %v, want %v", got, tt.want)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("behaviors = %+v, want %+v", got, tt.want)
			}
			for i := range tt.want {
				if got[i].PathPattern != tt.want[i].PathPattern ||
					got[i].TargetOriginID != tt.want[i].TargetOriginID ||
					got[i].ViewerProtocolPolicy != tt.want[i].ViewerProtocolPolicy ||
					got[i].Compress != tt.want[i].Compress ||
					got[i].IsDefault != tt.want[i].IsDefault ||
					len(got[i].AllowedMethods) != len(tt.want[i].AllowedMethods) {
					t.Errorf("behaviors[%d] = %+v, want %+v", i, got[i], tt.want[i])
					continue
				}
				for j := range tt.want[i].AllowedMethods {
					if got[i].AllowedMethods[j] != tt.want[i].AllowedMethods[j] {
						t.Errorf("behaviors[%d].AllowedMethods[%d] = %q, want %q", i, j, got[i].AllowedMethods[j], tt.want[i].AllowedMethods[j])
					}
				}
			}
		})
	}
}

func TestCloudfrontFromSummaryBehaviors(t *testing.T) {
	d := cftypes.DistributionSummary{
		Id:      aws.String("E123"),
		Comment: aws.String("test"),
		Status:  aws.String("Deployed"),
		CacheBehaviors: &cftypes.CacheBehaviors{Items: []cftypes.CacheBehavior{
			{
				PathPattern:          aws.String("/api/*"),
				TargetOriginId:       aws.String("origin-2"),
				ViewerProtocolPolicy: cftypes.ViewerProtocolPolicyAllowAll,
			},
		}},
		DefaultCacheBehavior: &cftypes.DefaultCacheBehavior{
			TargetOriginId:       aws.String("origin-default"),
			ViewerProtocolPolicy: cftypes.ViewerProtocolPolicyAllowAll,
		},
	}
	want := cloudfrontBehaviorsFromSummary(d)
	got := cloudfrontFromSummary(d).Behaviors
	if len(got) != len(want) {
		t.Fatalf("Behaviors = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].PathPattern != want[i].PathPattern ||
			got[i].TargetOriginID != want[i].TargetOriginID ||
			got[i].ViewerProtocolPolicy != want[i].ViewerProtocolPolicy ||
			got[i].Compress != want[i].Compress ||
			got[i].IsDefault != want[i].IsDefault ||
			len(got[i].AllowedMethods) != len(want[i].AllowedMethods) {
			t.Errorf("Behaviors[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestCloudFrontResourceJSONBehaviors(t *testing.T) {
	empty := CloudFrontResource{ID: "E123", Name: "test"}
	b, err := json.Marshal(empty)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"behaviors":null`) {
		t.Errorf("json = %s, want to contain %q", b, `"behaviors":null`)
	}

	withBehavior := CloudFrontResource{
		ID:   "E123",
		Name: "test",
		Behaviors: []CloudFrontBehavior{
			{
				PathPattern:          "/api/*",
				TargetOriginID:       "origin-1",
				ViewerProtocolPolicy: "allow-all",
				AllowedMethods:       []string{"GET", "HEAD"},
				Compress:             true,
				IsDefault:            false,
			},
		},
	}
	b, err = json.Marshal(withBehavior)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{
		`"path_pattern"`,
		`"target_origin_id"`,
		`"viewer_protocol_policy"`,
		`"allowed_methods"`,
		`"compress"`,
		`"is_default"`,
	} {
		if !strings.Contains(string(b), key) {
			t.Errorf("json = %s, want to contain %q", b, key)
		}
	}
}

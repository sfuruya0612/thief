package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwltypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/aws-sdk-go-v2/service/wafv2"
	waftypes "github.com/aws/aws-sdk-go-v2/service/wafv2/types"
	"github.com/sfuruya0612/thief/backend/internal/accesslog"
)

type fakeAccessLogELB struct {
	lbs   []elbv2types.LoadBalancer
	attrs map[string][]elbv2types.LoadBalancerAttribute
}

func (f *fakeAccessLogELB) DescribeLoadBalancers(context.Context, *elbv2.DescribeLoadBalancersInput, ...func(*elbv2.Options)) (*elbv2.DescribeLoadBalancersOutput, error) {
	return &elbv2.DescribeLoadBalancersOutput{LoadBalancers: f.lbs}, nil
}

func (f *fakeAccessLogELB) DescribeLoadBalancerAttributes(_ context.Context, in *elbv2.DescribeLoadBalancerAttributesInput, _ ...func(*elbv2.Options)) (*elbv2.DescribeLoadBalancerAttributesOutput, error) {
	return &elbv2.DescribeLoadBalancerAttributesOutput{Attributes: f.attrs[ptrStr(in.LoadBalancerArn)]}, nil
}

type fakeAccessLogCloudFront struct {
	dists   []cftypes.DistributionSummary
	configs map[string]*cloudfront.GetDistributionConfigOutput
}

func (f *fakeAccessLogCloudFront) ListDistributions(context.Context, *cloudfront.ListDistributionsInput, ...func(*cloudfront.Options)) (*cloudfront.ListDistributionsOutput, error) {
	return &cloudfront.ListDistributionsOutput{DistributionList: &cftypes.DistributionList{Items: f.dists}}, nil
}

func (f *fakeAccessLogCloudFront) GetDistributionConfig(_ context.Context, in *cloudfront.GetDistributionConfigInput, _ ...func(*cloudfront.Options)) (*cloudfront.GetDistributionConfigOutput, error) {
	out, ok := f.configs[ptrStr(in.Id)]
	if !ok || out == nil {
		return nil, errors.New("not found")
	}
	return out, nil
}

type fakeAccessLogWAF struct {
	acls []waftypes.WebACLSummary
	logs []waftypes.LoggingConfiguration
}

func (f *fakeAccessLogWAF) ListWebACLs(context.Context, *wafv2.ListWebACLsInput, ...func(*wafv2.Options)) (*wafv2.ListWebACLsOutput, error) {
	return &wafv2.ListWebACLsOutput{WebACLs: f.acls}, nil
}

func (f *fakeAccessLogWAF) ListLoggingConfigurations(context.Context, *wafv2.ListLoggingConfigurationsInput, ...func(*wafv2.Options)) (*wafv2.ListLoggingConfigurationsOutput, error) {
	return &wafv2.ListLoggingConfigurationsOutput{LoggingConfigurations: f.logs}, nil
}

type fakeAccessLogCWL struct {
	sources      []cwltypes.DeliverySource
	deliveries   []cwltypes.Delivery
	destinations []cwltypes.DeliveryDestination
}

func (f *fakeAccessLogCWL) DescribeDeliverySources(context.Context, *cloudwatchlogs.DescribeDeliverySourcesInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeDeliverySourcesOutput, error) {
	return &cloudwatchlogs.DescribeDeliverySourcesOutput{DeliverySources: f.sources}, nil
}

func (f *fakeAccessLogCWL) DescribeDeliveries(context.Context, *cloudwatchlogs.DescribeDeliveriesInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeDeliveriesOutput, error) {
	return &cloudwatchlogs.DescribeDeliveriesOutput{Deliveries: f.deliveries}, nil
}

func (f *fakeAccessLogCWL) DescribeDeliveryDestinations(context.Context, *cloudwatchlogs.DescribeDeliveryDestinationsInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeDeliveryDestinationsOutput, error) {
	return &cloudwatchlogs.DescribeDeliveryDestinationsOutput{DeliveryDestinations: f.destinations}, nil
}

func TestDiscoverAccessLogSources(t *testing.T) {
	albARN := "arn:aws:elasticloadbalancing:ap-northeast-1:123456789012:loadbalancer/app/prod-alb/abc"
	cfARN := "arn:aws:cloudfront::123456789012:distribution/E123"
	wafARN := "arn:aws:wafv2:us-east-1:123456789012:global/webacl/prod-waf/xyz"
	cwlARN := "arn:aws:logs:us-east-1:123456789012:log-group:aws-waf-logs-prod"

	elb := &fakeAccessLogELB{
		lbs: []elbv2types.LoadBalancer{
			{
				LoadBalancerArn:  aws.String(albARN),
				LoadBalancerName: aws.String("prod-alb"),
				DNSName:          aws.String("prod-alb-123.ap-northeast-1.elb.amazonaws.com"),
				Type:             elbv2types.LoadBalancerTypeEnumApplication,
			},
			{
				LoadBalancerArn:  aws.String("arn:aws:elasticloadbalancing:ap-northeast-1:123456789012:loadbalancer/app/disabled/def"),
				LoadBalancerName: aws.String("disabled-alb"),
				DNSName:          aws.String("disabled.example.com"),
				Type:             elbv2types.LoadBalancerTypeEnumApplication,
			},
		},
		attrs: map[string][]elbv2types.LoadBalancerAttribute{
			albARN: {
				{Key: aws.String("access_logs.s3.enabled"), Value: aws.String("true")},
				{Key: aws.String("access_logs.s3.bucket"), Value: aws.String("alb-logs")},
				{Key: aws.String("access_logs.s3.prefix"), Value: aws.String("prod")},
			},
			"arn:aws:elasticloadbalancing:ap-northeast-1:123456789012:loadbalancer/app/disabled/def": {
				{Key: aws.String("access_logs.s3.enabled"), Value: aws.String("false")},
			},
		},
	}
	cf := &fakeAccessLogCloudFront{
		dists: []cftypes.DistributionSummary{{
			ARN:        aws.String(cfARN),
			Id:         aws.String("E123"),
			Comment:    aws.String("prod-cf"),
			DomainName: aws.String("d111111abcdef8.cloudfront.net"),
			Aliases:    &cftypes.Aliases{Items: []string{"www.example.com"}},
		}},
		configs: map[string]*cloudfront.GetDistributionConfigOutput{
			"E123": {
				DistributionConfig: &cftypes.DistributionConfig{
					Origins: &cftypes.Origins{Items: []cftypes.Origin{{DomainName: aws.String("prod-alb-123.ap-northeast-1.elb.amazonaws.com")}}},
					Logging: &cftypes.LoggingConfig{Enabled: aws.Bool(true), Bucket: aws.String("cf-logs"), Prefix: aws.String("legacy")},
				},
			},
		},
	}
	waf := &fakeAccessLogWAF{
		acls: []waftypes.WebACLSummary{
			{ARN: aws.String(wafARN), Id: aws.String("xyz"), Name: aws.String("prod-waf")},
			{ARN: aws.String("arn:aws:wafv2:us-east-1:123456789012:global/webacl/no-log/abc"), Id: aws.String("abc"), Name: aws.String("no-log")},
		},
		logs: []waftypes.LoggingConfiguration{{
			ResourceArn:           aws.String(wafARN),
			LogDestinationConfigs: []string{cwlARN},
			LogScope:              waftypes.LogScopeCustomer,
		}},
	}
	cwl := &fakeAccessLogCWL{
		sources: []cwltypes.DeliverySource{{
			Name:         aws.String("cf-v2"),
			ResourceArns: []string{cfARN},
			Service:      aws.String("cloudfront"),
			LogType:      aws.String("ACCESS_LOGS"),
		}},
		deliveries: []cwltypes.Delivery{{
			DeliverySourceName:      aws.String("cf-v2"),
			DeliveryDestinationArn:  aws.String("arn:aws:logs:us-east-1:123456789012:delivery-destination:cf-v2"),
			RecordFields:            []string{"date", "time", "c-ip", "cs-uri-stem", "sc-status"},
			S3DeliveryConfiguration: &cwltypes.S3DeliveryConfiguration{SuffixPath: aws.String("{DistributionId}/{yyyy}/{MM}/{dd}/{HH}/")},
		}},
		destinations: []cwltypes.DeliveryDestination{{
			Arn:                              aws.String("arn:aws:logs:us-east-1:123456789012:delivery-destination:cf-v2"),
			DeliveryDestinationType:          cwltypes.DeliveryDestinationTypeS3,
			OutputFormat:                     cwltypes.OutputFormatJson,
			DeliveryDestinationConfiguration: &cwltypes.DeliveryDestinationConfiguration{DestinationResourceArn: aws.String("arn:aws:s3:::cf-logs/v2")},
		}},
	}

	got, err := discoverAccessLogSources(context.Background(), accessLogDiscoveryClients{
		elb: elb, cloudfront: cf, wafGlobal: waf, cloudwatch: cwl,
	}, "default", "ap-northeast-1", "123456789012")
	if err != nil {
		t.Fatal(err)
	}

	alb := findAccessLogSource(t, got.Sources, accesslog.SourceALB, "prod-alb")
	if !alb.Status.LoggingEnabled || alb.Destination.Bucket != "alb-logs" || alb.Destination.Prefix != "prod" {
		t.Errorf("ALB source = %+v", alb)
	}
	if alb.SourceKeyValues[0] != "app/prod-alb/abc" {
		t.Errorf("ALB source key = %v", alb.SourceKeyValues)
	}
	if alb.OriginOf != "E123" {
		t.Errorf("ALB origin = %q, want E123", alb.OriginOf)
	}

	disabled := findAccessLogSource(t, got.Sources, accesslog.SourceALB, "disabled-alb")
	if disabled.Status.LoggingEnabled || disabled.Destination.Type != accesslog.DestinationNone {
		t.Errorf("disabled ALB source = %+v", disabled)
	}

	cfLegacy := findAccessLogSourceByProfile(t, got.Sources, accesslog.SourceCloudFront, accesslog.SchemaCloudFrontLegacy)
	if !cfLegacy.Global || cfLegacy.Destination.Bucket != "cf-logs" {
		t.Errorf("CloudFront legacy source = %+v", cfLegacy)
	}
	cfV2 := findAccessLogSourceByProfile(t, got.Sources, accesslog.SourceCloudFront, accesslog.SchemaCloudFrontV2JSON)
	if cfV2.Destination.SuffixPath == "" || len(cfV2.Destination.RecordFields) == 0 {
		t.Errorf("CloudFront v2 source = %+v", cfV2)
	}

	wafSource := findAccessLogSource(t, got.Sources, accesslog.SourceWAF, "prod-waf")
	if wafSource.Engine != accesslog.EngineLogsInsights || wafSource.Destination.LogGroup != "aws-waf-logs-prod" {
		t.Errorf("WAF source = %+v", wafSource)
	}
	noLog := findAccessLogSource(t, got.Sources, accesslog.SourceWAF, "no-log")
	if noLog.Status.LoggingEnabled || noLog.Destination.Type != accesslog.DestinationNone {
		t.Errorf("WAF without logging = %+v", noLog)
	}
}

func TestDiscoverWAFRedactedURIPath(t *testing.T) {
	wafARN := "arn:aws:wafv2:ap-northeast-1:123456789012:regional/webacl/prod/xyz"
	client := &fakeAccessLogWAF{
		acls: []waftypes.WebACLSummary{{ARN: aws.String(wafARN), Id: aws.String("xyz"), Name: aws.String("prod")}},
		logs: []waftypes.LoggingConfiguration{{
			ResourceArn:           aws.String(wafARN),
			LogDestinationConfigs: []string{"arn:aws:s3:::waf-logs/prod"},
			RedactedFields:        []waftypes.FieldToMatch{{UriPath: &waftypes.UriPath{}}},
		}},
	}
	sources, _, err := discoverWAFSources(context.Background(), client, "default", "ap-northeast-1", "123456789012", waftypes.ScopeRegional, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("sources = %d, want 1", len(sources))
	}
	if sources[0].Engine != "" || sources[0].Status.Attribution {
		t.Errorf("redacted source should not be attributable: %+v", sources[0])
	}
}

func TestDiscoverAccessLogSourcesPartialFailure(t *testing.T) {
	cf := &fakeAccessLogCloudFront{
		dists: []cftypes.DistributionSummary{{
			ARN: aws.String("arn:aws:cloudfront::123456789012:distribution/E1"), Id: aws.String("E1"), DomainName: aws.String("d1.cloudfront.net"),
		}},
		configs: map[string]*cloudfront.GetDistributionConfigOutput{"E1": nil},
	}
	got, err := discoverAccessLogSources(context.Background(), accessLogDiscoveryClients{cloudfront: cf}, "default", "ap-northeast-1", "123456789012")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sources) != 1 {
		t.Fatalf("sources = %d, want 1", len(got.Sources))
	}
	if len(got.Warnings) == 0 {
		t.Error("expected a warning for the failed distribution")
	}
	if got.Sources[0].Status.DestinationReadable {
		t.Error("destination readable = true, want false")
	}
}

func TestMarkSharedSources(t *testing.T) {
	sources := []accesslog.Source{
		{ID: "a", Name: "alb-a", Destination: accesslog.Destination{Type: accesslog.DestinationS3, Bucket: "logs", Prefix: "alb"}},
		{ID: "b", Name: "alb-b", Destination: accesslog.Destination{Type: accesslog.DestinationS3, Bucket: "logs", Prefix: "alb"}},
		{ID: "c", Name: "alb-c", Destination: accesslog.Destination{Type: accesslog.DestinationS3, Bucket: "other", Prefix: "alb"}},
	}
	markSharedSources(sources)
	if len(sources[0].Status.SharedBy) != 1 || sources[0].Status.SharedBy[0] != "alb-b" {
		t.Errorf("shared by = %v", sources[0].Status.SharedBy)
	}
	if len(sources[2].Status.SharedBy) != 0 {
		t.Errorf("unexpected shared by = %v", sources[2].Status.SharedBy)
	}
}

func findAccessLogSource(t *testing.T, sources []accesslog.Source, sourceType accesslog.SourceType, name string) accesslog.Source {
	t.Helper()
	for _, source := range sources {
		if source.Type == sourceType && source.Name == name {
			return source
		}
	}
	t.Fatalf("source %s/%s not found in %+v", sourceType, name, sources)
	return accesslog.Source{}
}

func findAccessLogSourceByProfile(t *testing.T, sources []accesslog.Source, sourceType accesslog.SourceType, profile accesslog.SchemaProfile) accesslog.Source {
	t.Helper()
	for _, source := range sources {
		if source.Type == sourceType && source.SchemaProfile == profile {
			return source
		}
	}
	t.Fatalf("source %s/%s not found", sourceType, profile)
	return accesslog.Source{}
}

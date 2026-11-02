package aws

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"

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
	"golang.org/x/sync/errgroup"
)

// AccessLogSources はアクセスログのソースの検出結果を表す。
type AccessLogSources struct {
	Sources  []accesslog.Source `json:"sources"`
	Warnings []string           `json:"warnings,omitempty"`
}

type accessLogELBAPI interface {
	DescribeLoadBalancers(ctx context.Context, params *elbv2.DescribeLoadBalancersInput, optFns ...func(*elbv2.Options)) (*elbv2.DescribeLoadBalancersOutput, error)
	DescribeLoadBalancerAttributes(ctx context.Context, params *elbv2.DescribeLoadBalancerAttributesInput, optFns ...func(*elbv2.Options)) (*elbv2.DescribeLoadBalancerAttributesOutput, error)
}

type accessLogCloudFrontAPI interface {
	ListDistributions(ctx context.Context, params *cloudfront.ListDistributionsInput, optFns ...func(*cloudfront.Options)) (*cloudfront.ListDistributionsOutput, error)
	GetDistributionConfig(ctx context.Context, params *cloudfront.GetDistributionConfigInput, optFns ...func(*cloudfront.Options)) (*cloudfront.GetDistributionConfigOutput, error)
}

type accessLogWAFAPI interface {
	ListWebACLs(ctx context.Context, params *wafv2.ListWebACLsInput, optFns ...func(*wafv2.Options)) (*wafv2.ListWebACLsOutput, error)
	ListLoggingConfigurations(ctx context.Context, params *wafv2.ListLoggingConfigurationsInput, optFns ...func(*wafv2.Options)) (*wafv2.ListLoggingConfigurationsOutput, error)
}

type accessLogCWLogsAPI interface {
	DescribeDeliverySources(ctx context.Context, params *cloudwatchlogs.DescribeDeliverySourcesInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeDeliverySourcesOutput, error)
	DescribeDeliveries(ctx context.Context, params *cloudwatchlogs.DescribeDeliveriesInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeDeliveriesOutput, error)
	DescribeDeliveryDestinations(ctx context.Context, params *cloudwatchlogs.DescribeDeliveryDestinationsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeDeliveryDestinationsOutput, error)
}

type accessLogDiscoveryClients struct {
	elb         accessLogELBAPI
	cloudfront  accessLogCloudFrontAPI
	wafRegional accessLogWAFAPI
	wafGlobal   accessLogWAFAPI
	cloudwatch  accessLogCWLogsAPI
}

type accessLogDelivery struct {
	ResourceARN    string
	Service        string
	LogType        string
	Destination    accesslog.Destination
	OutputFormat   string
	RecordFields   []string
	FieldDelimiter string
	SuffixPath     string
	HiveCompatible bool
}

// DiscoverAccessLogSources は ALB、CloudFront、WAF のアクセスログの出力先を検出する。
func DiscoverAccessLogSources(ctx context.Context, profile, region string) (AccessLogSources, error) {
	var warnings []string
	accountID := ""
	if identity, err := GetCallerIdentity(ctx, profile); err != nil {
		warnings = append(warnings, fmt.Sprintf("resolve account id: %v", err))
	} else {
		accountID = identity.AccountID
	}

	clients := accessLogDiscoveryClients{}
	if c, err := newELBClient(ctx, profile, region); err == nil {
		clients.elb = c
	} else {
		warnings = append(warnings, fmt.Sprintf("create elbv2 client: %v", err))
	}
	if c, err := newCloudFrontClient(ctx, profile); err == nil {
		clients.cloudfront = c
	} else {
		warnings = append(warnings, fmt.Sprintf("create cloudfront client: %v", err))
	}
	if c, err := newWAFClient(ctx, profile, region); err == nil {
		clients.wafRegional = c
	} else {
		warnings = append(warnings, fmt.Sprintf("create waf regional client: %v", err))
	}
	if c, err := newWAFClient(ctx, profile, "us-east-1"); err == nil {
		clients.wafGlobal = c
	} else {
		warnings = append(warnings, fmt.Sprintf("create waf global client: %v", err))
	}
	if c, err := newCWLogsClient(ctx, profile, "us-east-1"); err == nil {
		clients.cloudwatch = c
	} else {
		warnings = append(warnings, fmt.Sprintf("create cloudwatch logs client: %v", err))
	}

	result, err := discoverAccessLogSources(ctx, clients, profile, region, accountID)
	if err != nil {
		return AccessLogSources{}, err
	}
	result.Warnings = append(warnings, result.Warnings...)
	return result, nil
}

func discoverAccessLogSources(ctx context.Context, clients accessLogDiscoveryClients, profile, region, accountID string) (AccessLogSources, error) {
	deliveries, warnings := discoverAccessLogDeliveries(ctx, clients.cloudwatch)
	var albSources, cfSources, wafRegional, wafGlobal []accesslog.Source
	var albWarnings, cfWarnings, wafRegionalWarnings, wafGlobalWarnings []string
	var origins map[string][]string

	g, gctx := errgroup.WithContext(ctx)
	if clients.elb != nil {
		g.Go(func() error {
			sources, w, err := discoverALBSources(gctx, clients.elb, profile, region, accountID, deliveries)
			if err != nil {
				albWarnings = append(w, fmt.Sprintf("discover ALB access logs: %v", err))
				return nil
			}
			albSources, albWarnings = sources, w
			return nil
		})
	}
	if clients.cloudfront != nil {
		g.Go(func() error {
			sources, o, w, err := discoverCloudFrontSources(gctx, clients.cloudfront, profile, region, accountID, deliveries)
			if err != nil {
				cfWarnings = append(w, fmt.Sprintf("discover CloudFront access logs: %v", err))
				return nil
			}
			cfSources, origins, cfWarnings = sources, o, w
			return nil
		})
	}
	if clients.wafRegional != nil {
		g.Go(func() error {
			sources, w, err := discoverWAFSources(gctx, clients.wafRegional, profile, region, accountID, waftypes.ScopeRegional, false)
			if err != nil {
				wafRegionalWarnings = append(w, fmt.Sprintf("discover regional WAF logs: %v", err))
				return nil
			}
			wafRegional, wafRegionalWarnings = sources, w
			return nil
		})
	}
	if clients.wafGlobal != nil {
		g.Go(func() error {
			sources, w, err := discoverWAFSources(gctx, clients.wafGlobal, profile, "us-east-1", accountID, waftypes.ScopeCloudfront, true)
			if err != nil {
				wafGlobalWarnings = append(w, fmt.Sprintf("discover global WAF logs: %v", err))
				return nil
			}
			wafGlobal, wafGlobalWarnings = sources, w
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return AccessLogSources{}, err
	}
	warnings = append(warnings, albWarnings...)
	warnings = append(warnings, cfWarnings...)
	warnings = append(warnings, wafRegionalWarnings...)
	warnings = append(warnings, wafGlobalWarnings...)

	sources := make([]accesslog.Source, 0, len(albSources)+len(cfSources)+len(wafRegional)+len(wafGlobal))
	sources = append(sources, albSources...)
	sources = append(sources, cfSources...)
	sources = append(sources, wafRegional...)
	sources = append(sources, wafGlobal...)
	markOriginALB(sources, origins)
	markSharedSources(sources)
	sortAccessLogSources(sources)
	return AccessLogSources{Sources: sources, Warnings: dedupeWarnings(warnings)}, nil
}

func discoverALBSources(ctx context.Context, client accessLogELBAPI, profile, region, accountID string, deliveries map[string][]accessLogDelivery) ([]accesslog.Source, []string, error) {
	lbs, err := listApplicationLoadBalancers(ctx, client)
	if err != nil {
		return nil, nil, err
	}
	sources := make([]accesslog.Source, len(lbs))
	warnings := make([]string, len(lbs))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(10)
	for i, lb := range lbs {
		g.Go(func() error {
			arn := ptrStr(lb.LoadBalancerArn)
			name := ptrStr(lb.LoadBalancerName)
			resourceID := albResourceIDFromARN(arn)
			source := accesslog.Source{
				Type:            accesslog.SourceALB,
				Name:            name,
				Region:          region,
				QueryRegion:     region,
				AccountID:       accountID,
				ResourceARN:     arn,
				ResourceID:      resourceID,
				DNSName:         ptrStr(lb.DNSName),
				Engine:          accesslog.EngineAthena,
				SchemaProfile:   accesslog.SchemaALBLegacy,
				SourceKeyValues: []string{resourceID},
				SuggestedTable:  "alb_access_logs",
				Status: accesslog.SourceStatus{
					DestinationReadable: true,
					Attribution:         true,
					SourceKey:           resourceID,
				},
			}
			attrs, err := client.DescribeLoadBalancerAttributes(gctx, &elbv2.DescribeLoadBalancerAttributesInput{LoadBalancerArn: aws.String(arn)})
			if err != nil {
				source.Destination = accesslog.Destination{Type: accesslog.DestinationNone}
				source.Engine = ""
				source.Status.DestinationReadable = false
				source.Status.Attribution = false
				source.Status.Issues = append(source.Status.Issues, fmt.Sprintf("cannot read access log attributes: %v", err))
				sources[i] = source
				warnings[i] = fmt.Sprintf("ALB %s: cannot read access log attributes: %v", name, err)
				return nil
			}
			enabled, bucket, prefix := albAccessLogSettings(attrs.Attributes)
			source.Status.LoggingEnabled = enabled
			if enabled {
				source.Destination = accesslog.Destination{Type: accesslog.DestinationS3, Bucket: bucket, Prefix: prefix, Format: "text"}
				source.Status.DestinationType = accesslog.DestinationS3
			} else {
				source.Destination = accesslog.Destination{Type: accesslog.DestinationNone}
				source.Status.DestinationType = accesslog.DestinationNone
				source.Status.Issues = append(source.Status.Issues, "Access logging is not enabled")
			}
			if vended, ok := deliveries[arn]; ok && len(vended) > 0 {
				source.Status.Issues = append(source.Status.Issues, "ALB vended logs are enabled but are not supported in v1")
				if !enabled {
					source.Destination = vended[0].Destination
					source.Status.LoggingEnabled = true
					source.Status.DestinationType = vended[0].Destination.Type
					source.Engine = ""
					source.Status.Attribution = false
				}
			}
			source.ID = accessLogSourceID(source.Type, arn, destinationKey(source.Destination), string(source.SchemaProfile))
			attachSetupSQL(&source)
			sources[i] = source
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, nil, err
	}
	return filterSources(sources), filterWarnings(warnings), nil
}

func discoverCloudFrontSources(ctx context.Context, client accessLogCloudFrontAPI, profile, region, accountID string, deliveries map[string][]accessLogDelivery) ([]accesslog.Source, map[string][]string, []string, error) {
	dists, err := listCloudFrontDistributions(ctx, client)
	if err != nil {
		return nil, nil, nil, err
	}
	sources := make([][]accesslog.Source, len(dists))
	originGroups := make([][]string, len(dists))
	warnings := make([]string, len(dists))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(10)
	for i, dist := range dists {
		g.Go(func() error {
			arn := ptrStr(dist.ARN)
			id := ptrStr(dist.Id)
			name := ptrStr(dist.Comment)
			if name == "" {
				name = id
			}
			domain := ptrStr(dist.DomainName)
			var aliases []string
			if dist.Aliases != nil {
				aliases = append(aliases, dist.Aliases.Items...)
			}
			config, err := client.GetDistributionConfig(gctx, &cloudfront.GetDistributionConfigInput{Id: aws.String(id)})
			if err != nil {
				warnings[i] = fmt.Sprintf("CloudFront %s: cannot read distribution config: %v", id, err)
				unknown := disabledCloudFrontSource(id, name, domain, aliases, region, accountID, arn)
				unknown.Status.Issues = []string{fmt.Sprintf("cannot read distribution config: %v", err)}
				unknown.Status.DestinationReadable = false
				sources[i] = []accesslog.Source{unknown}
				return nil
			}
			if config.DistributionConfig != nil && config.DistributionConfig.Origins != nil {
				for _, origin := range config.DistributionConfig.Origins.Items {
					originGroups[i] = append(originGroups[i], strings.ToLower(ptrStr(origin.DomainName)))
				}
			}
			legacyEnabled := false
			bucket, prefix := "", ""
			if config.DistributionConfig != nil && config.DistributionConfig.Logging != nil {
				legacyEnabled = ptrBool(config.DistributionConfig.Logging.Enabled)
				bucket = normalizeS3Bucket(ptrStr(config.DistributionConfig.Logging.Bucket))
				prefix = ptrStr(config.DistributionConfig.Logging.Prefix)
			}
			var out []accesslog.Source
			if legacyEnabled {
				source := cloudFrontSource(id, name, domain, aliases, region, accountID, arn, accesslog.Destination{
					Type: accesslog.DestinationS3, Bucket: bucket, Prefix: prefix, Format: "text",
				}, accesslog.SchemaCloudFrontLegacy, "cloudfront_legacy")
				source.Status.LoggingEnabled = true
				source.Status.DestinationType = accesslog.DestinationS3
				out = append(out, source)
			}
			for _, delivery := range deliveries[arn] {
				out = append(out, cloudFrontV2Source(id, name, domain, aliases, region, accountID, arn, delivery))
			}
			if len(out) == 0 {
				out = append(out, disabledCloudFrontSource(id, name, domain, aliases, region, accountID, arn))
			}
			if legacyEnabled && len(deliveries[arn]) > 0 {
				for j := range out {
					out[j].Status.Issues = append(out[j].Status.Issues, "both standard logging (legacy) and standard logging (v2) are enabled; selecting both duplicates records")
				}
			}
			sources[i] = out
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, nil, nil, err
	}
	origins := make(map[string][]string, len(dists))
	var flat []accesslog.Source
	for i, group := range sources {
		flat = append(flat, group...)
		if len(originGroups[i]) > 0 {
			origins[ptrStr(dists[i].Id)] = originGroups[i]
		}
	}
	return flat, origins, filterWarnings(warnings), nil
}

func discoverWAFSources(ctx context.Context, client accessLogWAFAPI, profile, region, accountID string, scope waftypes.Scope, global bool) ([]accesslog.Source, []string, error) {
	acls, err := listWAFWebACLs(ctx, client, scope)
	if err != nil {
		return nil, nil, err
	}
	logging, err := listWAFLoggingConfigurations(ctx, client, scope)
	if err != nil {
		return nil, nil, err
	}
	byARN := make(map[string]waftypes.LoggingConfiguration, len(logging))
	for _, config := range logging {
		byARN[ptrStr(config.ResourceArn)] = config
	}
	sources := make([]accesslog.Source, 0, len(acls))
	for _, acl := range acls {
		arn := ptrStr(acl.ARN)
		name := ptrStr(acl.Name)
		source := accesslog.Source{
			Type:           accesslog.SourceWAF,
			Name:           name,
			Region:         region,
			QueryRegion:    region,
			Global:         global,
			AccountID:      accountID,
			ResourceARN:    arn,
			ResourceID:     ptrStr(acl.Id),
			SuggestedTable: "waf_logs",
			Status: accesslog.SourceStatus{
				DestinationReadable: true,
				Attribution:         true,
				SourceKey:           arn,
			},
		}
		config, ok := byARN[arn]
		if !ok {
			source.Destination = accesslog.Destination{Type: accesslog.DestinationNone}
			source.Status.DestinationType = accesslog.DestinationNone
			source.Status.Issues = append(source.Status.Issues, "Access logging is not enabled")
			source.ID = accessLogSourceID(source.Type, arn, "none", string(source.SchemaProfile))
			sources = append(sources, source)
			continue
		}
		source.Status.LoggingEnabled = true
		dest := wafDestination(config.LogDestinationConfigs)
		source.Destination = dest
		source.Status.DestinationType = dest.Type
		source.SourceKeyValues = []string{arn}
		switch dest.Type {
		case accesslog.DestinationCloudWatchLogs:
			source.Engine = accesslog.EngineLogsInsights
			source.SchemaProfile = accesslog.SchemaWAFJSON
		case accesslog.DestinationS3:
			source.Engine = accesslog.EngineAthena
			source.SchemaProfile = accesslog.SchemaWAFJSON
		default:
			source.Status.Attribution = false
			source.Status.Issues = append(source.Status.Issues, "Firehose destinations are not supported in v1")
		}
		if config.LogScope != "" && config.LogScope != waftypes.LogScopeCustomer {
			source.Status.Issues = append(source.Status.Issues, fmt.Sprintf("logging configuration scope is %s", config.LogScope))
		}
		if config.LoggingFilter != nil {
			source.Status.Issues = append(source.Status.Issues, "logging filter is enabled; only the records kept by the filter are counted")
		}
		if config.ManagedByFirewallManager {
			source.Status.Issues = append(source.Status.Issues, "logging configuration is managed by Firewall Manager")
		}
		if uriPathRedacted(config.RedactedFields) {
			source.Status.Attribution = false
			source.Engine = ""
			source.Status.Issues = append(source.Status.Issues, "URI path is redacted in the WAF logs")
		}
		source.ID = accessLogSourceID(source.Type, arn, destinationKey(source.Destination), string(source.SchemaProfile))
		attachSetupSQL(&source)
		sources = append(sources, source)
	}
	return sources, nil, nil
}

func discoverAccessLogDeliveries(ctx context.Context, client accessLogCWLogsAPI) (map[string][]accessLogDelivery, []string) {
	if client == nil {
		return nil, nil
	}
	var warnings []string
	sources, err := listDeliverySources(ctx, client)
	if err != nil {
		return nil, []string{fmt.Sprintf("list CloudWatch Logs delivery sources: %v", err)}
	}
	deliveries, err := listDeliveries(ctx, client)
	if err != nil {
		return nil, []string{fmt.Sprintf("list CloudWatch Logs deliveries: %v", err)}
	}
	destinations, err := listDeliveryDestinations(ctx, client)
	if err != nil {
		return nil, []string{fmt.Sprintf("list CloudWatch Logs delivery destinations: %v", err)}
	}
	destByARN := make(map[string]cwltypes.DeliveryDestination, len(destinations))
	for _, d := range destinations {
		destByARN[ptrStr(d.Arn)] = d
	}
	result := make(map[string][]accessLogDelivery)
	for _, source := range sources {
		for _, delivery := range deliveries {
			if ptrStr(delivery.DeliverySourceName) != ptrStr(source.Name) {
				continue
			}
			dest, ok := destByARN[ptrStr(delivery.DeliveryDestinationArn)]
			if !ok {
				warnings = append(warnings, fmt.Sprintf("delivery destination %s was not found", ptrStr(delivery.DeliveryDestinationArn)))
				continue
			}
			d := accessLogDelivery{
				Service:        ptrStr(source.Service),
				LogType:        ptrStr(source.LogType),
				OutputFormat:   string(dest.OutputFormat),
				RecordFields:   delivery.RecordFields,
				FieldDelimiter: ptrStr(delivery.FieldDelimiter),
			}
			if delivery.S3DeliveryConfiguration != nil {
				d.SuffixPath = ptrStr(delivery.S3DeliveryConfiguration.SuffixPath)
				d.HiveCompatible = ptrBool(delivery.S3DeliveryConfiguration.EnableHiveCompatiblePath)
			}
			d.Destination = deliveryDestination(dest, d.SuffixPath, d.HiveCompatible)
			for _, arn := range source.ResourceArns {
				d.ResourceARN = arn
				result[arn] = append(result[arn], d)
			}
		}
	}
	return result, warnings
}

func listApplicationLoadBalancers(ctx context.Context, client accessLogELBAPI) ([]elbv2types.LoadBalancer, error) {
	var out []elbv2types.LoadBalancer
	paginator := elbv2.NewDescribeLoadBalancersPaginator(client, &elbv2.DescribeLoadBalancersInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe load balancers: %w", err)
		}
		for _, lb := range page.LoadBalancers {
			if lb.Type == elbv2types.LoadBalancerTypeEnumApplication {
				out = append(out, lb)
			}
		}
	}
	return out, nil
}

func albAccessLogSettings(attrs []elbv2types.LoadBalancerAttribute) (bool, string, string) {
	var enabled bool
	var bucket, prefix string
	for _, attr := range attrs {
		switch ptrStr(attr.Key) {
		case "access_logs.s3.enabled":
			enabled = strings.EqualFold(ptrStr(attr.Value), "true")
		case "access_logs.s3.bucket":
			bucket = ptrStr(attr.Value)
		case "access_logs.s3.prefix":
			prefix = ptrStr(attr.Value)
		}
	}
	return enabled, bucket, prefix
}

func listCloudFrontDistributions(ctx context.Context, client accessLogCloudFrontAPI) ([]cftypes.DistributionSummary, error) {
	var out []cftypes.DistributionSummary
	paginator := cloudfront.NewListDistributionsPaginator(client, &cloudfront.ListDistributionsInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list cloudfront distributions: %w", err)
		}
		if page.DistributionList != nil {
			out = append(out, page.DistributionList.Items...)
		}
	}
	return out, nil
}

func cloudFrontSource(id, name, domain string, aliases []string, region, accountID, arn string, destination accesslog.Destination, profile accesslog.SchemaProfile, suggested string) accesslog.Source {
	values := append([]string{id, domain}, aliases...)
	source := accesslog.Source{
		Type:            accesslog.SourceCloudFront,
		Name:            name,
		Region:          region,
		QueryRegion:     region,
		Global:          true,
		AccountID:       accountID,
		ResourceARN:     arn,
		ResourceID:      id,
		DomainName:      domain,
		Aliases:         aliases,
		Engine:          accesslog.EngineAthena,
		Destination:     destination,
		SchemaProfile:   profile,
		SourceKeyValues: values,
		SuggestedTable:  suggested,
		Status: accesslog.SourceStatus{
			LoggingEnabled:      true,
			DestinationType:     destination.Type,
			DestinationReadable: true,
			Attribution:         true,
			SourceKey:           domain,
		},
	}
	source.ID = accessLogSourceID(source.Type, arn, destinationKey(destination), string(source.SchemaProfile))
	attachSetupSQL(&source)
	return source
}

func disabledCloudFrontSource(id, name, domain string, aliases []string, region, accountID, arn string) accesslog.Source {
	source := accesslog.Source{
		Type:           accesslog.SourceCloudFront,
		Name:           name,
		Region:         region,
		QueryRegion:    region,
		Global:         true,
		AccountID:      accountID,
		ResourceARN:    arn,
		ResourceID:     id,
		DomainName:     domain,
		Aliases:        aliases,
		Destination:    accesslog.Destination{Type: accesslog.DestinationNone},
		SuggestedTable: "cloudfront_legacy",
		Status: accesslog.SourceStatus{
			DestinationType: accesslog.DestinationNone,
			Issues:          []string{"Access logging is not enabled"},
		},
	}
	source.ID = accessLogSourceID(source.Type, arn, "none", string(source.SchemaProfile))
	return source
}

func cloudFrontV2Source(id, name, domain string, aliases []string, region, accountID, arn string, delivery accessLogDelivery) accesslog.Source {
	source := cloudFrontSource(id, name, domain, aliases, region, accountID, arn, delivery.Destination, accesslog.SchemaCloudFrontV2JSON, "cloudfront_v2")
	source.Destination.RecordFields = delivery.RecordFields
	source.Destination.FieldDelimiter = delivery.FieldDelimiter
	source.Destination.SuffixPath = delivery.SuffixPath
	source.Destination.HiveCompatible = delivery.HiveCompatible
	if len(delivery.RecordFields) > 0 {
		for _, required := range []string{"c-ip", "cs-uri-stem", "sc-status", "date", "time"} {
			if !hasRecordField(delivery.RecordFields, required) {
				source.Engine = ""
				source.Status.Attribution = false
				source.Status.Issues = append(source.Status.Issues,
					fmt.Sprintf("CloudFront standard logging (v2) does not include the required field %s", required))
			}
		}
	}
	switch delivery.Destination.Type {
	case accesslog.DestinationS3:
		if strings.EqualFold(delivery.OutputFormat, string(cwltypes.OutputFormatParquet)) {
			source.Status.Issues = append(source.Status.Issues, "Parquet output requires an existing Athena table created by a Glue crawler")
		}
	case accesslog.DestinationCloudWatchLogs:
		source.Engine = ""
		source.Status.Attribution = false
		source.Status.Issues = append(source.Status.Issues, "CloudFront standard logging (v2) to CloudWatch Logs is not supported in v1")
	default:
		source.Engine = ""
		source.Status.Attribution = false
		source.Status.Issues = append(source.Status.Issues, "unsupported CloudFront standard logging destination")
	}
	source.ID = accessLogSourceID(source.Type, arn, destinationKey(source.Destination), string(source.SchemaProfile))
	attachSetupSQL(&source)
	if strings.EqualFold(delivery.OutputFormat, string(cwltypes.OutputFormatParquet)) {
		source.SetupSQL = ""
	}
	return source
}

func listWAFWebACLs(ctx context.Context, client accessLogWAFAPI, scope waftypes.Scope) ([]waftypes.WebACLSummary, error) {
	var out []waftypes.WebACLSummary
	var marker *string
	for {
		page, err := client.ListWebACLs(ctx, &wafv2.ListWebACLsInput{Scope: scope, NextMarker: marker})
		if err != nil {
			return nil, fmt.Errorf("list web acls scope=%s: %w", scope, err)
		}
		out = append(out, page.WebACLs...)
		if page.NextMarker == nil || *page.NextMarker == "" {
			break
		}
		marker = page.NextMarker
	}
	return out, nil
}

func listWAFLoggingConfigurations(ctx context.Context, client accessLogWAFAPI, scope waftypes.Scope) ([]waftypes.LoggingConfiguration, error) {
	var out []waftypes.LoggingConfiguration
	var marker *string
	for {
		page, err := client.ListLoggingConfigurations(ctx, &wafv2.ListLoggingConfigurationsInput{Scope: scope, NextMarker: marker})
		if err != nil {
			return nil, fmt.Errorf("list waf logging configurations scope=%s: %w", scope, err)
		}
		out = append(out, page.LoggingConfigurations...)
		if page.NextMarker == nil || *page.NextMarker == "" {
			break
		}
		marker = page.NextMarker
	}
	return out, nil
}

func wafDestination(arns []string) accesslog.Destination {
	for _, arn := range arns {
		switch {
		case strings.Contains(arn, ":log-group:"):
			return accesslog.Destination{Type: accesslog.DestinationCloudWatchLogs, LogGroup: logGroupNameFromARN(arn)}
		case strings.HasPrefix(arn, "arn:aws:s3:::"):
			bucket, prefix := s3BucketPrefixFromARN(arn)
			return accesslog.Destination{Type: accesslog.DestinationS3, Bucket: bucket, Prefix: prefix, Format: "json"}
		case strings.Contains(arn, ":deliverystream/"):
			return accesslog.Destination{Type: accesslog.DestinationFirehose}
		}
	}
	return accesslog.Destination{Type: accesslog.DestinationNone}
}

func uriPathRedacted(fields []waftypes.FieldToMatch) bool {
	for _, field := range fields {
		if field.UriPath != nil {
			return true
		}
	}
	return false
}

func listDeliverySources(ctx context.Context, client accessLogCWLogsAPI) ([]cwltypes.DeliverySource, error) {
	var out []cwltypes.DeliverySource
	var token *string
	for {
		page, err := client.DescribeDeliverySources(ctx, &cloudwatchlogs.DescribeDeliverySourcesInput{NextToken: token})
		if err != nil {
			return nil, fmt.Errorf("describe delivery sources: %w", err)
		}
		out = append(out, page.DeliverySources...)
		if page.NextToken == nil || *page.NextToken == "" {
			break
		}
		token = page.NextToken
	}
	return out, nil
}

func listDeliveries(ctx context.Context, client accessLogCWLogsAPI) ([]cwltypes.Delivery, error) {
	var out []cwltypes.Delivery
	var token *string
	for {
		page, err := client.DescribeDeliveries(ctx, &cloudwatchlogs.DescribeDeliveriesInput{NextToken: token})
		if err != nil {
			return nil, fmt.Errorf("describe deliveries: %w", err)
		}
		out = append(out, page.Deliveries...)
		if page.NextToken == nil || *page.NextToken == "" {
			break
		}
		token = page.NextToken
	}
	return out, nil
}

func listDeliveryDestinations(ctx context.Context, client accessLogCWLogsAPI) ([]cwltypes.DeliveryDestination, error) {
	var out []cwltypes.DeliveryDestination
	var token *string
	for {
		page, err := client.DescribeDeliveryDestinations(ctx, &cloudwatchlogs.DescribeDeliveryDestinationsInput{NextToken: token})
		if err != nil {
			return nil, fmt.Errorf("describe delivery destinations: %w", err)
		}
		out = append(out, page.DeliveryDestinations...)
		if page.NextToken == nil || *page.NextToken == "" {
			break
		}
		token = page.NextToken
	}
	return out, nil
}

func deliveryDestination(dest cwltypes.DeliveryDestination, suffix string, hive bool) accesslog.Destination {
	arn := ""
	if dest.DeliveryDestinationConfiguration != nil {
		arn = ptrStr(dest.DeliveryDestinationConfiguration.DestinationResourceArn)
	}
	format := string(dest.OutputFormat)
	switch dest.DeliveryDestinationType {
	case cwltypes.DeliveryDestinationTypeS3:
		bucket, prefix := s3BucketPrefixFromARN(arn)
		return accesslog.Destination{
			Type:           accesslog.DestinationS3,
			Bucket:         bucket,
			Prefix:         prefix,
			Format:         format,
			SuffixPath:     suffix,
			HiveCompatible: hive,
		}
	case cwltypes.DeliveryDestinationType("CloudWatchLogs"):
		return accesslog.Destination{Type: accesslog.DestinationCloudWatchLogs, LogGroup: logGroupNameFromARN(arn), Format: format}
	case cwltypes.DeliveryDestinationType("Firehose"):
		return accesslog.Destination{Type: accesslog.DestinationFirehose, Format: format}
	default:
		return accesslog.Destination{Type: accesslog.DestinationNone}
	}
}

func markOriginALB(sources []accesslog.Source, origins map[string][]string) {
	for i := range sources {
		if sources[i].Type != accesslog.SourceALB || sources[i].DNSName == "" {
			continue
		}
		domain := strings.ToLower(sources[i].DNSName)
		for distribution, domains := range origins {
			for _, origin := range domains {
				if origin == domain {
					sources[i].OriginOf = distribution
					break
				}
			}
			if sources[i].OriginOf != "" {
				sources[i].Status.Issues = append(sources[i].Status.Issues,
					"client IP is the CloudFront edge IP; select the CloudFront source to aggregate viewer IPs")
				break
			}
		}
	}
}

func markSharedSources(sources []accesslog.Source) {
	groups := make(map[string][]int)
	for i, source := range sources {
		if source.Destination.Type == accesslog.DestinationNone {
			continue
		}
		key := destinationKey(source.Destination) + "|" + string(source.SchemaProfile)
		if key == "" {
			continue
		}
		groups[key] = append(groups[key], i)
	}
	for _, indexes := range groups {
		if len(indexes) < 2 {
			continue
		}
		for _, i := range indexes {
			for _, j := range indexes {
				if i == j {
					continue
				}
				sources[i].Status.SharedBy = append(sources[i].Status.SharedBy, sources[j].Name)
			}
			sources[i].Status.SharedBy = dedupeWarnings(sources[i].Status.SharedBy)
			sources[i].Status.Issues = append(sources[i].Status.Issues, fmt.Sprintf("this destination is shared by %d resources", len(indexes)))
		}
	}
}

func sortAccessLogSources(sources []accesslog.Source) {
	order := map[accesslog.SourceType]int{accesslog.SourceALB: 0, accesslog.SourceCloudFront: 1, accesslog.SourceWAF: 2}
	sort.SliceStable(sources, func(i, j int) bool {
		if order[sources[i].Type] != order[sources[j].Type] {
			return order[sources[i].Type] < order[sources[j].Type]
		}
		if sources[i].Name != sources[j].Name {
			return sources[i].Name < sources[j].Name
		}
		return sources[i].ID < sources[j].ID
	})
}

func filterSources(sources []accesslog.Source) []accesslog.Source {
	out := make([]accesslog.Source, 0, len(sources))
	for _, source := range sources {
		if source.ID == "" {
			continue
		}
		out = append(out, source)
	}
	return out
}

func filterWarnings(warnings []string) []string {
	out := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		if warning != "" {
			out = append(out, warning)
		}
	}
	return out
}

func dedupeWarnings(warnings []string) []string {
	seen := make(map[string]struct{}, len(warnings))
	out := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		if warning == "" {
			continue
		}
		if _, ok := seen[warning]; ok {
			continue
		}
		seen[warning] = struct{}{}
		out = append(out, warning)
	}
	return out
}

func accessLogSourceID(sourceType accesslog.SourceType, resource, destination, profile string) string {
	raw := string(sourceType) + "|" + resource + "|" + destination + "|" + profile
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func destinationKey(destination accesslog.Destination) string {
	switch destination.Type {
	case accesslog.DestinationS3:
		return "s3://" + destination.Bucket + "/" + strings.Trim(destination.Prefix, "/")
	case accesslog.DestinationCloudWatchLogs:
		return "cwl:" + destination.LogGroup
	case accesslog.DestinationFirehose:
		return "firehose"
	default:
		return "none"
	}
}

func albResourceIDFromARN(arn string) string {
	const marker = ":loadbalancer/"
	if idx := strings.Index(arn, marker); idx >= 0 {
		return arn[idx+len(marker):]
	}
	return arn
}

func logGroupNameFromARN(arn string) string {
	const marker = ":log-group:"
	if idx := strings.Index(arn, marker); idx >= 0 {
		return strings.TrimSuffix(arn[idx+len(marker):], ":*")
	}
	return arn
}

func s3BucketPrefixFromARN(arn string) (string, string) {
	value := strings.TrimPrefix(arn, "arn:aws:s3:::")
	bucket, prefix, _ := strings.Cut(value, "/")
	return bucket, prefix
}

// attachSetupSQL は Athena のソースに、テーブル作成用の DDL のテンプレートを付ける。
// 生成した DDL は実行せず、画面でコピーするために使う。
func attachSetupSQL(source *accesslog.Source) {
	if !source.Status.LoggingEnabled || source.Engine != accesslog.EngineAthena {
		return
	}
	table := source.SuggestedTable
	if table == "" {
		table = "access_logs"
	}
	ddl, err := accesslog.BuildDDL(accesslog.DDLInput{
		Source:   *source,
		Database: "default",
		Table:    table,
	})
	if err == nil {
		source.SetupSQL = ddl
	}
}

func hasRecordField(fields []string, name string) bool {
	for _, field := range fields {
		if strings.EqualFold(field, name) {
			return true
		}
	}
	return false
}

func normalizeS3Bucket(bucket string) string {
	bucket = strings.TrimSpace(bucket)
	bucket = strings.TrimSuffix(bucket, ".s3.amazonaws.com")
	if i := strings.Index(bucket, ".s3."); i > 0 && strings.HasSuffix(bucket, ".amazonaws.com") {
		return bucket[:i]
	}
	return bucket
}

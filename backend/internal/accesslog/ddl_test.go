package accesslog

import (
	"errors"
	"strings"
	"testing"
)

func TestBuildDDLALB(t *testing.T) {
	got, err := BuildDDL(DDLInput{
		Source: Source{
			SchemaProfile: SchemaALBLegacy,
			AccountID:     "123456789012",
			Region:        "ap-northeast-1",
			Destination: Destination{
				Type:   DestinationS3,
				Bucket: "example-logs",
				Prefix: "alb",
			},
		},
		Database: "logs",
		Table:    "alb_access_logs",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`CREATE EXTERNAL TABLE IF NOT EXISTS "logs"."alb_access_logs"`,
		`PARTITIONED BY (day string)`,
		`s3://example-logs/alb/AWSLogs/123456789012/elasticloadbalancing/ap-northeast-1/`,
		`"projection.enabled" = "true"`,
		`"storage.location.template"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("DDL does not contain %q:\n%s", want, got)
		}
	}
}

func TestBuildDDLCloudFrontLegacy(t *testing.T) {
	got, err := BuildDDL(DDLInput{
		Source: Source{
			SchemaProfile: SchemaCloudFrontLegacy,
			Destination:   Destination{Type: DestinationS3, Bucket: "cf-logs", Prefix: "legacy"},
		},
		Database: "logs",
		Table:    "cloudfront_legacy",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`ROW FORMAT DELIMITED`,
		`FIELDS TERMINATED BY '\t'`,
		`'skip.header.line.count'='2'`,
		`s3://cf-logs/legacy/`,
		`パーティションを持たない`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("DDL does not contain %q:\n%s", want, got)
		}
	}
}

func TestBuildDDLCloudFrontV2(t *testing.T) {
	got, err := BuildDDL(DDLInput{
		Source: Source{
			SchemaProfile: SchemaCloudFrontV2JSON,
			ResourceID:    "E123456789",
			AccountID:     "123456789012",
			Destination: Destination{
				Type:         DestinationS3,
				Bucket:       "cf-logs",
				Prefix:       "v2",
				RecordFields: []string{"date", "time", "c-ip", "cs-uri-stem", "sc-status", "cs(User-Agent)"},
				SuffixPath:   "{DistributionId}/{yyyy}/{MM}/{dd}/{HH}/",
			},
		},
		Database: "logs",
		Table:    "cloudfront_v2",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"c-ip" string`,
		`"cs(User-Agent)" string`,
		`PARTITIONED BY (distributionid string, year int, month int, day int, hour int)`,
		`${distributionid}/${year}/${month}/${day}/${hour}/`,
		`"projection.distributionid.values" = 'E123456789'`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("DDL does not contain %q:\n%s", want, got)
		}
	}
}

func TestBuildDDLWAF(t *testing.T) {
	got, err := BuildDDL(DDLInput{
		Source: Source{
			SchemaProfile: SchemaWAFJSON,
			Name:          "prod-waf",
			AccountID:     "123456789012",
			Region:        "ap-northeast-1",
			Destination:   Destination{Type: DestinationS3, Bucket: "waf-logs", Prefix: "waf"},
		},
		Database: "logs",
		Table:    "waf_logs",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`PARTITIONED BY (log_time string)`,
		`httprequest struct<clientip:string`,
		`"projection.log_time.format" = "yyyy/MM/dd/HH/mm"`,
		`s3://waf-logs/waf/AWSLogs/123456789012/WAFLogs/ap-northeast-1/prod-waf/`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("DDL does not contain %q:\n%s", want, got)
		}
	}
}

func TestBuildDDLInvalidTable(t *testing.T) {
	_, err := BuildDDL(DDLInput{
		Source:   Source{SchemaProfile: SchemaALBLegacy},
		Database: "logs",
		Table:    "bad table",
	})
	if !errors.Is(err, ErrInvalidTable) {
		t.Fatalf("err = %v, want ErrInvalidTable", err)
	}
}

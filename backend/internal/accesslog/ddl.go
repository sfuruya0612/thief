package accesslog

import (
	"fmt"
	"strings"
)

// DDLInput は Athena のテーブルを作る DDL を組み立てるための入力を表す。
type DDLInput struct {
	Source   Source
	Database string
	Table    string
}

// BuildDDL はソースの出力先から Athena のテーブルを作る DDL を生成する。
// 生成した DDL は実行せず、利用者が Athena のコンソールで実行する。
func BuildDDL(in DDLInput) (string, error) {
	if !identRe.MatchString(in.Database) {
		return "", fmt.Errorf("%w: database %q", ErrInvalidTable, in.Database)
	}
	if !identRe.MatchString(in.Table) {
		return "", fmt.Errorf("%w: table %q", ErrInvalidTable, in.Table)
	}
	switch in.Source.SchemaProfile {
	case SchemaALBLegacy:
		return buildALBDDL(in)
	case SchemaCloudFrontLegacy:
		return buildCloudFrontLegacyDDL(in)
	case SchemaCloudFrontV2JSON:
		return buildCloudFrontV2DDL(in)
	case SchemaWAFJSON:
		return buildWAFDDL(in)
	default:
		return "", fmt.Errorf("%w: %s", ErrUnknownProfile, in.Source.SchemaProfile)
	}
}

func buildALBDDL(in DDLInput) (string, error) {
	account := placeholder(in.Source.AccountID, "{account_id}")
	region := placeholder(in.Source.Region, "{region}")
	location := s3Location(in.Source.Destination, "AWSLogs/"+account+"/elasticloadbalancing/"+region+"/")
	return fmt.Sprintf(`CREATE EXTERNAL TABLE IF NOT EXISTS %s.%s (
  type string,
  time string,
  elb string,
  client_ip string,
  client_port int,
  target_ip string,
  target_port int,
  request_processing_time double,
  target_processing_time double,
  response_processing_time double,
  elb_status_code int,
  target_status_code string,
  received_bytes bigint,
  sent_bytes bigint,
  request_verb string,
  request_url string,
  request_proto string,
  user_agent string,
  ssl_cipher string,
  ssl_protocol string,
  target_group_arn string,
  trace_id string,
  domain_name string,
  chosen_cert_arn string,
  matched_rule_priority string,
  request_creation_time string,
  actions_executed string,
  redirect_url string,
  lambda_error_reason string,
  target_port_list string,
  target_status_code_list string,
  classification string,
  classification_reason string,
  conn_trace_id string
)
PARTITIONED BY (day string)
ROW FORMAT SERDE 'org.apache.hadoop.hive.serde2.RegexSerDe'
WITH SERDEPROPERTIES (
  'serialization.format' = '1',
  'input.regex' = '([^ ]*) ([^ ]*) ([^ ]*) ([^ ]*):([0-9]*) ([^ ]*)[:-]([0-9]*) ([-.0-9]*) ([-.0-9]*) ([-.0-9]*) (|[-0-9]*) (-|[-0-9]*) ([-0-9]*) ([-0-9]*) \"([^ ]*) (.*) (- |[^ ]*)\" \"([^\"]*)\" ([A-Z0-9-_]+) ([A-Za-z0-9.-]*) ([^ ]*) \"([^\"]*)\" \"([^\"]*)\" \"([^\"]*)\" ([-.0-9]*) ([^ ]*) \"([^\"]*)\" \"([^\"]*)\" \"([^ ]*)\" \"([^\\s]+?)\" \"([^\\s]+)\" \"([^ ]*)\" \"([^ ]*)\" ?([^ ]*)?'
)
LOCATION '%s'
TBLPROPERTIES (
  "projection.enabled" = "true",
  "projection.day.type" = "date",
  "projection.day.range" = "2026/01/01,NOW",
  "projection.day.format" = "yyyy/MM/dd",
  "projection.day.interval" = "1",
  "projection.day.interval.unit" = "DAYS",
  "storage.location.template" = '%s${day}/'
);`, quoteIdent(in.Database), quoteIdent(in.Table), escapeSQL(location), escapeSQL(location)), nil
}

func buildCloudFrontLegacyDDL(in DDLInput) (string, error) {
	location := s3Location(in.Source.Destination, "")
	return fmt.Sprintf(`CREATE EXTERNAL TABLE IF NOT EXISTS %s.%s (
  "date" date,
  time string,
  x_edge_location string,
  sc_bytes bigint,
  c_ip string,
  cs_method string,
  cs_host string,
  cs_uri_stem string,
  sc_status int,
  cs_referrer string,
  cs_user_agent string,
  cs_uri_query string,
  cs_cookie string,
  x_edge_result_type string,
  x_edge_request_id string,
  x_host_header string,
  cs_protocol string,
  cs_bytes bigint,
  time_taken float,
  x_forwarded_for string,
  ssl_protocol string,
  ssl_cipher string,
  x_edge_response_result_type string,
  cs_protocol_version string,
  fle_status string,
  fle_encrypted_fields int,
  c_port int,
  time_to_first_byte float,
  x_edge_detailed_result_type string,
  sc_content_type string,
  sc_content_len bigint,
  sc_range_start bigint,
  sc_range_end bigint
)
ROW FORMAT DELIMITED
FIELDS TERMINATED BY '\t'
LOCATION '%s'
TBLPROPERTIES ('skip.header.line.count'='2');
-- CloudFront の legacy 標準ログは S3 のパスに日付のパーティションを持たないため、
-- このテーブルはパーティションを持たない。スキャン量を抑えるには標準ログ v2 の
-- パーティションを使うか、ログを日付のパスに再配置する必要がある。`, quoteIdent(in.Database), quoteIdent(in.Table), escapeSQL(location)), nil
}

func buildCloudFrontV2DDL(in DDLInput) (string, error) {
	fields := in.Source.Destination.RecordFields
	if len(fields) == 0 {
		fields = []string{"date", "time", "x-edge-location", "sc-bytes", "c-ip", "cs-method", "cs(Host)", "cs-uri-stem", "sc-status", "cs(Referer)", "cs(User-Agent)", "cs-uri-query", "x-host-header", "x-edge-request-id", "x-edge-response-result-type", "x-edge-detailed-result-type"}
	}
	columns := make([]string, 0, len(fields))
	for _, field := range fields {
		columns = append(columns, "  "+quoteIdent(field)+" string")
	}
	account := placeholder(in.Source.AccountID, "{account_id}")
	suffix := in.Source.Destination.SuffixPath
	if suffix == "" {
		suffix = "AWSLogs/" + account + "/CloudFront/{DistributionId}/{yyyy}/{MM}/{dd}/{HH}/"
	}
	location := s3Location(in.Source.Destination, "")
	template := suffix
	replacements := []struct{ from, to string }{
		{"{DistributionId}", "${distributionid}"},
		{"{distributionid}", "${distributionid}"},
		{"{yyyy}", "${year}"},
		{"{MM}", "${month}"},
		{"{dd}", "${day}"},
		{"{HH}", "${hour}"},
		{"{accountid}", "${accountid}"},
		{"{account-id}", "${accountid}"},
	}
	for _, r := range replacements {
		template = strings.ReplaceAll(template, r.from, r.to)
	}
	hasDistribution := strings.Contains(template, "${distributionid}")
	partitionColumns := []string{"year int", "month int", "day int", "hour int"}
	if hasDistribution {
		partitionColumns = append([]string{"distributionid string"}, partitionColumns...)
	}
	properties := []string{
		`"projection.enabled" = "true"`,
		`"projection.year.type" = "integer"`,
		`"projection.year.range" = "2026,2030"`,
		`"projection.month.type" = "integer"`,
		`"projection.month.range" = "1,12"`,
		`"projection.month.digits" = "2"`,
		`"projection.day.type" = "integer"`,
		`"projection.day.range" = "1,31"`,
		`"projection.day.digits" = "2"`,
		`"projection.hour.type" = "integer"`,
		`"projection.hour.range" = "0,23"`,
		`"projection.hour.digits" = "2"`,
	}
	if hasDistribution {
		distributionID := placeholder(in.Source.ResourceID, "{distribution_id}")
		properties = append(properties,
			`"projection.distributionid.type" = "enum"`,
			fmt.Sprintf(`"projection.distributionid.values" = %s`, sqlString(distributionID)),
		)
	}
	template = strings.TrimSuffix(template, "/") + "/"
	properties = append(properties, fmt.Sprintf(`"storage.location.template" = %s`, sqlString(location+template)))
	return fmt.Sprintf(`CREATE EXTERNAL TABLE IF NOT EXISTS %s.%s (
%s
)
PARTITIONED BY (%s)
ROW FORMAT SERDE 'org.openx.data.jsonserde.JsonSerDe'
STORED AS INPUTFORMAT 'org.apache.hadoop.mapred.TextInputFormat'
OUTPUTFORMAT 'org.apache.hadoop.hive.ql.io.HiveIgnoreKeyTextOutputFormat'
LOCATION '%s'
TBLPROPERTIES (
  %s
);`, quoteIdent(in.Database), quoteIdent(in.Table), strings.Join(columns, ",\n"), strings.Join(partitionColumns, ", "), escapeSQL(location), strings.Join(properties, ",\n  ")), nil
}

func buildWAFDDL(in DDLInput) (string, error) {
	account := placeholder(in.Source.AccountID, "{account_id}")
	region := placeholder(in.Source.Region, "{region}")
	scope := region
	if in.Source.Global {
		scope = "cloudfront"
	}
	webACL := placeholder(in.Source.Name, "{webacl}")
	suffix := "AWSLogs/" + account + "/WAFLogs/" + scope + "/" + webACL + "/"
	location := s3Location(in.Source.Destination, suffix)
	return fmt.Sprintf(`CREATE EXTERNAL TABLE IF NOT EXISTS %s.%s (
  timestamp bigint,
  formatversion int,
  webaclid string,
  terminatingruleid string,
  terminatingruletype string,
  action string,
  terminatingrulematchdetails array<struct<conditiontype:string,sensitivitylevel:string,location:string,matcheddata:array<string>>>,
  httpsourcename string,
  httpsourceid string,
  rulegrouplist array<struct<rulegroupid:string,terminatingrule:struct<ruleid:string,action:string,rulematchdetails:array<struct<conditiontype:string,sensitivitylevel:string,location:string,matcheddata:array<string>>>>,nonterminatingmatchingrules:array<struct<ruleid:string,action:string,overriddenaction:string,rulematchdetails:array<struct<conditiontype:string,sensitivitylevel:string,location:string,matcheddata:array<string>>>,challengeresponse:struct<responsecode:string,solvetimestamp:string>,captcharesponse:struct<responsecode:string,solvetimestamp:string>>>,excludedrules:string>>,
  ratebasedrulelist array<struct<ratebasedruleid:string,limitkey:string,maxrateallowed:int>>,
  nonterminatingmatchingrules array<struct<ruleid:string,action:string,rulematchdetails:array<struct<conditiontype:string,sensitivitylevel:string,location:string,matcheddata:array<string>>>,challengeresponse:struct<responsecode:string,solvetimestamp:string>,captcharesponse:struct<responsecode:string,solvetimestamp:string>>>,
  requestheadersinserted array<struct<name:string,value:string>>,
  responsecodesent string,
  httprequest struct<clientip:string,country:string,headers:array<struct<name:string,value:string>>,uri:string,args:string,httpversion:string,httpmethod:string,requestid:string,fragment:string,scheme:string,host:string>,
  labels array<struct<name:string>>,
  captcharesponse struct<responsecode:string,solvetimestamp:string,failurereason:string>,
  challengeresponse struct<responsecode:string,solvetimestamp:string,failurereason:string>,
  ja3fingerprint string,
  ja4fingerprint string,
  oversizefields string,
  requestbodysize int,
  requestbodysizeinspectedbywaf int
)
PARTITIONED BY (log_time string)
ROW FORMAT SERDE 'org.openx.data.jsonserde.JsonSerDe'
STORED AS INPUTFORMAT 'org.apache.hadoop.mapred.TextInputFormat'
OUTPUTFORMAT 'org.apache.hadoop.hive.ql.io.HiveIgnoreKeyTextOutputFormat'
LOCATION '%s'
TBLPROPERTIES (
  "projection.enabled" = "true",
  "projection.log_time.type" = "date",
  "projection.log_time.format" = "yyyy/MM/dd/HH/mm",
  "projection.log_time.interval" = "1",
  "projection.log_time.interval.unit" = "MINUTES",
  "projection.log_time.range" = "2026/01/01/00/00,NOW",
  "storage.location.template" = '%s${log_time}/'
);`, quoteIdent(in.Database), quoteIdent(in.Table), escapeSQL(location), escapeSQL(location)), nil
}

func s3Location(d Destination, suffix string) string {
	prefix := strings.Trim(d.Prefix, "/")
	parts := []string{"s3://" + d.Bucket}
	if prefix != "" {
		parts = append(parts, prefix)
	}
	if suffix != "" {
		parts = append(parts, strings.Trim(suffix, "/"))
	}
	return strings.Join(parts, "/") + "/"
}

func placeholder(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func escapeSQL(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func sqlString(s string) string {
	return "'" + escapeSQL(s) + "'"
}

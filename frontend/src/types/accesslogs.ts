// Access Logs の Raw (backend JSON) と Row (UI 用) 型定義。
// backend/internal/accesslog と backend/internal/api/access_log_models.go の JSON タグをミラーする。
import type { BaseRow } from './common';

export type AccessLogSourceType = 'alb' | 'cloudfront' | 'waf';
export type AccessLogEngine = 'athena' | 'logs_insights' | '';
export type AccessLogDestinationType = 'none' | 's3' | 'cloudwatch_logs' | 'firehose';
export type AccessLogQueryState =
  'queued' | 'running' | 'succeeded' | 'partial' | 'failed' | 'cancelled';
export type AccessLogView = 'pairs' | 'ips';

export interface AccessLogDestinationRaw {
  type: AccessLogDestinationType;
  bucket?: string;
  prefix?: string;
  log_group?: string;
  format?: string;
  field_delimiter?: string;
  record_fields?: string[];
  suffix_path?: string;
  hive_compatible?: boolean;
}

export interface AccessLogSourceStatusRaw {
  logging_enabled: boolean;
  destination_type: AccessLogDestinationType;
  destination_readable: boolean;
  table_configured: boolean;
  table_valid: boolean;
  data_present?: boolean;
  last_delivery_at?: string;
  source_key?: string;
  attribution: boolean;
  shared_by?: string[];
  issues?: string[];
}

export interface AccessLogSourceRaw {
  id: string;
  type: AccessLogSourceType;
  name: string;
  region: string;
  query_region: string;
  global?: boolean;
  account_id?: string;
  resource_arn?: string;
  resource_id?: string;
  dns_name?: string;
  domain_name?: string;
  aliases?: string[];
  origin_of?: string;
  engine: AccessLogEngine;
  destination: AccessLogDestinationRaw;
  schema_profile?: string;
  status: AccessLogSourceStatusRaw;
  source_key_values?: string[];
  suggested_table?: string;
  setup_sql?: string;
}

export interface AccessLogSourcesResponseRaw {
  sources: AccessLogSourceRaw[];
  warnings?: string[];
}

export interface AccessLogTableValidationRaw {
  valid: boolean;
  schema_profile?: string;
  columns?: string[];
  partition_keys?: string[];
  missing_columns?: string[];
  source_key_column?: string;
  attribution: boolean;
  location?: string;
  input_format?: string;
  serde?: string;
  issues?: string[];
}

export interface AccessLogTableValidateResponseRaw {
  validation: AccessLogTableValidationRaw;
  setup_sql?: string;
}

export interface AccessLogPairRaw {
  source_key?: string;
  client_ip: string;
  uri_path: string;
  request_count: number;
  blocked_count: number;
  challenged_count: number;
  count_matched: number;
  client_error_count: number;
  server_error_count: number;
  sources?: string[];
  source_counts?: Record<string, number>;
  first_seen?: string;
  last_seen?: string;
  sample_user_agent?: string;
  sample_rule_id?: string;
  has_query?: boolean;
  sample_query?: string;
  flags?: string[];
}

export interface AccessLogIPRaw {
  source_key?: string;
  client_ip: string;
  request_count: number;
  blocked_count: number;
  challenged_count: number;
  count_matched: number;
  client_error_count: number;
  server_error_count: number;
  distinct_uris: number;
  sources?: string[];
  source_counts?: Record<string, number>;
  first_seen?: string;
  last_seen?: string;
  flags?: string[];
}

export interface AccessLogSourceSummaryRaw {
  source_id: string;
  records: number;
  blocked: number;
  challenged: number;
  client_errors: number;
  server_errors: number;
  bytes_scanned: number;
  records_scanned: number;
}

export interface AccessLogSummaryRaw {
  source_count: number;
  record_count: number;
  blocked_count: number;
  challenged_count: number;
  client_error_count: number;
  server_error_count: number;
  distinct_ips: number;
  distinct_uris: number;
  by_sources?: AccessLogSourceSummaryRaw[];
  duplicate_warning: boolean;
}

export interface AccessLogSourceErrorRaw {
  code: string;
  message: string;
}

export interface AccessLogQuerySourceStatusRaw {
  source_id: string;
  name: string;
  type: AccessLogSourceType;
  engine: AccessLogEngine;
  state: AccessLogQueryState;
  bytes_scanned: number;
  records_scanned: number;
  records_matched: number;
  error?: AccessLogSourceErrorRaw;
}

export interface AccessLogQueryResponseRaw {
  job_id: string;
  state: AccessLogQueryState;
  sources: AccessLogQuerySourceStatusRaw[];
  warnings?: string[];
}

export interface AccessLogResultsResponseRaw {
  rows: AccessLogPairRaw[];
  ips?: AccessLogIPRaw[];
  summary: AccessLogSummaryRaw;
  warnings?: string[];
  sources: AccessLogQuerySourceStatusRaw[];
}

export interface AccessLogTableMapping {
  catalog: string;
  database: string;
  table: string;
}

export interface AccessLogTableValidateRequest {
  source_id: string;
  catalog: string;
  database: string;
  table: string;
  workgroup?: string;
}

export interface AccessLogQueryStartSource {
  id: string;
  table?: AccessLogTableMapping;
}

export interface AccessLogQueryStartRequest {
  view: AccessLogView;
  sources: AccessLogQueryStartSource[];
  from: string;
  to: string;
  filter: {
    only_blocked?: boolean;
    status_classes?: string[];
    ip?: string;
    uri_prefix?: string;
  };
  limit: number;
  confirm_long_range: boolean;
  workgroup?: string;
  output_location?: string;
}

export interface AccessLogQueryStartResponseRaw {
  job_id: string;
}

export interface AccessLogDestination {
  type: AccessLogDestinationType;
  bucket?: string;
  prefix?: string;
  logGroup?: string;
  format?: string;
  recordFields?: string[];
  suffixPath?: string;
  hiveCompatible?: boolean;
}

export interface AccessLogSourceStatus {
  loggingEnabled: boolean;
  destinationType: AccessLogDestinationType;
  destinationReadable: boolean;
  tableConfigured: boolean;
  tableValid: boolean;
  dataPresent?: boolean;
  lastDeliveryAt?: string;
  sourceKey?: string;
  attribution: boolean;
  sharedBy: string[];
  issues: string[];
}

export interface AccessLogSource {
  id: string;
  type: AccessLogSourceType;
  name: string;
  region: string;
  queryRegion: string;
  global: boolean;
  accountId?: string;
  resourceArn?: string;
  resourceId?: string;
  dnsName?: string;
  domainName?: string;
  aliases: string[];
  originOf?: string;
  engine: AccessLogEngine;
  destination: AccessLogDestination;
  schemaProfile?: string;
  status: AccessLogSourceStatus;
  sourceKeyValues: string[];
  suggestedTable?: string;
  setupSql?: string;
}

export interface AccessLogSourcesResponse {
  sources: AccessLogSource[];
  warnings: string[];
}

export interface AccessLogTableValidation {
  valid: boolean;
  schemaProfile?: string;
  columns: string[];
  partitionKeys: string[];
  missingColumns: string[];
  sourceKeyColumn?: string;
  attribution: boolean;
  location?: string;
  inputFormat?: string;
  serde?: string;
  issues: string[];
}

export interface AccessLogTableValidateResponse {
  validation: AccessLogTableValidation;
  setupSql?: string;
}

export interface AccessLogPairRow extends BaseRow {
  clientIp: string;
  uriPath: string;
  requestCount: number;
  blockedCount: number;
  challengedCount: number;
  countMatched: number;
  clientErrorCount: number;
  serverErrorCount: number;
  sourceIds: string[];
  sourceLabels: string[];
  firstSeen?: string;
  lastSeen?: string;
  sampleUserAgent?: string;
  sampleRuleId?: string;
  hasQuery: boolean;
  sampleQuery?: string;
  flags: string[];
}

export interface AccessLogIPRow extends BaseRow {
  clientIp: string;
  requestCount: number;
  blockedCount: number;
  challengedCount: number;
  countMatched: number;
  clientErrorCount: number;
  serverErrorCount: number;
  distinctUris: number;
  sourceIds: string[];
  sourceLabels: string[];
  firstSeen?: string;
  lastSeen?: string;
  flags: string[];
}

export interface AccessLogSourceSummary {
  sourceId: string;
  records: number;
  blocked: number;
  challenged: number;
  clientErrors: number;
  serverErrors: number;
  bytesScanned: number;
  recordsScanned: number;
}

export interface AccessLogSummary {
  sourceCount: number;
  recordCount: number;
  blockedCount: number;
  challengedCount: number;
  clientErrorCount: number;
  serverErrorCount: number;
  distinctIps: number;
  distinctUris: number;
  bySources: AccessLogSourceSummary[];
  duplicateWarning: boolean;
}

export interface AccessLogQuerySourceStatus {
  sourceId: string;
  name: string;
  type: AccessLogSourceType;
  engine: AccessLogEngine;
  state: AccessLogQueryState;
  bytesScanned: number;
  recordsScanned: number;
  recordsMatched: number;
  error?: { code: string; message: string };
}

export interface AccessLogQuery {
  jobId: string;
  state: AccessLogQueryState;
  sources: AccessLogQuerySourceStatus[];
  warnings: string[];
}

export interface AccessLogResults {
  rows: AccessLogPairRow[];
  ips: AccessLogIPRow[];
  summary: AccessLogSummary;
  warnings: string[];
  sources: AccessLogQuerySourceStatus[];
}

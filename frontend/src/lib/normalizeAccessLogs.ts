// Access Logs の Raw (backend JSON) を UI 用の Row に変換する純関数。
import type {
  AccessLogDestination,
  AccessLogDestinationRaw,
  AccessLogIPRaw,
  AccessLogIPRow,
  AccessLogPairRaw,
  AccessLogPairRow,
  AccessLogQuery,
  AccessLogQueryResponseRaw,
  AccessLogQuerySourceStatusRaw,
  AccessLogResults,
  AccessLogResultsResponseRaw,
  AccessLogSource,
  AccessLogSourceRaw,
  AccessLogSourceStatusRaw,
  AccessLogSourceSummaryRaw,
  AccessLogSummaryRaw,
  AccessLogTableValidation,
  AccessLogTableValidationRaw,
} from '../types/accesslogs';

export function accessLogDestinationFromRaw(
  raw: AccessLogDestinationRaw | undefined,
): AccessLogDestination {
  return {
    type: raw?.type ?? 'none',
    bucket: raw?.bucket,
    prefix: raw?.prefix,
    logGroup: raw?.log_group,
    format: raw?.format,
    recordFields: raw?.record_fields ?? [],
    suffixPath: raw?.suffix_path,
    hiveCompatible: raw?.hive_compatible ?? false,
  };
}

function sourceStatusFromRaw(raw: AccessLogSourceStatusRaw) {
  return {
    loggingEnabled: raw.logging_enabled,
    destinationType: raw.destination_type,
    destinationReadable: raw.destination_readable,
    tableConfigured: raw.table_configured,
    tableValid: raw.table_valid,
    dataPresent: raw.data_present,
    lastDeliveryAt: raw.last_delivery_at,
    sourceKey: raw.source_key,
    attribution: raw.attribution,
    sharedBy: raw.shared_by ?? [],
    issues: raw.issues ?? [],
  };
}

export function accessLogSourceFromRaw(raw: AccessLogSourceRaw): AccessLogSource {
  return {
    id: raw.id,
    type: raw.type,
    name: raw.name,
    region: raw.region,
    queryRegion: raw.query_region,
    global: raw.global ?? false,
    accountId: raw.account_id,
    resourceArn: raw.resource_arn,
    resourceId: raw.resource_id,
    dnsName: raw.dns_name,
    domainName: raw.domain_name,
    aliases: raw.aliases ?? [],
    originOf: raw.origin_of,
    engine: raw.engine,
    destination: accessLogDestinationFromRaw(raw.destination),
    schemaProfile: raw.schema_profile,
    status: sourceStatusFromRaw(raw.status),
    sourceKeyValues: raw.source_key_values ?? [],
    suggestedTable: raw.suggested_table,
    setupSql: raw.setup_sql,
  };
}

export function accessLogSourcesFromRaw(raw: {
  sources: AccessLogSourceRaw[];
  warnings?: string[];
}): { sources: AccessLogSource[]; warnings: string[] } {
  return {
    sources: raw.sources.map(accessLogSourceFromRaw),
    warnings: raw.warnings ?? [],
  };
}

export function accessLogTableValidationFromRaw(
  raw: AccessLogTableValidationRaw,
): AccessLogTableValidation {
  return {
    valid: raw.valid,
    schemaProfile: raw.schema_profile,
    columns: raw.columns ?? [],
    partitionKeys: raw.partition_keys ?? [],
    missingColumns: raw.missing_columns ?? [],
    sourceKeyColumn: raw.source_key_column,
    attribution: raw.attribution,
    location: raw.location,
    inputFormat: raw.input_format,
    serde: raw.serde,
    issues: raw.issues ?? [],
  };
}

export function accessLogPairFromRaw(
  raw: AccessLogPairRaw,
  sourceNames: Record<string, string>,
): AccessLogPairRow {
  const sourceIds = raw.sources ?? [];
  return {
    id: `${raw.client_ip}|${raw.uri_path}`,
    name: raw.uri_path,
    state: raw.flags?.includes('blocked') ? 'blocked' : undefined,
    clientIp: raw.client_ip,
    uriPath: raw.uri_path,
    requestCount: raw.request_count ?? 0,
    blockedCount: raw.blocked_count ?? 0,
    challengedCount: raw.challenged_count ?? 0,
    countMatched: raw.count_matched ?? 0,
    clientErrorCount: raw.client_error_count ?? 0,
    serverErrorCount: raw.server_error_count ?? 0,
    sourceIds,
    sourceLabels: sourceIds.map((id) => sourceNames[id] ?? id),
    firstSeen: raw.first_seen,
    lastSeen: raw.last_seen,
    sampleUserAgent: raw.sample_user_agent,
    sampleRuleId: raw.sample_rule_id,
    hasQuery: raw.has_query ?? false,
    sampleQuery: raw.sample_query,
    flags: raw.flags ?? [],
  };
}

export function accessLogIPFromRaw(
  raw: AccessLogIPRaw,
  sourceNames: Record<string, string>,
): AccessLogIPRow {
  const sourceIds = raw.sources ?? [];
  return {
    id: raw.client_ip,
    name: raw.client_ip,
    state: raw.flags?.includes('blocked') ? 'blocked' : undefined,
    clientIp: raw.client_ip,
    requestCount: raw.request_count ?? 0,
    blockedCount: raw.blocked_count ?? 0,
    challengedCount: raw.challenged_count ?? 0,
    countMatched: raw.count_matched ?? 0,
    clientErrorCount: raw.client_error_count ?? 0,
    serverErrorCount: raw.server_error_count ?? 0,
    distinctUris: raw.distinct_uris ?? 0,
    sourceIds,
    sourceLabels: sourceIds.map((id) => sourceNames[id] ?? id),
    firstSeen: raw.first_seen,
    lastSeen: raw.last_seen,
    flags: raw.flags ?? [],
  };
}

function sourceSummaryFromRaw(raw: AccessLogSourceSummaryRaw) {
  return {
    sourceId: raw.source_id,
    records: raw.records ?? 0,
    blocked: raw.blocked ?? 0,
    challenged: raw.challenged ?? 0,
    clientErrors: raw.client_errors ?? 0,
    serverErrors: raw.server_errors ?? 0,
    bytesScanned: raw.bytes_scanned ?? 0,
    recordsScanned: raw.records_scanned ?? 0,
  };
}

export function accessLogSummaryFromRaw(raw: AccessLogSummaryRaw) {
  return {
    sourceCount: raw.source_count ?? 0,
    recordCount: raw.record_count ?? 0,
    blockedCount: raw.blocked_count ?? 0,
    challengedCount: raw.challenged_count ?? 0,
    clientErrorCount: raw.client_error_count ?? 0,
    serverErrorCount: raw.server_error_count ?? 0,
    distinctIps: raw.distinct_ips ?? 0,
    distinctUris: raw.distinct_uris ?? 0,
    bySources: (raw.by_sources ?? []).map(sourceSummaryFromRaw),
    duplicateWarning: raw.duplicate_warning ?? false,
  };
}

function querySourceFromRaw(raw: AccessLogQuerySourceStatusRaw) {
  return {
    sourceId: raw.source_id,
    name: raw.name,
    type: raw.type,
    engine: raw.engine,
    state: raw.state,
    bytesScanned: raw.bytes_scanned ?? 0,
    recordsScanned: raw.records_scanned ?? 0,
    recordsMatched: raw.records_matched ?? 0,
    error: raw.error,
  };
}

export function accessLogQueryFromRaw(raw: AccessLogQueryResponseRaw): AccessLogQuery {
  return {
    jobId: raw.job_id,
    state: raw.state,
    sources: raw.sources.map(querySourceFromRaw),
    warnings: raw.warnings ?? [],
  };
}

export function accessLogResultsFromRaw(
  raw: AccessLogResultsResponseRaw,
  sourceNames: Record<string, string>,
): AccessLogResults {
  return {
    rows: raw.rows.map((row) => accessLogPairFromRaw(row, sourceNames)),
    ips: (raw.ips ?? []).map((row) => accessLogIPFromRaw(row, sourceNames)),
    summary: accessLogSummaryFromRaw(raw.summary),
    warnings: raw.warnings ?? [],
    sources: raw.sources.map(querySourceFromRaw),
  };
}

// accessLogDestinationKey は同じ出力先を共有するソースを同じキーにまとめる。
// テーブルの対応付けを出力先の単位で保存するために使う。
export function accessLogDestinationKey(source: AccessLogSource): string {
  const destination = source.destination;
  const profile = source.schemaProfile ?? source.engine;
  switch (destination.type) {
    case 's3':
      return `s3://${destination.bucket ?? ''}/${(destination.prefix ?? '').replace(/^\/+|\/+$/g, '')}|${profile}`;
    case 'cloudwatch_logs':
      return `cwl:${destination.logGroup ?? ''}|${profile}`;
    default:
      return `none:${source.id}`;
  }
}

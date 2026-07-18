import type {
  CFNStackDetailRaw,
  CFNStackEventRaw,
  CFNStackResourceRaw,
  CostRaw,
  CWLogEventPageRaw,
  CWLogGroupRaw,
  DynamoItemRaw,
  DynamoTableSchemaRaw,
  ECRImageRaw,
  ECSContainerRaw,
  ECSServiceRaw,
  ECSTaskRaw,
  ELBListenerRaw,
  ELBRuleRaw,
  ELBTargetGroupRaw,
  ELBTargetHealthRaw,
  ForecastRaw,
  RegionRaw,
  S3ObjectRaw,
} from '../types/aws';
import type {
  CallerIdentityRaw,
  ObjectListEnvelopeRaw,
  ObjectPreviewRaw,
  ProfileRaw,
} from '../types/common';
import type {
  BQDatasetRaw,
  BQFieldRaw,
  BQTableRaw,
  DatadogCostRaw,
  TiDBClusterRaw,
  TiDBCostRaw,
  TiDBProjectRaw,
} from '../types/nonaws';
import type {
  AthenaCatalogRaw,
  AthenaDatabaseRaw,
  AthenaExecutionRaw,
  AthenaResultPageRaw,
  AthenaTableRaw,
  AthenaWorkgroupRaw,
  BQDryRunRaw,
  BQHistoryItemRaw,
  BQJobInfoRaw,
  BQJobStatusRaw,
  BQResultPageRaw,
  SnippetRaw,
} from '../types/query';
import type {
  CloudRunResourceRaw,
  GcpProjectRaw,
  GcsBucketRaw,
  GcsObjectRaw,
  LogEntryPageRaw,
} from '../types/gcp';
import { GCP_SERVICE_TO_PATH, SERVICE_TO_PATH } from '../lib/serviceMeta';
import { apiBaseUrl, apiDelete, apiGet, apiGetList, apiPost, apiPostForm } from './client';

export function getProfiles(): Promise<ProfileRaw[]> {
  // バックエンドは users がない場合 null を返しうるので配列に正規化する
  return apiGetList<ProfileRaw>('/api/aws/profiles');
}

// backend 起動待ちの疎通確認用。認証やクラウド呼び出しを伴わない。
export function getHealth(): Promise<{ status: string }> {
  return apiGet('/api/health');
}

// 選択されたプロファイル 1 件だけ STS GetCallerIdentity で Account ID を確定する。
// 一覧取得 (getProfiles) は ~/.aws/config の静的パースのみで SSO ログイン不要だが、
// role_arn / credential_process 系プロファイルでは Account ID が config に無いため、
// 選択時にこちらで補完する。
export function getProfileIdentity(profile: string): Promise<CallerIdentityRaw> {
  return apiGet<CallerIdentityRaw>(`/api/aws/profiles/${encodeURIComponent(profile)}/identity`);
}

export function getResources<TRaw>(
  service: string,
  profile: string,
  region: string,
  opts?: { refresh?: boolean },
): Promise<TRaw[]> {
  const seg = SERVICE_TO_PATH[service];
  if (!seg) {
    return Promise.reject(new Error(`unknown service key: ${service}`));
  }
  return apiGetList<TRaw>(`/api/aws/profiles/${encodeURIComponent(profile)}/${seg}`, {
    region,
    refresh: opts?.refresh ? true : undefined,
  });
}

// Cost Explorer の検索条件。省略時のバックエンド側デフォルトは Granularity: DAILY /
// GroupByDimension: SERVICE / Months: 1 (直近 1 ヶ月)。
// startDate/endDate (YYYY-MM-DD) を両方指定すると任意期間の取得になり、months は無視される。
// サービス名でのフィルタはブラウザ側 (取得済みデータへのフィルタ) で行うため API には持たない。
export interface CostQueryOptions {
  includeToday?: boolean;
  granularity?: string;
  groupBy?: string;
  startDate?: string;
  endDate?: string;
  months?: number;
}

export function getCost(
  profile: string,
  region: string,
  opts?: CostQueryOptions,
): Promise<CostRaw[]> {
  return apiGetList<CostRaw>(`/api/aws/profiles/${encodeURIComponent(profile)}/cost`, {
    region,
    include_today: opts?.includeToday ? true : undefined,
    granularity: opts?.granularity,
    group_by: opts?.groupBy,
    start: opts?.startDate,
    end: opts?.endDate,
    months: opts?.months !== undefined ? String(opts.months) : undefined,
  });
}

export function getCostForecast(profile: string, region: string): Promise<ForecastRaw[]> {
  return apiGetList<ForecastRaw>(`/api/aws/profiles/${encodeURIComponent(profile)}/cost/forecast`, {
    region,
  });
}

// SSO ログインを開始する (バックエンドが `aws sso login` を起動する)
export function postSSOLogin(profile: string): Promise<void> {
  return apiPost<void>(`/api/aws/profiles/${encodeURIComponent(profile)}/sso/login`);
}

// ============================================================
// Region (DescribeRegions からの動的取得)
// ============================================================
export function getRegions(profile: string): Promise<RegionRaw[]> {
  return apiGetList<RegionRaw>(`/api/aws/profiles/${encodeURIComponent(profile)}/regions`);
}

// ============================================================
// S3 Objects (Drawer の Objects タブ)
// ============================================================
// バックエンドは {objects, truncated} エンベロープを返す (1000 件で打ち切られた場合の通知用)。
export async function getS3Objects(
  profile: string,
  region: string,
  bucket: string,
  prefix?: string,
): Promise<ObjectListEnvelopeRaw<S3ObjectRaw>> {
  return apiGet<ObjectListEnvelopeRaw<S3ObjectRaw>>(
    `/api/aws/profiles/${encodeURIComponent(profile)}/s3/${encodeURIComponent(bucket)}/objects`,
    { region, prefix },
  );
}

export function uploadS3Object(
  profile: string,
  region: string,
  bucket: string,
  key: string,
  file: File,
): Promise<{ status: string; key: string }> {
  const formData = new FormData();
  formData.append('file', file);
  return apiPostForm<{ status: string; key: string }>(
    `/api/aws/profiles/${encodeURIComponent(profile)}/s3/${encodeURIComponent(bucket)}/objects/upload`,
    formData,
    { region, key },
  );
}

export function s3DownloadUrl(
  profile: string,
  region: string,
  bucket: string,
  key: string,
): string {
  const url = new URL(
    `/api/aws/profiles/${encodeURIComponent(profile)}/s3/${encodeURIComponent(bucket)}/objects/download`,
    apiBaseUrl(),
  );
  url.searchParams.set('region', region);
  url.searchParams.set('key', key);
  return url.toString();
}

export function getS3ObjectPreview(
  profile: string,
  region: string,
  bucket: string,
  key: string,
): Promise<ObjectPreviewRaw> {
  return apiGet<ObjectPreviewRaw>(
    `/api/aws/profiles/${encodeURIComponent(profile)}/s3/${encodeURIComponent(bucket)}/objects/preview`,
    { region, key },
  );
}

// ============================================================
// ECS Services / Tasks / Containers (Terminal タブの Exec 対象選択に使う)
// ============================================================
export function getECSServices(
  profile: string,
  region: string,
  cluster: string,
): Promise<ECSServiceRaw[]> {
  return apiGetList<ECSServiceRaw>(
    `/api/aws/profiles/${encodeURIComponent(profile)}/ecs/${encodeURIComponent(cluster)}/services`,
    { region },
  );
}

export function getECSTasks(
  profile: string,
  region: string,
  cluster: string,
  service?: string,
): Promise<ECSTaskRaw[]> {
  return apiGetList<ECSTaskRaw>(
    `/api/aws/profiles/${encodeURIComponent(profile)}/ecs/${encodeURIComponent(cluster)}/tasks`,
    { region, service },
  );
}

export function getECSContainers(
  profile: string,
  region: string,
  cluster: string,
  task: string,
): Promise<ECSContainerRaw[]> {
  return apiGetList<ECSContainerRaw>(
    `/api/aws/profiles/${encodeURIComponent(profile)}/ecs/${encodeURIComponent(cluster)}/tasks/${encodeURIComponent(task)}/containers`,
    { region },
  );
}

// ============================================================
// ECR (Drawer の Images タブでリポジトリごとにタグ一覧を取得する)
// ============================================================
export function getECRImages(
  profile: string,
  region: string,
  repo: string,
): Promise<ECRImageRaw[]> {
  return apiGetList<ECRImageRaw>(
    `/api/aws/profiles/${encodeURIComponent(profile)}/ecr/${encodeURIComponent(repo)}/images`,
    { region },
  );
}

// ============================================================
// CloudFormation (Drawer の Overview / Events / Resources タブ)
// ============================================================
export function getCFNStackDetail(
  profile: string,
  region: string,
  stack: string,
): Promise<CFNStackDetailRaw> {
  return apiGet<CFNStackDetailRaw>(
    `/api/aws/profiles/${encodeURIComponent(profile)}/cfn/stacks/${encodeURIComponent(stack)}`,
    { region },
  );
}

export function getCFNStackEvents(
  profile: string,
  region: string,
  stack: string,
): Promise<CFNStackEventRaw[]> {
  return apiGetList<CFNStackEventRaw>(
    `/api/aws/profiles/${encodeURIComponent(profile)}/cfn/stacks/${encodeURIComponent(stack)}/events`,
    { region },
  );
}

export function getCFNStackResources(
  profile: string,
  region: string,
  stack: string,
): Promise<CFNStackResourceRaw[]> {
  return apiGetList<CFNStackResourceRaw>(
    `/api/aws/profiles/${encodeURIComponent(profile)}/cfn/stacks/${encodeURIComponent(stack)}/resources`,
    { region },
  );
}

// ============================================================
// ELB Listener / Rule / TargetGroup / TargetHealth (Drawer の Listeners / Targets タブ)
// ============================================================
export function getELBListeners(
  profile: string,
  region: string,
  lbArn: string,
): Promise<ELBListenerRaw[]> {
  return apiGetList<ELBListenerRaw>(
    `/api/aws/profiles/${encodeURIComponent(profile)}/elb/listeners`,
    { region, lb_arn: lbArn },
  );
}

export function getELBRules(
  profile: string,
  region: string,
  listenerArn: string,
): Promise<ELBRuleRaw[]> {
  return apiGetList<ELBRuleRaw>(`/api/aws/profiles/${encodeURIComponent(profile)}/elb/rules`, {
    region,
    listener_arn: listenerArn,
  });
}

export function getELBTargetGroups(
  profile: string,
  region: string,
  lbArn: string,
): Promise<ELBTargetGroupRaw[]> {
  return apiGetList<ELBTargetGroupRaw>(
    `/api/aws/profiles/${encodeURIComponent(profile)}/elb/target-groups`,
    { region, lb_arn: lbArn },
  );
}

export function getELBTargetHealth(
  profile: string,
  region: string,
  tgArn: string,
): Promise<ELBTargetHealthRaw[]> {
  return apiGetList<ELBTargetHealthRaw>(
    `/api/aws/profiles/${encodeURIComponent(profile)}/elb/target-health`,
    { region, tg_arn: tgArn },
  );
}

// ============================================================
// DynamoDB Item 検索 (Drawer の Items タブ)
// ============================================================
export function getDynamoSchema(
  profile: string,
  region: string,
  table: string,
): Promise<DynamoTableSchemaRaw> {
  return apiGet<DynamoTableSchemaRaw>(
    `/api/aws/profiles/${encodeURIComponent(profile)}/dynamo/${encodeURIComponent(table)}/schema`,
    { region },
  );
}

export interface DynamoItemQueryOptions {
  pkValue?: string;
  skValue?: string;
  attrName?: string;
  attrValue?: string;
  limit?: number;
}

export function getDynamoItems(
  profile: string,
  region: string,
  table: string,
  opts: DynamoItemQueryOptions = {},
): Promise<DynamoItemRaw[]> {
  return apiGetList<DynamoItemRaw>(
    `/api/aws/profiles/${encodeURIComponent(profile)}/dynamo/${encodeURIComponent(table)}/items`,
    {
      region,
      pk_val: opts.pkValue,
      sk_val: opts.skValue,
      attr_name: opts.attrName,
      attr_val: opts.attrValue,
      limit: opts.limit !== undefined ? String(opts.limit) : undefined,
    },
  );
}

// ============================================================
// BigQuery
// ============================================================
export function getBQDatasets(projectId?: string): Promise<BQDatasetRaw[]> {
  return apiGetList<BQDatasetRaw>('/api/bigquery/datasets', { project_id: projectId });
}

export function getBQTables(dataset: string, projectId?: string): Promise<BQTableRaw[]> {
  return apiGetList<BQTableRaw>(`/api/bigquery/datasets/${encodeURIComponent(dataset)}/tables`, {
    project_id: projectId,
  });
}

export function getBQSchema(
  dataset: string,
  table: string,
  projectId?: string,
): Promise<BQFieldRaw[]> {
  return apiGetList<BQFieldRaw>(
    `/api/bigquery/datasets/${encodeURIComponent(dataset)}/tables/${encodeURIComponent(table)}/schema`,
    { project_id: projectId },
  );
}

// ============================================================
// BigQuery クエリエディタ (非同期ジョブ)
// ============================================================
export function postBQQueryStart(sql: string, projectId?: string): Promise<BQJobInfoRaw> {
  return apiPost<BQJobInfoRaw>('/api/bigquery/query', { project_id: projectId, sql });
}

export function postBQDryRun(sql: string, projectId?: string): Promise<BQDryRunRaw> {
  return apiPost<BQDryRunRaw>('/api/bigquery/query/dryrun', { project_id: projectId, sql });
}

export function getBQQueryJob(
  jobId: string,
  location: string,
  projectId?: string,
): Promise<BQJobStatusRaw> {
  return apiGet<BQJobStatusRaw>(`/api/bigquery/query/jobs/${encodeURIComponent(jobId)}`, {
    location: location || undefined,
    project_id: projectId,
  });
}

export function getBQQueryResults(
  jobId: string,
  location: string,
  projectId?: string,
  pageToken?: string,
  pageSize?: number,
): Promise<BQResultPageRaw> {
  return apiGet<BQResultPageRaw>(`/api/bigquery/query/jobs/${encodeURIComponent(jobId)}/results`, {
    location: location || undefined,
    project_id: projectId,
    page_token: pageToken || undefined,
    page_size: pageSize !== undefined ? String(pageSize) : undefined,
  });
}

export function deleteBQQueryJob(
  jobId: string,
  location: string,
  projectId?: string,
): Promise<void> {
  return apiDelete(`/api/bigquery/query/jobs/${encodeURIComponent(jobId)}`, {
    location: location || undefined,
    project_id: projectId,
  });
}

export function getBQQueryHistory(projectId?: string, max?: number): Promise<BQHistoryItemRaw[]> {
  return apiGetList<BQHistoryItemRaw>('/api/bigquery/query/history', {
    project_id: projectId,
    max: max !== undefined ? String(max) : undefined,
  });
}

// ============================================================
// Athena クエリエディタ
// ============================================================
function athenaPath(profile: string, suffix: string): string {
  return `/api/aws/profiles/${encodeURIComponent(profile)}/athena/${suffix}`;
}

export function getAthenaCatalogs(profile: string, region: string): Promise<AthenaCatalogRaw[]> {
  return apiGetList<AthenaCatalogRaw>(athenaPath(profile, 'catalogs'), { region });
}

export function getAthenaDatabases(
  profile: string,
  region: string,
  catalog?: string,
): Promise<AthenaDatabaseRaw[]> {
  return apiGetList<AthenaDatabaseRaw>(athenaPath(profile, 'databases'), {
    region,
    catalog: catalog || undefined,
  });
}

export function getAthenaWorkgroups(
  profile: string,
  region: string,
): Promise<AthenaWorkgroupRaw[]> {
  return apiGetList<AthenaWorkgroupRaw>(athenaPath(profile, 'workgroups'), { region });
}

export function getAthenaTables(
  profile: string,
  region: string,
  database: string,
  catalog?: string,
): Promise<AthenaTableRaw[]> {
  return apiGetList<AthenaTableRaw>(athenaPath(profile, 'tables'), {
    region,
    database,
    catalog: catalog || undefined,
  });
}

export interface AthenaQueryStartBody {
  sql: string;
  catalog?: string;
  database?: string;
  workgroup?: string;
  output_location?: string;
}

export function postAthenaQueryStart(
  profile: string,
  region: string,
  body: AthenaQueryStartBody,
): Promise<AthenaExecutionRaw> {
  return apiPost<AthenaExecutionRaw>(athenaPath(profile, 'query'), body, { region });
}

export function getAthenaQueryExecution(
  profile: string,
  region: string,
  id: string,
): Promise<AthenaExecutionRaw> {
  return apiGet<AthenaExecutionRaw>(athenaPath(profile, `query/${encodeURIComponent(id)}`), {
    region,
  });
}

export function getAthenaQueryResults(
  profile: string,
  region: string,
  id: string,
  nextToken?: string,
  max?: number,
): Promise<AthenaResultPageRaw> {
  return apiGet<AthenaResultPageRaw>(
    athenaPath(profile, `query/${encodeURIComponent(id)}/results`),
    {
      region,
      next_token: nextToken || undefined,
      max: max !== undefined ? String(max) : undefined,
    },
  );
}

export function deleteAthenaQuery(profile: string, region: string, id: string): Promise<void> {
  return apiDelete(athenaPath(profile, `query/${encodeURIComponent(id)}`), { region });
}

export function getAthenaQueryHistory(
  profile: string,
  region: string,
  workgroup?: string,
  max?: number,
): Promise<AthenaExecutionRaw[]> {
  return apiGetList<AthenaExecutionRaw>(athenaPath(profile, 'query/history'), {
    region,
    workgroup: workgroup || undefined,
    max: max !== undefined ? String(max) : undefined,
  });
}

// ============================================================
// Datadog
// ============================================================
export function getDatadogHistorical(
  startMonth?: string,
  endMonth?: string,
  view?: string,
): Promise<DatadogCostRaw[]> {
  return apiGetList<DatadogCostRaw>('/api/datadog/cost/historical', {
    start_month: startMonth,
    end_month: endMonth,
    view,
  });
}

export function getDatadogEstimated(
  startMonth?: string,
  endMonth?: string,
  view?: string,
): Promise<DatadogCostRaw[]> {
  return apiGetList<DatadogCostRaw>('/api/datadog/cost/estimated', {
    start_month: startMonth,
    end_month: endMonth,
    view,
  });
}

// ============================================================
// TiDB
// ============================================================
export function getTiDBProjects(): Promise<TiDBProjectRaw[]> {
  return apiGetList<TiDBProjectRaw>('/api/tidb/projects');
}

export function getTiDBClusters(projectId: string): Promise<TiDBClusterRaw[]> {
  return apiGetList<TiDBClusterRaw>(`/api/tidb/projects/${encodeURIComponent(projectId)}/clusters`);
}

// TiDB Cloud の billing API は月単位でしか取得できないため、start/end (YYYY-MM) で
// 期間を指定するとバックエンドが月ごとに集約して返す。
export interface TiDBCostQueryOptions {
  start?: string;
  end?: string;
}

export function getTiDBCost(opts?: TiDBCostQueryOptions): Promise<TiDBCostRaw[]> {
  return apiGetList<TiDBCostRaw>('/api/tidb/cost', {
    start: opts?.start,
    end: opts?.end,
  });
}

// ============================================================
// GCP
// ============================================================
// GCP プロジェクト一覧。バックエンドはローカルディスクキャッシュ (~/.config/thief/gcp-projects.json)
// を返す。refresh=true を渡すと Cloud Resource Manager から再取得しキャッシュを更新する (手動更新)。
export function getGcpProjects(opts?: { refresh?: boolean }): Promise<GcpProjectRaw[]> {
  return apiGetList<GcpProjectRaw>('/api/gcp/projects', {
    refresh: opts?.refresh ? true : undefined,
  });
}

// service (cloudrun / gcs) 単位で GCP リソース一覧を取得する。
// パスセグメントは呼び出し側で GCP_SERVICE_TO_PATH から解決する形と揃えるため、
// この関数内で GCP_SERVICE_TO_PATH を参照する (AWS 側 getResources と対称)。
export function getGcpResources<TRaw>(service: string, projectId: string): Promise<TRaw[]> {
  const seg = GCP_SERVICE_TO_PATH[service];
  if (!seg) {
    return Promise.reject(new Error(`unknown gcp service key: ${service}`));
  }
  return apiGetList<TRaw>(`/api/gcp/${seg}`, { project_id: projectId });
}

// 型を明示したい呼び出し側向けのエイリアス (使わなくても可)
export function getCloudRunResources(projectId: string): Promise<CloudRunResourceRaw[]> {
  return getGcpResources<CloudRunResourceRaw>('cloudrun', projectId);
}

export function getGcsBuckets(projectId: string): Promise<GcsBucketRaw[]> {
  return getGcpResources<GcsBucketRaw>('gcs', projectId);
}

// GCS バケット内のオブジェクト一覧 (Drawer の Objects タブ相当)
// バックエンドは {objects, truncated} エンベロープを返す (1000 件で打ち切られた場合の通知用)。
export async function getGcsObjects(
  projectId: string,
  bucket: string,
  prefix?: string,
): Promise<ObjectListEnvelopeRaw<GcsObjectRaw>> {
  return apiGet<ObjectListEnvelopeRaw<GcsObjectRaw>>(
    `/api/gcp/gcs/${encodeURIComponent(bucket)}/objects`,
    {
      project_id: projectId,
      prefix,
    },
  );
}

export function uploadGcsObject(
  projectId: string,
  bucket: string,
  key: string,
  file: File,
): Promise<{ status: string; key: string }> {
  const formData = new FormData();
  formData.append('file', file);
  return apiPostForm<{ status: string; key: string }>(
    `/api/gcp/gcs/${encodeURIComponent(bucket)}/objects/upload`,
    formData,
    { project_id: projectId, key },
  );
}

export function gcsDownloadUrl(projectId: string, bucket: string, key: string): string {
  const url = new URL(`/api/gcp/gcs/${encodeURIComponent(bucket)}/objects/download`, apiBaseUrl());
  url.searchParams.set('project_id', projectId);
  url.searchParams.set('key', key);
  return url.toString();
}

export function getGcsObjectPreview(
  projectId: string,
  bucket: string,
  key: string,
): Promise<ObjectPreviewRaw> {
  return apiGet<ObjectPreviewRaw>(`/api/gcp/gcs/${encodeURIComponent(bucket)}/objects/preview`, {
    project_id: projectId,
    key,
  });
}

// ============================================================
// Cloud Logging (期間指定 + フィルターでのログエントリ取得。Live Tail は api/terminal.ts の
// gcpLoggingTailUrl 経由の WebSocket で別途行う)
// ============================================================
export interface GcpLogEntriesQuery {
  filter?: string;
  start?: string;
  end?: string;
  pageToken?: string;
  pageSize?: number;
}

// クエリ実行のたびに結果が変わりうる読み取りのため、キャッシュ (serveCached) を経由しない
// 専用エンドポイントを叩く (BigQuery クエリ実行と同じ方針)。
export function getGcpLogEntries(
  projectId: string,
  opts: GcpLogEntriesQuery = {},
): Promise<LogEntryPageRaw> {
  return apiGet<LogEntryPageRaw>('/api/gcp/logging/entries', {
    project_id: projectId,
    filter: opts.filter || undefined,
    start: opts.start || undefined,
    end: opts.end || undefined,
    page_token: opts.pageToken || undefined,
    page_size: opts.pageSize !== undefined ? String(opts.pageSize) : undefined,
  });
}

// ============================================================
// クエリスニペット (backend のサービス別ディレクトリへのファイル保存)
// ============================================================
export function getSnippets(service: string): Promise<SnippetRaw[]> {
  return apiGetList<SnippetRaw>(`/api/snippets/${encodeURIComponent(service)}`);
}

export function postSnippet(service: string, name: string, sql: string): Promise<SnippetRaw> {
  return apiPost<SnippetRaw>(`/api/snippets/${encodeURIComponent(service)}`, { name, sql });
}

export function deleteSnippet(service: string, name: string): Promise<void> {
  return apiDelete(`/api/snippets/${encodeURIComponent(service)}/${encodeURIComponent(name)}`);
}

// ============================================================
// CloudWatch Logs (ログビューア)
// ロググループ一覧 (キャッシュあり) と、選択ロググループ横断のイベント検索 (キャッシュなし)。
// Live Tail は api/terminal.ts の cwLogsTailUrl 経由の WebSocket で別途行う。
// ============================================================
export function getCWLogGroups(profile: string, region: string): Promise<CWLogGroupRaw[]> {
  return apiGetList<CWLogGroupRaw>(`/api/aws/profiles/${encodeURIComponent(profile)}/logs/groups`, {
    region,
  });
}

export interface CWLogEventsQuery {
  groups: string[];
  filter?: string;
  start?: string;
  end?: string;
  pageToken?: string;
  limit?: number;
}

export function getCWLogEvents(
  profile: string,
  region: string,
  opts: CWLogEventsQuery,
): Promise<CWLogEventPageRaw> {
  return apiGet<CWLogEventPageRaw>(`/api/aws/profiles/${encodeURIComponent(profile)}/logs/events`, {
    region,
    group: opts.groups,
    filter: opts.filter || undefined,
    start: opts.start || undefined,
    end: opts.end || undefined,
    page_token: opts.pageToken || undefined,
    limit: opts.limit !== undefined ? String(opts.limit) : undefined,
  });
}

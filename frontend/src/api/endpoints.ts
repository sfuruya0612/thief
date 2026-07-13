import type {
  CostRaw,
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
import type { CallerIdentityRaw, ProfileRaw } from '../types/common';
import type {
  BQDatasetRaw,
  BQFieldRaw,
  BQQueryResult,
  BQTableRaw,
  DatadogCostRaw,
  TiDBClusterRaw,
  TiDBCostRaw,
  TiDBProjectRaw,
} from '../types/nonaws';
import type { CloudRunResourceRaw, GcpProjectRaw, GcsBucketRaw, GcsObjectRaw } from '../types/gcp';
import { GCP_SERVICE_TO_PATH, SERVICE_TO_PATH } from '../lib/serviceMeta';
import { apiBaseUrl, apiGet, apiPost, apiPostForm } from './client';

export function getProfiles(): Promise<ProfileRaw[]> {
  // バックエンドは users がない場合 null を返しうるので配列に正規化する
  return apiGet<ProfileRaw[] | null>('/api/aws/profiles').then((v) => v ?? []);
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
  return apiGet<TRaw[] | null>(`/api/aws/profiles/${encodeURIComponent(profile)}/${seg}`, {
    region,
    refresh: opts?.refresh ? true : undefined,
  }).then((v) => v ?? []);
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
  return apiGet<CostRaw[] | null>(`/api/aws/profiles/${encodeURIComponent(profile)}/cost`, {
    region,
    include_today: opts?.includeToday ? true : undefined,
    granularity: opts?.granularity,
    group_by: opts?.groupBy,
    start: opts?.startDate,
    end: opts?.endDate,
    months: opts?.months !== undefined ? String(opts.months) : undefined,
  }).then((v) => v ?? []);
}

export function getCostForecast(profile: string, region: string): Promise<ForecastRaw[]> {
  return apiGet<ForecastRaw[] | null>(
    `/api/aws/profiles/${encodeURIComponent(profile)}/cost/forecast`,
    { region },
  ).then((v) => v ?? []);
}

// SSO ログインを開始する (バックエンドが `aws sso login` を起動する)
export function postSSOLogin(profile: string): Promise<void> {
  return apiPost<void>(`/api/aws/profiles/${encodeURIComponent(profile)}/sso/login`);
}

// ============================================================
// Region (DescribeRegions からの動的取得)
// ============================================================
export function getRegions(profile: string): Promise<RegionRaw[]> {
  return apiGet<RegionRaw[] | null>(
    `/api/aws/profiles/${encodeURIComponent(profile)}/regions`,
  ).then((v) => v ?? []);
}

// ============================================================
// S3 Objects (Drawer の Objects タブ)
// ============================================================
export function getS3Objects(
  profile: string,
  region: string,
  bucket: string,
  prefix?: string,
): Promise<S3ObjectRaw[]> {
  return apiGet<S3ObjectRaw[] | null>(
    `/api/aws/profiles/${encodeURIComponent(profile)}/s3/${encodeURIComponent(bucket)}/objects`,
    { region, prefix },
  ).then((v) => v ?? []);
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

// ============================================================
// ECS Services / Tasks / Containers (Terminal タブの Exec 対象選択に使う)
// ============================================================
export function getECSServices(
  profile: string,
  region: string,
  cluster: string,
): Promise<ECSServiceRaw[]> {
  return apiGet<ECSServiceRaw[] | null>(
    `/api/aws/profiles/${encodeURIComponent(profile)}/ecs/${encodeURIComponent(cluster)}/services`,
    { region },
  ).then((v) => v ?? []);
}

export function getECSTasks(
  profile: string,
  region: string,
  cluster: string,
  service?: string,
): Promise<ECSTaskRaw[]> {
  return apiGet<ECSTaskRaw[] | null>(
    `/api/aws/profiles/${encodeURIComponent(profile)}/ecs/${encodeURIComponent(cluster)}/tasks`,
    { region, service },
  ).then((v) => v ?? []);
}

export function getECSContainers(
  profile: string,
  region: string,
  cluster: string,
  task: string,
): Promise<ECSContainerRaw[]> {
  return apiGet<ECSContainerRaw[] | null>(
    `/api/aws/profiles/${encodeURIComponent(profile)}/ecs/${encodeURIComponent(cluster)}/tasks/${encodeURIComponent(task)}/containers`,
    { region },
  ).then((v) => v ?? []);
}

// ============================================================
// ECR (Drawer の Images タブでリポジトリごとにタグ一覧を取得する)
// ============================================================
export function getECRImages(
  profile: string,
  region: string,
  repo: string,
): Promise<ECRImageRaw[]> {
  return apiGet<ECRImageRaw[] | null>(
    `/api/aws/profiles/${encodeURIComponent(profile)}/ecr/${encodeURIComponent(repo)}/images`,
    { region },
  ).then((v) => v ?? []);
}

// ============================================================
// ELB Listener / Rule / TargetGroup / TargetHealth (Drawer の Listeners / Targets タブ)
// ============================================================
export function getELBListeners(
  profile: string,
  region: string,
  lbArn: string,
): Promise<ELBListenerRaw[]> {
  return apiGet<ELBListenerRaw[] | null>(
    `/api/aws/profiles/${encodeURIComponent(profile)}/elb/listeners`,
    { region, lb_arn: lbArn },
  ).then((v) => v ?? []);
}

export function getELBRules(
  profile: string,
  region: string,
  listenerArn: string,
): Promise<ELBRuleRaw[]> {
  return apiGet<ELBRuleRaw[] | null>(`/api/aws/profiles/${encodeURIComponent(profile)}/elb/rules`, {
    region,
    listener_arn: listenerArn,
  }).then((v) => v ?? []);
}

export function getELBTargetGroups(
  profile: string,
  region: string,
  lbArn: string,
): Promise<ELBTargetGroupRaw[]> {
  return apiGet<ELBTargetGroupRaw[] | null>(
    `/api/aws/profiles/${encodeURIComponent(profile)}/elb/target-groups`,
    { region, lb_arn: lbArn },
  ).then((v) => v ?? []);
}

export function getELBTargetHealth(
  profile: string,
  region: string,
  tgArn: string,
): Promise<ELBTargetHealthRaw[]> {
  return apiGet<ELBTargetHealthRaw[] | null>(
    `/api/aws/profiles/${encodeURIComponent(profile)}/elb/target-health`,
    { region, tg_arn: tgArn },
  ).then((v) => v ?? []);
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
}

export function getDynamoItems(
  profile: string,
  region: string,
  table: string,
  opts: DynamoItemQueryOptions = {},
): Promise<DynamoItemRaw[]> {
  return apiGet<DynamoItemRaw[] | null>(
    `/api/aws/profiles/${encodeURIComponent(profile)}/dynamo/${encodeURIComponent(table)}/items`,
    {
      region,
      pk_val: opts.pkValue,
      sk_val: opts.skValue,
      attr_name: opts.attrName,
      attr_val: opts.attrValue,
    },
  ).then((v) => v ?? []);
}

// ============================================================
// BigQuery
// ============================================================
export function getBQDatasets(projectId?: string): Promise<BQDatasetRaw[]> {
  return apiGet<BQDatasetRaw[] | null>('/api/bigquery/datasets', { project_id: projectId }).then(
    (v) => v ?? [],
  );
}

export function getBQTables(dataset: string, projectId?: string): Promise<BQTableRaw[]> {
  return apiGet<BQTableRaw[] | null>(
    `/api/bigquery/datasets/${encodeURIComponent(dataset)}/tables`,
    { project_id: projectId },
  ).then((v) => v ?? []);
}

export function getBQSchema(
  dataset: string,
  table: string,
  projectId?: string,
): Promise<BQFieldRaw[]> {
  return apiGet<BQFieldRaw[] | null>(
    `/api/bigquery/datasets/${encodeURIComponent(dataset)}/tables/${encodeURIComponent(table)}/schema`,
    { project_id: projectId },
  ).then((v) => v ?? []);
}

export function postBQQuery(sql: string, projectId?: string): Promise<BQQueryResult> {
  return apiPost<BQQueryResult>('/api/bigquery/query', { project_id: projectId, sql });
}

// ============================================================
// Datadog
// ============================================================
export function getDatadogHistorical(
  startMonth?: string,
  endMonth?: string,
  view?: string,
): Promise<DatadogCostRaw[]> {
  return apiGet<DatadogCostRaw[] | null>('/api/datadog/cost/historical', {
    start_month: startMonth,
    end_month: endMonth,
    view,
  }).then((v) => v ?? []);
}

export function getDatadogEstimated(
  startMonth?: string,
  endMonth?: string,
): Promise<DatadogCostRaw[]> {
  return apiGet<DatadogCostRaw[] | null>('/api/datadog/cost/estimated', {
    start_month: startMonth,
    end_month: endMonth,
  }).then((v) => v ?? []);
}

// ============================================================
// TiDB
// ============================================================
export function getTiDBProjects(): Promise<TiDBProjectRaw[]> {
  return apiGet<TiDBProjectRaw[] | null>('/api/tidb/projects').then((v) => v ?? []);
}

export function getTiDBClusters(projectId: string): Promise<TiDBClusterRaw[]> {
  return apiGet<TiDBClusterRaw[] | null>(
    `/api/tidb/projects/${encodeURIComponent(projectId)}/clusters`,
  ).then((v) => v ?? []);
}

export function getTiDBCost(month?: string): Promise<TiDBCostRaw[]> {
  return apiGet<TiDBCostRaw[] | null>('/api/tidb/cost', { month }).then((v) => v ?? []);
}

// ============================================================
// GCP
// ============================================================
// GCP プロジェクト一覧 (Cloud Resource Manager)
export function getGcpProjects(): Promise<GcpProjectRaw[]> {
  return apiGet<GcpProjectRaw[] | null>('/api/gcp/projects').then((v) => v ?? []);
}

// service (cloudrun / gcs) 単位で GCP リソース一覧を取得する。
// パスセグメントは呼び出し側で GCP_SERVICE_TO_PATH から解決する形と揃えるため、
// この関数内で GCP_SERVICE_TO_PATH を参照する (AWS 側 getResources と対称)。
export function getGcpResources<TRaw>(service: string, projectId: string): Promise<TRaw[]> {
  const seg = GCP_SERVICE_TO_PATH[service];
  if (!seg) {
    return Promise.reject(new Error(`unknown gcp service key: ${service}`));
  }
  return apiGet<TRaw[] | null>(`/api/gcp/${seg}`, { project_id: projectId }).then((v) => v ?? []);
}

// 型を明示したい呼び出し側向けのエイリアス (使わなくても可)
export function getCloudRunResources(projectId: string): Promise<CloudRunResourceRaw[]> {
  return getGcpResources<CloudRunResourceRaw>('cloudrun', projectId);
}

export function getGcsBuckets(projectId: string): Promise<GcsBucketRaw[]> {
  return getGcpResources<GcsBucketRaw>('gcs', projectId);
}

// GCS バケット内のオブジェクト一覧 (Drawer の Objects タブ相当)
export function getGcsObjects(
  projectId: string,
  bucket: string,
  prefix?: string,
): Promise<GcsObjectRaw[]> {
  return apiGet<GcsObjectRaw[] | null>(`/api/gcp/gcs/${encodeURIComponent(bucket)}/objects`, {
    project_id: projectId,
    prefix,
  }).then((v) => v ?? []);
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

import type {
  CostRaw,
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
import type { Profile } from '../types/common';
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
import { SERVICE_TO_PATH } from '../lib/serviceMeta';
import { apiBaseUrl, apiGet, apiPost, apiPostForm } from './client';

export function getProfiles(): Promise<Profile[]> {
  // バックエンドは users がない場合 null を返しうるので配列に正規化する
  return apiGet<Profile[] | null>('/api/aws/profiles').then((v) => v ?? []);
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

export function getCost(
  profile: string,
  region: string,
  includeToday?: boolean,
): Promise<CostRaw[]> {
  return apiGet<CostRaw[] | null>(`/api/aws/profiles/${encodeURIComponent(profile)}/cost`, {
    region,
    include_today: includeToday ? true : undefined,
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

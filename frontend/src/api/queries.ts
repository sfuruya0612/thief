import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { CostRow } from '../types/aws';
import {
  bqDatasetFromRaw,
  bqFieldFromRaw,
  bqTableFromRaw,
  datadogCostFromRaw,
  tidbClusterFromRaw,
  tidbCostFromRaw,
  tidbProjectFromRaw,
} from '../lib/normalizeNonAws';
import { ecrImageFromRaw, ecsContainerFromRaw, ecsTaskFromRaw } from '../lib/normalize';
import {
  getBQDatasets,
  getBQSchema,
  getBQTables,
  getCost,
  getCostForecast,
  getDatadogEstimated,
  getDatadogHistorical,
  getECRImages,
  getECSContainers,
  getECSTasks,
  getProfiles,
  getRegions,
  getResources,
  getTiDBClusters,
  getTiDBCost,
  getTiDBProjects,
  postBQQuery,
  postSSOLogin,
} from './endpoints';

export function useProfiles() {
  return useQuery({
    queryKey: ['aws', 'profiles'],
    queryFn: getProfiles,
    staleTime: 5 * 60 * 1000,
  });
}

// TRaw を fetch し normalizer で TRow に変換する汎用フック
export function useResources<TRaw, TRow>(
  service: string,
  profile: string,
  region: string,
  normalizer: (raw: TRaw, region: string) => TRow,
) {
  return useQuery({
    queryKey: ['aws', service, profile, region],
    queryFn: async () => {
      const raws = await getResources<TRaw>(service, profile, region);
      return raws.map((r) => normalizer(r, region));
    },
    staleTime: 60_000,
    enabled: !!profile && !!service,
  });
}

export function useCost(profile: string, region: string) {
  return useQuery({
    queryKey: ['aws', 'cost', profile, region],
    queryFn: async (): Promise<CostRow[]> => {
      const raws = await getCost(profile, region);
      return raws.map((r) => ({
        timePeriod: r.time_period,
        service: r.service,
        unblendedAmount: r.unblended_amount,
        netAmortizedAmount: r.net_amortized_amount,
        unit: r.unit,
      }));
    },
    staleTime: 60_000,
    enabled: !!profile,
  });
}

export function useCostForecast(profile: string, region: string) {
  return useQuery({
    queryKey: ['aws', 'cost-forecast', profile, region],
    queryFn: async () => {
      const raws = await getCostForecast(profile, region);
      return raws.map((r) => ({
        timePeriod: r.time_period,
        amount: r.amount,
        unit: r.unit,
      }));
    },
    staleTime: 60_000,
    enabled: !!profile,
  });
}

// ============================================================
// Region (DescribeRegions からの動的取得)
// リージョン一覧は一度取得したら保持する (staleTime: Infinity)
// ============================================================
export function useRegions(profile: string) {
  return useQuery({
    queryKey: ['aws', 'regions', profile],
    queryFn: async () => {
      const raws = await getRegions(profile);
      return raws.map((r) => ({ code: r.code, name: r.name }));
    },
    staleTime: Infinity,
    enabled: !!profile,
  });
}

// ============================================================
// ECS Services / Tasks / Containers (Terminal タブの Exec 対象選択に使う)
// ============================================================
export function useECSTasks(profile: string, region: string, cluster: string, service?: string) {
  return useQuery({
    queryKey: ['aws', 'ecs-tasks', profile, region, cluster, service],
    queryFn: async () => (await getECSTasks(profile, region, cluster, service)).map(ecsTaskFromRaw),
    staleTime: 60_000,
    enabled: !!profile && !!cluster,
  });
}

export function useECSContainers(profile: string, region: string, cluster: string, task: string) {
  return useQuery({
    queryKey: ['aws', 'ecs-containers', profile, region, cluster, task],
    queryFn: async () =>
      (await getECSContainers(profile, region, cluster, task)).map(ecsContainerFromRaw),
    staleTime: 60_000,
    enabled: !!profile && !!cluster && !!task,
  });
}

// ============================================================
// ECR (Drawer の Images タブ)
// ============================================================
export function useECRImages(profile: string, region: string, repo: string) {
  return useQuery({
    queryKey: ['aws', 'ecr-images', profile, region, repo],
    queryFn: async () => (await getECRImages(profile, region, repo)).map(ecrImageFromRaw),
    staleTime: 60_000,
    enabled: !!profile && !!repo,
  });
}

// SSO 期限切れ (401 SSO_TOKEN_EXPIRED) から再ログインを起動するミューテーション
export function useSSOLogin(profile: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => postSSOLogin(profile),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['aws'] });
    },
  });
}

// ============================================================
// BigQuery
// ============================================================
export function useBQDatasets(projectId?: string) {
  return useQuery({
    queryKey: ['bigquery', 'datasets', projectId],
    queryFn: async () => (await getBQDatasets(projectId)).map(bqDatasetFromRaw),
    staleTime: 60_000,
  });
}

export function useBQTables(dataset: string, projectId?: string) {
  return useQuery({
    queryKey: ['bigquery', 'tables', dataset, projectId],
    queryFn: async () => (await getBQTables(dataset, projectId)).map(bqTableFromRaw),
    staleTime: 60_000,
    enabled: !!dataset,
  });
}

export function useBQSchema(dataset: string, table: string, projectId?: string) {
  return useQuery({
    queryKey: ['bigquery', 'schema', dataset, table, projectId],
    queryFn: async () => (await getBQSchema(dataset, table, projectId)).map(bqFieldFromRaw),
    staleTime: 60_000,
    enabled: !!dataset && !!table,
  });
}

export function useBQQuery() {
  return useMutation({
    mutationFn: ({ sql, projectId }: { sql: string; projectId?: string }) =>
      postBQQuery(sql, projectId),
  });
}

// ============================================================
// Datadog
// ============================================================
export function useDatadogHistorical(startMonth?: string, endMonth?: string, view?: string) {
  return useQuery({
    queryKey: ['datadog', 'historical', startMonth, endMonth, view],
    queryFn: async () =>
      (await getDatadogHistorical(startMonth, endMonth, view)).map(datadogCostFromRaw),
    staleTime: 60_000,
  });
}

export function useDatadogEstimated(startMonth?: string, endMonth?: string) {
  return useQuery({
    queryKey: ['datadog', 'estimated', startMonth, endMonth],
    queryFn: async () => (await getDatadogEstimated(startMonth, endMonth)).map(datadogCostFromRaw),
    staleTime: 60_000,
  });
}

// ============================================================
// TiDB
// ============================================================
export function useTiDBProjects() {
  return useQuery({
    queryKey: ['tidb', 'projects'],
    queryFn: async () => (await getTiDBProjects()).map(tidbProjectFromRaw),
    staleTime: 60_000,
  });
}

export function useTiDBClusters(projectId: string) {
  return useQuery({
    queryKey: ['tidb', 'clusters', projectId],
    queryFn: async () => (await getTiDBClusters(projectId)).map(tidbClusterFromRaw),
    staleTime: 60_000,
    enabled: !!projectId,
  });
}

export function useTiDBCost(month?: string) {
  return useQuery({
    queryKey: ['tidb', 'cost', month],
    queryFn: async () => (await getTiDBCost(month)).map(tidbCostFromRaw),
    staleTime: 60_000,
  });
}

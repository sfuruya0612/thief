import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { CostRow, SSOLoginStartRow } from '../types/aws';
import { ApiError } from '../types/common';
import type { AppView, BaseRow } from '../types/common';
import type { QueryStatusRow } from '../types/query';
import { gcpProjectFromRaw, gcsObjectFromRaw } from '../lib/normalizeGcp';
import { priceTableFromRaw } from '../lib/normalizePricing';
import {
  athenaExecutionFromRaw,
  athenaHistoryFromRaw,
  athenaTableFromRaw,
  bqHistoryFromRaw,
  bqJobStatusFromRaw,
  snippetFromRaw,
} from '../lib/normalizeQuery';
import type { QueryEditorService } from '../lib/queryEditorStorage';
import {
  bqDatasetFromRaw,
  bqFieldFromRaw,
  bqTableFromRaw,
  datadogCostFromRaw,
  tidbClusterFromRaw,
  tidbCostFromRaw,
  tidbProjectFromRaw,
} from '../lib/normalizeNonAws';
import {
  cacheParameterFromRaw,
  callerIdentityFromRaw,
  cfnStackDetailFromRaw,
  cfnStackEventFromRaw,
  cfnStackResourceFromRaw,
  cwLogGroupFromRaw,
  dynamoTableSchemaFromRaw,
  ecrImageFromRaw,
  ecsContainerFromRaw,
  ecsServiceFromRaw,
  ecsTaskFromRaw,
  elbListenerFromRaw,
  elbRuleFromRaw,
  elbTargetGroupFromRaw,
  elbTargetHealthFromRaw,
  objectPreviewFromRaw,
  profileFromRaw,
  rdsClusterParameterGroupFromRaw,
  rdsParameterFromRaw,
  s3ObjectFromRaw,
  ssoLoginStartFromRaw,
  wafRuleFromRaw,
} from '../lib/normalize';
import {
  type AthenaQueryStartBody,
  type CostQueryOptions,
  deleteAthenaQuery,
  deleteBQQueryJob,
  deleteSnippet,
  type DynamoItemQueryOptions,
  getAthenaCatalogs,
  getAthenaDatabases,
  getAthenaQueryExecution,
  getAthenaQueryHistory,
  getAthenaQueryResults,
  getAthenaTables,
  getAthenaWorkgroups,
  getBQDatasets,
  getBQQueryHistory,
  getBQQueryJob,
  getBQQueryResults,
  getBQSchema,
  getBQTables,
  getCacheParameters,
  getCFNStackDetail,
  getCFNStackEvents,
  getCFNStackResources,
  getCost,
  getCostForecast,
  type CWLogEventsQuery,
  getCWLogEvents,
  getCWLogGroups,
  getDatadogEstimated,
  getDatadogHistorical,
  getDynamoItems,
  getDynamoSchema,
  getECRImages,
  getECSContainers,
  getECSServices,
  getECSTasks,
  getELBListeners,
  getELBRules,
  getELBTargetGroups,
  getELBTargetHealth,
  type GcpLogEntriesQuery,
  getGcpLogEntries,
  getGcpProjects,
  getGcpResources,
  getGcsObjectPreview,
  getGcsObjects,
  getHealth,
  getPricing,
  getProfileIdentity,
  getProfiles,
  getRDSClusterParameters,
  getRDSParameters,
  getRegions,
  getResources,
  getS3ObjectPreview,
  getS3Objects,
  getSecretValue,
  getSnippets,
  getSSMParameterValue,
  getTiDBClusters,
  getTiDBCost,
  getTiDBProjects,
  getWAFRules,
  postAthenaQueryStart,
  postBQDryRun,
  postBQQueryStart,
  postSnippet,
  postCacheInvalidate,
  postSSOLoginComplete,
  postSSOLoginStart,
  type TiDBCostQueryOptions,
  updateSecretValue,
  updateSSMParameter,
  uploadGcsObject,
  uploadS3Object,
} from './endpoints';

// backend 未起動時の一時的な取得失敗から自動復旧するためのポーリング間隔。
// 成功後は refetchInterval が false を返すため通常時は無停止ポーリングにならない。
const PROFILE_LIST_ERROR_RETRY_INTERVAL = 15_000;

export function useProfiles() {
  return useQuery({
    queryKey: ['aws', 'profiles'],
    queryFn: async () => (await getProfiles()).map(profileFromRaw),
    staleTime: 5 * 60 * 1000,
    refetchInterval: (query) =>
      query.state.status === 'error' ? PROFILE_LIST_ERROR_RETRY_INTERVAL : false,
  });
}

// backend 起動待ちの検知用ポーリング間隔。
const HEALTH_CHECK_RETRY_INTERVAL = 2_000;

// backend の起動待ちを検知する。成功後は staleTime: Infinity かつ refetchInterval が
// false を返すため再取得されず、起動時の初回疎通のみを対象にする
// (セッション途中の一時的な network_error では待機表示に戻らない)。
export function useHealthCheck() {
  return useQuery({
    queryKey: ['health'],
    queryFn: getHealth,
    retry: false,
    staleTime: Infinity,
    refetchInterval: (query) =>
      query.state.status === 'error' ? HEALTH_CHECK_RETRY_INTERVAL : false,
  });
}

// backend のリソースキャッシュを view 単位で一括破棄する (TopBar の Refresh 用)。
// invalidateQueries との実行順序の制御は lib/refreshView.ts 側で行う。
export function useCacheInvalidate() {
  return useMutation({
    mutationFn: (view: AppView) => postCacheInvalidate(view),
  });
}

// 選択中プロファイルの Account ID を STS で確定する。プロファイルを切り替えるたび
// に 1 件だけ発火する (一覧取得時に全プロファイル分呼ぶことはしない)。
export function useProfileIdentity(profile: string) {
  return useQuery({
    queryKey: ['aws', 'profile-identity', profile],
    queryFn: async () => callerIdentityFromRaw(await getProfileIdentity(profile)),
    staleTime: 5 * 60 * 1000,
    enabled: !!profile,
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
    enabled: !!profile && !!service,
  });
}

export function useCost(profile: string, region: string, opts?: CostQueryOptions) {
  return useQuery({
    queryKey: [
      'aws',
      'cost',
      profile,
      region,
      opts?.granularity,
      opts?.groupBy,
      opts?.service,
      opts?.account,
      opts?.startDate,
      opts?.endDate,
      opts?.months,
    ],
    queryFn: async (): Promise<CostRow[]> => {
      const raws = await getCost(profile, region, opts);
      return raws.map((r) => ({
        id: `${r.time_period}/${r.service}`,
        timePeriod: r.time_period,
        service: r.service,
        unblendedAmount: r.unblended_amount,
        netAmortizedAmount: r.net_amortized_amount,
        unit: r.unit,
      }));
    },
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
    enabled: !!profile,
  });
}

// ============================================================
// Pricing (AWS Price List / Savings Plans の正規化レート表)
// 単価はアカウント非依存のため queryKey に profile を含めない (プロファイル間で共有する)。
// バックエンドのファイルキャッシュ (TTL なし) を正とし、staleTime: Infinity で自動再取得は
// しない。更新は useRefreshPricing (mutation) からのみ行う。
//
// ec2-spot (issue 0056) はバックエンドのディスクキャッシュを経由しないライブ取得の
// ため、staleTime を有限値にして呼び出す (PricingPanel 参照)。ただし
// refetchOnWindowFocus: false / refetchInterval なしの全体設定のもとでは、有限
// staleTime でも自動再取得はマウント時 (refetchOnMount) と手動更新に限られ、パネルを
// 開いたままにしても継続的には最新化されない (鮮度の実現範囲は UI 側で明示する)。
// ============================================================
export function usePricing(
  profile: string,
  region: string,
  service: string,
  enabled = true,
  staleTime: number = Infinity,
) {
  return useQuery({
    queryKey: ['aws', 'pricing', service, region],
    queryFn: async () => priceTableFromRaw(await getPricing(profile, region, service)),
    staleTime,
    enabled: enabled && !!profile && !!region,
  });
}

// 明示的な更新 (ツールバーの更新ボタン)。invalidateQueries はバックエンドのファイル
// キャッシュにヒットして強制再取得にならないため、refresh=true を渡した結果を
// setQueryData で直接差し替える (useRefreshGcpProjects と同じ方針)。
export function useRefreshPricing(profile: string, region: string, service: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () => priceTableFromRaw(await getPricing(profile, region, service, true)),
    onSuccess: (table) => {
      queryClient.setQueryData(['aws', 'pricing', service, region], table);
    },
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
// Secrets Manager / SSM Parameter Store (Drawer の Value タブ)
// ============================================================
// 値は一覧に含めないため、Value タブを開いたときにオンデマンドで取得する。機密値をキャッシュに
// 常時載せない方針のため staleTime: 0 を明示し、開くたびに取得する (S3 オブジェクトプレビューと
// 同じ考え方。グローバル既定の 60 秒に引きずられないようにする)。
export function useSecretValue(profile: string, region: string, name: string) {
  return useQuery({
    queryKey: ['aws', 'secret-value', profile, region, name],
    queryFn: async () => (await getSecretValue(profile, region, name)).value,
    staleTime: 0,
    enabled: !!profile && !!name,
  });
}

export function useSSMValue(profile: string, region: string, name: string) {
  return useQuery({
    queryKey: ['aws', 'ssm-value', profile, region, name],
    queryFn: async () => (await getSSMParameterValue(profile, region, name)).value,
    staleTime: 0,
    enabled: !!profile && !!name,
  });
}

// 更新成功後は一覧クエリ (メタデータの LastChanged / Version) と値クエリの両方を invalidate して、
// Overview と Value タブを最新化する。
export function useSecretUpdate(profile: string, region: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ name, value }: { name: string; value: string }) =>
      updateSecretValue(profile, region, name, value),
    onSuccess: (_data, { name }) => {
      void queryClient.invalidateQueries({ queryKey: ['aws', 'secrets', profile, region] });
      void queryClient.invalidateQueries({
        queryKey: ['aws', 'secret-value', profile, region, name],
      });
    },
  });
}

export function useSSMUpdate(profile: string, region: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ name, value }: { name: string; value: string }) =>
      updateSSMParameter(profile, region, name, value),
    onSuccess: (_data, { name }) => {
      void queryClient.invalidateQueries({ queryKey: ['aws', 'ssm', profile, region] });
      void queryClient.invalidateQueries({
        queryKey: ['aws', 'ssm-value', profile, region, name],
      });
    },
  });
}

// ============================================================
// S3 Objects (Drawer の Objects タブ)
// ============================================================
// バックエンドは最大 1000 件で打ち切る ({objects, truncated} エンベロープ)。
// prefix ごとに独立した queryKey で、検索確定 (prefix 変更) のたびに再取得される。
export function useS3Objects(profile: string, region: string, bucket: string, prefix?: string) {
  return useQuery({
    queryKey: ['aws', 's3-objects', profile, region, bucket, prefix],
    queryFn: async () => {
      const { objects, truncated } = await getS3Objects(profile, region, bucket, prefix);
      return { objects: (objects ?? []).map(s3ObjectFromRaw), truncated };
    },
    enabled: !!profile && !!bucket,
  });
}

export function useS3Upload(profile: string, region: string, bucket: string, prefix?: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ key, file }: { key: string; file: File }) =>
      uploadS3Object(profile, region, bucket, `${prefix ?? ''}${key}`, file),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ['aws', 's3-objects', profile, region, bucket],
      });
      void queryClient.invalidateQueries({
        queryKey: ['aws', 's3-object-preview', profile, region, bucket],
      });
    },
  });
}

// enabled: !!key でプレビュー対象確定時 (行の Preview アクションをクリックした後) のみ取得する。
// プレビューは開くたびに最新の中身を読みたい取得系のため staleTime: 0 を明示する。
export function useS3ObjectPreview(
  profile: string,
  region: string,
  bucket: string,
  key: string | undefined,
) {
  return useQuery({
    queryKey: ['aws', 's3-object-preview', profile, region, bucket, key],
    queryFn: async () =>
      objectPreviewFromRaw(await getS3ObjectPreview(profile, region, bucket, key!)),
    staleTime: 0,
    enabled: !!profile && !!bucket && !!key,
  });
}

// ============================================================
// ECS Services / Tasks / Containers (Terminal タブの Exec 対象選択に使う)
// ============================================================
export function useECSServices(profile: string, region: string, cluster: string) {
  return useQuery({
    queryKey: ['aws', 'ecs-services', profile, region, cluster],
    queryFn: async () => (await getECSServices(profile, region, cluster)).map(ecsServiceFromRaw),
    enabled: !!profile && !!cluster,
  });
}

export function useECSTasks(profile: string, region: string, cluster: string, service?: string) {
  return useQuery({
    queryKey: ['aws', 'ecs-tasks', profile, region, cluster, service],
    queryFn: async () => (await getECSTasks(profile, region, cluster, service)).map(ecsTaskFromRaw),
    enabled: !!profile && !!cluster,
  });
}

export function useECSContainers(profile: string, region: string, cluster: string, task: string) {
  return useQuery({
    queryKey: ['aws', 'ecs-containers', profile, region, cluster, task],
    queryFn: async () =>
      (await getECSContainers(profile, region, cluster, task)).map(ecsContainerFromRaw),
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
    enabled: !!profile && !!repo,
  });
}

// ============================================================
// CloudFormation (Drawer の Overview / Events / Resources タブ)
// ============================================================
export function useCFNStackDetail(profile: string, region: string, stack: string) {
  return useQuery({
    queryKey: ['aws', 'cfn-detail', profile, region, stack],
    queryFn: async () => cfnStackDetailFromRaw(await getCFNStackDetail(profile, region, stack)),
    enabled: !!profile && !!stack,
  });
}

export function useCFNStackEvents(profile: string, region: string, stack: string) {
  return useQuery({
    queryKey: ['aws', 'cfn-events', profile, region, stack],
    queryFn: async () =>
      (await getCFNStackEvents(profile, region, stack)).map(cfnStackEventFromRaw),
    // デプロイ進行中の確認が主用途で backend も 30 秒 TTL のため短めにする
    staleTime: 30_000,
    enabled: !!profile && !!stack,
  });
}

export function useCFNStackResources(profile: string, region: string, stack: string) {
  return useQuery({
    queryKey: ['aws', 'cfn-resources', profile, region, stack],
    queryFn: async () =>
      (await getCFNStackResources(profile, region, stack)).map(cfnStackResourceFromRaw),
    enabled: !!profile && !!stack,
  });
}

// ============================================================
// ELB Listener / Rule / TargetGroup / TargetHealth (Drawer の Listeners / Targets タブ)
// ============================================================
export function useELBListeners(profile: string, region: string, lbArn: string) {
  return useQuery({
    queryKey: ['aws', 'elb-listeners', profile, region, lbArn],
    queryFn: async () => (await getELBListeners(profile, region, lbArn)).map(elbListenerFromRaw),
    enabled: !!profile && !!lbArn,
  });
}

export function useELBRules(profile: string, region: string, listenerArn: string) {
  return useQuery({
    queryKey: ['aws', 'elb-rules', profile, region, listenerArn],
    queryFn: async () => (await getELBRules(profile, region, listenerArn)).map(elbRuleFromRaw),
    enabled: !!profile && !!listenerArn,
  });
}

export function useELBTargetGroups(profile: string, region: string, lbArn: string) {
  return useQuery({
    queryKey: ['aws', 'elb-target-groups', profile, region, lbArn],
    queryFn: async () =>
      (await getELBTargetGroups(profile, region, lbArn)).map(elbTargetGroupFromRaw),
    enabled: !!profile && !!lbArn,
  });
}

export function useELBTargetHealth(profile: string, region: string, tgArn: string) {
  return useQuery({
    queryKey: ['aws', 'elb-target-health', profile, region, tgArn],
    queryFn: async () =>
      (await getELBTargetHealth(profile, region, tgArn)).map(elbTargetHealthFromRaw),
    enabled: !!profile && !!tgArn,
  });
}

// ============================================================
// RDS / ElastiCache パラメータグループ (Drawer の Parameters タブ)
// enabled: !!group でパラメータグループ確定時のみ取得する。
// ============================================================
export function useRDSParameters(profile: string, region: string, group: string) {
  return useQuery({
    queryKey: ['aws', 'rds-parameters', profile, region, group],
    queryFn: async () => (await getRDSParameters(profile, region, group)).map(rdsParameterFromRaw),
    enabled: !!profile && !!group,
  });
}

export function useRDSClusterParameters(profile: string, region: string, clusterId: string) {
  return useQuery({
    queryKey: ['aws', 'rds-cluster-parameters', profile, region, clusterId],
    queryFn: async () =>
      rdsClusterParameterGroupFromRaw(await getRDSClusterParameters(profile, region, clusterId)),
    enabled: !!profile && !!clusterId,
  });
}

export function useCacheParameters(profile: string, region: string, group: string) {
  return useQuery({
    queryKey: ['aws', 'elasticache-parameters', profile, region, group],
    queryFn: async () =>
      (await getCacheParameters(profile, region, group)).map(cacheParameterFromRaw),
    enabled: !!profile && !!group,
  });
}

// ============================================================
// WAF ルール (Drawer の Rules タブ)
// ============================================================
// scope は一覧キャッシュから引き直して得るため、確定するまで (空文字の間) は無効化する。
export function useWAFRules(
  profile: string,
  region: string,
  scope: string,
  name: string,
  id: string,
) {
  return useQuery({
    queryKey: ['aws', 'waf-rules', profile, region, scope, id],
    queryFn: async () => (await getWAFRules(profile, region, scope, name, id)).map(wafRuleFromRaw),
    enabled: !!profile && !!scope && !!id,
  });
}

// ============================================================
// DynamoDB Item 検索 (Drawer の Items タブ)
// ============================================================
export function useDynamoSchema(profile: string, region: string, table: string) {
  return useQuery({
    queryKey: ['aws', 'dynamo-schema', profile, region, table],
    queryFn: async () => dynamoTableSchemaFromRaw(await getDynamoSchema(profile, region, table)),
    enabled: !!profile && !!table,
  });
}

// pkValue 未指定時はプレビュー (Scan) を、指定時は Query を返す。取得件数は opts.limit
// (未指定時はバックエンド既定値)。attrName/attrValue は PK/SK 以外の任意属性による
// 絞り込み (FilterExpression) で、pkValue/skValue と併用できる。
export function useDynamoItems(
  profile: string,
  region: string,
  table: string,
  opts: DynamoItemQueryOptions = {},
) {
  const { pkValue, skValue, attrName, attrValue, limit } = opts;
  return useQuery({
    queryKey: [
      'aws',
      'dynamo-items',
      profile,
      region,
      table,
      pkValue,
      skValue,
      attrName,
      attrValue,
      limit,
    ],
    queryFn: () => getDynamoItems(profile, region, table, opts),
    enabled: !!profile && !!table,
  });
}

// useSSOLogin のミューテーション入力。authWindow は Login ボタンの click ハンドラが
// 同期的に window.open した空タブ (ポップアップブロック時は null)。onStarted は start
// 応答の認可 URL をバナーのフォールバック表示へ渡すコールバック。
export interface SSOLoginInput {
  authWindow: Window | null;
  onStarted?: (started: SSOLoginStartRow) => void;
  // 開いておいた空タブを自動制御できないと判明したとき (start 応答時点でユーザが
  // 手動で閉じていた場合) に呼ぶ。バナーはフォールバックリンクの表示に切り替える。
  onTabUnavailable?: () => void;
}

// SSO 期限切れ (401 SSO_TOKEN_EXPIRED) からの再ログインで、デバイス認可フロー全体
// (start で認可 URL を取得 → 認可タブを遷移 → complete でトークン取得を待機) を実行する
// ミューテーション。認可タブを frontend 自身が window.open で開くことで、認可完了後に
// タブを close() でき、フォーカスが opener である frontend のタブへ戻る (issue 0149)。
export function useSSOLogin(profile: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ authWindow, onStarted, onTabUnavailable }: SSOLoginInput) => {
      let started: SSOLoginStartRow;
      try {
        started = ssoLoginStartFromRaw(await postSSOLoginStart(profile));
      } catch (err) {
        // 認可ページへ遷移する前の失敗。ユーザが認可の途中ということはないので、
        // 開いておいた空タブは閉じてよい。
        authWindow?.close();
        throw err;
      }
      onStarted?.(started);
      if (authWindow && !authWindow.closed) {
        if (started.verificationUriComplete) {
          authWindow.location.replace(started.verificationUriComplete);
        } else {
          // RFC 8628 §3.2 で verification_uri_complete は OPTIONAL。無い場合は
          // バナーに verification_uri と user_code を表示するため、空タブは閉じる。
          authWindow.close();
        }
      } else if (authWindow) {
        // start の応答待ちの間にユーザが空タブを手動で閉じた場合 (closed)。閉じた
        // タブの location 操作はブラウザによって例外になりうるため触らず、バナーの
        // フォールバックリンクからの認可継続に切り替える (セッションは backend が
        // 保持しており、別タブで認可を完了すれば complete は成功する)。
        onTabUnavailable?.();
      }
      try {
        await postSSOLoginComplete(profile, started.sessionId);
      } catch (err) {
        // 認可拒否 (deny) はユーザが認可ページでの操作を終えているため認可タブを
        // 閉じてよい。その他の失敗はまだ認可の途中かもしれないので閉じない。
        if (err instanceof ApiError && err.code === 'SSO_LOGIN_ACCESS_DENIED') {
          authWindow?.close();
        }
        throw err;
      }
      authWindow?.close();
    },
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
  });
}

export function useBQTables(dataset: string, projectId?: string) {
  return useQuery({
    queryKey: ['bigquery', 'tables', dataset, projectId],
    queryFn: async () => (await getBQTables(dataset, projectId)).map(bqTableFromRaw),
    enabled: !!dataset,
  });
}

export function useBQSchema(dataset: string, table: string, projectId?: string) {
  return useQuery({
    queryKey: ['bigquery', 'schema', dataset, table, projectId],
    queryFn: async () => (await getBQSchema(dataset, table, projectId)).map(bqFieldFromRaw),
    enabled: !!dataset && !!table,
  });
}

// ============================================================
// BigQuery クエリエディタ (非同期ジョブ)
// ============================================================

// 結果取得 1 ページあたりの行数 (バックエンド既定と揃える)
const QUERY_RESULT_PAGE_SIZE = 500;

// 実行中ジョブのポーリング間隔 (ms)
const QUERY_POLL_INTERVAL = 1000;

// 実行履歴の staleTime。実行直後の再取得を優先して短めにする
const QUERY_HISTORY_STALE_TIME = 15_000;

// 実行中 / 待機中はポーリングを続け、終了状態になったら止める
function pollWhileActive(data: QueryStatusRow | undefined): number | false {
  return data && (data.state === 'queued' || data.state === 'running')
    ? QUERY_POLL_INTERVAL
    : false;
}

export function useBQStartQuery(projectId?: string) {
  return useMutation({
    mutationFn: async (sql: string) => {
      const raw = await postBQQueryStart(sql, projectId);
      return { jobId: raw.job_id, location: raw.location };
    },
  });
}

export function useBQDryRun(projectId?: string) {
  return useMutation({
    mutationFn: async (sql: string) => (await postBQDryRun(sql, projectId)).total_bytes_processed,
  });
}

export function useBQQueryJob(jobId?: string, location?: string, projectId?: string) {
  return useQuery({
    queryKey: ['bigquery', 'query-job', projectId, jobId],
    queryFn: async () => bqJobStatusFromRaw(await getBQQueryJob(jobId!, location ?? '', projectId)),
    enabled: !!jobId,
    refetchInterval: (query) => pollWhileActive(query.state.data),
    staleTime: 0,
  });
}

// 完了したジョブの結果をページ単位で取得する (fetchNextPage で追加読み込み)
export function useBQQueryResults(
  jobId: string | undefined,
  location: string | undefined,
  projectId: string | undefined,
  enabled: boolean,
) {
  return useInfiniteQuery({
    queryKey: ['bigquery', 'query-results', projectId, jobId],
    queryFn: ({ pageParam }) =>
      getBQQueryResults(jobId!, location ?? '', projectId, pageParam, QUERY_RESULT_PAGE_SIZE),
    initialPageParam: '',
    getNextPageParam: (last) => last.next_page_token || undefined,
    enabled: enabled && !!jobId,
    // 完了ジョブの結果は不変
    staleTime: Infinity,
  });
}

export function useBQCancelJob(projectId?: string) {
  return useMutation({
    mutationFn: ({ jobId, location }: { jobId: string; location?: string }) =>
      deleteBQQueryJob(jobId, location ?? '', projectId),
  });
}

export function useBQQueryHistory(projectId?: string, enabled = true) {
  return useQuery({
    queryKey: ['bigquery', 'query-history', projectId],
    queryFn: async () => (await getBQQueryHistory(projectId)).map(bqHistoryFromRaw),
    staleTime: QUERY_HISTORY_STALE_TIME,
    enabled,
  });
}

// ============================================================
// Athena クエリエディタ
// ============================================================
export function useAthenaCatalogs(profile: string, region: string) {
  return useQuery({
    queryKey: ['aws', 'athena-catalogs', profile, region],
    queryFn: () => getAthenaCatalogs(profile, region),
    enabled: !!profile,
  });
}

export function useAthenaDatabases(profile: string, region: string, catalog?: string) {
  return useQuery({
    queryKey: ['aws', 'athena-databases', profile, region, catalog],
    queryFn: () => getAthenaDatabases(profile, region, catalog),
    enabled: !!profile,
  });
}

export function useAthenaWorkgroups(profile: string, region: string) {
  return useQuery({
    queryKey: ['aws', 'athena-workgroups', profile, region],
    queryFn: () => getAthenaWorkgroups(profile, region),
    enabled: !!profile,
  });
}

export function useAthenaTables(
  profile: string,
  region: string,
  database?: string,
  catalog?: string,
) {
  return useQuery({
    queryKey: ['aws', 'athena-tables', profile, region, catalog, database],
    queryFn: async () =>
      (await getAthenaTables(profile, region, database!, catalog)).map(athenaTableFromRaw),
    enabled: !!profile && !!database,
  });
}

export function useAthenaStartQuery(profile: string, region: string) {
  return useMutation({
    mutationFn: async (body: AthenaQueryStartBody) =>
      athenaExecutionFromRaw(await postAthenaQueryStart(profile, region, body)),
  });
}

export function useAthenaExecution(profile: string, region: string, id?: string) {
  return useQuery({
    queryKey: ['aws', 'athena-execution', profile, region, id],
    queryFn: async () =>
      athenaExecutionFromRaw(await getAthenaQueryExecution(profile, region, id!)),
    enabled: !!profile && !!id,
    refetchInterval: (query) => pollWhileActive(query.state.data),
    staleTime: 0,
  });
}

export function useAthenaResults(profile: string, region: string, id?: string, enabled = false) {
  return useInfiniteQuery({
    queryKey: ['aws', 'athena-results', profile, region, id],
    queryFn: ({ pageParam }) =>
      getAthenaQueryResults(profile, region, id!, pageParam, QUERY_RESULT_PAGE_SIZE),
    initialPageParam: '',
    getNextPageParam: (last) => last.next_token || undefined,
    enabled: enabled && !!profile && !!id,
    // 完了した実行の結果は不変
    staleTime: Infinity,
  });
}

export function useAthenaStopQuery(profile: string, region: string) {
  return useMutation({
    mutationFn: (id: string) => deleteAthenaQuery(profile, region, id),
  });
}

export function useAthenaQueryHistory(
  profile: string,
  region: string,
  workgroup?: string,
  enabled = true,
) {
  return useQuery({
    queryKey: ['aws', 'athena-history', profile, region, workgroup],
    queryFn: async () =>
      (await getAthenaQueryHistory(profile, region, workgroup)).map(athenaHistoryFromRaw),
    staleTime: QUERY_HISTORY_STALE_TIME,
    enabled: enabled && !!profile,
  });
}

// ============================================================
// クエリスニペット (backend のサービス別ディレクトリへのファイル保存)
// ============================================================
export function useSnippets(service: QueryEditorService) {
  return useQuery({
    queryKey: ['snippets', service],
    queryFn: async () => (await getSnippets(service)).map(snippetFromRaw),
    // 保存ディレクトリへ手動配置した .sql も遅滞なく拾えるよう短めにする
    staleTime: QUERY_HISTORY_STALE_TIME,
  });
}

export function useSaveSnippet(service: QueryEditorService) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ name, sql }: { name: string; sql: string }) => postSnippet(service, name, sql),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['snippets', service] }),
  });
}

export function useDeleteSnippet(service: QueryEditorService) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => deleteSnippet(service, name),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['snippets', service] }),
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
  });
}

export function useDatadogEstimated(startMonth?: string, endMonth?: string, view?: string) {
  return useQuery({
    queryKey: ['datadog', 'estimated', startMonth, endMonth, view],
    queryFn: async () =>
      (await getDatadogEstimated(startMonth, endMonth, view)).map(datadogCostFromRaw),
  });
}

// ============================================================
// TiDB
// ============================================================
export function useTiDBProjects() {
  return useQuery({
    queryKey: ['tidb', 'projects'],
    queryFn: async () => (await getTiDBProjects()).map(tidbProjectFromRaw),
  });
}

export function useTiDBClusters(projectId: string) {
  return useQuery({
    queryKey: ['tidb', 'clusters', projectId],
    queryFn: async () => (await getTiDBClusters(projectId)).map(tidbClusterFromRaw),
    enabled: !!projectId,
  });
}

export function useTiDBCost(opts?: TiDBCostQueryOptions) {
  return useQuery({
    queryKey: ['tidb', 'cost', opts?.start, opts?.end],
    queryFn: async () => (await getTiDBCost(opts)).map(tidbCostFromRaw),
  });
}

// ============================================================
// GCP
// ============================================================
export function useGcpProjects() {
  return useQuery({
    queryKey: ['gcp', 'projects'],
    queryFn: async () => (await getGcpProjects()).map(gcpProjectFromRaw),
    staleTime: 5 * 60 * 1000,
    refetchInterval: (query) =>
      query.state.status === 'error' ? PROFILE_LIST_ERROR_RETRY_INTERVAL : false,
  });
}

// プロジェクト一覧の手動更新 (Cloud Resource Manager から再取得しローカルキャッシュを上書き)。
export function useRefreshGcpProjects() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () => (await getGcpProjects({ refresh: true })).map(gcpProjectFromRaw),
    onSuccess: (projects) => {
      queryClient.setQueryData(['gcp', 'projects'], projects);
    },
  });
}

// service (cloudrun / gcs) 単位で GCP リソースを取得する汎用フック。
// AWS 側 useResources と対称の形。normalizer に渡す Raw 型は呼び出し側で確定させる。
export function useGcpResources<TRaw, TRow extends BaseRow>(
  service: string,
  projectId: string,
  normalizer: (raw: TRaw) => TRow,
) {
  return useQuery({
    queryKey: ['gcp', service, projectId],
    queryFn: async () => {
      const raws = await getGcpResources<TRaw>(service, projectId);
      return raws.map(normalizer);
    },
    enabled: !!projectId,
  });
}

// バックエンドは最大 1000 件で打ち切る ({objects, truncated} エンベロープ)。
// prefix ごとに独立した queryKey で、検索確定 (prefix 変更) のたびに再取得される。
export function useGcsObjects(projectId: string, bucket: string, prefix?: string) {
  return useQuery({
    queryKey: ['gcp', 'gcs-objects', projectId, bucket, prefix],
    queryFn: async () => {
      const { objects, truncated } = await getGcsObjects(projectId, bucket, prefix);
      return { objects: (objects ?? []).map((raw, idx) => gcsObjectFromRaw(raw, idx)), truncated };
    },
    enabled: !!projectId && !!bucket,
  });
}

export function useGcsUpload(projectId: string, bucket: string, prefix?: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ key, file }: { key: string; file: File }) =>
      uploadGcsObject(projectId, bucket, `${prefix ?? ''}${key}`, file),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ['gcp', 'gcs-objects', projectId, bucket],
      });
      void queryClient.invalidateQueries({
        queryKey: ['gcp', 'gcs-object-preview', projectId, bucket],
      });
    },
  });
}

// enabled: !!key でプレビュー対象確定時のみ取得する。useS3ObjectPreview と対称 (staleTime: 0 も同じ)。
export function useGcsObjectPreview(projectId: string, bucket: string, key: string | undefined) {
  return useQuery({
    queryKey: ['gcp', 'gcs-object-preview', projectId, bucket, key],
    queryFn: async () => objectPreviewFromRaw(await getGcsObjectPreview(projectId, bucket, key!)),
    staleTime: 0,
    enabled: !!projectId && !!bucket && !!key,
  });
}

// ============================================================
// Cloud Logging (期間指定 + フィルターでのログエントリ取得。ページング対応)
// ============================================================

// 1 ページあたりの取得件数 (バックエンド既定と揃える必要はないが、多すぎるとログ 1 画面が
// 重くなるため BigQuery/Athena の結果ページングより小さめにする)。
const GCP_LOG_ENTRY_PAGE_SIZE = 200;

// runToken は「実行」ボタンを押すたびに呼び出し側でインクリメントする値。同じ filter/期間で
// 再実行しても新しい queryKey になるため、useBQQueryResults 等のジョブ ID ベースの結果取得と
// 同様に、確定した 1 回の実行結果を不変 (staleTime: Infinity) として扱える。
export function useGcpLogEntries(
  projectId: string,
  runToken: number,
  query: Pick<GcpLogEntriesQuery, 'filter' | 'start' | 'end'>,
  enabled: boolean,
) {
  return useInfiniteQuery({
    queryKey: ['gcp', 'logging-entries', projectId, runToken],
    queryFn: ({ pageParam }) =>
      getGcpLogEntries(projectId, {
        ...query,
        pageToken: pageParam,
        pageSize: GCP_LOG_ENTRY_PAGE_SIZE,
      }),
    initialPageParam: '',
    getNextPageParam: (last) => last.next_page_token || undefined,
    enabled: enabled && !!projectId && runToken > 0,
    staleTime: Infinity,
  });
}

// ============================================================
// CloudWatch Logs (ログビューア)
// ============================================================

// ロググループ一覧。変化が緩やかなためやや長めの staleTime にする。
export function useCWLogGroups(profile: string, region: string) {
  return useQuery({
    queryKey: ['aws', 'cwlogs-groups', profile, region],
    queryFn: async () => (await getCWLogGroups(profile, region)).map(cwLogGroupFromRaw),
    staleTime: 5 * 60 * 1000,
    enabled: !!profile,
  });
}

// 1 ページあたりの取得件数 (ロググループ 1 つあたりの上限。複数選択時はこの倍数まで返る)。
const CW_LOG_EVENT_PAGE_SIZE = 200;

// useGcpLogEntries と同じく runToken を queryKey に混ぜて「確定した 1 回の実行結果」を
// 不変 (staleTime: Infinity) として扱う。groups が空、または runToken が 0 のときは無効。
export function useCWLogEvents(
  profile: string,
  region: string,
  runToken: number,
  query: Omit<CWLogEventsQuery, 'pageToken' | 'limit'>,
  enabled: boolean,
) {
  return useInfiniteQuery({
    queryKey: ['aws', 'cwlogs-events', profile, region, runToken],
    queryFn: ({ pageParam }) =>
      getCWLogEvents(profile, region, {
        ...query,
        pageToken: pageParam,
        limit: CW_LOG_EVENT_PAGE_SIZE,
      }),
    initialPageParam: '',
    getNextPageParam: (last) => last.next_page_token || undefined,
    enabled: enabled && !!profile && query.groups.length > 0 && runToken > 0,
    staleTime: Infinity,
  });
}

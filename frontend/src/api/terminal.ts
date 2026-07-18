// EC2 Start Session / ECS Exec Command / GCP Cloud Logging Live Tail 用の WebSocket URL 構築ヘルパー
// apiGet/apiPost (client.ts) は fetch ベースのため WebSocket には使えず、別系統として用意する。
import { apiBaseUrl } from './client';

// http(s) の BASE_URL を ws(s) に変換した上でパス・クエリを組み立てる。
// 配列値は同名キーを繰り返す (例: group=a&group=b。CloudWatch Logs の複数ロググループ指定)。
function buildWsUrl(path: string, params?: Record<string, string | string[] | undefined>): string {
  const url = new URL(path, apiBaseUrl());
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v === undefined) continue;
      if (Array.isArray(v)) {
        for (const item of v) url.searchParams.append(k, item);
      } else {
        url.searchParams.set(k, v);
      }
    }
  }
  return url.toString();
}

// EC2 インスタンスへの SSM Start Session を開始する WebSocket URL を組み立てる
export function ec2SessionUrl(profile: string, instance: string, region: string): string {
  return buildWsUrl(
    `/api/aws/profiles/${encodeURIComponent(profile)}/ec2/${encodeURIComponent(instance)}/session`,
    { region },
  );
}

// ECS タスクコンテナへの Exec Command を開始する WebSocket URL を組み立てる
export function ecsExecUrl(
  profile: string,
  cluster: string,
  task: string,
  container: string,
  region: string,
  command?: string,
): string {
  return buildWsUrl(
    `/api/aws/profiles/${encodeURIComponent(profile)}/ecs/${encodeURIComponent(cluster)}/tasks/${encodeURIComponent(task)}/exec`,
    { region, container, command },
  );
}

// GCP Cloud Logging の Live Tail (フィルター適用状態のままの新着ログ受信) を開始する
// WebSocket URL を組み立てる。
export function gcpLoggingTailUrl(projectId: string, filter: string): string {
  return buildWsUrl('/api/gcp/logging/tail', {
    project_id: projectId,
    filter: filter || undefined,
  });
}

// CloudWatch Logs の Live Tail (選択ロググループ横断の新着ログ受信) を開始する
// WebSocket URL を組み立てる。groups はロググループ ARN の配列。
export function cwLogsTailUrl(
  profile: string,
  region: string,
  groups: string[],
  filter: string,
): string {
  return buildWsUrl(`/api/aws/profiles/${encodeURIComponent(profile)}/logs/tail`, {
    region,
    group: groups,
    filter: filter || undefined,
  });
}

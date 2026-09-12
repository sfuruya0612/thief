// Datadog の OAuth 再ログイン (DatadogAuthBanner から使う) を 2 つのフックに分ける。
//
//   - useDatadogLoginStart: login/start を呼んで認可タブを認可 URL へ遷移させる
//   - useDatadogLoginStatus: login/status をポーリングして完了を検知する
//
// AWS の useSSOLogin は complete がサーバ側でブロックして完了を待つ 1 回の POST だが、
// Datadog の完了検知は login/status のポーリングであり、待ち方の構造が異なるため共通化
// しない。ポーリングはこのリポジトリの規約 (useProfiles / useHealthCheck / useBQQueryJob と
// 同じ useQuery + refetchInterval) に従う。
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { getDatadogLoginStatus, postDatadogLoginStart } from '../api/endpoints';
import { datadogLoginStartFromRaw, datadogLoginStatusFromRaw } from '../lib/normalizeNonAws';
import type { DatadogLoginStartRow, DatadogLoginStatusRow } from '../types/nonaws';

// ポーリング間隔 (ms)。ブラウザでの認可はユーザの操作待ちなので短くしすぎない。
export const DATADOG_LOGIN_POLL_INTERVAL = 2000;

// ポーリングの打ち切り時間 (ms)。backend のログインセッション TTL (10 分) より十分短くし、
// ユーザがタブを放置したまま無限にポーリングし続けないようにする。
export const DATADOG_LOGIN_POLL_TIMEOUT = 120_000;

// useDatadogLoginStart のミューテーション入力。authWindow は Login ボタンの click
// ハンドラが同期的に window.open した空タブ (ポップアップブロック時は null)。
export interface DatadogLoginStartInput {
  authWindow: Window | null;
  // 開いておいた空タブを自動制御できないと判明したとき (start 応答時点でユーザが手動で
  // 閉じていた場合) に呼ぶ。バナーはフォールバックリンクの表示に切り替える。
  onTabUnavailable?: () => void;
}

// 進行中のログイン。state は login/status のキー、deadline はポーリングの打ち切り時刻
// (epoch ms)、authWindow は完了時に閉じる認可タブ。
export interface DatadogLoginSession {
  state: string;
  deadline: number;
  authWindow: Window | null;
}

// login/start を呼び、認可タブを認可 URL へ遷移させる。完了の待機は行わない
// (useDatadogLoginStatus が引き継ぐ)。
export function useDatadogLoginStart() {
  return useMutation({
    mutationFn: async ({
      authWindow,
      onTabUnavailable,
    }: DatadogLoginStartInput): Promise<DatadogLoginStartRow> => {
      let started: DatadogLoginStartRow;
      try {
        started = datadogLoginStartFromRaw(await postDatadogLoginStart());
      } catch (err) {
        // 認可ページへ遷移する前の失敗。ユーザが認可の途中ということはないので、
        // 開いておいた空タブは閉じてよい。
        authWindow?.close();
        throw err;
      }
      if (authWindow && !authWindow.closed) {
        authWindow.location.replace(started.authorizationUrl);
      } else if (authWindow) {
        // start の応答待ちの間にユーザが空タブを手動で閉じた場合。閉じたタブの location
        // 操作はブラウザによって例外になりうるため触らず、バナーのフォールバックリンク
        // からの認可継続に切り替える (ログインセッションは backend が保持しており、別タブで
        // 認可を完了すれば login/status は succeeded になる)。
        onTabUnavailable?.();
      }
      return started;
    },
  });
}

// login/status をポーリングし、認可の完了を検知する。session が null の間は無効。
//
// 停止条件は 3 つあり、いずれも refetchInterval が false を返して止まる。
//
//   - succeeded: 認可タブを閉じ、Datadog のクエリを無効化して再取得させる
//   - failed / 打ち切り / login/status のエラー: queryFn が throw して error 状態になる
//     (404 DATADOG_LOGIN_SESSION_NOT_FOUND は再試行しても復帰しないため retry も行わない)
export function useDatadogLoginStatus(session: DatadogLoginSession | null) {
  const queryClient = useQueryClient();
  return useQuery({
    queryKey: ['datadog', 'login-status', session?.state],
    queryFn: async (): Promise<DatadogLoginStatusRow> => {
      // session は enabled で非 null を保証している。
      const active = session!;
      if (Date.now() >= active.deadline) {
        throw new Error('datadog login timed out before the authorization completed');
      }
      const status = datadogLoginStatusFromRaw(await getDatadogLoginStatus(active.state));
      if (status.status === 'failed') {
        throw new Error(status.errorMessage || 'datadog login failed');
      }
      if (status.status === 'succeeded') {
        active.authWindow?.close();
        // 自分自身 (['datadog', 'login-status', ...]) を除いて無効化する。含めると
        // 再取得 → succeeded → 無効化 の繰り返しになる。
        await queryClient.invalidateQueries({
          queryKey: ['datadog'],
          predicate: (query) => query.queryKey[1] !== 'login-status',
        });
      }
      return status;
    },
    enabled: session !== null,
    refetchInterval: (query) =>
      query.state.status === 'error' || query.state.data?.status !== 'pending'
        ? false
        : DATADOG_LOGIN_POLL_INTERVAL,
    // 進行状態は毎回取りに行く。再試行は打ち切り時間の判定を曖昧にするので行わない。
    staleTime: 0,
    retry: false,
    gcTime: 0,
  });
}

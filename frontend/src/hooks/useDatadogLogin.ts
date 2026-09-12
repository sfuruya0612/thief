// Datadog の OAuth 再ログインを 3 つのフックに分ける。
//
//   - useDatadogLoginStart: login/start を呼んで認可タブを認可 URL へ遷移させる
//   - useDatadogLoginStatus: login/status をポーリングして完了を検知する
//   - useDatadogLoginFlow: 上の 2 つと表示状態をまとめ、1 回の begin() で開始できる形にする
//     (バナーのボタンと、未ログインの組織タブのクリックによる自動開始の両方から使う)
//
// AWS の useSSOLogin は complete がサーバ側でブロックして完了を待つ 1 回の POST だが、
// Datadog の完了検知は login/status のポーリングであり、待ち方の構造が異なるため共通化
// しない。ポーリングはこのリポジトリの規約 (useProfiles / useHealthCheck / useBQQueryJob と
// 同じ useQuery + refetchInterval) に従う。
import { useCallback, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { getDatadogLoginStatus, postDatadogLoginStart } from '../api/endpoints';
import { datadogLoginStartFromRaw, datadogLoginStatusFromRaw } from '../lib/normalizeNonAws';
import { ApiError } from '../types/common';
import type { DatadogLoginStartRow, DatadogLoginStatusRow } from '../types/nonaws';

// backend が login/start で返す、redirect_uri 未登録 (OAuthRedirectBase の変更後に
// クライアントの再登録が要る状態) のエラーコード。再ログインでは解決しないため、
// 通常のログイン失敗とは別の文言を出す。
export const REDIRECT_URI_NOT_REGISTERED_CODE = 'DATADOG_REDIRECT_URI_NOT_REGISTERED';

// ポーリング間隔 (ms)。ブラウザでの認可はユーザの操作待ちなので短くしすぎない。
export const DATADOG_LOGIN_POLL_INTERVAL = 2000;

// ポーリングの打ち切り時間 (ms)。backend のログインセッション TTL (10 分) より十分短くし、
// ユーザがタブを放置したまま無限にポーリングし続けないようにする。
export const DATADOG_LOGIN_POLL_TIMEOUT = 120_000;

// useDatadogLoginStart のミューテーション入力。authWindow は click ハンドラが同期的に
// window.open した空タブ (ポップアップブロック時は null)。
export interface DatadogLoginStartInput {
  // ログイン対象の組織 (空文字は親組織)。トークンは org ごとに別ファイルへ保存される。
  org: string;
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
      org,
      authWindow,
      onTabUnavailable,
    }: DatadogLoginStartInput): Promise<DatadogLoginStartRow> => {
      let started: DatadogLoginStartRow;
      try {
        started = datadogLoginStartFromRaw(await postDatadogLoginStart(org));
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

// 1 回のログイン操作 (認可タブを開く → login/start → login/status のポーリング) と、
// その表示状態をまとめたもの。
export interface DatadogLoginFlow {
  // 指定した組織のログインを開始する。認可タブを同期的に開くため、必ず click などの
  // イベントハンドラから直接呼ぶこと (await を挟むと transient activation が失効し
  // ポップアップブロックにかかる)。
  begin: (org: string) => void;
  loggingIn: boolean;
  failed: boolean;
  redirectNotRegistered: boolean;
  // 認可タブを自動制御できないときに提示する認可 URL。それ以外は空文字。
  fallbackUrl: string;
}

export function useDatadogLoginFlow(): DatadogLoginFlow {
  const start = useDatadogLoginStart();
  // 進行中のログイン。login/start の成功で作り、useDatadogLoginStatus のポーリング対象になる。
  const [session, setSession] = useState<DatadogLoginSession | null>(null);
  // start 応答の認可 URL。フォールバック表示にだけ使う。
  const [started, setStarted] = useState<DatadogLoginStartRow | null>(null);
  // 認可タブを自動制御できない状態。ポップアップブロックで開けなかった場合と、開いた空タブを
  // start 応答前にユーザが手動で閉じていた場合 (onTabUnavailable) の 2 経路で true になる。
  const [tabUnavailable, setTabUnavailable] = useState(false);
  const status = useDatadogLoginStatus(session);

  const { mutate } = start;
  const begin = useCallback(
    (org: string) => {
      // 認可タブは start の応答を待たずに同期処理で開く。await を挟むと transient
      // activation が失効してポップアップブロックにかかりうるため。
      const authWindow = window.open('', '_blank');
      setTabUnavailable(authWindow === null);
      setStarted(null);
      setSession(null);
      mutate(
        { org, authWindow, onTabUnavailable: () => setTabUnavailable(true) },
        {
          onSuccess: (row) => {
            setStarted(row);
            setSession({
              state: row.state,
              deadline: Date.now() + DATADOG_LOGIN_POLL_TIMEOUT,
              authWindow,
            });
          },
        },
      );
    },
    [mutate],
  );

  // ポーリングは succeeded かエラー (failed / 打ち切り / セッション消失) で止まる。
  const polling = session !== null && !status.isError && status.data?.status !== 'succeeded';
  const loggingIn = start.isPending || polling;

  return {
    begin,
    loggingIn,
    failed: start.isError || status.isError,
    redirectNotRegistered:
      start.error instanceof ApiError && start.error.code === REDIRECT_URI_NOT_REGISTERED_CODE,
    fallbackUrl: polling && started !== null && tabUnavailable ? started.authorizationUrl : '',
  };
}

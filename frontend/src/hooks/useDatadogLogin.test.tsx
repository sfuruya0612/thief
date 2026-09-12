import { QueryClient, QueryClientProvider, type Query } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../types/common';
import {
  DATADOG_LOGIN_POLL_INTERVAL,
  DATADOG_LOGIN_POLL_TIMEOUT,
  useDatadogLoginStart,
  useDatadogLoginStatus,
  type DatadogLoginSession,
} from './useDatadogLogin';

vi.mock('../api/endpoints', () => ({
  postDatadogLoginStart: vi.fn(),
  getDatadogLoginStatus: vi.fn(),
}));

import { getDatadogLoginStatus, postDatadogLoginStart } from '../api/endpoints';

const mockedStart = vi.mocked(postDatadogLoginStart);
const mockedStatus = vi.mocked(getDatadogLoginStatus);

const startBody = {
  state: 'state-1',
  authorization_url: 'https://app.datadoghq.com/oauth2/v1/authorize?x=1',
};

// 認可タブの WindowProxy のモック。closed は「ユーザが手動で閉じた後か」を表す。
function newAuthWindow(closed = false) {
  return { closed, close: vi.fn(), location: { replace: vi.fn() } };
}

function newQueryClient(): QueryClient {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } });
}

function wrapperFor(client: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

function sessionFor(
  authWindow: Window | null,
  offset = DATADOG_LOGIN_POLL_TIMEOUT,
): DatadogLoginSession {
  return { state: startBody.state, deadline: Date.now() + offset, authWindow };
}

beforeEach(() => {
  mockedStart.mockReset();
  mockedStatus.mockReset();
});

describe('useDatadogLoginStart', () => {
  it('start 応答の authorization_url へ認可タブを遷移させる', async () => {
    mockedStart.mockResolvedValue(startBody);
    const authWindow = newAuthWindow();
    const { result } = renderHook(() => useDatadogLoginStart(), {
      wrapper: wrapperFor(newQueryClient()),
    });

    result.current.mutate({ org: 'suborg1', authWindow: authWindow as unknown as Window });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(authWindow.location.replace).toHaveBeenCalledWith(startBody.authorization_url);
    expect(result.current.data).toEqual({
      state: startBody.state,
      authorizationUrl: startBody.authorization_url,
    });
    // ログイン対象の組織が login/start へ届くこと。届かないと、どの組織のタブから
    // 始めても親組織のトークンだけが更新される。
    expect(mockedStart).toHaveBeenCalledWith('suborg1');
  });

  it('start が失敗した場合は開いた空タブを閉じてエラーにする', async () => {
    mockedStart.mockRejectedValue(
      new ApiError(409, 'DATADOG_REDIRECT_URI_NOT_REGISTERED', 'redirect uri is not registered'),
    );
    const authWindow = newAuthWindow();
    const { result } = renderHook(() => useDatadogLoginStart(), {
      wrapper: wrapperFor(newQueryClient()),
    });

    result.current.mutate({ org: 'suborg1', authWindow: authWindow as unknown as Window });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(authWindow.close).toHaveBeenCalledTimes(1);
    expect(authWindow.location.replace).not.toHaveBeenCalled();
  });

  it('start 応答時点で空タブが閉じられていた場合は location を操作せず onTabUnavailable を呼ぶ', async () => {
    mockedStart.mockResolvedValue(startBody);
    const authWindow = newAuthWindow(true);
    const onTabUnavailable = vi.fn();
    const { result } = renderHook(() => useDatadogLoginStart(), {
      wrapper: wrapperFor(newQueryClient()),
    });

    result.current.mutate({
      org: '',
      authWindow: authWindow as unknown as Window,
      onTabUnavailable,
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(onTabUnavailable).toHaveBeenCalledTimes(1);
    expect(authWindow.location.replace).not.toHaveBeenCalled();
    expect(authWindow.close).not.toHaveBeenCalled();
  });
});

// ポーリングの検証では偽タイマーを使う。@testing-library/react の waitFor は vitest の
// 偽タイマーを自動では進めないため、時間の経過と再取得の解決は act で包んで手動で進める。
async function advance(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

describe('useDatadogLoginStatus', () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it('session が null の間は login/status を呼ばない', () => {
    renderHook(() => useDatadogLoginStatus(null), { wrapper: wrapperFor(newQueryClient()) });

    expect(mockedStatus).not.toHaveBeenCalled();
  });

  it('pending の間はポーリングし、succeeded で停止して認可タブを閉じ datadog クエリを無効化する', async () => {
    vi.useFakeTimers();
    mockedStatus
      .mockResolvedValueOnce({ status: 'pending' })
      .mockResolvedValue({ status: 'succeeded' });
    const authWindow = newAuthWindow();
    const client = newQueryClient();
    const invalidate = vi.spyOn(client, 'invalidateQueries');
    const { result } = renderHook(
      () => useDatadogLoginStatus(sessionFor(authWindow as unknown as Window)),
      { wrapper: wrapperFor(client) },
    );

    await advance(1);
    expect(result.current.data?.status).toBe('pending');
    expect(authWindow.close).not.toHaveBeenCalled();

    // 再取得の解決と、その結果が観測に反映されるまでを 2 段階で進める。
    await advance(DATADOG_LOGIN_POLL_INTERVAL);
    await advance(1);
    expect(result.current.data?.status).toBe('succeeded');
    expect(authWindow.close).toHaveBeenCalledTimes(1);

    // 無効化の対象は Datadog のクエリ全体だが、ポーリング自身
    // (['datadog', 'login-status', ...]) は除く。含めると再取得が止まらなくなる。
    const options = invalidate.mock.calls.at(0)?.[0];
    expect(options?.queryKey).toEqual(['datadog']);
    const matches = (queryKey: unknown[]) =>
      options?.predicate?.({ queryKey } as unknown as Query) ?? false;
    expect(matches(['datadog', 'historical', '2026-09'])).toBe(true);
    expect(matches(['datadog', 'login-status', startBody.state])).toBe(false);

    // 完了後はポーリングが止まる (時間を進めても呼び出しが増えない)。
    const calls = mockedStatus.mock.calls.length;
    await advance(DATADOG_LOGIN_POLL_INTERVAL * 5);
    expect(mockedStatus.mock.calls.length).toBe(calls);
  });

  it('failed はエラーで終了しポーリングを止める', async () => {
    vi.useFakeTimers();
    mockedStatus.mockResolvedValue({
      status: 'failed',
      error_message: 'datadog authorization failed: access_denied',
    });
    const { result } = renderHook(() => useDatadogLoginStatus(sessionFor(null)), {
      wrapper: wrapperFor(newQueryClient()),
    });

    await advance(1);
    expect(result.current.isError).toBe(true);
    expect(result.current.error?.message).toContain('access_denied');

    const calls = mockedStatus.mock.calls.length;
    await advance(DATADOG_LOGIN_POLL_INTERVAL * 5);
    expect(mockedStatus.mock.calls.length).toBe(calls);
  });

  it('セッション消失 (404 DATADOG_LOGIN_SESSION_NOT_FOUND) は打ち切り時間を待たず停止する', async () => {
    vi.useFakeTimers();
    mockedStatus.mockRejectedValue(
      new ApiError(
        404,
        'DATADOG_LOGIN_SESSION_NOT_FOUND',
        'datadog login session not found or expired',
      ),
    );
    const { result } = renderHook(() => useDatadogLoginStatus(sessionFor(null)), {
      wrapper: wrapperFor(newQueryClient()),
    });

    await advance(1);
    expect(result.current.isError).toBe(true);
    expect(mockedStatus).toHaveBeenCalledTimes(1);

    // 打ち切り時間 (120 秒) を大きく超えて進めても再試行しない。
    await advance(DATADOG_LOGIN_POLL_TIMEOUT * 2);
    expect(mockedStatus).toHaveBeenCalledTimes(1);
  });

  it('打ち切り時間に達したら login/status を呼ばずにエラーで終了する', async () => {
    // 既に打ち切り時刻を過ぎた session を渡し、最初のポーリングで打ち切りに当たる状況を作る。
    const { result } = renderHook(() => useDatadogLoginStatus(sessionFor(null, -1)), {
      wrapper: wrapperFor(newQueryClient()),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.message).toContain('timed out');
    expect(mockedStatus).not.toHaveBeenCalled();
  });
});

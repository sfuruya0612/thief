// useDatadogDashboards / useDatadogDashboard / useDatadogMetricsQueries の enabled 判定の
// 検証。org は空文字 (親組織自身) も有効な取得対象であり、!!org で弾くと親組織タブで
// クエリが永久に発火しなくなる (issue 0171 のラウンド 2 レビューで発見された回帰)。
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type {
  DatadogDashboardDetailRaw,
  DatadogDashboardRaw,
  DatadogMetricQueryResultRaw,
} from '../types/nonaws';

vi.mock('./endpoints', () => ({
  getDatadogDashboards: vi.fn(),
  getDatadogDashboard: vi.fn(),
  getDatadogMetricsQuery: vi.fn(),
}));

import { getDatadogDashboard, getDatadogDashboards, getDatadogMetricsQuery } from './endpoints';
import { useDatadogDashboard, useDatadogDashboards, useDatadogMetricsQueries } from './queries';

const mockedGetDatadogDashboards = vi.mocked(getDatadogDashboards);
const mockedGetDatadogDashboard = vi.mocked(getDatadogDashboard);
const mockedGetDatadogMetricsQuery = vi.mocked(getDatadogMetricsQuery);

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

const dashboards: DatadogDashboardRaw[] = [
  { id: 'abc-123', title: 'Overview', description: '', url: '' },
];

const dashboardDetail: DatadogDashboardDetailRaw = {
  id: 'abc-123',
  title: 'Overview',
  description: '',
  url: '',
  widgets: [],
};

const metricsResult: DatadogMetricQueryResultRaw = {
  query: 'avg:system.cpu.user{*}',
  series: [],
};

describe('useDatadogDashboards', () => {
  beforeEach(() => {
    mockedGetDatadogDashboards.mockReset();
  });

  it('org が空文字 (親組織自身) でも取得する', async () => {
    mockedGetDatadogDashboards.mockResolvedValue(dashboards);
    const { result } = renderHook(() => useDatadogDashboards(''), { wrapper });

    await waitFor(() => expect(result.current.data).toEqual([{ ...dashboards[0] }]));
    expect(mockedGetDatadogDashboards).toHaveBeenCalledWith('');
  });
});

describe('useDatadogDashboard', () => {
  beforeEach(() => {
    mockedGetDatadogDashboard.mockReset();
  });

  it('org が空文字でも id があれば取得する', async () => {
    mockedGetDatadogDashboard.mockResolvedValue(dashboardDetail);
    const { result } = renderHook(() => useDatadogDashboard('', 'abc-123'), { wrapper });

    await waitFor(() => expect(result.current.data?.id).toBe('abc-123'));
    expect(mockedGetDatadogDashboard).toHaveBeenCalledWith('', 'abc-123');
  });

  it('id が空の間は取得しない', () => {
    renderHook(() => useDatadogDashboard('', ''), { wrapper });
    expect(mockedGetDatadogDashboard).not.toHaveBeenCalled();
  });
});

describe('useDatadogMetricsQueries', () => {
  beforeEach(() => {
    mockedGetDatadogMetricsQuery.mockReset();
  });

  it('org が空文字でもクエリがあれば取得する', async () => {
    mockedGetDatadogMetricsQuery.mockResolvedValue(metricsResult);
    const { result } = renderHook(
      () =>
        useDatadogMetricsQueries('', ['avg:system.cpu.user{*}'], {
          from: 1700000000,
          to: 1700003600,
        }),
      { wrapper },
    );

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(mockedGetDatadogMetricsQuery).toHaveBeenCalledWith(
      '',
      'avg:system.cpu.user{*}',
      1700000000,
      1700003600,
    );
  });

  it('クエリが空の間は取得しない', () => {
    renderHook(() => useDatadogMetricsQueries('', [], { from: 1700000000, to: 1700003600 }), {
      wrapper,
    });
    expect(mockedGetDatadogMetricsQuery).not.toHaveBeenCalled();
  });
});

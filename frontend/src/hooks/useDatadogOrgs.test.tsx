import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { DatadogOrgRaw } from '../types/nonaws';
import { STORAGE_KEY } from '../lib/storage';
import { useActiveDatadogOrg } from './useDatadogOrgs';

vi.mock('../api/endpoints', () => ({
  getDatadogOrgs: vi.fn(),
}));

import { getDatadogOrgs } from '../api/endpoints';

const mockedGetDatadogOrgs = vi.mocked(getDatadogOrgs);

const raw = (id: string, loggedIn = true, isSelf = false): DatadogOrgRaw => ({
  id,
  name: id.toUpperCase(),
  logged_in: loggedIn,
  is_self: isSelf,
});

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe('useActiveDatadogOrg', () => {
  beforeEach(() => {
    localStorage.clear();
    mockedGetDatadogOrgs.mockReset();
  });

  it('初回ロードで開いているタブが無ければ先頭を自動オープンする', async () => {
    mockedGetDatadogOrgs.mockResolvedValue([raw('abc123'), raw('sub456', false)]);
    const { result } = renderHook(() => useActiveDatadogOrg(), { wrapper });

    await waitFor(() => expect(result.current.activeOrg).toBe('abc123'));
    expect(result.current.openOrgs).toEqual(['abc123']);
    expect(result.current.orgs.map((o) => o.loggedIn)).toEqual([true, false]);
  });

  it('永続化された組織タブを復元し、一覧に無くても先頭へ飛ばさない', async () => {
    // 一覧から消えた組織のタブが勝手に別組織へ切り替わると、別組織のコストを
    // 同じタブに表示することになる。
    localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({ datadogOrgSessions: { open: ['gone456'], active: 'gone456' } }),
    );
    mockedGetDatadogOrgs.mockResolvedValue([raw('abc123')]);
    const { result } = renderHook(() => useActiveDatadogOrg(), { wrapper });

    await waitFor(() => expect(result.current.orgs.length).toBe(1));
    expect(result.current.activeOrg).toBe('gone456');
    expect(result.current.openOrgs).toEqual(['gone456']);
  });

  it('一覧が空なら何も開かない', async () => {
    mockedGetDatadogOrgs.mockResolvedValue([]);
    const { result } = renderHook(() => useActiveDatadogOrg(), { wrapper });

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.openOrgs).toEqual([]);
    expect(result.current.activeOrg).toBe('');
  });
});

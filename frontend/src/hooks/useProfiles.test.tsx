import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { STORAGE_KEY, type PersistedState } from '../lib/storage';
import { useProfiles } from './useProfiles';

vi.mock('../api/endpoints', () => ({
  getProfiles: vi.fn(),
}));

import { getProfiles } from '../api/endpoints';

const mockedGetProfiles = vi.mocked(getProfiles);

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

const readRaw = (): PersistedState => JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}');

describe('useProfiles', () => {
  beforeEach(() => {
    localStorage.clear();
    mockedGetProfiles.mockReset();
  });

  it('初回ロードで開いているタブが無ければ先頭を自動オープンする', async () => {
    mockedGetProfiles.mockResolvedValue([{ name: 'first' }, { name: 'second' }]);
    const { result } = renderHook(() => useProfiles(), { wrapper });

    await waitFor(() => expect(result.current.activeProfile).toBe('first'));
    expect(result.current.openProfiles).toEqual(['first']);
  });

  it('永続化されたタブがあれば自動オープンしない', async () => {
    localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({ awsSessions: { open: ['saved'], active: 'saved' } }),
    );
    mockedGetProfiles.mockResolvedValue([{ name: 'first' }, { name: 'saved' }]);
    const { result } = renderHook(() => useProfiles(), { wrapper });

    await waitFor(() => expect(result.current.profiles.length).toBe(2));
    expect(result.current.openProfiles).toEqual(['saved']);
    expect(result.current.activeProfile).toBe('saved');
  });

  it('一覧に無い開きタブを prune しない', async () => {
    localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({ awsSessions: { open: ['ghost', 'real'], active: 'ghost' } }),
    );
    mockedGetProfiles.mockResolvedValue([{ name: 'real' }]);
    const { result } = renderHook(() => useProfiles(), { wrapper });

    await waitFor(() => expect(result.current.profiles.length).toBe(1));
    expect(result.current.openProfiles).toEqual(['ghost', 'real']);
    expect(result.current.activeProfile).toBe('ghost');
  });

  it('一覧取得エラーでは自動オープンの shot を消費しない', async () => {
    mockedGetProfiles.mockRejectedValue(new Error('backend down'));
    const { result } = renderHook(() => useProfiles(), { wrapper });

    await waitFor(() => expect(result.current.error).not.toBeNull());
    expect(result.current.openProfiles).toEqual([]);
  });

  it('全タブを閉じても自動で開き直さない', async () => {
    mockedGetProfiles.mockResolvedValue([{ name: 'first' }]);
    const { result } = renderHook(() => useProfiles(), { wrapper });
    await waitFor(() => expect(result.current.activeProfile).toBe('first'));

    act(() => result.current.closeProfile('first'));
    expect(result.current.openProfiles).toEqual([]);
    expect(result.current.activeProfile).toBe('');
    expect(readRaw().activeProfile).toBeUndefined();
  });

  it('openProfile / activateProfile / moveProfile が状態を更新する', async () => {
    mockedGetProfiles.mockResolvedValue([{ name: 'a' }, { name: 'b' }, { name: 'c' }]);
    const { result } = renderHook(() => useProfiles(), { wrapper });
    await waitFor(() => expect(result.current.activeProfile).toBe('a'));

    act(() => result.current.openProfile('b'));
    act(() => result.current.openProfile('c'));
    expect(result.current.openProfiles).toEqual(['a', 'b', 'c']);

    act(() => result.current.activateProfile('a'));
    expect(result.current.activeProfile).toBe('a');

    act(() => result.current.moveProfile(0, 2));
    expect(result.current.openProfiles).toEqual(['b', 'c', 'a']);
    expect(readRaw().awsSessions?.open).toEqual(['b', 'c', 'a']);
  });
});

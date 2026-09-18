import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { useCachedQueryData } from './useCachedQueryData';

const KEY = ['aws', 'ecr', 'test', 'ap-northeast-1'];

function newClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } });
}

function wrapperFor(client: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

describe('useCachedQueryData', () => {
  it('キャッシュ済みデータを返し、setQueryData の更新に追従する', async () => {
    const client = newClient();
    const { result } = renderHook(() => useCachedQueryData<number[]>(KEY), {
      wrapper: wrapperFor(client),
    });
    expect(result.current).toBeUndefined();

    act(() => client.setQueryData(KEY, [1, 2, 3]));
    await waitFor(() => expect(result.current).toEqual([1, 2, 3]));
  });

  it('同じ queryKey の実クエリの options を変更しない', async () => {
    const client = newClient();
    const fn = vi.fn(async () => [{ id: 1 }]);
    await client.fetchQuery({ queryKey: KEY, queryFn: fn });
    const query = client.getQueryCache().get(JSON.stringify(KEY));
    expect(query?.options.queryFn).toBe(fn);

    const { result } = renderHook(() => useCachedQueryData<{ id: number }[]>(KEY), {
      wrapper: wrapperFor(client),
    });
    expect(result.current).toEqual([{ id: 1 }]);

    // フックの購読後も共有クエリの queryFn は実クエリのまま
    expect(query?.options.queryFn).toBe(fn);
  });
});

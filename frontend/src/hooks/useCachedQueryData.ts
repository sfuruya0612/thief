// サイドバーの件数バッジのように「他所で埋まったクエリキャッシュを読むだけで、自身は
// fetch しない」箇所向けの読み取り専用フック。
//
// useQuery に queryFn: skipToken を渡すと同じ queryKey の QueryObserver が共有クエリの
// options (queryFn / enabled) を skipToken で上書きし、invalidateQueries の再取得が
// Missing queryFn で失敗する (issue 0187)。ここでは QueryObserver を作らず、
// queryClient のクエリキャッシュを useSyncExternalStore で直接購読する。これにより
// 共有クエリの options には一切触れず、dev 用の console.error も出さない。
import { useCallback, useSyncExternalStore } from 'react';
import { hashKey, useQueryClient } from '@tanstack/react-query';
import type { QueryKey } from '@tanstack/react-query';

// useCachedQueryData は queryKey に一致するクエリのキャッシュ済みデータを返す。
// クエリが存在しない (まだ取得されていない) 間は undefined を返す。
export function useCachedQueryData<T>(queryKey: QueryKey): T | undefined {
  const queryClient = useQueryClient();
  const queryHash = hashKey(queryKey);

  const subscribe = useCallback(
    (onStoreChange: () => void) =>
      queryClient.getQueryCache().subscribe((event) => {
        if (event.query.queryHash === queryHash) onStoreChange();
      }),
    [queryClient, queryHash],
  );

  const getSnapshot = useCallback(
    () => queryClient.getQueryCache().get(queryHash)?.state.data as T | undefined,
    [queryClient, queryHash],
  );

  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}

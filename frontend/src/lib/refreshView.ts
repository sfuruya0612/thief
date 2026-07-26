// TopBar の Refresh 押下時の処理列。
// backend のリソースキャッシュを view 単位で破棄してから TanStack Query を
// 無効化する (先に無効化すると再取得が破棄前の backend キャッシュ HIT を受け取り、
// 古いデータが表示され続けるため)。issues/0079 を参照。
import type { AppView } from '../types/common';

export interface RefreshViewDeps {
  // POST /api/cache/invalidate?view=... を呼ぶ (api/queries.ts の useCacheInvalidate)
  postCacheInvalidate: (view: AppView) => Promise<void>;
  // TanStack Query の queryKey 前方一致の無効化
  invalidateQueries: (queryKey: string[]) => Promise<void>;
}

// createViewRefresher は Refresh 1 回分の処理列を実行する関数を返す。
// 実行中の再入は no-op (呼び出し側のボタン無効化とは独立にここでも保証する)。
export function createViewRefresher(deps: RefreshViewDeps): (view: AppView) => Promise<void> {
  let running = false;
  return async (view: AppView): Promise<void> => {
    if (running) return;
    running = true;
    try {
      try {
        await deps.postCacheInvalidate(view);
      } catch {
        // backend 側の破棄に失敗しても query の無効化は行う
        // (backend 停止時に Refresh が完全な no-op になるのを避ける)
      }
      const invalidations = [deps.invalidateQueries([view])];
      // BigQuery のクエリキーは歴史的経緯で 'gcp' ではなく 'bigquery' 始まりのため合わせて更新する
      if (view === 'gcp') {
        invalidations.push(deps.invalidateQueries(['bigquery']));
      }
      await Promise.all(invalidations);
    } finally {
      running = false;
    }
  };
}

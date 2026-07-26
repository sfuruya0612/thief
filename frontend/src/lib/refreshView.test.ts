import { describe, expect, it, vi } from 'vitest';
import { createViewRefresher } from './refreshView';

// resolve を外から制御できる Promise を作る (実行順序の検証用)
function deferred<T = void>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe('createViewRefresher', () => {
  it('破棄 POST のレスポンス受領後に invalidateQueries が実行される', async () => {
    const post = deferred();
    const postCacheInvalidate = vi.fn(() => post.promise);
    const invalidateQueries = vi.fn(() => Promise.resolve());
    const refresh = createViewRefresher({ postCacheInvalidate, invalidateQueries });

    const done = refresh('aws');
    // POST が未解決の間は invalidateQueries は呼ばれない
    await Promise.resolve();
    expect(postCacheInvalidate).toHaveBeenCalledWith('aws');
    expect(invalidateQueries).not.toHaveBeenCalled();

    post.resolve();
    await done;
    expect(invalidateQueries).toHaveBeenCalledWith(['aws']);
  });

  it('POST が reject されても invalidateQueries が実行される', async () => {
    const postCacheInvalidate = vi.fn(() => Promise.reject(new Error('backend down')));
    const invalidateQueries = vi.fn(() => Promise.resolve());
    const refresh = createViewRefresher({ postCacheInvalidate, invalidateQueries });

    await refresh('datadog');
    expect(invalidateQueries).toHaveBeenCalledWith(['datadog']);
  });

  it('実行中の再入は no-op になる', async () => {
    const post = deferred();
    const postCacheInvalidate = vi.fn(() => post.promise);
    const invalidateQueries = vi.fn(() => Promise.resolve());
    const refresh = createViewRefresher({ postCacheInvalidate, invalidateQueries });

    const first = refresh('aws');
    const second = refresh('aws');
    post.resolve();
    await Promise.all([first, second]);

    expect(postCacheInvalidate).toHaveBeenCalledTimes(1);
    expect(invalidateQueries).toHaveBeenCalledTimes(1);

    // 完了後は再度実行できる
    await refresh('aws');
    expect(postCacheInvalidate).toHaveBeenCalledTimes(2);
  });

  it("view === 'gcp' で ['bigquery'] も無効化される", async () => {
    const postCacheInvalidate = vi.fn(() => Promise.resolve());
    const invalidateQueries = vi.fn(() => Promise.resolve());
    const refresh = createViewRefresher({ postCacheInvalidate, invalidateQueries });

    await refresh('gcp');
    expect(invalidateQueries).toHaveBeenCalledWith(['gcp']);
    expect(invalidateQueries).toHaveBeenCalledWith(['bigquery']);
    expect(invalidateQueries).toHaveBeenCalledTimes(2);
  });
});

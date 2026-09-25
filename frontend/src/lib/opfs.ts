// OPFS (Origin Private File System) のヘルパー。オブジェクト SQL 検索が使う thief-query/
// ディレクトリの走査・削除と、起動時の残骸削除を担う。Worker (opfsWriter.worker.ts) からは
// 使わない (Worker 専用 API の createSyncAccessHandle を使う都合で Worker を自己完結させる)。
import { isObjectQuerySupported, objectQueryLockName, OBJECT_QUERY_OPFS_DIR } from './objectQuery';

// openObjectQueryDir は thief-query/ ディレクトリのハンドルを返す。create=false で存在しない
// 場合は NotFoundError を投げる。
export async function openObjectQueryDir(create: boolean): Promise<FileSystemDirectoryHandle> {
  const root = await navigator.storage.getDirectory();
  return root.getDirectoryHandle(OBJECT_QUERY_OPFS_DIR, { create });
}

// removeObjectQueryFile は thief-query/ 配下のファイルを削除する。既に無い場合は何もしない。
export async function removeObjectQueryFile(fileName: string): Promise<void> {
  try {
    const dir = await openObjectQueryDir(false);
    await dir.removeEntry(fileName);
  } catch (err) {
    if (err instanceof DOMException && err.name === 'NotFoundError') return;
    console.warn('failed to remove object query file', err);
  }
}

// cleanupStaleObjectQueryFiles は前回のセッションが残した thief-query/ 配下のファイルを削除する。
// main.tsx の描画前に呼ぶ。メインスレッドの機能検出 (OPFS / Web Locks) が偽なら何もしない
// (非対応ブラウザで navigator.storage.getDirectory を呼ぶと例外になる)。他タブが取り込み中または
// クエリ中のファイルは Web Locks のロックを保持しているため残し、ロックを取得できたファイル
// だけを残骸とみなす。走査と削除の失敗は警告に留め、描画を止めない。1 件の削除に失敗しても
// 残りの走査は続ける (ファイル単位で catch する)。
export async function cleanupStaleObjectQueryFiles(): Promise<void> {
  if (!isObjectQuerySupported()) return;
  try {
    const root = await navigator.storage.getDirectory();
    const dir = await root
      .getDirectoryHandle(OBJECT_QUERY_OPFS_DIR, { create: false })
      .catch(() => null);
    if (!dir) return;
    for await (const [name, handle] of dir.entries()) {
      if (handle.kind !== 'file') continue;
      try {
        await navigator.locks.request(
          objectQueryLockName(name),
          { ifAvailable: true },
          async (lock) => {
            if (lock === null) return;
            // ロックを保持したまま削除する。ロックを解放した後に削除すると、その隙間に
            // 同じ名前で書き込みを始める処理があれば消してしまう。
            await dir.removeEntry(name);
          },
        );
      } catch (err) {
        console.warn('failed to remove stale object query file', err);
      }
    }
  } catch (err) {
    console.warn('failed to clean up stale object query files', err);
  }
}

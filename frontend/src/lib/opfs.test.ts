// OPFS のヘルパー (起動時の残骸削除) のテスト。jsdom は OPFS を持たないため、
// navigator.storage.getDirectory と navigator.locks.request をスタブする。
import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanupStaleObjectQueryFiles, removeObjectQueryFile } from './opfs';

// stubOpfs は thief-query/ 配下に entries を持つ OPFS と、heldLocks のロックだけを
// 取得できない Web Locks をスタブする。削除されたファイル名を removed に記録する。
function stubOpfs(entries: string[], heldLocks: string[] = []) {
  const removed: string[] = [];
  const requested: string[] = [];
  const dir = {
    kind: 'directory' as const,
    entries: async function* () {
      for (const name of entries) {
        yield [name, { kind: 'file', name }] as [string, { kind: 'file'; name: string }];
      }
    },
    removeEntry: vi.fn(async (name: string) => {
      removed.push(name);
    }),
  };
  const root = {
    getDirectoryHandle: vi.fn(async () => dir),
  };
  Object.defineProperty(globalThis.navigator, 'storage', {
    value: { getDirectory: async () => root },
    configurable: true,
  });
  Object.defineProperty(globalThis.navigator, 'locks', {
    value: {
      request: async (
        name: string,
        _options: unknown,
        callback: (lock: unknown) => Promise<void>,
      ) => {
        requested.push(name);
        return callback(heldLocks.includes(name) ? null : { name });
      },
    },
    configurable: true,
  });
  return { removed, requested, dir, root };
}

describe('cleanupStaleObjectQueryFiles', () => {
  const originalStorage = Object.getOwnPropertyDescriptor(globalThis.navigator, 'storage');
  const originalLocks = Object.getOwnPropertyDescriptor(globalThis.navigator, 'locks');

  afterEach(() => {
    if (originalStorage) {
      Object.defineProperty(globalThis.navigator, 'storage', originalStorage);
    } else {
      Reflect.deleteProperty(globalThis.navigator, 'storage');
    }
    if (originalLocks) {
      Object.defineProperty(globalThis.navigator, 'locks', originalLocks);
    } else {
      Reflect.deleteProperty(globalThis.navigator, 'locks');
    }
  });

  it('ロックを取得できたファイルだけを削除し、他タブが保持しているファイルは残す', async () => {
    const { removed, requested } = stubOpfs(['a.csv', 'b.csv'], ['thief-query:b.csv']);

    await cleanupStaleObjectQueryFiles();

    expect(removed).toEqual(['a.csv']);
    expect(requested).toEqual(['thief-query:a.csv', 'thief-query:b.csv']);
  });

  it('thief-query/ が無い場合は何もしない', async () => {
    Object.defineProperty(globalThis.navigator, 'storage', {
      value: {
        getDirectory: async () => ({
          getDirectoryHandle: async () =>
            Promise.reject(new DOMException('missing', 'NotFoundError')),
        }),
      },
      configurable: true,
    });
    Object.defineProperty(globalThis.navigator, 'locks', {
      value: { request: vi.fn() },
      configurable: true,
    });

    await expect(cleanupStaleObjectQueryFiles()).resolves.toBeUndefined();
    expect(globalThis.navigator.locks.request).not.toHaveBeenCalled();
  });

  it('機能検出が偽の場合は何もせず例外も出さない', async () => {
    // navigator.storage / navigator.locks をスタブしない (jsdom の既定 = 非対応)
    await expect(cleanupStaleObjectQueryFiles()).resolves.toBeUndefined();
  });

  it('1 件の削除に失敗しても残りのファイルの走査と削除は続く', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const { removed, dir } = stubOpfs(['a.csv', 'b.csv', 'c.csv']);
    dir.removeEntry.mockImplementation(async (name: string) => {
      if (name === 'a.csv') throw new Error('remove failed');
      removed.push(name);
    });

    await cleanupStaleObjectQueryFiles();

    expect(removed).toEqual(['b.csv', 'c.csv']);
    expect(warn).toHaveBeenCalled();
    warn.mockRestore();
  });

  it('走査の失敗は警告に留める', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    Object.defineProperty(globalThis.navigator, 'storage', {
      value: {
        getDirectory: async () => ({
          getDirectoryHandle: async () => ({
            kind: 'directory',
            entries: () => {
              throw new Error('scan failed');
            },
          }),
        }),
      },
      configurable: true,
    });
    Object.defineProperty(globalThis.navigator, 'locks', {
      value: { request: vi.fn() },
      configurable: true,
    });

    await expect(cleanupStaleObjectQueryFiles()).resolves.toBeUndefined();
    expect(warn).toHaveBeenCalled();
    warn.mockRestore();
  });
});

describe('removeObjectQueryFile', () => {
  const originalStorage = Object.getOwnPropertyDescriptor(globalThis.navigator, 'storage');

  afterEach(() => {
    if (originalStorage) {
      Object.defineProperty(globalThis.navigator, 'storage', originalStorage);
    } else {
      Reflect.deleteProperty(globalThis.navigator, 'storage');
    }
  });

  it('ファイルを削除する', async () => {
    const { removed } = stubOpfs(['a.csv']);
    await removeObjectQueryFile('a.csv');
    expect(removed).toEqual(['a.csv']);
  });

  it('存在しないファイルの削除は警告を出さずに無視する', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    Object.defineProperty(globalThis.navigator, 'storage', {
      value: {
        getDirectory: async () => ({
          getDirectoryHandle: async () => ({
            removeEntry: async () => {
              throw new DOMException('missing', 'NotFoundError');
            },
          }),
        }),
      },
      configurable: true,
    });

    await expect(removeObjectQueryFile('missing.csv')).resolves.toBeUndefined();
    expect(warn).not.toHaveBeenCalled();
    warn.mockRestore();
  });
});

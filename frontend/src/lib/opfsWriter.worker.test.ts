// OPFS 取り込み Worker 本体のテスト。jsdom は Worker のグローバルと OPFS と Web Locks を
// 持たないため、navigator.storage / navigator.locks / FileSystemFileHandle / fetch /
// postMessage / close をスタブして、モジュールの onmessage へ直接メッセージを流す。
// 上限判定とエラー判定そのものは opfsWriterLimits.test.ts が担い、ここでは分岐の経路
// (ロックの取得順、中断、後始末) を固定する。
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { OpfsWriterEvent, OpfsWriterRequest } from './opfsIngest';

const DIR_NAME = 'thief-query';
const FILE_NAME = 'obj.csv';
const INGEST: OpfsWriterRequest = {
  type: 'ingest',
  url: 'http://127.0.0.1:8089/api/aws/profiles/p/s3/b/objects/download?key=data.csv',
  dirName: DIR_NAME,
  fileName: FILE_NAME,
  lockName: 'thief-query:obj.csv',
  maxBytes: 1024,
};

const originalStorage = Object.getOwnPropertyDescriptor(globalThis.navigator, 'storage');
const originalLocks = Object.getOwnPropertyDescriptor(globalThis.navigator, 'locks');

function flush(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

// fakeResponse は fetch が返す Response の必要な部分だけを持つ。chunks は本文のバイト列。
function fakeResponse(options: {
  ok?: boolean;
  status?: number;
  contentLength?: string | null;
  chunks?: Uint8Array[];
  body?: null;
  json?: () => Promise<unknown>;
  // reader を渡すと本文の読み取りを差し替えられる (中断の再現に使う)。
  reader?: { read: () => Promise<{ done: boolean; value?: Uint8Array }> };
}) {
  const chunks = options.chunks ?? [];
  let index = 0;
  return {
    ok: options.ok ?? true,
    status: options.status ?? 200,
    statusText: 'stub',
    headers: {
      get: (name: string) => (name === 'Content-Length' ? (options.contentLength ?? null) : null),
    },
    json: options.json ?? (async () => ({})),
    body:
      options.body === null
        ? null
        : {
            getReader: () =>
              options.reader ?? {
                read: async () =>
                  index < chunks.length
                    ? { done: false, value: chunks[index++] }
                    : { done: true, value: undefined },
              },
          },
  };
}

interface SetupOptions {
  // supported が false なら createSyncAccessHandle を持たないブラウザを模す。
  supported?: boolean;
  // onCreateAccess は createSyncAccessHandle の待機中に割り込む処理。
  onCreateAccess?: () => Promise<void> | void;
  fetchImpl?: (url: string, init: { signal: AbortSignal }) => Promise<unknown>;
  // writeImpl を渡すと SyncAccessHandle の write を差し替えられる (書き込みの失敗を模す)。
  writeImpl?: (buffer: BufferSource) => number;
}

function setup(options: SetupOptions = {}) {
  const calls: string[] = [];
  const posted: OpfsWriterEvent[] = [];
  const writes: number[] = [];

  const access = {
    write: (buffer: BufferSource) => {
      calls.push('write');
      writes.push(buffer.byteLength);
      if (options.writeImpl) return options.writeImpl(buffer);
      return buffer.byteLength;
    },
    flush: vi.fn(),
    close: vi.fn(),
  };
  const fileHandle = {
    createSyncAccessHandle: async () => {
      calls.push('createSyncAccessHandle');
      await options.onCreateAccess?.();
      return access;
    },
  };
  const removeEntry = vi.fn(async () => {
    calls.push('removeEntry');
  });
  const dir = {
    getFileHandle: async () => {
      calls.push('getFileHandle');
      return fileHandle;
    },
    removeEntry,
  };
  Object.defineProperty(globalThis.navigator, 'storage', {
    value: { getDirectory: async () => ({ getDirectoryHandle: async () => dir }) },
    configurable: true,
  });
  Object.defineProperty(globalThis.navigator, 'locks', {
    value: {
      request: async (_name: string, callback: () => Promise<void>) => {
        calls.push('lock');
        await callback();
      },
    },
    configurable: true,
  });

  class FakeFileSystemFileHandle {}
  if (options.supported !== false) {
    (FakeFileSystemFileHandle.prototype as Record<string, unknown>).createSyncAccessHandle =
      () => {};
  }
  vi.stubGlobal('FileSystemFileHandle', FakeFileSystemFileHandle);
  vi.stubGlobal(
    'fetch',
    vi.fn(options.fetchImpl ?? (async () => fakeResponse({ chunks: [new Uint8Array(8)] }))),
  );
  vi.stubGlobal('postMessage', (event: OpfsWriterEvent) => {
    posted.push(event);
  });
  vi.stubGlobal('close', vi.fn());

  return { calls, posted, writes, removeEntry, access };
}

// send は Worker モジュールを読み込み直してメッセージを流す。モジュールスコープの状態
// (terminated / abortController) をテスト間で持ち越さないため、毎回読み込み直す。
async function loadWorker(): Promise<(request: OpfsWriterRequest) => void> {
  vi.resetModules();
  await import('./opfsWriter.worker');
  const handler = self.onmessage as ((event: MessageEvent<OpfsWriterRequest>) => void) | null;
  if (!handler) throw new Error('worker did not register onmessage');
  return (request: OpfsWriterRequest) => {
    handler({ data: request } as MessageEvent<OpfsWriterRequest>);
  };
}

afterEach(() => {
  vi.unstubAllGlobals();
  self.onmessage = null;
  if (originalStorage) Object.defineProperty(globalThis.navigator, 'storage', originalStorage);
  if (originalLocks) Object.defineProperty(globalThis.navigator, 'locks', originalLocks);
});

describe('opfsWriter.worker', () => {
  it('ロックを取ってから書き込み、終了指示でファイルを削除して閉じる', async () => {
    const ctx = setup();
    const send = await loadWorker();

    send(INGEST);
    await flush();

    // ロックは OPFS のファイルを作る前に取る (取ること自体も固定する)。
    expect(ctx.calls[0]).toBe('lock');
    expect(ctx.calls.indexOf('lock')).toBeLessThan(ctx.calls.indexOf('getFileHandle'));
    expect(ctx.writes).toEqual([8]);
    expect(ctx.posted).toContainEqual({ type: 'done', written: 8 });
    // 終了指示が来るまでロックとファイルを保持する。
    expect(ctx.removeEntry).not.toHaveBeenCalled();
    expect(ctx.posted.some((e) => e.type === 'closed')).toBe(false);

    send({ type: 'terminate' });
    await flush();

    expect(ctx.removeEntry).toHaveBeenCalledWith(FILE_NAME);
    expect(ctx.posted).toContainEqual({ type: 'closed' });
    expect(self.close).toHaveBeenCalledTimes(1);
  });

  it('createSyncAccessHandle の待機中に終了指示を受けたら fetch を始めずに中断する', async () => {
    let send: (request: OpfsWriterRequest) => void = () => {};
    const ctx = setup({
      onCreateAccess: () => {
        send({ type: 'terminate' });
      },
    });
    send = await loadWorker();

    send(INGEST);
    await flush();

    expect(fetch).not.toHaveBeenCalled();
    expect(ctx.posted).toContainEqual({ type: 'aborted' });
    expect(ctx.removeEntry).toHaveBeenCalledWith(FILE_NAME);
    expect(ctx.posted).toContainEqual({ type: 'closed' });
  });

  it('HTTP エラーを status と code 付きで返し、ファイルを削除する', async () => {
    const ctx = setup({
      fetchImpl: async () =>
        fakeResponse({
          ok: false,
          status: 401,
          json: async () => ({ error: 'token expired', code: 'SSO_TOKEN_EXPIRED' }),
        }),
    });
    const send = await loadWorker();

    send(INGEST);
    await flush();

    expect(ctx.posted).toContainEqual({
      type: 'error',
      kind: 'http',
      status: 401,
      code: 'SSO_TOKEN_EXPIRED',
      message: 'token expired',
    });
    expect(ctx.removeEntry).toHaveBeenCalledWith(FILE_NAME);
    expect(ctx.posted).toContainEqual({ type: 'closed' });
  });

  it('Content-Length が上限を超える場合は本文を読まずに tooLarge を返す', async () => {
    const ctx = setup({
      fetchImpl: async () =>
        fakeResponse({ contentLength: '2048', chunks: [new Uint8Array(2048)] }),
    });
    const send = await loadWorker();

    send(INGEST);
    await flush();

    expect(ctx.writes).toEqual([]);
    expect(ctx.posted.find((e) => e.type === 'error')).toMatchObject({ kind: 'tooLarge' });
    expect(ctx.removeEntry).toHaveBeenCalledWith(FILE_NAME);
  });

  it('累積が上限を超えたら tooLarge を返し、ファイルを削除する', async () => {
    const ctx = setup({
      fetchImpl: async () => fakeResponse({ chunks: [new Uint8Array(768), new Uint8Array(768)] }),
    });
    const send = await loadWorker();

    send(INGEST);
    await flush();

    // 上限を超えたバッチまでは書いてから中断する (残骸は削除する)。
    expect(ctx.writes).toEqual([768, 768]);
    expect(ctx.posted.find((e) => e.type === 'error')).toMatchObject({ kind: 'tooLarge' });
    expect(ctx.removeEntry).toHaveBeenCalledWith(FILE_NAME);
    expect(ctx.posted).toContainEqual({ type: 'closed' });
  });

  it('書き込み中の QuotaExceededError を quota として返し、ファイルを削除する', async () => {
    const ctx = setup({
      writeImpl: () => {
        throw new DOMException('no space', 'QuotaExceededError');
      },
    });
    const send = await loadWorker();

    send(INGEST);
    await flush();

    expect(ctx.posted.find((e) => e.type === 'error')).toMatchObject({ kind: 'quota' });
    expect(ctx.removeEntry).toHaveBeenCalledWith(FILE_NAME);
    expect(ctx.posted).toContainEqual({ type: 'closed' });
  });

  it('取り込み中の終了指示で fetch を中断し、ファイルを削除して閉じる', async () => {
    const ctx = setup({
      // 本文の読み取りを保留し、AbortSignal で AbortError にする (実際の fetch と同じ振る舞い)。
      fetchImpl: async (_url, init) =>
        fakeResponse({
          reader: {
            read: () =>
              new Promise((_resolve, reject) => {
                init.signal.addEventListener('abort', () => {
                  reject(new DOMException('aborted', 'AbortError'));
                });
              }),
          },
        }),
    });
    const send = await loadWorker();

    send(INGEST);
    await flush();
    expect(fetch).toHaveBeenCalledTimes(1);

    send({ type: 'terminate' });
    await flush();

    expect(ctx.posted).toContainEqual({ type: 'aborted' });
    expect(ctx.removeEntry).toHaveBeenCalledWith(FILE_NAME);
    expect(ctx.posted).toContainEqual({ type: 'closed' });
  });

  it('createSyncAccessHandle の無いブラウザでは unsupported を返して閉じる', async () => {
    const ctx = setup({ supported: false });
    const send = await loadWorker();

    send(INGEST);
    await flush();

    expect(ctx.posted[0]).toMatchObject({ type: 'error', kind: 'unsupported' });
    expect(ctx.posted).toContainEqual({ type: 'closed' });
    expect(ctx.calls).not.toContain('lock');
    expect(self.close).toHaveBeenCalledTimes(1);
  });
});

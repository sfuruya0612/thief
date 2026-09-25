// OPFS 取り込み (メインスレッド側) のテスト。jsdom は Web Worker と OPFS を持たないため、
// Worker と navigator.storage をスタブし、Worker へ送るメッセージと受け取るイベントの
// 扱いだけを検証する。Worker の中身は opfsWriter.worker.test.ts が、上限とエラーの判定は
// opfsWriterLimits.test.ts が担う。
import { afterEach, describe, expect, it, vi } from 'vitest';
import { createOpfsIngestor } from './opfsIngest';
import type { OpfsWriterEvent, OpfsWriterRequest } from './opfsIngest';
import { ObjectQueryIngestError } from './objectQuery';
import { ApiError } from '../types/common';

// FakeWorker は postMessage されたメッセージを記録し、テストが任意のイベントを流し込める
// Worker のスタブ。生成されたインスタンスは instances に積む。
class FakeWorker {
  static instances: FakeWorker[] = [];
  onmessage: ((event: MessageEvent<OpfsWriterEvent>) => void) | null = null;
  onerror: ((event: ErrorEvent) => void) | null = null;
  readonly posted: OpfsWriterRequest[] = [];
  terminated = false;

  constructor() {
    FakeWorker.instances.push(this);
  }

  postMessage(message: OpfsWriterRequest): void {
    this.posted.push(message);
  }

  terminate(): void {
    this.terminated = true;
  }

  // emit は Worker からメインスレッドへのイベントを流す。
  emit(event: OpfsWriterEvent): void {
    this.onmessage?.({ data: event } as MessageEvent<OpfsWriterEvent>);
  }
}

const originalStorage = Object.getOwnPropertyDescriptor(globalThis.navigator, 'storage');

// stubStorage は navigator.storage.estimate をスタブする。estimate を省くと成功扱いにする。
function stubStorage(estimate?: () => Promise<StorageEstimate>) {
  Object.defineProperty(globalThis.navigator, 'storage', {
    value: {
      estimate: estimate ?? (async () => ({ quota: 1 << 30, usage: 0 })),
      getDirectory: async () => ({
        getDirectoryHandle: async () => ({ removeEntry: async () => {} }),
      }),
    },
    configurable: true,
  });
}

const REQUEST = {
  url: 'http://127.0.0.1:8089/api/aws/profiles/p/s3/b/objects/download?key=data.csv',
  storageName: 'thief-query/abc.csv',
  lockName: 'thief-query:abc.csv',
  maxBytes: 1 << 30,
  size: 4096,
};

// flush はマイクロタスクを 1 巡させ、await の連鎖を進める。
function flush(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

describe('OpfsIngestor', () => {
  afterEach(() => {
    FakeWorker.instances = [];
    vi.unstubAllGlobals();
    if (originalStorage) {
      Object.defineProperty(globalThis.navigator, 'storage', originalStorage);
    } else {
      Reflect.deleteProperty(globalThis.navigator, 'storage');
    }
  });

  it('Worker に取り込み依頼を送り、進捗を中継して完了で resolve する', async () => {
    vi.stubGlobal('Worker', FakeWorker);
    stubStorage();
    const ingestor = createOpfsIngestor();
    const onProgress = vi.fn();

    const done = ingestor.ingest(REQUEST, onProgress);
    await flush();

    const worker = FakeWorker.instances[0];
    expect(worker.posted[0]).toEqual({
      type: 'ingest',
      url: REQUEST.url,
      dirName: 'thief-query',
      fileName: 'abc.csv',
      lockName: REQUEST.lockName,
      maxBytes: REQUEST.maxBytes,
    });

    worker.emit({ type: 'progress', written: 1024, total: 4096 });
    expect(onProgress).toHaveBeenCalledWith({ type: 'progress', written: 1024, total: 4096 });

    worker.emit({ type: 'done', written: 4096 });
    await expect(done).resolves.toBeUndefined();
  });

  it('空き容量の確認中に terminate されると Worker を起動せず中断する', async () => {
    vi.stubGlobal('Worker', FakeWorker);
    let releaseEstimate: () => void = () => {};
    stubStorage(
      () =>
        new Promise<StorageEstimate>((resolve) => {
          releaseEstimate = () => resolve({ quota: 1 << 30, usage: 0 });
        }),
    );
    const ingestor = createOpfsIngestor();

    const done = ingestor.ingest(REQUEST, vi.fn());
    await flush();
    // この時点ではまだ Worker を作っていない (空き容量の確認待ち)
    expect(FakeWorker.instances).toHaveLength(0);

    await ingestor.terminate();
    releaseEstimate();

    await expect(done).rejects.toMatchObject({ name: 'AbortError' });
    // 終了指示の後に空き容量の確認が終わっても Worker は起動しない
    // (起動すると誰も終了指示を送らないまま OPFS のファイルとロックを保持し続ける)
    expect(FakeWorker.instances).toHaveLength(0);
  });

  it('terminate 済みのインジェスタに ingest を依頼しても Worker を起動しない', async () => {
    vi.stubGlobal('Worker', FakeWorker);
    stubStorage();
    const ingestor = createOpfsIngestor();

    await ingestor.terminate();

    await expect(ingestor.ingest(REQUEST, vi.fn())).rejects.toMatchObject({ name: 'AbortError' });
    expect(FakeWorker.instances).toHaveLength(0);
  });

  it('空き容量が足りない場合は Worker を起動せず quota のエラーで reject する', async () => {
    vi.stubGlobal('Worker', FakeWorker);
    stubStorage(async () => ({ quota: 4096, usage: 2048 }));
    const ingestor = createOpfsIngestor();

    const err = await ingestor.ingest(REQUEST, vi.fn()).catch((e: unknown) => e);

    expect(err).toBeInstanceOf(ObjectQueryIngestError);
    expect((err as ObjectQueryIngestError).kind).toBe('quota');
    expect(FakeWorker.instances).toHaveLength(0);
  });

  it('空き容量を取得できない場合は取り込みを続ける', async () => {
    vi.stubGlobal('Worker', FakeWorker);
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    stubStorage(() => Promise.reject(new Error('estimate failed')));
    const ingestor = createOpfsIngestor();

    void ingestor.ingest(REQUEST, vi.fn());
    await flush();

    expect(FakeWorker.instances).toHaveLength(1);
    expect(warn).toHaveBeenCalled();
    warn.mockRestore();
  });

  it('estimate に quota / usage が無い場合は取り込みを続ける', async () => {
    vi.stubGlobal('Worker', FakeWorker);
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    stubStorage(async () => ({}));
    const ingestor = createOpfsIngestor();

    void ingestor.ingest(REQUEST, vi.fn());
    await flush();

    expect(FakeWorker.instances).toHaveLength(1);
    expect(warn).toHaveBeenCalled();
    warn.mockRestore();
  });

  it('storageName にディレクトリが無い場合は opfs のエラーで reject する', async () => {
    vi.stubGlobal('Worker', FakeWorker);
    stubStorage();
    const ingestor = createOpfsIngestor();

    const err = await ingestor
      .ingest({ ...REQUEST, storageName: 'abc.csv' }, vi.fn())
      .catch((e: unknown) => e);

    expect(err).toBeInstanceOf(ObjectQueryIngestError);
    expect((err as ObjectQueryIngestError).kind).toBe('opfs');
    expect(FakeWorker.instances).toHaveLength(0);
  });

  it('Worker の http エラーは ApiError に組み直す', async () => {
    vi.stubGlobal('Worker', FakeWorker);
    stubStorage();
    const ingestor = createOpfsIngestor();
    const done = ingestor.ingest(REQUEST, vi.fn());
    await flush();

    FakeWorker.instances[0].emit({
      type: 'error',
      kind: 'http',
      status: 401,
      code: 'SSO_TOKEN_EXPIRED',
      message: 'sso token expired',
    });

    const err = await done.catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).statusCode).toBe(401);
    expect((err as ApiError).code).toBe('SSO_TOKEN_EXPIRED');
  });

  it('Worker の network エラーは ApiError(0, network_error) に正規化する', async () => {
    vi.stubGlobal('Worker', FakeWorker);
    stubStorage();
    const ingestor = createOpfsIngestor();
    const done = ingestor.ingest(REQUEST, vi.fn());
    await flush();

    FakeWorker.instances[0].emit({ type: 'error', kind: 'network', message: 'failed to fetch' });

    const err = await done.catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).statusCode).toBe(0);
    expect((err as ApiError).code).toBe('network_error');
  });

  it.each(['unsupported', 'tooLarge', 'quota', 'opfs'] as const)(
    'Worker の %s エラーは ObjectQueryIngestError に組み直す',
    async (kind) => {
      vi.stubGlobal('Worker', FakeWorker);
      stubStorage();
      const ingestor = createOpfsIngestor();
      const done = ingestor.ingest(REQUEST, vi.fn());
      await flush();

      FakeWorker.instances[0].emit({ type: 'error', kind, message: `${kind} error` });

      const err = await done.catch((e: unknown) => e);
      expect(err).toBeInstanceOf(ObjectQueryIngestError);
      expect((err as ObjectQueryIngestError).kind).toBe(kind);
    },
  );

  it('terminate は Worker に終了指示を送り、closed を受けてから Worker を止める', async () => {
    vi.stubGlobal('Worker', FakeWorker);
    stubStorage();
    const ingestor = createOpfsIngestor();
    const done = ingestor.ingest(REQUEST, vi.fn());
    await flush();

    const worker = FakeWorker.instances[0];
    worker.emit({ type: 'done', written: 4096 });
    await done;

    const terminating = ingestor.terminate();
    expect(worker.posted[1]).toEqual({ type: 'terminate' });
    worker.emit({ type: 'closed' });
    await terminating;

    expect(worker.terminated).toBe(true);
  });

  it('Worker の onerror は opfs のエラーとして reject する', async () => {
    vi.stubGlobal('Worker', FakeWorker);
    stubStorage();
    const ingestor = createOpfsIngestor();
    const done = ingestor.ingest(REQUEST, vi.fn());
    await flush();

    FakeWorker.instances[0].onerror?.({ message: 'worker crashed' } as ErrorEvent);

    const err = await done.catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ObjectQueryIngestError);
    expect((err as ObjectQueryIngestError).kind).toBe('opfs');
  });
});

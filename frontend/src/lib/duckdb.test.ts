import { beforeEach, describe, expect, it, vi } from 'vitest';
import { tableFromArrays } from 'apache-arrow';

// @duckdb/duckdb-wasm を差し替えて、duckdb.ts の配線 (打ち切り時に接続の cancelSent を
// 呼ぶこと、初期化に失敗したら次の呼び出しでやり直すこと) を検証する。行の収集そのものは
// duckdbResult.test.ts が受け持つため、ここでは呼び出しの有無だけを見る。
const stub = vi.hoisted(() => {
  const connection = {
    query: vi.fn(async () => undefined),
    close: vi.fn(async () => undefined),
    send: vi.fn(),
    cancelSent: vi.fn(async () => true),
  };
  const db = {
    instantiate: vi.fn(async () => undefined),
    connect: vi.fn(async () => connection),
    registerOPFSFileName: vi.fn(async () => undefined),
    terminate: vi.fn(async () => undefined),
    dropFile: vi.fn(async () => undefined),
  };
  return { connection, db };
});

vi.mock('@duckdb/duckdb-wasm', () => ({
  AsyncDuckDB: class {
    constructor() {
      return stub.db;
    }
  },
  ConsoleLogger: class {},
  LogLevel: { WARNING: 2 },
}));

// makeReader は conn.send が返すバッチ列を模す。各バッチは行数だけを指定する。Arrow の
// リーダーと同じく、open() を呼ぶまで schema は undefined にする (docs/issues/closed/0206)。
function makeReader(batchSizes: number[]) {
  const schema = tableFromArrays({ n: [0] }).schema;
  const batches = batchSizes.map(
    (size) => tableFromArrays({ n: Array.from({ length: size }, (_, i) => i) }).batches[0],
  );
  let opened = false;
  return {
    async open() {
      opened = true;
    },
    get schema() {
      return opened ? schema : undefined;
    },
    [Symbol.asyncIterator]() {
      let index = 0;
      return {
        async next() {
          if (index >= batches.length) return { done: true as const, value: undefined };
          const value = batches[index];
          index += 1;
          return { done: false as const, value };
        },
      };
    },
  };
}

async function importEngine() {
  vi.resetModules();
  const mod = await import('./duckdb');
  return mod.objectQueryEngine;
}

describe('objectQueryEngine.run', () => {
  beforeEach(() => {
    stub.connection.query.mockClear();
    stub.connection.close.mockClear();
    stub.connection.send.mockReset();
    stub.connection.cancelSent.mockClear();
    stub.db.instantiate.mockReset();
    stub.db.instantiate.mockResolvedValue(undefined);
    stub.db.connect.mockReset();
    stub.db.connect.mockResolvedValue(stub.connection);
    stub.db.terminate.mockClear();
  });

  it('上限を超えたら打ち切り、接続の cancelSent を 1 回呼ぶ', async () => {
    stub.connection.send.mockResolvedValue(makeReader([10001]));
    const engine = await importEngine();

    const result = await engine.run('SELECT * FROM obj');

    expect(result.truncated).toBe(true);
    expect(result.rows).toHaveLength(10000);
    expect(stub.connection.cancelSent).toHaveBeenCalledTimes(1);
  });

  it('上限以下なら打ち切らず、cancelSent も呼ばない', async () => {
    stub.connection.send.mockResolvedValue(makeReader([3]));
    const engine = await importEngine();

    const result = await engine.run('SELECT * FROM obj');

    expect(result.truncated).toBe(false);
    expect(result.rows).toHaveLength(3);
    expect(stub.connection.cancelSent).not.toHaveBeenCalled();
  });

  it('別のオブジェクトのビューに置き換わっていたら dropView は何もしない', async () => {
    stub.connection.send.mockResolvedValue(makeReader([1]));
    const engine = await importEngine();

    await engine.registerView('opfs://thief-query/a.csv', "read_csv('a')", []);
    await engine.registerView('opfs://thief-query/b.csv', "read_csv('b')", []);
    stub.connection.query.mockClear();

    // 先に閉じたパネルの解放。ビューはもう b のものなので消さない。
    await engine.dropView('opfs://thief-query/a.csv');
    expect(stub.connection.query).not.toHaveBeenCalled();

    // b のパネルの解放では消す。
    await engine.dropView('opfs://thief-query/b.csv');
    expect(stub.connection.query).toHaveBeenCalledWith('DROP VIEW IF EXISTS obj');
  });

  it('別のオブジェクトの登録が始まっていれば、CREATE の完了前でも dropView は何もしない', async () => {
    stub.connection.send.mockResolvedValue(makeReader([1]));
    const engine = await importEngine();

    await engine.registerView('opfs://thief-query/a.csv', "read_csv('a')", []);
    stub.connection.query.mockClear();
    // b の CREATE の応答を保留したまま、a の解放を走らせる。
    let resolveCreate: () => void = () => {};
    stub.connection.query.mockImplementationOnce(
      () => new Promise<undefined>((resolve) => (resolveCreate = () => resolve(undefined))),
    );
    const registering = engine.registerView('opfs://thief-query/b.csv', "read_csv('b')", []);

    await engine.dropView('opfs://thief-query/a.csv');

    expect(stub.connection.query).not.toHaveBeenCalledWith('DROP VIEW IF EXISTS obj');
    // 保留していた CREATE を解決して登録を終わらせる。
    await new Promise((resolve) => setTimeout(resolve, 0));
    resolveCreate();
    await registering;
  });

  it('registerView は既存のビューを置き換える', async () => {
    stub.connection.send.mockResolvedValue(makeReader([1]));
    const engine = await importEngine();

    await engine.registerView('opfs://thief-query/a.csv', "read_csv('a')", []);

    expect(stub.connection.query).toHaveBeenCalledWith(
      "CREATE OR REPLACE VIEW obj AS SELECT * FROM read_csv('a')",
    );
  });

  it('接続に失敗しても、次の呼び出しでやり直せる', async () => {
    // 1 回目は初期化の SET 用、2 回目が共有接続。共有接続の取得を失敗させる。
    stub.db.connect
      .mockResolvedValueOnce(stub.connection)
      .mockRejectedValueOnce(new Error('connect failed'));
    const engine = await importEngine();

    await expect(
      engine.registerView('opfs://thief-query/a.csv', "read_csv('a')", []),
    ).rejects.toThrow('connect failed');
    await engine.cancelSent();

    expect(stub.connection.cancelSent).toHaveBeenCalledTimes(1);
    expect(stub.db.connect).toHaveBeenCalledTimes(3);
  });

  it('初期化に失敗しても、次の呼び出しでやり直せる', async () => {
    stub.db.instantiate.mockRejectedValueOnce(new Error('wasm load failed'));
    stub.connection.send.mockResolvedValue(makeReader([1]));
    const engine = await importEngine();

    await expect(engine.run('SELECT 1')).rejects.toThrow('wasm load failed');

    const result = await engine.run('SELECT 1');
    expect(result.rows).toHaveLength(1);
    expect(stub.db.instantiate).toHaveBeenCalledTimes(2);
    // 失敗した 1 回目の AsyncDuckDB は Worker ごと終了させる。
    expect(stub.db.terminate).toHaveBeenCalledTimes(1);
  });

  it('後始末の terminate が失敗しても、初期化のエラーをそのまま伝える', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    stub.db.instantiate.mockRejectedValueOnce(new Error('wasm load failed'));
    stub.db.terminate.mockRejectedValueOnce(new Error('terminate failed'));
    const engine = await importEngine();

    await expect(
      engine.registerView('opfs://thief-query/a.csv', "read_csv('a')", []),
    ).rejects.toThrow('wasm load failed');

    expect(warn).toHaveBeenCalled();
    warn.mockRestore();
  });
});

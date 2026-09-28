import { describe, expect, it, vi } from 'vitest';
import { RecordBatchReader, tableFromArrays, tableToIPC } from 'apache-arrow';
import { batchToRows, cellToString, collectQueryResult, columnsOf } from './duckdbResult';

// makeBatch は 1 バッチだけの Arrow テーブルから RecordBatch を取り出す。
function makeBatch(values: Record<string, unknown[]>) {
  const table = tableFromArrays(values);
  return table.batches[0];
}

describe('columnsOf', () => {
  it('スキーマのフィールド名を定義順に返す', () => {
    const table = tableFromArrays({ name: ['a'], count: [1] });
    expect(columnsOf(table.schema)).toEqual(['name', 'count']);
  });
});

describe('cellToString', () => {
  it('null / undefined は空文字列', () => {
    expect(cellToString(null)).toBe('');
    expect(cellToString(undefined)).toBe('');
  });

  it('boolean は true / false', () => {
    expect(cellToString(true)).toBe('true');
    expect(cellToString(false)).toBe('false');
  });

  it('整数と BigInt は 10 進の文字列', () => {
    expect(cellToString(42)).toBe('42');
    expect(cellToString(-7)).toBe('-7');
    expect(cellToString(10n ** 12n)).toBe('1000000000000');
  });

  it('小数は 10 進の文字列', () => {
    expect(cellToString(1.5)).toBe('1.5');
  });

  it('Date は ISO 8601 (UTC) の文字列', () => {
    expect(cellToString(new Date('2026-01-02T03:04:05.000Z'))).toBe('2026-01-02T03:04:05.000Z');
  });

  it('文字列はそのまま', () => {
    expect(cellToString('hello')).toBe('hello');
  });
});

describe('batchToRows', () => {
  it('列の順序を保ったまま行へ変換する', () => {
    const batch = makeBatch({ name: ['a', 'b'], size: [1, 2] });
    expect(batchToRows(batch)).toEqual([
      ['a', '1'],
      ['b', '2'],
    ]);
  });

  it('null は空文字列にする', () => {
    const batch = makeBatch({ name: ['a', 'b'], size: [1, null] });
    expect(batchToRows(batch)).toEqual([
      ['a', '1'],
      ['b', ''],
    ]);
  });

  it('Timestamp は ISO 8601 の文字列にする', () => {
    const batch = makeBatch({
      at: [new Date('2026-01-02T03:04:05.000Z')],
    });
    expect(batchToRows(batch)).toEqual([['2026-01-02T03:04:05.000Z']]);
  });

  it('行が無いバッチは空配列にする', () => {
    const batch = makeBatch({ name: [] as string[] });
    expect(batchToRows(batch)).toEqual([]);
  });
});

// makeReader は collectQueryResult に渡すストリームのスタブを返す。batches はバッチごとの
// 行数の配列で、pulled には iterator.next() が呼ばれた回数、calls には open() と next() が
// 呼ばれた順序が入る (先読みと open() の順序の検証に使う)。Arrow の RecordBatchReader と
// 同じく、open() を呼ぶまで schema は undefined にする (docs/issues/closed/0206)。
function makeReader(batchSizes: number[]) {
  const table = tableFromArrays({ n: [0] });
  const batches = batchSizes.map(
    (size) => tableFromArrays({ n: Array.from({ length: size }, (_, i) => i) }).batches[0],
  );
  const state = { pulled: 0, calls: [] as ('open' | 'next')[] };
  const reader = {
    async open() {
      state.calls.push('open');
    },
    get schema() {
      return state.calls.includes('open') ? table.schema : undefined;
    },
    [Symbol.asyncIterator]() {
      let index = 0;
      return {
        async next() {
          state.pulled += 1;
          state.calls.push('next');
          if (index >= batches.length) return { done: true as const, value: undefined };
          const value = batches[index];
          index += 1;
          return { done: false as const, value };
        },
      };
    },
  };
  return { reader, state };
}

describe('collectQueryResult', () => {
  it('上限に達しなければ全行を返し、打ち切らず cancel も呼ばない', async () => {
    const cancel = vi.fn(async () => {});
    const { reader } = makeReader([2, 3]);
    const result = await collectQueryResult(reader, 10, cancel);
    expect(result.columns).toEqual(['n']);
    expect(result.rows).toHaveLength(5);
    expect(result.truncated).toBe(false);
    expect(cancel).not.toHaveBeenCalled();
  });

  it('バッチの途中で上限に達したら残り行を捨てて打ち切り、cancel を呼ぶ', async () => {
    const cancel = vi.fn(async () => {});
    const { reader, state } = makeReader([10]);
    const result = await collectQueryResult(reader, 4, cancel);
    expect(result.rows).toHaveLength(4);
    expect(result.rows.map((r) => r[0])).toEqual(['0', '1', '2', '3']);
    expect(result.truncated).toBe(true);
    expect(cancel).toHaveBeenCalledTimes(1);
    // このバッチだけで打ち切りが確定するため、次のバッチは先読みしない
    expect(state.pulled).toBe(1);
  });

  it('上限ちょうどで次のバッチがあれば打ち切り、cancel を呼ぶ', async () => {
    const cancel = vi.fn(async () => {});
    const { reader, state } = makeReader([4, 1]);
    const result = await collectQueryResult(reader, 4, cancel);
    expect(result.rows).toHaveLength(4);
    expect(result.truncated).toBe(true);
    expect(cancel).toHaveBeenCalledTimes(1);
    // 上限ちょうどのときだけ次のバッチを 1 つ先読みする
    expect(state.pulled).toBe(2);
  });

  it('上限ちょうどで次のバッチが無ければ打ち切らず、cancel も呼ばない', async () => {
    const cancel = vi.fn(async () => {});
    const { reader, state } = makeReader([4]);
    const result = await collectQueryResult(reader, 4, cancel);
    expect(result.rows).toHaveLength(4);
    expect(result.truncated).toBe(false);
    expect(cancel).not.toHaveBeenCalled();
    expect(state.pulled).toBe(2);
  });

  it('上限ちょうどで末尾に 0 行のバッチが続いても打ち切らない', async () => {
    const cancel = vi.fn(async () => {});
    const { reader, state } = makeReader([4, 0]);
    const result = await collectQueryResult(reader, 4, cancel);
    expect(result.rows).toHaveLength(4);
    expect(result.truncated).toBe(false);
    expect(cancel).not.toHaveBeenCalled();
    // 0 行のバッチを読み飛ばし、末尾に達するまで先読みする
    expect(state.pulled).toBe(3);
  });

  it('上限ちょうどで 0 行のバッチの後に行があれば打ち切る', async () => {
    const cancel = vi.fn(async () => {});
    const { reader, state } = makeReader([4, 0, 1]);
    const result = await collectQueryResult(reader, 4, cancel);
    expect(result.rows).toHaveLength(4);
    expect(result.truncated).toBe(true);
    expect(cancel).toHaveBeenCalledTimes(1);
    expect(state.pulled).toBe(3);
  });

  it('0 行のバッチが挟まっても行数と打ち切りの判定は変わらない', async () => {
    const cancel = vi.fn(async () => {});
    const { reader } = makeReader([0, 2, 0]);
    const result = await collectQueryResult(reader, 10, cancel);
    expect(result.rows).toHaveLength(2);
    expect(result.truncated).toBe(false);
    expect(cancel).not.toHaveBeenCalled();
  });

  it('1 バッチも無い場合は空の結果を返す', async () => {
    const cancel = vi.fn(async () => {});
    const { reader } = makeReader([]);
    const result = await collectQueryResult(reader, 10, cancel);
    expect(result.columns).toEqual(['n']);
    expect(result.rows).toEqual([]);
    expect(result.truncated).toBe(false);
    expect(cancel).not.toHaveBeenCalled();
  });

  it('open() を呼ぶまで schema を持たないリーダーでも、最初の next() より前に open() して列名を読む', async () => {
    const cancel = vi.fn(async () => {});
    const { reader, state } = makeReader([1]);
    // send() が返した直後のリーダーと同じく、open() の前は schema が無い
    expect(reader.schema).toBeUndefined();
    const result = await collectQueryResult(reader, 10, cancel);
    expect(state.calls[0]).toBe('open');
    expect(state.calls.filter((c) => c === 'open')).toHaveLength(1);
    expect(result.columns).toEqual(['n']);
    expect(result.rows).toEqual([['0']]);
  });

  it('open() の後も schema が無いリーダーでは列も行も無い結果を返す', async () => {
    const cancel = vi.fn(async () => {});
    const reader = {
      async open() {},
      schema: undefined,
      [Symbol.asyncIterator]() {
        return {
          async next() {
            return { done: true as const, value: undefined };
          },
        };
      },
    };
    const result = await collectQueryResult(reader, 10, cancel);
    expect(result).toEqual({ columns: [], rows: [], truncated: false });
    expect(cancel).not.toHaveBeenCalled();
  });
});

// makeArrowStreamReader は Arrow の IPC stream 形式のバイト列を非同期の小さなチャンクで流し、
// RecordBatchReader.from() が返す未 open の AsyncRecordBatchStreamReader を返す。
// AsyncDuckDBConnection.send() が返すリーダーと同じ構築経路である (docs/issues/closed/0206)。
async function makeArrowStreamReader(values: Record<string, unknown[]>) {
  const ipc = tableToIPC(tableFromArrays(values), 'stream');
  async function* chunks() {
    const size = 64;
    for (let i = 0; i < ipc.byteLength; i += size) yield ipc.subarray(i, i + size);
  }
  return RecordBatchReader.from(chunks());
}

describe('collectQueryResult (Arrow の AsyncRecordBatchStreamReader)', () => {
  it('open() していないリーダーから列名と行を読む', async () => {
    const cancel = vi.fn(async () => {});
    const reader = await makeArrowStreamReader({ id: [1, 2, 3], name: ['a', 'b', 'c'] });
    // send() が返した直後のリーダーは schema を持たない (ここで schema.fields を読むと
    // "Cannot read properties of undefined (reading 'fields')" になる)
    expect(reader.schema).toBeUndefined();
    const result = await collectQueryResult(reader, 10, cancel);
    expect(result.columns).toEqual(['id', 'name']);
    expect(result.rows).toEqual([
      ['1', 'a'],
      ['2', 'b'],
      ['3', 'c'],
    ]);
    expect(result.truncated).toBe(false);
    expect(cancel).not.toHaveBeenCalled();
  });

  it('上限を超えたら打ち切り、cancel を呼ぶ', async () => {
    const cancel = vi.fn(async () => {});
    const reader = await makeArrowStreamReader({ id: [1, 2, 3] });
    const result = await collectQueryResult(reader, 2, cancel);
    expect(result.columns).toEqual(['id']);
    expect(result.rows).toEqual([['1'], ['2']]);
    expect(result.truncated).toBe(true);
    expect(cancel).toHaveBeenCalledTimes(1);
  });

  it('バイト列が空でスキーマの無いストリームでは列も行も無い結果を返す', async () => {
    const cancel = vi.fn(async () => {});
    const reader = await RecordBatchReader.from((async function* () {})());
    const result = await collectQueryResult(reader, 10, cancel);
    expect(result).toEqual({ columns: [], rows: [], truncated: false });
    expect(cancel).not.toHaveBeenCalled();
  });
});

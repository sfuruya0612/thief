import { describe, expect, it, vi } from 'vitest';
import { tableFromArrays } from 'apache-arrow';
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
// 行数の配列で、pulled には iterator.next() が呼ばれた回数が入る (先読みの検証に使う)。
function makeReader(batchSizes: number[]) {
  const table = tableFromArrays({ n: [0] });
  const batches = batchSizes.map(
    (size) => tableFromArrays({ n: Array.from({ length: size }, (_, i) => i) }).batches[0],
  );
  const state = { pulled: 0 };
  const reader = {
    schema: table.schema,
    [Symbol.asyncIterator]() {
      let index = 0;
      return {
        async next() {
          state.pulled += 1;
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
});

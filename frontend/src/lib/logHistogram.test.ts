import { describe, expect, it } from 'vitest';
import { buildHistogram, dominantLevel, type HistogramItem, rangeFromItems } from './logHistogram';

describe('rangeFromItems', () => {
  it('空配列は {0, 0}', () => {
    expect(rangeFromItems([])).toEqual({ startMs: 0, endMs: 0 });
  });

  it('単一時刻は 1 分幅にフォールバックする', () => {
    const r = rangeFromItems([{ tsMs: 1000, level: 'info' }]);
    expect(r.startMs).toBe(1000);
    expect(r.endMs).toBe(61000);
  });

  it('min/max を返す', () => {
    const items: HistogramItem[] = [
      { tsMs: 500, level: 'info' },
      { tsMs: 2000, level: 'err' },
      { tsMs: 1000, level: 'warn' },
    ];
    expect(rangeFromItems(items)).toEqual({ startMs: 500, endMs: 2000 });
  });
});

describe('buildHistogram', () => {
  it('指定バケット数で severity 別に集計する', () => {
    const items: HistogramItem[] = [
      { tsMs: 0, level: 'info' },
      { tsMs: 5, level: 'err' },
      { tsMs: 90, level: 'warn' },
      { tsMs: 99, level: 'err' },
    ];
    const buckets = buildHistogram(items, 0, 100, 10);
    expect(buckets).toHaveLength(10);
    // 最初のバケット [0,10) に info + err の 2 件
    expect(buckets[0].total).toBe(2);
    expect(buckets[0].info).toBe(1);
    expect(buckets[0].err).toBe(1);
    // 最後のバケット [90,100] に warn + err の 2 件
    expect(buckets[9].total).toBe(2);
    expect(buckets[9].warn).toBe(1);
    expect(buckets[9].err).toBe(1);
    const total = buckets.reduce((s, b) => s + b.total, 0);
    expect(total).toBe(4);
  });

  it('範囲外の要素は除外する', () => {
    const items: HistogramItem[] = [
      { tsMs: -50, level: 'info' },
      { tsMs: 50, level: 'info' },
      { tsMs: 500, level: 'info' },
    ];
    const buckets = buildHistogram(items, 0, 100, 10);
    const total = buckets.reduce((s, b) => s + b.total, 0);
    expect(total).toBe(1);
  });

  it('範囲不正なら items から範囲を導出する', () => {
    const items: HistogramItem[] = [
      { tsMs: 1000, level: 'info' },
      { tsMs: 2000, level: 'err' },
    ];
    const buckets = buildHistogram(items, 100, 100, 5);
    expect(buckets).toHaveLength(5);
    const total = buckets.reduce((s, b) => s + b.total, 0);
    expect(total).toBe(2);
  });

  it('bucketCount が 0 以下なら空配列', () => {
    expect(buildHistogram([], 0, 100, 0)).toEqual([]);
  });
});

describe('dominantLevel', () => {
  it('err > warn > info の優先で最多レベルを返す', () => {
    expect(dominantLevel({ startMs: 0, endMs: 1, err: 3, warn: 1, info: 1, total: 5 })).toBe('err');
    expect(dominantLevel({ startMs: 0, endMs: 1, err: 0, warn: 2, info: 5, total: 7 })).toBe(
      'info',
    );
    expect(dominantLevel({ startMs: 0, endMs: 1, err: 0, warn: 2, info: 2, total: 4 })).toBe(
      'warn',
    );
    expect(dominantLevel({ startMs: 0, endMs: 1, err: 0, warn: 0, info: 0, total: 0 })).toBe(
      'info',
    );
  });

  it('同数のときはより重いレベルを採用する', () => {
    expect(dominantLevel({ startMs: 0, endMs: 1, err: 2, warn: 2, info: 2, total: 6 })).toBe('err');
  });
});

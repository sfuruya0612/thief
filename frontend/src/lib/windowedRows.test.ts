import { describe, expect, it } from 'vitest';
import { computeVisibleRange } from './windowedRows';

describe('computeVisibleRange', () => {
  // 標準的な設定: 行高 20px、ビューポート 100px、overscan 2。
  const base = {
    scrollTop: 0,
    viewportHeight: 100,
    listTop: 0,
    rowHeight: 20,
    rowCount: 100,
    overscan: 2,
  };

  it('リスト先頭が可視領域の先頭に一致するとき、先頭からビューポート分 + overscan を描画する', () => {
    const r = computeVisibleRange(base);
    // 可視は行 0..5 (100/20=5)。overscan 2 で上は 0 に clamp、下は 5+2=7。
    expect(r.start).toBe(0);
    expect(r.end).toBe(7);
    expect(r.topPad).toBe(0);
    expect(r.bottomPad).toBe((100 - 7) * 20);
  });

  it('中間までスクロールすると、可視範囲が overscan 込みで前後に広がる', () => {
    const r = computeVisibleRange({ ...base, scrollTop: 400 });
    // relTop=400 → floor(400/20)=20, -2 = 18。relBottom=500 → ceil(500/20)=25, +2 = 27。
    expect(r.start).toBe(18);
    expect(r.end).toBe(27);
    expect(r.topPad).toBe(18 * 20);
    expect(r.bottomPad).toBe((100 - 27) * 20);
  });

  it('最下部までスクロールすると end が rowCount に clamp され bottomPad が 0 になる', () => {
    // 総高さ 2000px、ビューポート 100px。最下部 scrollTop=1900。
    const r = computeVisibleRange({ ...base, scrollTop: 1900 });
    expect(r.end).toBe(100);
    expect(r.bottomPad).toBe(0);
    // relTop=1900 → 95, -2 = 93。
    expect(r.start).toBe(93);
    expect(r.topPad).toBe(93 * 20);
  });

  it('listTop が正 (テーブルがスクロール開始位置より下) の場合、その分だけ範囲がずれる', () => {
    // テーブルはコンテンツ座標 500px から始まる。scrollTop=500 でテーブル先頭が可視上端。
    const r = computeVisibleRange({ ...base, listTop: 500, scrollTop: 500 });
    expect(r.start).toBe(0);
    expect(r.end).toBe(7);
    expect(r.topPad).toBe(0);
  });

  it('テーブルがまだビューポートより下にある (relBottom <= 0) とき、0 行描画し全高を bottomPad にする', () => {
    // listTop=1000 だが scrollTop=0、ビューポート 100px。テーブルは 1000px 下 → 見えない。
    const r = computeVisibleRange({ ...base, listTop: 1000, scrollTop: 0 });
    expect(r.start).toBe(0);
    expect(r.end).toBe(0);
    expect(r.topPad).toBe(0);
    expect(r.bottomPad).toBe(100 * 20);
  });

  it('テーブルがビューポートより上に通り過ぎた (relTop >= 総高さ) とき、0 行描画し全高を topPad にする', () => {
    // 総高さ 2000px。listTop=0 で scrollTop=5000 → テーブルは遥か上。
    const r = computeVisibleRange({ ...base, scrollTop: 5000 });
    expect(r.start).toBe(100);
    expect(r.end).toBe(100);
    expect(r.topPad).toBe(100 * 20);
    expect(r.bottomPad).toBe(0);
  });

  it('topPad + 描画行の高さ + bottomPad が常に総高さに一致する (不変条件)', () => {
    for (const scrollTop of [0, 137, 400, 999, 1900, 3000]) {
      const r = computeVisibleRange({ ...base, scrollTop });
      const drawn = (r.end - r.start) * base.rowHeight;
      expect(r.topPad + drawn + r.bottomPad).toBe(base.rowCount * base.rowHeight);
    }
  });

  it('rowCount が 0 のとき空範囲を返す', () => {
    const r = computeVisibleRange({ ...base, rowCount: 0 });
    expect(r).toEqual({ start: 0, end: 0, topPad: 0, bottomPad: 0 });
  });

  it('rowHeight が 0 のとき windowing 無効 (全行描画・スペーサー 0) を返す', () => {
    const r = computeVisibleRange({ ...base, rowHeight: 0 });
    expect(r.start).toBe(0);
    expect(r.end).toBe(base.rowCount);
    expect(r.topPad).toBe(0);
    expect(r.bottomPad).toBe(0);
  });

  it('viewportHeight が 0 のとき windowing 無効 (全行描画) を返す', () => {
    const r = computeVisibleRange({ ...base, viewportHeight: 0 });
    expect(r.start).toBe(0);
    expect(r.end).toBe(base.rowCount);
  });

  it('非有限な scrollTop に対して windowing 無効 (全行描画) にフォールバックする', () => {
    const r = computeVisibleRange({ ...base, scrollTop: Number.NaN });
    expect(r.start).toBe(0);
    expect(r.end).toBe(base.rowCount);
  });

  it('overscan が 0 のとき、可視行ちょうど (境界の丸め込みのみ) を描画する', () => {
    const r = computeVisibleRange({ ...base, scrollTop: 400, overscan: 0 });
    // relTop=400 → 20、relBottom=500 → 25。
    expect(r.start).toBe(20);
    expect(r.end).toBe(25);
  });

  it('overscan が負や非有限のときは 0 として扱う', () => {
    const r = computeVisibleRange({ ...base, scrollTop: 400, overscan: -5 });
    expect(r.start).toBe(20);
    expect(r.end).toBe(25);
  });

  it('端数のスクロール位置でも start/end が行境界に正しく丸められる', () => {
    // scrollTop=410: relTop=410 → floor(410/20)=20, -2=18。relBottom=510 → ceil(510/20)=26, +2=28。
    const r = computeVisibleRange({ ...base, scrollTop: 410 });
    expect(r.start).toBe(18);
    expect(r.end).toBe(28);
  });
});

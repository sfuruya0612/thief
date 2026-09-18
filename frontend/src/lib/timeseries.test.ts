// 系列過多時の Other 集約、色の固定順割当 (dataviz の指針)、メトリクス取得の時間窓の検証。
import { describe, expect, it } from 'vitest';
import {
  collapseSeries,
  DEFAULT_MAX_SERIES,
  DEFAULT_METRICS_SPAN_SECONDS,
  isolatedPointFlags,
  metricsWindow,
  OTHER_SERIES_NAME,
  seriesColors,
  unitFormatter,
  type TimeseriesPoint,
  type TimeseriesSeries,
} from './timeseries';

// line は 1 本の直線の系列を作る。magnitude は値の大きさで、Other 集約の順位付けに効く。
function line(name: string, magnitude: number): TimeseriesSeries {
  return {
    name,
    points: [
      { t: 1_000, v: magnitude },
      { t: 2_000, v: magnitude },
    ],
  };
}

function manySeries(count: number): TimeseriesSeries[] {
  // 大きさを降順にしておき、集約の結果がどの系列を残したか判別できるようにする。
  return Array.from({ length: count }, (_, i) => line(`s${i}`, count - i));
}

describe('collapseSeries', () => {
  it('上限以下ならそのまま返す', () => {
    const series = manySeries(DEFAULT_MAX_SERIES);
    expect(collapseSeries(series)).toEqual(series);
  });

  it('上限を超えたら大きい順に残し、残りを Other へ集約する', () => {
    const collapsed = collapseSeries(manySeries(12));
    expect(collapsed).toHaveLength(DEFAULT_MAX_SERIES);
    expect(collapsed.slice(0, DEFAULT_MAX_SERIES - 1).map((s) => s.name)).toEqual([
      's0',
      's1',
      's2',
      's3',
      's4',
      's5',
      's6',
    ]);
    expect(collapsed[collapsed.length - 1].name).toBe(OTHER_SERIES_NAME);
  });

  it('Other は集約した系列を時刻ごとに足し合わせる', () => {
    const series: TimeseriesSeries[] = [
      { name: 'big', points: [{ t: 1_000, v: 100 }] },
      { name: 'a', points: [{ t: 1_000, v: 2 }] },
      { name: 'b', points: [{ t: 1_000, v: 3 }] },
    ];
    const collapsed = collapseSeries(series, 2);
    expect(collapsed.map((s) => s.name)).toEqual(['big', OTHER_SERIES_NAME]);
    expect(collapsed[1].points).toEqual([{ t: 1_000, v: 5 }]);
  });

  it('欠測 (null) は 0 として足し込まない', () => {
    const series: TimeseriesSeries[] = [
      { name: 'big', points: [{ t: 1_000, v: 100 }] },
      {
        name: 'a',
        points: [
          { t: 1_000, v: null },
          { t: 2_000, v: 4 },
        ],
      },
      { name: 'b', points: [{ t: 1_000, v: 3 }] },
    ];
    const collapsed = collapseSeries(series, 2);
    expect(collapsed[1].points).toEqual([
      { t: 1_000, v: 3 },
      { t: 2_000, v: 4 },
    ]);
  });

  it('集約対象の全系列が欠測の時刻を null として残す (前後の点が線で繋がらない)', () => {
    const T = 1_700_000_000_000;
    const grid = (vals: (number | null)[]) => vals.map((v, i) => ({ t: T + i * 60_000, v }));
    const series: TimeseriesSeries[] = [
      { name: 'big', points: grid([100, 100, 100, 100, 100]) },
      { name: 'a', points: grid([1, null, null, null, 1]) },
      { name: 'b', points: grid([1, null, null, null, 1]) },
      { name: 'c', points: grid([null, null, null, null, null]) },
    ];
    const other = collapseSeries(series, 2)[1];
    expect(other.name).toBe(OTHER_SERIES_NAME);
    expect(other.points).toEqual(grid([2, null, null, null, 2]));
  });

  it('一部の系列だけが値を持つ時刻は、その値の和になる', () => {
    const T = 1_700_000_000_000;
    const grid = (vals: (number | null)[]) => vals.map((v, i) => ({ t: T + i * 60_000, v }));
    const series: TimeseriesSeries[] = [
      { name: 'big', points: grid([100, 100, 100]) },
      { name: 'a', points: grid([1, 2, null]) },
      { name: 'b', points: grid([null, 3, null]) },
    ];
    const other = collapseSeries(series, 2)[1];
    expect(other.points).toEqual(grid([1, 5, null]));
  });
});

describe('seriesColors', () => {
  it('系列の色を index で固定順に割り当てる (同じデータなら常に同じ色)', () => {
    const first = seriesColors(manySeries(3));
    expect(seriesColors(manySeries(3))).toEqual(first);
    // 色が系列ごとに異なること (同じ色の使い回しで系列を区別できなくなるのを防ぐ)。
    expect(new Set(first).size).toBe(3);
  });

  it('Other には個別の系列と重ならない専用色を割り当てる', () => {
    const colors = seriesColors(collapseSeries(manySeries(12)));
    const otherColor = colors[colors.length - 1];
    expect(colors.slice(0, -1)).not.toContain(otherColor);
  });

  it('系列がパレットの色数を超えても色を使い回して割り当て切る', () => {
    // collapseSeries を通さず直接渡された場合でも、色が undefined にならないこと。
    const colors = seriesColors(manySeries(DEFAULT_MAX_SERIES + 2));
    expect(colors).toHaveLength(DEFAULT_MAX_SERIES + 2);
    expect(colors.every((c) => typeof c === 'string' && c !== '')).toBe(true);
  });
});

describe('isolatedPointFlags', () => {
  // points は [t, v] の並びから系列の点列を作る。孤立点の判定は値の並びだけで決まる。
  function points(values: (number | null)[]): TimeseriesPoint[] {
    return values.map((v, i) => ({ t: (i + 1) * 1_000, v }));
  }

  it('点が 1 つだけの系列はその点が孤立点になる', () => {
    expect(isolatedPointFlags(points([2]))).toEqual([true]);
  });

  it('先頭の点は次が欠測なら孤立点になる', () => {
    expect(isolatedPointFlags(points([1, null, null]))).toEqual([true, false, false]);
  });

  it('末尾の点は前が欠測なら孤立点になる', () => {
    expect(isolatedPointFlags(points([null, null, 1]))).toEqual([false, false, true]);
  });

  it('中間の点は前後とも欠測なら孤立点になる', () => {
    expect(isolatedPointFlags(points([1, null, 5, null, 1]))).toEqual([
      true,
      false,
      true,
      false,
      true,
    ]);
  });

  it('値の連続する区間の点は孤立点にならない (線分を持つため)', () => {
    expect(isolatedPointFlags(points([1, 2, 3]))).toEqual([false, false, false]);
  });

  it('片側だけ値が続く点も孤立点にならない', () => {
    expect(isolatedPointFlags(points([1, 2, null, 3, 4]))).toEqual([
      false,
      false,
      false,
      false,
      false,
    ]);
  });

  it('全点が欠測なら孤立点は無い (欠測に marker を出さない)', () => {
    expect(isolatedPointFlags(points([null, null, null]))).toEqual([false, false, false]);
  });

  it('点が無ければ空を返す', () => {
    expect(isolatedPointFlags([])).toEqual([]);
  });
});

describe('metricsWindow', () => {
  it('既定では直近 1 時間を返す', () => {
    const w = metricsWindow(undefined, new Date('2026-09-12T10:30:00.000Z'));
    expect(w.to - w.from).toBe(DEFAULT_METRICS_SPAN_SECONDS);
    expect(w.to).toBe(Date.parse('2026-09-12T10:30:00.000Z') / 1000);
  });

  it('秒以下を切り捨てて分に丸める (同じ 1 分の間は同じ窓になる)', () => {
    // 丸めないと再描画のたびにクエリキーが変わり、キャッシュが一切効かない。
    const a = metricsWindow(3600, new Date('2026-09-12T10:30:01.500Z'));
    const b = metricsWindow(3600, new Date('2026-09-12T10:30:59.999Z'));
    expect(a).toEqual(b);
    expect(a.to).toBe(Date.parse('2026-09-12T10:30:00.000Z') / 1000);
  });

  it('分をまたぐと窓が進む', () => {
    const a = metricsWindow(3600, new Date('2026-09-12T10:30:59.000Z'));
    const b = metricsWindow(3600, new Date('2026-09-12T10:31:00.000Z'));
    expect(b.to - a.to).toBe(60);
    expect(b.from - a.from).toBe(60);
  });

  it('遡る長さを指定できる', () => {
    const w = metricsWindow(900, new Date('2026-09-12T10:30:00.000Z'));
    expect(w.to - w.from).toBe(900);
  });
});

describe('unitFormatter', () => {
  it('単位があれば値の後ろに付ける', () => {
    expect(unitFormatter('%')(12.5)).toBe('12.5 %');
  });

  it('単位が無ければ値だけを出す', () => {
    expect(unitFormatter('')(1234)).toBe((1234).toLocaleString());
  });
});

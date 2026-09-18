// TimeseriesChart が組み立てる option が dataviz の指針 (色の固定順割当、系列過多時の
// Other 集約、二軸グラフ禁止、ホバー必須) を満たすことの検証。
// 集約と色割当そのものの検証は lib/timeseries.test.ts。
// echarts-for-react は jsdom (canvas 未実装) では描画できないため、option を捕まえる
// スタブに差し替えて option の内容だけを確認する。
import { render } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { TimeseriesChart } from './TimeseriesChart';
import { DEFAULT_MAX_SERIES, OTHER_SERIES_NAME, type TimeseriesSeries } from '../../lib/timeseries';

const captured = vi.hoisted(() => ({ option: {} as Record<string, unknown> }));

// SeriesOption は option.series の要素のうち、検証に使う範囲だけを写した型。
// データ項目は [t, v] の配列か、symbol を上書きしたオブジェクトのどちらかになる。
type SeriesDatum = [number, number | null] | { value: [number, number | null]; symbol: string };

interface SeriesOption {
  name: string;
  showSymbol: boolean;
  symbol: string;
  data: SeriesDatum[];
}

vi.mock('echarts-for-react', () => ({
  default: (props: { option: Record<string, unknown> }) => {
    captured.option = props.option;
    return <div data-testid="echarts-stub" />;
  },
}));

function manySeries(count: number): TimeseriesSeries[] {
  // 大きさを降順にしておき、集約の結果がどの系列を残したか判別できるようにする。
  return Array.from({ length: count }, (_, i) => ({
    name: `s${i}`,
    points: [
      { t: 1_000, v: count - i },
      { t: 2_000, v: count - i },
    ],
  }));
}

describe('TimeseriesChart の option', () => {
  beforeEach(() => {
    captured.option = {};
  });

  it('系列の色を index で固定順に割り当てる (同じデータなら常に同じ色)', () => {
    render(<TimeseriesChart series={manySeries(3)} />);
    const first = captured.option.color as string[];

    captured.option = {};
    render(<TimeseriesChart series={manySeries(3)} />);
    expect(captured.option.color).toEqual(first);
    expect(new Set(first).size).toBe(3);
  });

  it('系列が多すぎるとき Other に集約して描く', () => {
    render(<TimeseriesChart series={manySeries(12)} />);
    const series = captured.option.series as { name: string }[];
    expect(series).toHaveLength(DEFAULT_MAX_SERIES);
    expect(series[series.length - 1].name).toBe(OTHER_SERIES_NAME);
    // Other の色は個別の系列と重ならない。
    const colors = captured.option.color as string[];
    expect(colors.slice(0, -1)).not.toContain(colors[colors.length - 1]);
  });

  it('y 軸は 1 本だけで二軸にしない', () => {
    render(<TimeseriesChart series={manySeries(3)} />);
    expect(Array.isArray(captured.option.yAxis)).toBe(false);
    expect(captured.option.yAxis).toHaveProperty('type', 'value');
  });

  it('ホバー (tooltip) を常に有効にする', () => {
    render(<TimeseriesChart series={manySeries(1)} />);
    expect(captured.option.tooltip).toMatchObject({ trigger: 'axis' });
  });

  it('欠測 (null) は 0 に潰さずそのまま渡して線を途切れさせる', () => {
    render(
      <TimeseriesChart
        series={[
          {
            name: 'a',
            points: [
              { t: 1_000, v: 1 },
              { t: 2_000, v: 2 },
              { t: 3_000, v: null },
            ],
          },
        ]}
      />,
    );
    const series = captured.option.series as SeriesOption[];
    expect(series[0].data).toEqual([
      [1_000, 1],
      [2_000, 2],
      [3_000, null],
    ]);
  });

  it('点が 1 つだけの系列はその点に symbol を付ける (線分を持てず何も描かれないのを防ぐ)', () => {
    render(<TimeseriesChart series={[{ name: 'Running', points: [{ t: 1_000, v: 2 }] }]} />);
    const series = captured.option.series as SeriesOption[];
    // showSymbol: false のままだと、データ項目の symbol に関係なく marker が描かれない。
    expect(series[0].showSymbol).toBe(true);
    expect(series[0].symbol).toBe('none');
    expect(series[0].data).toEqual([{ value: [1_000, 2], symbol: 'circle' }]);
  });

  it('欠測に挟まれた孤立点にだけ symbol を付け、値の連続する区間の点には付けない', () => {
    render(
      <TimeseriesChart
        series={[
          {
            name: 'a',
            points: [
              { t: 1_000, v: 1 },
              { t: 2_000, v: 2 },
              { t: 3_000, v: null },
              { t: 4_000, v: 9 },
              { t: 5_000, v: null },
            ],
          },
        ]}
      />,
    );
    const series = captured.option.series as SeriesOption[];
    expect(series[0].data).toEqual([
      [1_000, 1],
      [2_000, 2],
      [3_000, null],
      { value: [4_000, 9], symbol: 'circle' },
      [5_000, null],
    ]);
  });

  it('Other に集約された欠測に挟まれた点にも symbol を付ける (集約後も孤立点を判定する)', () => {
    const T = 1_700_000_000_000;
    const grid = (vals: (number | null)[]) => vals.map((v, i) => ({ t: T + i * 60_000, v }));
    render(
      <TimeseriesChart
        maxSeries={2}
        series={[
          { name: 'big', points: grid([100, 100, 100, 100, 100]) },
          { name: 'a', points: grid([1, null, null, null, 1]) },
          { name: 'b', points: grid([1, null, null, null, 1]) },
        ]}
      />,
    );
    const series = captured.option.series as SeriesOption[];
    const other = series[series.length - 1];
    expect(other.name).toBe(OTHER_SERIES_NAME);
    // 中央 3 時刻は null として残り、両端の孤立点だけに symbol が付く。
    expect(other.data).toEqual([
      { value: [T, 2], symbol: 'circle' },
      [T + 60_000, null],
      [T + 120_000, null],
      [T + 180_000, null],
      { value: [T + 240_000, 2], symbol: 'circle' },
    ]);
  });

  it('値が連続する系列には symbol を付けない (marker が線を覆わない)', () => {
    render(<TimeseriesChart series={manySeries(1)} />);
    const series = captured.option.series as SeriesOption[];
    expect(series[0].data).toEqual([
      [1_000, 1],
      [2_000, 1],
    ]);
  });

  it('X 軸の範囲を渡すと xAxis の min と max に入る (軸が期間に追随する)', () => {
    const xRange = { start: 1_699_913_600_000, end: 1_700_000_000_000 };
    render(
      <TimeseriesChart
        series={[{ name: 'Running', points: [{ t: 1_699_999_000_000, v: 2 }] }]}
        xRange={xRange}
      />,
    );
    expect(captured.option.xAxis).toMatchObject({
      type: 'time',
      min: xRange.start,
      max: xRange.end,
    });
  });

  it('X 軸の範囲を渡さないと min と max を入れない (点の範囲から軸を決める従来どおりの挙動)', () => {
    render(
      <TimeseriesChart series={[{ name: 'Running', points: [{ t: 1_699_999_000_000, v: 2 }] }]} />,
    );
    const xAxis = captured.option.xAxis as Record<string, unknown>;
    expect(xAxis).toMatchObject({ type: 'time' });
    expect(xAxis).not.toHaveProperty('min');
    expect(xAxis).not.toHaveProperty('max');
  });

  it('系列が無いときは軸だけのグラフを描かず No data を出す', () => {
    const { getByText } = render(<TimeseriesChart series={[]} />);
    expect(getByText('No data')).toBeInTheDocument();
    expect(captured.option).toEqual({});
  });
});

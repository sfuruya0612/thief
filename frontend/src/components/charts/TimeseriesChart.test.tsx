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
              { t: 2_000, v: null },
            ],
          },
        ]}
      />,
    );
    const series = captured.option.series as { data: [number, number | null][] }[];
    expect(series[0].data).toEqual([
      [1_000, 1],
      [2_000, null],
    ]);
  });

  it('系列が無いときは軸だけのグラフを描かず No data を出す', () => {
    const { getByText } = render(<TimeseriesChart series={[]} />);
    expect(getByText('No data')).toBeInTheDocument();
    expect(captured.option).toEqual({});
  });
});

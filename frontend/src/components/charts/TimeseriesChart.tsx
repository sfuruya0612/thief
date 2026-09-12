// 時系列の折れ線グラフ。echarts-for-react の ReactECharts をラップする。
// データ整形 (Raw -> Row) は呼び出し側の責務とし、このコンポーネントは描画のみを担う
// (CostChart と同じ関心の分離)。色の固定順割当と Other 集約は lib/timeseries.ts。
import ReactECharts from 'echarts-for-react';
import { useTweaks } from '../../hooks/useTweaks';
import { collapseSeries, DEFAULT_MAX_SERIES, seriesColors } from '../../lib/timeseries';
import type { TimeseriesSeries } from '../../lib/timeseries';

export interface TimeseriesChartProps {
  series: TimeseriesSeries[];
  height?: number;
  // 値の表示形式。単位を付ける場合に渡す。
  valueFormatter?: (v: number) => string;
  // 個別に色を割り当てる系列の上限。これを超えた分は Other へ集約する。
  maxSeries?: number;
}

// ダークテーマ時の軸・凡例文字色 (CostChart と同じ直値。ECharts は CSS 変数を解釈しない)
const THEME_TEXT_COLOR: Record<'dark' | 'light', string> = {
  dark: '#a8a8b0',
  light: '#5c5c66',
};

export function TimeseriesChart({
  series,
  height = 260,
  valueFormatter,
  maxSeries = DEFAULT_MAX_SERIES,
}: TimeseriesChartProps) {
  const { tweaks } = useTweaks();
  const textColor = THEME_TEXT_COLOR[tweaks.theme];

  // 系列が 1 つも無いときは軸だけのグラフを描かない。空の軸は「値が 0 だった」と
  // 読めてしまい、データが無いことと区別が付かない。
  if (series.length === 0) {
    return (
      <div className="empty-hint" style={{ height }}>
        No data
      </div>
    );
  }

  const shown = collapseSeries(series, maxSeries);
  const format = valueFormatter ?? ((v: number) => v.toLocaleString());

  const option = {
    backgroundColor: 'transparent',
    textStyle: { color: textColor, fontFamily: 'var(--font-sans)' },
    color: seriesColors(shown),
    grid: { left: 56, right: 16, top: 24, bottom: 48 },
    // ホバーは常に有効にする。点の値を読めないグラフは概形しか伝えられない。
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'line' },
      valueFormatter: (v: number | null) => (v === null ? '—' : format(v)),
    },
    legend: { type: 'scroll', bottom: 0, textStyle: { color: textColor } },
    xAxis: {
      type: 'time',
      axisLine: { lineStyle: { color: textColor } },
      axisLabel: { color: textColor },
    },
    // y 軸は 1 本だけ。左右で異なる尺度の軸を並べると、目盛りの取り方次第で任意の
    // 相関を作れてしまうため、二軸グラフは使わない。
    yAxis: {
      type: 'value',
      axisLabel: { color: textColor, formatter: (v: number) => format(v) },
      splitLine: { lineStyle: { color: tweaks.theme === 'dark' ? '#35353c' : '#e4e4e8' } },
    },
    series: shown.map((s) => ({
      name: s.name,
      type: 'line',
      showSymbol: false,
      emphasis: { focus: 'series' },
      data: s.points.map((p) => [p.t, p.v]),
    })),
  };

  return <ReactECharts option={option} style={{ height }} notMerge />;
}

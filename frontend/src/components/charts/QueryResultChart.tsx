// クエリ結果 (任意の列を X 軸と Y 軸に選ぶ) の棒・折れ線グラフ。echarts-for-react の
// ReactECharts をラップする。データ整形 (string[][] -> カテゴリと系列) は
// lib/queryChart.ts の純関数の責務とし、このコンポーネントは描画のみを担う
// (TimeseriesChart / CostChart と同じ関心の分離)。
import ReactECharts from 'echarts-for-react';
import { useTweaks } from '../../hooks/useTweaks';
import { THEME_TEXT_COLOR } from './chartTheme';
import { SERIES_COLORS } from '../../lib/timeseries';
import { queryChartCategories, queryChartSeries } from '../../lib/queryChart';
import type { QueryResultChartType } from '../../lib/queryChart';

export interface QueryResultChartProps {
  columns: string[];
  rows: string[][];
  // xIndex はカテゴリ軸に置く列の index。
  xIndex: number;
  // yIndexes は値の系列にする列の index。配列の順が系列の順になる。
  yIndexes: number[];
  type: QueryResultChartType;
  height?: number;
}

export function QueryResultChart({
  columns,
  rows,
  xIndex,
  yIndexes,
  type,
  height = 320,
}: QueryResultChartProps) {
  const { tweaks } = useTweaks();
  const textColor = THEME_TEXT_COLOR[tweaks.theme];
  const series = queryChartSeries(columns, rows, yIndexes);

  const option = {
    backgroundColor: 'transparent',
    textStyle: { color: textColor, fontFamily: 'var(--font-sans)' },
    // 系列の色は時系列グラフと同じ固定順のパレットを使う。
    color: SERIES_COLORS,
    grid: { left: 56, right: 16, top: 24, bottom: 48 },
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: type === 'bar' ? 'shadow' : 'line' },
    },
    legend: { type: 'scroll', bottom: 0, textStyle: { color: textColor } },
    // x 軸は値の大小を持たないカテゴリ軸にする。X 列の値は文字列のまま置く。
    xAxis: {
      type: 'category',
      data: queryChartCategories(rows, xIndex),
      axisLine: { lineStyle: { color: textColor } },
      axisLabel: { color: textColor },
    },
    yAxis: {
      type: 'value',
      axisLabel: { color: textColor },
      splitLine: { lineStyle: { color: tweaks.theme === 'dark' ? '#35353c' : '#e4e4e8' } },
    },
    // 欠損は null のまま渡す。折れ線は null の点で線を途切れさせ、棒は棒を描かない。
    series: series.map((s) => ({
      name: s.name,
      type,
      emphasis: { focus: 'series' },
      data: s.data,
    })),
  };

  return <ReactECharts option={option} style={{ height }} notMerge />;
}

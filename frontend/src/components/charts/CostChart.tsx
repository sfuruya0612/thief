// Cost Explorer の積み上げ棒グラフ。echarts-for-react の ReactECharts をラップする。
// データ整形 (CostRow[] -> categories/series) は呼び出し側 (CostExplorerPanel) の責務とし、
// このコンポーネントは描画のみを担う (Raw/Row 分離と同じ関心の分離)。
import ReactECharts from 'echarts-for-react';
import { useTweaks } from '../../hooks/useTweaks';

export interface CostChartSeries {
  name: string;
  data: number[];
}

export interface CostChartProps {
  categories: string[];
  series: CostChartSeries[];
  height?: number;
}

// ダークテーマ時の軸・凡例文字色 (app.css の --text-2 相当を直値で持つ。ECharts は CSS 変数を解釈しないため)
const THEME_TEXT_COLOR: Record<'dark' | 'light', string> = {
  dark: '#a8a8b0',
  light: '#5c5c66',
};

export function CostChart({ categories, series, height = 320 }: CostChartProps) {
  const { tweaks } = useTweaks();
  const textColor = THEME_TEXT_COLOR[tweaks.theme];

  const option = {
    backgroundColor: 'transparent',
    textStyle: { color: textColor, fontFamily: 'var(--font-sans)' },
    grid: { left: 56, right: 16, top: 32, bottom: 48 },
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      valueFormatter: (v: number) =>
        `$${v.toLocaleString(undefined, { maximumFractionDigits: 2 })}`,
    },
    legend: {
      type: 'scroll',
      bottom: 0,
      textStyle: { color: textColor },
    },
    xAxis: {
      type: 'category',
      data: categories,
      axisLine: { lineStyle: { color: textColor } },
      axisLabel: { color: textColor },
    },
    yAxis: {
      type: 'value',
      axisLabel: { color: textColor, formatter: (v: number) => `$${v}` },
      splitLine: { lineStyle: { color: tweaks.theme === 'dark' ? '#35353c' : '#e4e4e8' } },
    },
    series: series.map((s) => ({
      name: s.name,
      type: 'bar',
      stack: 'cost',
      emphasis: { focus: 'series' },
      data: s.data,
    })),
  };

  return <ReactECharts option={option} style={{ height }} notMerge />;
}

// 時系列グラフの描画規範のうち、React に依存しない部分 (色の固定順パレットと
// 系列過多時の Other 集約) をまとめる。描画は components/charts/TimeseriesChart.tsx。
// Dashboards の timeseries ウィジェットと Metrics の双方から同じ規範で使う。

export interface TimeseriesPoint {
  // エポックミリ秒。
  t: number;
  // 欠測は null。線を途切れさせて「0 だった」と誤読させない。
  v: number | null;
}

export interface TimeseriesSeries {
  name: string;
  points: TimeseriesPoint[];
}

// SERIES_COLORS は系列に固定順で割り当てるパレット。系列の index で決まるため、
// 同じデータなら常に同じ色になる。呼び出しごとに色が変わると、読み手が前回の色の
// 記憶を使えなくなる。
export const SERIES_COLORS = [
  '#5b8ff9',
  '#5ad8a6',
  '#f6bd16',
  '#e8684a',
  '#945fb9',
  '#6dc8ec',
  '#ff9845',
  '#269a99',
];

// OTHER_COLOR は Other 専用の無彩色。個別の系列には使わず、集約された残りであることを
// 色でも区別できるようにする。
export const OTHER_COLOR = '#8a8a94';

// OTHER_SERIES_NAME は集約された残りの系列名。
export const OTHER_SERIES_NAME = 'Other';

// DEFAULT_MAX_SERIES は個別に描く系列の既定の上限。パレットの色数と揃える。
export const DEFAULT_MAX_SERIES = SERIES_COLORS.length;

// seriesMagnitude は系列の大きさ (絶対値の総和) を返す。どの系列を個別に残すかの順位付けに使う。
function seriesMagnitude(s: TimeseriesSeries): number {
  return s.points.reduce((sum, p) => sum + Math.abs(p.v ?? 0), 0);
}

// sumPoints は複数系列を時刻ごとに足し合わせる。時刻が揃っていない系列があっても、
// 値を持つ系列だけを足す (欠測を 0 とみなして足し込まない)。
function sumPoints(series: TimeseriesSeries[]): TimeseriesPoint[] {
  const totals = new Map<number, number>();
  for (const s of series) {
    for (const p of s.points) {
      if (p.v === null) continue;
      totals.set(p.t, (totals.get(p.t) ?? 0) + p.v);
    }
  }
  return [...totals.entries()].sort((a, b) => a[0] - b[0]).map(([t, v]) => ({ t, v }));
}

// collapseSeries は系列が多すぎるとき、大きい順に maxSeries - 1 本を残し、残りを
// Other へ足し合わせる。凡例と色の数が増えすぎると、どの線がどの系列か追えなくなり
// グラフが読めなくなるため、色を割り当てる系列数に上限を設ける。
export function collapseSeries(
  series: TimeseriesSeries[],
  maxSeries: number = DEFAULT_MAX_SERIES,
): TimeseriesSeries[] {
  if (series.length <= maxSeries) return series;
  const ranked = [...series].sort((a, b) => seriesMagnitude(b) - seriesMagnitude(a));
  return [
    ...ranked.slice(0, maxSeries - 1),
    { name: OTHER_SERIES_NAME, points: sumPoints(ranked.slice(maxSeries - 1)) },
  ];
}

// seriesColors は表示する系列に割り当てる色を index の順で返す。Other だけは専用の
// 無彩色にして、集約された残りであることを色でも区別できるようにする。
export function seriesColors(series: TimeseriesSeries[]): string[] {
  return series.map((s, i) =>
    s.name === OTHER_SERIES_NAME ? OTHER_COLOR : SERIES_COLORS[i % SERIES_COLORS.length],
  );
}

// unitFormatter は単位付きの値の表示形式を返す。単位の無いメトリクスでは数値だけを出す。
// Dashboards のウィジェットと Metrics の双方が同じ形式で値を出すために共有する。
export function unitFormatter(unit: string): (v: number) => string {
  if (!unit) return (v: number) => v.toLocaleString();
  return (v: number) => `${v.toLocaleString()} ${unit}`;
}

// MetricsWindow はメトリクスを取得する時間窓。どちらも Unix 秒。
export interface MetricsWindow {
  from: number;
  to: number;
}

// DEFAULT_METRICS_SPAN_SECONDS は既定で遡る長さ (1 時間)。
export const DEFAULT_METRICS_SPAN_SECONDS = 3600;

// metricsWindow は「直近 spanSeconds」の時間窓を分単位に切り下げて返す。
//
// 秒までの現在時刻をそのまま使うと、再描画のたびに窓がずれてクエリキーが変わり、
// backend と TanStack Query のどちらのキャッシュも一切効かなくなる。分に丸めることで、
// 同じ 1 分の間は同じキーになり、かつ時間の経過とともに窓が進む。
export function metricsWindow(
  spanSeconds: number = DEFAULT_METRICS_SPAN_SECONDS,
  now: Date = new Date(),
): MetricsWindow {
  const to = Math.floor(now.getTime() / 60_000) * 60;
  return { from: to - spanSeconds, to };
}

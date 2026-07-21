// 月単位 (YYYY-MM) の期間指定ヘルパー。月次コスト表示 (Datadog / TiDB) で共用する。

export function toMonthInputValue(d: Date): string {
  return d.toISOString().slice(0, 7);
}

// defaultMonthRange は「当月を含む直近 3 ヶ月」を初期表示範囲として返す。
export function defaultMonthRange(): { start: string; end: string } {
  const end = new Date();
  const start = new Date(end);
  start.setMonth(start.getMonth() - 2);
  return { start: toMonthInputValue(start), end: toMonthInputValue(end) };
}

// lastMonthsRange は「当月を含む直近 months ヶ月」の範囲を返す (プリセットボタン用)。
export function lastMonthsRange(months: number): { start: string; end: string } {
  const end = new Date();
  const start = new Date(end);
  start.setMonth(start.getMonth() - (months - 1));
  return { start: toMonthInputValue(start), end: toMonthInputValue(end) };
}

// labelKey は cost namespace の翻訳キー。表示側 (MonthlyCostPanel) が t() で解決する。
export const MONTH_RANGE_PRESETS = [
  { labelKey: 'monthRange.presets.last3Months', months: 3 },
  { labelKey: 'monthRange.presets.last6Months', months: 6 },
  { labelKey: 'monthRange.presets.last12Months', months: 12 },
];

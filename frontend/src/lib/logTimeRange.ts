// Cloud Logging ビュー (CloudLoggingView) の期間プリセット → start/end (RFC3339, backend への
// クエリパラメータ) 変換。custom は呼び出し側が個別の日時入力欄で管理するため、この関数の対象外。

export type LogTimeRangePreset = '15m' | '1h' | '6h' | '24h' | '7d';

// PresetOption は期間セレクタの選択肢 (プリセット + カスタム)。ログビューアで共有する。
export type PresetOption = LogTimeRangePreset | 'custom';

// 期間セレクタに表示する選択肢の順序。ラベルは i18n (logviewer:logToolbarActions.presets.*) 側で引く。
export const PRESET_OPTIONS: readonly PresetOption[] = ['15m', '1h', '6h', '24h', '7d', 'custom'];

export interface LogTimeRange {
  start: string;
  end: string;
}

const PRESET_MS: Record<LogTimeRangePreset, number> = {
  '15m': 15 * 60 * 1000,
  '1h': 60 * 60 * 1000,
  '6h': 6 * 60 * 60 * 1000,
  '24h': 24 * 60 * 60 * 1000,
  '7d': 7 * 24 * 60 * 60 * 1000,
};

// プリセット期間を、now を終端とする RFC3339 の start/end に変換する純関数。
// now を引数に取ることでテストから固定時刻を注入できるようにする。
export function presetToRange(preset: LogTimeRangePreset, now: Date = new Date()): LogTimeRange {
  const start = new Date(now.getTime() - PRESET_MS[preset]);
  return { start: start.toISOString(), end: now.toISOString() };
}

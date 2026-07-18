// ログビューア上部ツールバーの操作群 (ライブテールトグル + 期間指定 + エクスポート)。
// CloudWatch Logs / Cloud Logging で共通。
import { PRESET_LABELS, type PresetOption } from '../../lib/logTimeRange';

export interface LogToolbarActionsProps {
  live: boolean;
  onToggleLive: () => void;
  preset: PresetOption;
  onPresetChange: (p: PresetOption) => void;
  customStart: string;
  customEnd: string;
  onCustomStartChange: (v: string) => void;
  onCustomEndChange: (v: string) => void;
  exportLabel: string;
  onExport: () => void;
  exportDisabled?: boolean;
}

export function LogToolbarActions({
  live,
  onToggleLive,
  preset,
  onPresetChange,
  customStart,
  customEnd,
  onCustomStartChange,
  onCustomEndChange,
  exportLabel,
  onExport,
  exportDisabled,
}: LogToolbarActionsProps) {
  return (
    <>
      <button
        className={`lv-live-toggle ${live ? 'on' : 'off'}`}
        onClick={onToggleLive}
        title="ライブテールの ON/OFF"
      >
        <span className="lv-live-dot" />
        ライブテール {live ? 'ON' : 'OFF'}
      </button>

      <select
        className="btn sm lv-range-select"
        value={preset}
        onChange={(e) => onPresetChange(e.target.value as PresetOption)}
        disabled={live}
        title="表示期間"
      >
        {(Object.keys(PRESET_LABELS) as PresetOption[]).map((p) => (
          <option key={p} value={p}>
            {PRESET_LABELS[p]}
          </option>
        ))}
      </select>

      {preset === 'custom' && !live && (
        <>
          <input
            type="datetime-local"
            className="btn sm"
            value={customStart}
            onChange={(e) => onCustomStartChange(e.target.value)}
          />
          <input
            type="datetime-local"
            className="btn sm"
            value={customEnd}
            onChange={(e) => onCustomEndChange(e.target.value)}
          />
        </>
      )}

      <button className="btn sm" onClick={onExport} disabled={exportDisabled}>
        {exportLabel}
      </button>
    </>
  );
}

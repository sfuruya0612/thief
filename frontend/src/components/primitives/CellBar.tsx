export interface CellBarProps {
  value: number;
  max?: number;
  unit?: string;
  tone?: 'info';
}

export function CellBar({ value, max = 100, unit = '%', tone }: CellBarProps) {
  const pct = Math.min(100, (value / max) * 100);
  let color = 'var(--ok)';
  if (pct > 85) color = 'var(--err)';
  else if (pct > 65) color = 'var(--warn)';
  else if (tone === 'info') color = 'var(--info)';
  return (
    <span className="cell-bar">
      <span className="bar">
        <div style={{ width: `${pct}%`, background: color }} />
      </span>
      <span className="val">
        {value}
        {unit}
      </span>
    </span>
  );
}

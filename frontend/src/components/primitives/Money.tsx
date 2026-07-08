export interface MoneyProps {
  value: number | undefined;
}

export function Money({ value }: MoneyProps) {
  const n = value ?? 0;
  const digits = n > 100 ? 0 : 2;
  return (
    <span style={{ fontVariantNumeric: 'tabular-nums' }}>
      <span style={{ color: 'var(--text-3)' }}>$</span>
      {n.toLocaleString(undefined, { maximumFractionDigits: digits })}
    </span>
  );
}

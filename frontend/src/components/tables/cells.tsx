// 列定義 (columns.tsx / gcpColumns.tsx / nonAwsColumns.tsx) で共有するセル部品と
// インラインスタイル定数。値を変える場合はここだけを修正する。
export const monoStyle = { fontFamily: 'var(--font-mono)' } as const;
export const mutedMono = { fontFamily: 'var(--font-mono)', color: 'var(--text-2)' } as const;
export const dimMono = { fontFamily: 'var(--font-mono)', color: 'var(--text-3)' } as const;
export const dashStyle = { color: 'var(--text-4)' } as const;

// Dash は値が無いセルに表示するプレースホルダ。
export function Dash() {
  return <span style={dashStyle}>—</span>;
}

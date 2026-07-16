// Drawer リストタブ共通の Loading プレースホルダ。
// DataTable の isLoading (スピナー) とは意図的に別物で、Drawer 内はテキスト表示を維持する。
export function DrawerLoading() {
  return <div style={{ padding: 20, color: 'var(--text-3)' }}>Loading…</div>;
}

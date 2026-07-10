// 初回取得中 (isLoading) にテーブル領域へ表示するローディング表示。react-spinners の PacmanLoader を使う。
import { PacmanLoader } from 'react-spinners';

export function Loading() {
  return (
    <div className="empty-hint" style={{ flexDirection: 'column', gap: 12, padding: 40 }}>
      <PacmanLoader color="var(--accent)" size={20} />
      <span>Loading…</span>
    </div>
  );
}

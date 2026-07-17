// 全セッションタブを閉じたときの空状態パネル。
// 自動で開き直さない代わりに、タブバーの ＋ ボタンへの導線を示す。
export interface SessionEmptyStateProps {
  title: string;
  hint: string;
}

export function SessionEmptyState({ title, hint }: SessionEmptyStateProps) {
  return (
    <div className="session-empty">
      <div className="session-empty-title">{title}</div>
      <div className="session-empty-hint">{hint}</div>
    </div>
  );
}

// 表示用フォーマッタ

// ISO8601 の launch_time から "128d 6h" 形式の稼働時間文字列を生成する
export function formatUptime(launchTimeIso: string): string {
  if (!launchTimeIso) return '';
  const launched = new Date(launchTimeIso).getTime();
  if (isNaN(launched)) return '';
  const now = Date.now();
  const diff = Math.max(0, now - launched);
  const totalHours = Math.floor(diff / 3_600_000);
  const days = Math.floor(totalHours / 24);
  const hours = totalHours % 24;
  if (days === 0) {
    const minutes = Math.floor((diff % 3_600_000) / 60_000);
    return `${hours}h ${minutes}m`;
  }
  return `${days}d ${hours}h`;
}

// 金額表示: undefined / 0 は em dash に置き換える
export function formatMoney(v: number | undefined): string {
  if (v === undefined || v === 0) return '—';
  const digits = v > 100 ? 0 : 2;
  return `$${v.toLocaleString(undefined, { maximumFractionDigits: digits, minimumFractionDigits: digits })}`;
}

// ARN の末尾セグメントを取り出す (例: arn:aws:ecs:region:account:task/cluster/task-id -> task-id)。
// ECS タスク ARN は "/" を含むため URL パスセグメントにそのまま使えず、この短縮 ID を使う
// (ECS API は DescribeTasks/ExecuteCommand の task 引数にこの短縮 ID を受け付ける)。
export function arnSuffix(arn: string): string {
  const idx = arn.lastIndexOf('/');
  return idx === -1 ? arn : arn.slice(idx + 1);
}

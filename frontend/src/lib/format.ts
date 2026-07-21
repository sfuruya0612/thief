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

// Pricing の単価表示。formatMoney と異なり 0 を em dash に隠さない (All Upfront RI の
// 時間単価 $0/hr のように、0 そのものが意味のある値になるため)。RI/SP の単価は 4 桁の
// 小数まで意味を持つ (例: $0.0864) ため、formatMoney の 0/2 桁ルールではなく常に
// 最大 4 桁まで表示する。
export function formatUnitPrice(v: number): string {
  return `$${v.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 4 })}`;
}

// Pricing の unit ("Hrs" / "vCPU-Hours" / "GB-Hours") を単価の接尾辞として表示する。
// 未知の unit はそのまま "/<unit>" にする (バックエンドの許可リスト変更に追従できるように、
// 未知値を空文字やエラーにしない)。
export function formatPricingUnit(unit: string): string {
  switch (unit) {
    case 'Hrs':
      return '/時間';
    case 'vCPU-Hours':
      return '/vCPU時間';
    case 'GB-Hours':
      return '/GB時間';
    default:
      return unit ? `/${unit}` : '';
  }
}

// Pricing の RI 実効時間単価の On-Demand 比節減率 (issue 0057)。符号をそのまま表示する
// (正: 割安、負: 割高な異常値。隠さず表示するため符号を反転・除去しない)。
export function formatPercent(v: number): string {
  return `${v.toFixed(1)}%`;
}

// キャッシュ鮮度表示用の "MM/DD HH:mm" (ローカル時刻)。不正な日時は空文字を返す。
export function formatFetchedAt(iso: string): string {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return '';
  const mm = String(d.getMonth() + 1).padStart(2, '0');
  const dd = String(d.getDate()).padStart(2, '0');
  const hh = String(d.getHours()).padStart(2, '0');
  const mi = String(d.getMinutes()).padStart(2, '0');
  return `${mm}/${dd} ${hh}:${mi}`;
}

// ARN の末尾セグメントを取り出す (例: arn:aws:ecs:region:account:task/cluster/task-id -> task-id)。
// ECS タスク ARN は "/" を含むため URL パスセグメントにそのまま使えず、この短縮 ID を使う
// (ECS API は DescribeTasks/ExecuteCommand の task 引数にこの短縮 ID を受け付ける)。
export function arnSuffix(arn: string): string {
  const idx = arn.lastIndexOf('/');
  return idx === -1 ? arn : arn.slice(idx + 1);
}

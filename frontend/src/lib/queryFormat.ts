// クエリエディタ用の表示フォーマッタと CSV 変換

// BigQuery オンデマンド料金 (USD / TiB)。ドライラン結果からの概算コスト表示に使う。
export const BQ_ON_DEMAND_USD_PER_TIB = 6.25;

const TIB = 1024 ** 4;

// 経過時間を "2.3s" / "45s" / "1m 5s" 形式で返す (BigQuery 表示用)
export function formatDurationSeconds(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) return '';
  const sec = ms / 1000;
  if (sec < 10) return `${sec.toFixed(1)}s`;
  if (sec < 60) return `${Math.round(sec)}s`;
  const m = Math.floor(sec / 60);
  const s = Math.round(sec % 60);
  return `${m}m ${s}s`;
}

// 経過時間を "00:06" / "01:02" / "1:02:03" 形式で返す (Athena 表示用)
export function formatDurationClock(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) return '';
  const totalSec = Math.floor(ms / 1000);
  const h = Math.floor(totalSec / 3600);
  const m = Math.floor((totalSec % 3600) / 60);
  const s = totalSec % 60;
  const pad = (n: number) => String(n).padStart(2, '0');
  if (h > 0) return `${h}:${pad(m)}:${pad(s)}`;
  return `${pad(m)}:${pad(s)}`;
}

// ISO8601 タイムスタンプを "07-16 22:04" 形式 (ローカル時刻) で返す
export function formatTimestampShort(iso: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (isNaN(d.getTime())) return '';
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

// ドライランの処理バイト数からオンデマンド料金の概算 (USD) を計算する
export function estimateBQCostUSD(bytes: number): number {
  if (!Number.isFinite(bytes) || bytes <= 0) return 0;
  return (bytes / TIB) * BQ_ON_DEMAND_USD_PER_TIB;
}

// 概算 USD を "$0.006" / "$1.24" / "$130" 形式で返す
export function formatApproxUSD(usd: number): string {
  if (!Number.isFinite(usd) || usd < 0) return '';
  if (usd < 0.01) return `$${usd.toFixed(3)}`;
  if (usd < 100) return `$${usd.toFixed(2)}`;
  return `$${Math.round(usd).toLocaleString()}`;
}

// 長い ID を "job_ab12…f9" のように短縮する
export function shortId(id: string, head = 8, tail = 2): string {
  if (id.length <= head + tail + 1) return id;
  return `${id.slice(0, head)}…${id.slice(-tail)}`;
}

// ステータスバーの CLI ヒント表示用に SQL を 1 行へ潰して切り詰める
export function cliHintSql(sql: string, max = 60): string {
  const collapsed = sql.replace(/\s+/g, ' ').trim();
  if (collapsed.length <= max) return collapsed;
  return `${collapsed.slice(0, max)}…`;
}

// S3 オブジェクトパスからディレクトリ部分 (末尾スラッシュ付き) を取り出す
export function s3Dir(path: string): string {
  const idx = path.lastIndexOf('/');
  return idx === -1 ? path : path.slice(0, idx + 1);
}

// RFC 4180 に従って CSV 文字列を組み立てる (改行は CRLF、必要なセルのみ引用)
export function toCsv(columns: string[], rows: string[][]): string {
  const escapeCell = (cell: string): string => {
    if (/[",\r\n]/.test(cell)) {
      return `"${cell.replace(/"/g, '""')}"`;
    }
    return cell;
  };
  const lines = [columns, ...rows].map((row) => row.map(escapeCell).join(','));
  return lines.join('\r\n');
}

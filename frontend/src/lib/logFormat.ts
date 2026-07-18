// ログビューア共通の整形ヘルパー。タイムスタンプの時刻表示と、表示中の行を CSV / JSON へ
// 書き出すエクスポート (クリップボードコピー) 用の純関数。

// formatLogClock は RFC3339 タイムスタンプをローカル時刻の HH:MM:SS.mmm へ整形する。
// パースできない場合は入力をそのまま返す。
export function formatLogClock(rfc3339: string): string {
  const d = new Date(rfc3339);
  if (Number.isNaN(d.getTime())) return rfc3339;
  const p2 = (n: number) => String(n).padStart(2, '0');
  const ms = String(d.getMilliseconds()).padStart(3, '0');
  return `${p2(d.getHours())}:${p2(d.getMinutes())}:${p2(d.getSeconds())}.${ms}`;
}

// csvField は CSV の 1 セルを RFC4180 準拠でエスケープする。カンマ・引用符・改行を含む場合のみ
// ダブルクォートで囲み、内部の引用符は 2 個に増やす。
function csvField(value: string): string {
  if (/[",\r\n]/.test(value)) {
    return `"${value.replace(/"/g, '""')}"`;
  }
  return value;
}

// rowsToCsv はヘッダー行 + データ行を CSV 文字列 (改行区切り) にする。
export function rowsToCsv(headers: string[], rows: string[][]): string {
  const lines = [headers.map(csvField).join(',')];
  for (const row of rows) {
    lines.push(row.map(csvField).join(','));
  }
  return lines.join('\n');
}

// rowsToJson はオブジェクト配列を 2 スペースインデントの JSON 文字列にする。
export function rowsToJson(objs: unknown[]): string {
  return JSON.stringify(objs, null, 2);
}

// jsonFieldsOf は JSON オブジェクト文字列を { key, value } の配列にする。オブジェクトでない
// (配列・プリミティブ・パース不能) 場合は null を返す。ログの展開表示でフィールド化に使う。
export function jsonFieldsOf(text: string): { key: string; value: string }[] | null {
  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch {
    return null;
  }
  if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
    return null;
  }
  return Object.entries(parsed as Record<string, unknown>).map(([key, value]) => ({
    key,
    value: typeof value === 'string' ? value : JSON.stringify(value),
  }));
}

// ログビューア (CloudWatch Logs / Cloud Logging) 共通の severity 判定。
// GCP は構造化された Severity 文字列を持つため normalizeGcp.ts の logSeverityLevel を使う。
// CloudWatch のイベントは構造化 severity を持たないため、メッセージ本文から推定する。

export type SeverityLevel = 'err' | 'warn' | 'info';

// cwSeverityFromMessage は CloudWatch Logs のメッセージ本文から severity を推定する。
// 構造化フィールドが無いため、慣用的なレベル語 (ERROR / WARN 等) の有無で 3 段階へ丸める。
// 判定はメッセージ先頭付近 (200 文字) に限定して誤検出を抑える。
export function cwSeverityFromMessage(message: string): SeverityLevel {
  const head = message.slice(0, 200).toUpperCase();
  if (/\b(ERROR|ERR|FATAL|CRITICAL|CRIT|EMERG|EMERGENCY|ALERT|PANIC|EXCEPTION)\b/.test(head)) {
    return 'err';
  }
  if (/\b(WARN|WARNING)\b/.test(head)) {
    return 'warn';
  }
  return 'info';
}

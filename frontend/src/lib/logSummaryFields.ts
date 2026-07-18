// Cloud Logging の SUMMARY 列に優先表示するフィールドの選択・組み立てを行う純関数。
// Google Cloud Logs Explorer の「サマリー フィールド」相当の機能で使う。
import type { LogEntryRow } from '../types/gcp';
import { jsonFieldsOf } from './logFormat';

const RESOURCE_TYPE_KEY = 'resource.type';
const TRACE_KEY = 'trace';
const JSON_PAYLOAD_PREFIX = 'jsonPayload.';
const LABELS_PREFIX = 'labels.';

// availableSummaryFieldKeys は行群から選択候補のフィールドキーを、出現順を保ったまま重複なく列挙する。
export function availableSummaryFieldKeys(rows: LogEntryRow[]): string[] {
  const seen = new Set<string>();
  const keys: string[] = [];
  const add = (key: string) => {
    if (!seen.has(key)) {
      seen.add(key);
      keys.push(key);
    }
  };
  for (const row of rows) {
    const fields = jsonFieldsOf(row.payload);
    if (fields) {
      for (const f of fields) add(`${JSON_PAYLOAD_PREFIX}${f.key}`);
    }
    if (row.resourceType) add(RESOURCE_TYPE_KEY);
    for (const key of Object.keys(row.labels)) add(`${LABELS_PREFIX}${key}`);
    if (row.trace) add(TRACE_KEY);
  }
  return keys;
}

// resolveSummaryField は 1 行から指定フィールドキーの値を取り出す。行にそのフィールドが
// 存在しない場合は undefined を返す。
function resolveSummaryField(row: LogEntryRow, key: string): string | undefined {
  if (key === RESOURCE_TYPE_KEY) return row.resourceType || undefined;
  if (key === TRACE_KEY) return row.trace || undefined;
  if (key.startsWith(LABELS_PREFIX)) return row.labels[key.slice(LABELS_PREFIX.length)];
  if (key.startsWith(JSON_PAYLOAD_PREFIX)) {
    const shortKey = key.slice(JSON_PAYLOAD_PREFIX.length);
    return jsonFieldsOf(row.payload)?.find((f) => f.key === shortKey)?.value;
  }
  return undefined;
}

// buildSummaryText は選択されたフィールドキー (先頭優先の順序) から SUMMARY 列の表示文字列を
// 組み立てる。フィールド未選択、または選択キーが行に 1 つも存在しない場合は元の payload を
// そのまま返す (後方互換)。
export function buildSummaryText(row: LogEntryRow, fieldKeys: string[]): string {
  if (fieldKeys.length === 0) return row.payload;
  const parts: string[] = [];
  for (const key of fieldKeys) {
    const value = resolveSummaryField(row, key);
    if (value !== undefined) parts.push(`${key}=${value}`);
  }
  return parts.length > 0 ? parts.join('  ') : row.payload;
}

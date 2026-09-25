// OPFS 取り込み Worker (opfsWriter.worker.ts) が使う上限判定とエラー判定の純関数。
//
// Worker の本体は jsdom で動かせない (createSyncAccessHandle と OPFS が無い) ため、判定だけを
// このファイルに切り出して単体テストで固定する。Worker から import できるよう、アプリの
// モジュール (i18n を含む) と DuckDB / OPFS の API には依存させない。
// 上限の意味は lib/objectQuery.ts の objectQueryOverLimit と同じ (上限ちょうどは許容する) で、
// あちらは i18n を import するため Worker からは使えない。

// parseContentLength は Content-Length ヘッダの値を数値にする。ヘッダが無い場合と、10 進の
// 非負整数でない場合は null を返す (長さ不明として扱い、累積バイト数の判定に委ねる)。
export function parseContentLength(raw: string | null): number | null {
  if (raw === null || !/^\d+$/.test(raw)) return null;
  return Number(raw);
}

// exceedsMaxBytes は bytes が上限を超えているかを返す。上限ちょうどは許容する。
export function exceedsMaxBytes(bytes: number, maxBytes: number): boolean {
  return bytes > maxBytes;
}

// isFetchAbortError は fetch やストリーム読み取りの中断かを判定する。
export function isFetchAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === 'AbortError';
}

// isQuotaError は OPFS への書き込みが空き容量不足で失敗したかを判定する。
export function isQuotaError(err: unknown): boolean {
  return err instanceof DOMException && err.name === 'QuotaExceededError';
}

// isNotFoundError は OPFS のエントリが既に無いことを示すエラーかを判定する。
export function isNotFoundError(err: unknown): boolean {
  return err instanceof DOMException && err.name === 'NotFoundError';
}

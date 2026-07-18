// ログビューアのヒストグラム集計。取得済みのログ行をクライアント側で時間バケットに集計し、
// severity 別の件数を持つバケット列を作る純関数。件数は「取得済みの行」に基づくため、
// バックエンドの全件集計とは一致しない (実装範囲上の割り切り)。

import type { SeverityLevel } from './logSeverity';

export interface HistogramItem {
  tsMs: number;
  level: SeverityLevel;
}

export interface HistogramBucket {
  startMs: number;
  endMs: number;
  err: number;
  warn: number;
  info: number;
  total: number;
}

// DEFAULT_HISTOGRAM_BUCKETS は既定のバケット数 (デザインの棒本数に合わせる)。
export const DEFAULT_HISTOGRAM_BUCKETS = 20;

// rangeFromItems は items の最小/最大タイムスタンプから範囲を導出する。
// items が空、または全て同一時刻の場合は 1 分幅にフォールバックする。
export function rangeFromItems(items: HistogramItem[]): { startMs: number; endMs: number } {
  if (items.length === 0) {
    return { startMs: 0, endMs: 0 };
  }
  let min = items[0].tsMs;
  let max = items[0].tsMs;
  for (const it of items) {
    if (it.tsMs < min) min = it.tsMs;
    if (it.tsMs > max) max = it.tsMs;
  }
  if (max <= min) {
    max = min + 60_000;
  }
  return { startMs: min, endMs: max };
}

// buildHistogram は items を [startMs, endMs] 区間で bucketCount 個のバケットに集計する。
// 範囲が不正 (endMs <= startMs) の場合は items の min/max から導出しなおす。
export function buildHistogram(
  items: HistogramItem[],
  startMs: number,
  endMs: number,
  bucketCount: number = DEFAULT_HISTOGRAM_BUCKETS,
): HistogramBucket[] {
  if (bucketCount <= 0) return [];

  let lo = startMs;
  let hi = endMs;
  if (!(hi > lo)) {
    const r = rangeFromItems(items);
    lo = r.startMs;
    hi = r.endMs;
  }
  if (!(hi > lo)) {
    // それでも範囲が確定しない (items 空) 場合は空バケットを返さず 0 件バケット列にする。
    hi = lo + 60_000;
  }

  const width = (hi - lo) / bucketCount;
  const buckets: HistogramBucket[] = Array.from({ length: bucketCount }, (_, i) => ({
    startMs: lo + i * width,
    endMs: lo + (i + 1) * width,
    err: 0,
    warn: 0,
    info: 0,
    total: 0,
  }));

  for (const it of items) {
    if (it.tsMs < lo || it.tsMs > hi) continue;
    let idx = Math.floor((it.tsMs - lo) / width);
    if (idx < 0) idx = 0;
    if (idx >= bucketCount) idx = bucketCount - 1;
    buckets[idx][it.level] += 1;
    buckets[idx].total += 1;
  }
  return buckets;
}

// dominantLevel は 1 バケット内で最も件数の多い severity を返す (単色ヒストグラムの色分け用)。
// err > warn > info の優先順で、同数のときはより重いレベルを採用する。0 件は 'info'。
export function dominantLevel(bucket: HistogramBucket): SeverityLevel {
  if (bucket.err > 0 && bucket.err >= bucket.warn && bucket.err >= bucket.info) return 'err';
  if (bucket.warn > 0 && bucket.warn >= bucket.info) return 'warn';
  return 'info';
}

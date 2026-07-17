// Live Tail (CloudLoggingView) で無制限に DOM へ追記し続けるとタブがメモリと再描画で
// 破綻するため、保持する行数に上限を設けて古い行から捨てる純関数。

// 保持する行数の既定上限。
export const MAX_LOG_LINES = 5000;

// lines の末尾に incoming を追加し、maxLines を超えた分だけ先頭 (古い行) から切り捨てる。
export function appendLogLines<T>(
  lines: T[],
  incoming: T[],
  maxLines: number = MAX_LOG_LINES,
): T[] {
  if (incoming.length === 0) return lines;
  const merged = lines.concat(incoming);
  if (merged.length <= maxLines) return merged;
  return merged.slice(merged.length - maxLines);
}

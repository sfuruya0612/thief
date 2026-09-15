// TerminalDock の statuses state (id ごとの接続状態) から、tabs.open に無い id の
// エントリを取り除く純関数。TerminalDock.tsx の useEffect から呼ばれる
// (堅牢性 > 性能: セッションの開閉を繰り返しても statuses が際限なく蓄積しないようにする)。
import type { ConnectionStatus } from './Terminal';

export function pruneStatuses(
  statuses: Record<string, ConnectionStatus>,
  openIds: readonly string[],
): Record<string, ConnectionStatus> {
  const openIdSet = new Set(openIds);
  const staleIds = Object.keys(statuses).filter((id) => !openIdSet.has(id));
  if (staleIds.length === 0) return statuses;
  const next = { ...statuses };
  for (const id of staleIds) delete next[id];
  return next;
}

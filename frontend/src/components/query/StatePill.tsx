// クエリ実行状態のピル表示 (RUNNING 橙 / SUCCEEDED 緑 / FAILED 赤 / CANCELLED 灰)
import type { QueryRunState } from '../../types/query';

export interface StatePillProps {
  state: QueryRunState;
  label: string;
}

export function StatePill({ state, label }: StatePillProps) {
  return <span className={`qe-pill ${state}`}>{label}</span>;
}

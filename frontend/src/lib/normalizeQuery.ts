// クエリエディタ (BigQuery / Athena) の Raw → Row 変換関数
import type {
  AthenaExecutionRaw,
  AthenaResultPageRaw,
  AthenaTableRaw,
  AthenaTableRow,
  BQHistoryItemRaw,
  BQJobStatusRaw,
  BQResultPageRaw,
  NamedQuery,
  QueryHistoryRow,
  QueryResultData,
  QueryRunState,
  QueryStatusRow,
  SnippetRaw,
} from '../types/query';
import { formatTimestampShort } from './queryFormat';

// BigQuery のジョブ状態 (PENDING/RUNNING/DONE + エラー有無) を共通 5 状態へ正規化する
export function bqRunState(state: string, errorMessage?: string): QueryRunState {
  if (state === 'DONE') return errorMessage ? 'failed' : 'succeeded';
  if (state === 'RUNNING') return 'running';
  return 'queued';
}

// BigQuery の状態表示ラベル (デザイン 2a の「● 完了」に合わせて日本語)
const BQ_STATE_LABELS: Record<QueryRunState, string> = {
  queued: '待機中',
  running: '実行中',
  succeeded: '完了',
  failed: '失敗',
  cancelled: 'キャンセル',
};

export function bqJobStatusFromRaw(raw: BQJobStatusRaw): QueryStatusRow {
  const state = bqRunState(raw.state, raw.error_message);
  return {
    id: raw.job_id,
    state,
    stateLabel: BQ_STATE_LABELS[state],
    errorMessage: raw.error_message,
    elapsedMs: raw.elapsed_ms,
    bytes: raw.total_bytes_processed,
    location: raw.location,
    submittedAt: raw.start_time,
  };
}

export function bqHistoryFromRaw(raw: BQHistoryItemRaw): QueryHistoryRow {
  const state = bqRunState(raw.state, raw.error_message);
  return {
    id: raw.job_id,
    state,
    stateLabel: BQ_STATE_LABELS[state],
    sql: raw.sql,
    elapsedMs: raw.elapsed_ms,
    bytes: raw.total_bytes_processed,
    startedAt: formatTimestampShort(raw.start_time ?? ''),
    location: raw.location,
  };
}

// BigQuery の結果ページ列を 1 つの表示用データへ統合する
export function bqResultsFromPages(pages: BQResultPageRaw[]): QueryResultData {
  const columns = pages.find((p) => (p.columns ?? []).length > 0)?.columns ?? [];
  const rows = pages.flatMap((p) => p.rows ?? []);
  const totalRows = pages.length > 0 ? pages[0].total_rows : undefined;
  return { columns, rows, totalRows };
}

// Athena の実行状態 (QUEUED/RUNNING/SUCCEEDED/FAILED/CANCELLED) を共通 5 状態へ正規化する
export function athenaRunState(state: string): QueryRunState {
  switch (state) {
    case 'SUCCEEDED':
      return 'succeeded';
    case 'FAILED':
      return 'failed';
    case 'CANCELLED':
      return 'cancelled';
    case 'RUNNING':
      return 'running';
    default:
      return 'queued';
  }
}

export function athenaExecutionFromRaw(raw: AthenaExecutionRaw): QueryStatusRow {
  return {
    id: raw.id,
    state: athenaRunState(raw.state),
    stateLabel: raw.state || 'QUEUED',
    errorMessage: raw.state_reason,
    elapsedMs: raw.elapsed_ms,
    bytes: raw.bytes_scanned,
    outputLocation: raw.output_location,
    submittedAt: raw.submitted_at,
  };
}

export function athenaHistoryFromRaw(raw: AthenaExecutionRaw): QueryHistoryRow {
  return {
    id: raw.id,
    state: athenaRunState(raw.state),
    stateLabel: raw.state || 'QUEUED',
    sql: raw.sql,
    elapsedMs: raw.elapsed_ms,
    bytes: raw.bytes_scanned,
    startedAt: formatTimestampShort(raw.submitted_at ?? ''),
    outputLocation: raw.output_location,
  };
}

// Athena の結果ページ列を 1 つの表示用データへ統合する
export function athenaResultsFromPages(pages: AthenaResultPageRaw[]): QueryResultData {
  const columns = (pages.find((p) => (p.columns ?? []).length > 0)?.columns ?? []).map(
    (c) => c.name,
  );
  const rows = pages.flatMap((p) => p.rows ?? []);
  return { columns, rows };
}

export function athenaTableFromRaw(raw: AthenaTableRaw): AthenaTableRow {
  return {
    name: raw.name,
    type: raw.type,
    columns: (raw.columns ?? []).map((c) => ({ name: c.name, type: c.type })),
    partitionKeys: (raw.partition_keys ?? []).map((c) => ({ name: c.name, type: c.type })),
  };
}

// スニペットはファイル名 (name) が一意なので id にそのまま使う
export function snippetFromRaw(raw: SnippetRaw): NamedQuery {
  return {
    id: raw.name,
    name: raw.name,
    sql: raw.sql,
    updatedAt: raw.updated_at,
  };
}

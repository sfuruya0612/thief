// クエリエディタ (BigQuery / Athena) の Raw (backend JSON 形状) / Row (UI 形状) 型定義
// Raw は backend/internal/bigquery/job.go と backend/internal/aws/athena.go の JSON タグをミラーする

// ============================================================
// BigQuery Raw
// ============================================================
export interface BQJobInfoRaw {
  job_id: string;
  location: string;
  state: string;
}

export interface BQJobStatusRaw {
  job_id: string;
  location: string;
  state: string;
  error_message?: string;
  start_time?: string;
  end_time?: string;
  elapsed_ms: number;
  total_bytes_processed: number;
  cache_hit: boolean;
}

export interface BQDryRunRaw {
  total_bytes_processed: number;
}

export interface BQResultPageRaw {
  columns: string[] | null;
  rows: string[][] | null;
  total_rows: number;
  next_page_token?: string;
}

export interface BQHistoryItemRaw {
  job_id: string;
  location: string;
  state: string;
  sql: string;
  start_time?: string;
  end_time?: string;
  elapsed_ms: number;
  total_bytes_processed: number;
  error_message?: string;
}

// ============================================================
// Athena Raw
// ============================================================
export interface AthenaCatalogRaw {
  name: string;
  type: string;
}

export interface AthenaDatabaseRaw {
  name: string;
  description?: string;
}

export interface AthenaWorkgroupRaw {
  name: string;
  state: string;
  description?: string;
}

export interface AthenaColumnRaw {
  name: string;
  type: string;
}

export interface AthenaTableRaw {
  name: string;
  type: string;
  columns: AthenaColumnRaw[] | null;
  partition_keys: AthenaColumnRaw[] | null;
}

export interface AthenaExecutionRaw {
  id: string;
  sql: string;
  state: string;
  state_reason?: string;
  submitted_at?: string;
  completed_at?: string;
  elapsed_ms: number;
  bytes_scanned: number;
  output_location?: string;
  workgroup?: string;
  catalog?: string;
  database?: string;
}

export interface AthenaResultColumnRaw {
  name: string;
  type: string;
}

export interface AthenaResultPageRaw {
  columns: AthenaResultColumnRaw[] | null;
  rows: string[][] | null;
  next_token?: string;
}

// ============================================================
// Row (UI 表示用)
// ============================================================

// BigQuery / Athena のジョブ状態を UI 共通の 5 状態へ正規化したもの
export type QueryRunState = 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled';

// 実行中 / 完了したクエリの状態表示 (ツールバー・結果タブのステータス行で使う)
export interface QueryStatusRow {
  id: string;
  state: QueryRunState;
  stateLabel: string;
  errorMessage?: string;
  elapsedMs: number;
  bytes: number;
  location?: string;
  outputLocation?: string;
  submittedAt?: string;
}

// クエリ結果 (1 ページ分を蓄積した表示用データ)
export interface QueryResultData {
  columns: string[];
  rows: string[][];
  totalRows?: number;
}

// 履歴タブの 1 行
export interface QueryHistoryRow {
  id: string;
  state: QueryRunState;
  stateLabel: string;
  sql: string;
  elapsedMs: number;
  bytes: number;
  startedAt: string;
  location?: string;
  outputLocation?: string;
}

export interface AthenaTableRow {
  name: string;
  type: string;
  columns: { name: string; type: string }[];
  partitionKeys: { name: string; type: string }[];
}

// ============================================================
// スニペット (backend のファイル保存 API)
// ============================================================
export interface SnippetRaw {
  name: string;
  sql: string;
  updated_at: string;
}

// ============================================================
// エディタ UI 状態 (localStorage 永続化対象)
// ============================================================
export interface QueryTab {
  id: string;
  name: string;
  sql: string;
}

// スニペット / 保存クエリ共通の表示形。
// 保存クエリは localStorage、スニペットは backend のファイル保存 API が実体。
export interface NamedQuery {
  id: string;
  name: string;
  sql: string;
  updatedAt: string;
}

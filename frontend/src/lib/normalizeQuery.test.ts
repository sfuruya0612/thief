import { describe, expect, it } from 'vitest';
import {
  athenaExecutionFromRaw,
  athenaResultsFromPages,
  athenaRunState,
  athenaTableFromRaw,
  bqJobStatusFromRaw,
  bqResultsFromPages,
  bqRunState,
  snippetFromRaw,
} from './normalizeQuery';

describe('bqRunState', () => {
  it('DONE でエラー無しは succeeded', () => {
    expect(bqRunState('DONE')).toBe('succeeded');
  });
  it('DONE でエラー有りは failed', () => {
    expect(bqRunState('DONE', 'syntax error')).toBe('failed');
  });
  it('RUNNING は running、それ以外は queued', () => {
    expect(bqRunState('RUNNING')).toBe('running');
    expect(bqRunState('PENDING')).toBe('queued');
    expect(bqRunState('')).toBe('queued');
  });
});

describe('bqJobStatusFromRaw', () => {
  it('完了ジョブを日本語ラベル付きで正規化する', () => {
    const row = bqJobStatusFromRaw({
      job_id: 'job1',
      location: 'US',
      state: 'DONE',
      elapsed_ms: 2300,
      total_bytes_processed: 1_240_000_000,
      cache_hit: false,
      start_time: '2026-07-16T12:00:00Z',
    });
    expect(row).toMatchObject({
      id: 'job1',
      state: 'succeeded',
      stateLabel: '完了',
      elapsedMs: 2300,
      bytes: 1_240_000_000,
      location: 'US',
    });
  });
});

describe('bqResultsFromPages', () => {
  it('複数ページの行を連結し先頭ページの total_rows を使う', () => {
    const data = bqResultsFromPages([
      { columns: ['a'], rows: [['1'], ['2']], total_rows: 3, next_page_token: 't' },
      { columns: ['a'], rows: [['3']], total_rows: 3 },
    ]);
    expect(data.columns).toEqual(['a']);
    expect(data.rows).toEqual([['1'], ['2'], ['3']]);
    expect(data.totalRows).toBe(3);
  });
  it('null の columns / rows を空として扱う', () => {
    const data = bqResultsFromPages([{ columns: null, rows: null, total_rows: 0 }]);
    expect(data.columns).toEqual([]);
    expect(data.rows).toEqual([]);
  });
});

describe('athenaRunState', () => {
  it('各状態を共通 5 状態へ写像する', () => {
    expect(athenaRunState('SUCCEEDED')).toBe('succeeded');
    expect(athenaRunState('FAILED')).toBe('failed');
    expect(athenaRunState('CANCELLED')).toBe('cancelled');
    expect(athenaRunState('RUNNING')).toBe('running');
    expect(athenaRunState('QUEUED')).toBe('queued');
    expect(athenaRunState('')).toBe('queued');
  });
});

describe('athenaExecutionFromRaw', () => {
  it('実行情報を英語ラベルのまま正規化する', () => {
    const row = athenaExecutionFromRaw({
      id: 'exec1',
      sql: 'SELECT 1',
      state: 'SUCCEEDED',
      elapsed_ms: 6000,
      bytes_scanned: 890_000_000,
      output_location: 's3://bucket/out/',
    });
    expect(row).toMatchObject({
      id: 'exec1',
      state: 'succeeded',
      stateLabel: 'SUCCEEDED',
      elapsedMs: 6000,
      bytes: 890_000_000,
      outputLocation: 's3://bucket/out/',
    });
  });
});

describe('athenaResultsFromPages', () => {
  it('カラム名を取り出し行を連結する', () => {
    const data = athenaResultsFromPages([
      {
        columns: [
          { name: 'status', type: 'integer' },
          { name: 'requests', type: 'bigint' },
        ],
        rows: [['502', '1204']],
        next_token: 't',
      },
      { columns: null, rows: [['503', '377']] },
    ]);
    expect(data.columns).toEqual(['status', 'requests']);
    expect(data.rows).toEqual([
      ['502', '1204'],
      ['503', '377'],
    ]);
  });
});

describe('athenaTableFromRaw', () => {
  it('カラムとパーティション列を分離したまま正規化する', () => {
    const row = athenaTableFromRaw({
      name: 'alb_logs',
      type: 'EXTERNAL_TABLE',
      columns: [{ name: 'status', type: 'int' }],
      partition_keys: [{ name: 'date', type: 'string' }],
    });
    expect(row.columns).toEqual([{ name: 'status', type: 'int' }]);
    expect(row.partitionKeys).toEqual([{ name: 'date', type: 'string' }]);
  });
  it('null の配列を空配列にする', () => {
    const row = athenaTableFromRaw({
      name: 't',
      type: '',
      columns: null,
      partition_keys: null,
    });
    expect(row.columns).toEqual([]);
    expect(row.partitionKeys).toEqual([]);
  });
});

describe('snippetFromRaw', () => {
  it('ファイル名を id と name に使い snake_case を camelCase へ変換する', () => {
    const row = snippetFromRaw({
      name: 'monthly cost',
      sql: 'SELECT 1',
      updated_at: '2026-07-17T00:00:00Z',
    });
    expect(row).toEqual({
      id: 'monthly cost',
      name: 'monthly cost',
      sql: 'SELECT 1',
      updatedAt: '2026-07-17T00:00:00Z',
    });
  });
});

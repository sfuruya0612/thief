import { describe, expect, it } from 'vitest';
import type { LogEntryRow } from '../types/gcp';
import { availableSummaryFieldKeys, buildSummaryText } from './logSummaryFields';

function row(overrides: Partial<LogEntryRow> = {}): LogEntryRow {
  return {
    id: '1',
    timestamp: '2026-07-18T03:04:05.678Z',
    severity: 'ERROR',
    logName: 'projects/p/logs/l',
    resourceType: 'cloud_run_revision',
    resourceLabels: {},
    labels: {},
    payload: '{"message":"boom","route":"/v1/pay"}',
    insertId: 'abc',
    trace: '',
    ...overrides,
  };
}

describe('availableSummaryFieldKeys', () => {
  it('jsonPayload / resource.type / labels / trace のキーを列挙する', () => {
    const keys = availableSummaryFieldKeys([
      row({
        labels: { pod: 'p-1' },
        trace: 'projects/p/traces/t1',
      }),
    ]);
    expect(keys).toEqual([
      'jsonPayload.message',
      'jsonPayload.route',
      'resource.type',
      'labels.pod',
      'trace',
    ]);
  });

  it('複数行にまたがるキーを出現順を保ったまま重複なく集約する', () => {
    const keys = availableSummaryFieldKeys([
      row({ payload: '{"a":1}', resourceType: '' }),
      row({ payload: '{"b":2,"a":3}', resourceType: '' }),
    ]);
    expect(keys).toEqual(['jsonPayload.a', 'jsonPayload.b']);
  });

  it('payload が JSON オブジェクトでない場合は jsonPayload 系のキーを含めない', () => {
    const keys = availableSummaryFieldKeys([row({ payload: 'plain text message' })]);
    expect(keys).toEqual(['resource.type']);
  });

  it('resourceType / labels / trace が空の場合はそのキーを含めない', () => {
    const keys = availableSummaryFieldKeys([
      row({ payload: '{}', resourceType: '', labels: {}, trace: '' }),
    ]);
    expect(keys).toEqual([]);
  });
});

describe('buildSummaryText', () => {
  it('フィールド未選択の場合は元の payload をそのまま返す', () => {
    expect(buildSummaryText(row(), [])).toBe('{"message":"boom","route":"/v1/pay"}');
  });

  it('選択したフィールドを選択順に key=value で連結する', () => {
    const text = buildSummaryText(row({ trace: 'projects/p/traces/t1' }), [
      'jsonPayload.route',
      'trace',
      'jsonPayload.message',
    ]);
    expect(text).toBe(
      'jsonPayload.route=/v1/pay  trace=projects/p/traces/t1  jsonPayload.message=boom',
    );
  });

  it('resource.type / labels.* を解決できる', () => {
    const text = buildSummaryText(row({ labels: { pod: 'p-1' } }), ['resource.type', 'labels.pod']);
    expect(text).toBe('resource.type=cloud_run_revision  labels.pod=p-1');
  });

  it('選択キーが行に存在しない場合はそのフィールドを飛ばす', () => {
    const text = buildSummaryText(row(), ['jsonPayload.route', 'jsonPayload.missing']);
    expect(text).toBe('jsonPayload.route=/v1/pay');
  });

  it('選択キーが 1 つも行に存在しない場合は元の payload をそのまま返す', () => {
    const text = buildSummaryText(row(), ['jsonPayload.missing', 'labels.missing']);
    expect(text).toBe('{"message":"boom","route":"/v1/pay"}');
  });
});

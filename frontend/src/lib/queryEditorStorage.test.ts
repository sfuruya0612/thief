import { beforeEach, describe, expect, it } from 'vitest';
import {
  loadAthenaContext,
  loadEditorTabs,
  loadNamedQueries,
  saveAthenaContext,
  saveEditorTabs,
  saveNamedQueries,
  untitledName,
} from './queryEditorStorage';

beforeEach(() => {
  localStorage.clear();
});

describe('loadEditorTabs', () => {
  it('未保存時は defaultSql を持つ 1 タブを返す', () => {
    const state = loadEditorTabs('bigquery', 'proj-1', 'SELECT 1');
    expect(state.tabs).toHaveLength(1);
    expect(state.tabs[0].name).toBe('untitled 1');
    expect(state.tabs[0].sql).toBe('SELECT 1');
    expect(state.activeTabId).toBe(state.tabs[0].id);
  });

  it('保存済みタブ状態を復元する', () => {
    const tabs = [
      { id: 't1', name: 'daily_kpi.sql', sql: 'SELECT 1' },
      { id: 't2', name: 'untitled 2', sql: 'SELECT 2' },
    ];
    saveEditorTabs('bigquery', 'proj-1', { tabs, activeTabId: 't2' });
    const state = loadEditorTabs('bigquery', 'proj-1', '');
    expect(state.tabs).toEqual(tabs);
    expect(state.activeTabId).toBe('t2');
  });

  it('activeTabId が存在しないタブを指す場合は先頭タブへフォールバックする', () => {
    saveEditorTabs('bigquery', 'proj-1', {
      tabs: [{ id: 't1', name: 'a', sql: '' }],
      activeTabId: 'gone',
    });
    const state = loadEditorTabs('bigquery', 'proj-1', '');
    expect(state.activeTabId).toBe('t1');
  });

  it('スコープごとに独立して保存される', () => {
    saveEditorTabs('bigquery', 'proj-1', {
      tabs: [{ id: 't1', name: 'a', sql: 'A' }],
      activeTabId: 't1',
    });
    const other = loadEditorTabs('bigquery', 'proj-2', 'DEFAULT');
    expect(other.tabs[0].sql).toBe('DEFAULT');
  });
});

describe('untitledName', () => {
  it('既存の untitled 連番の次を返す', () => {
    expect(
      untitledName([
        { id: 't1', name: 'untitled 1', sql: '' },
        { id: 't2', name: 'daily.sql', sql: '' },
        { id: 't3', name: 'untitled 3', sql: '' },
      ]),
    ).toBe('untitled 4');
  });
  it('untitled タブが無ければ untitled 1', () => {
    expect(untitledName([{ id: 't1', name: 'a.sql', sql: '' }])).toBe('untitled 1');
  });
});

describe('loadNamedQueries / saveNamedQueries', () => {
  it('サービスとスコープごとに別キーへ保存される', () => {
    saveNamedQueries('athena', 'prof-1', 'saved', [
      { id: 's1', name: 'q', sql: 'SELECT 1', updatedAt: '2026-07-16' },
    ]);
    expect(loadNamedQueries('athena', 'prof-1', 'saved')).toHaveLength(1);
    expect(loadNamedQueries('athena', 'prof-2', 'saved')).toHaveLength(0);
    expect(loadNamedQueries('bigquery', 'prof-1', 'saved')).toHaveLength(0);
  });
  it('不正な要素を除外する', () => {
    localStorage.setItem(
      'cloudlens:qe:v1:athena:prof-1:saved',
      JSON.stringify([{ id: 's1', name: 'ok', sql: 'SELECT 1', updatedAt: '' }, { broken: true }]),
    );
    expect(loadNamedQueries('athena', 'prof-1', 'saved')).toHaveLength(1);
  });
});

describe('loadAthenaContext / saveAthenaContext', () => {
  it('プロファイルごとにセレクタ状態と出力先を保持する', () => {
    saveAthenaContext('prof-1', {
      catalog: 'AwsDataCatalog',
      database: 'db1',
      workgroup: 'primary',
      outputLocation: 's3://bucket/results/',
    });
    expect(loadAthenaContext('prof-1')).toEqual({
      catalog: 'AwsDataCatalog',
      database: 'db1',
      workgroup: 'primary',
      outputLocation: 's3://bucket/results/',
    });
    expect(loadAthenaContext('prof-2')).toEqual({});
  });
});

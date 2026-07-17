import { beforeEach, describe, expect, it } from 'vitest';
import { STORAGE_KEY, loadPersisted, savePersisted } from './storage';

const setRaw = (value: unknown) => localStorage.setItem(STORAGE_KEY, JSON.stringify(value));

describe('loadPersisted のセッションマイグレーション', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('旧形式 (activeProfile / gcpProject のみ) から単一タブを生成する', () => {
    setRaw({ activeProfile: 'prof-a', gcpProject: 'proj-x' });
    const state = loadPersisted();
    expect(state.awsSessions).toEqual({ open: ['prof-a'], active: 'prof-a' });
    expect(state.gcpSessions).toEqual({ open: ['proj-x'], active: 'proj-x' });
  });

  it('旧形式も無いフレッシュ状態ではセッションを生成しない', () => {
    const state = loadPersisted();
    expect(state.awsSessions).toBeUndefined();
    expect(state.gcpSessions).toBeUndefined();
  });

  it('awsSessions 定義済みで active と一致する activeProfile は変化を起こさない', () => {
    setRaw({
      activeProfile: 'b',
      awsSessions: { open: ['a', 'b'], active: 'b' },
    });
    expect(loadPersisted().awsSessions).toEqual({ open: ['a', 'b'], active: 'b' });
  });

  it('旧バージョンで選択変更された activeProfile をタブ集合へ合流させる', () => {
    // 新版が {open:[a,b], active:b} を書いた後、旧版で d を選択したケース
    setRaw({
      activeProfile: 'd',
      awsSessions: { open: ['a', 'b'], active: 'b' },
    });
    expect(loadPersisted().awsSessions).toEqual({ open: ['a', 'b', 'd'], active: 'd' });
  });

  it('旧バージョンで既オープンのタブへ切替されたケースは activate のみ', () => {
    setRaw({
      activeProfile: 'a',
      awsSessions: { open: ['a', 'b'], active: 'b' },
    });
    expect(loadPersisted().awsSessions).toEqual({ open: ['a', 'b'], active: 'a' });
  });

  it('全タブ閉状態 (open=[]) は旧フィールドが無ければ維持される', () => {
    setRaw({ awsSessions: { open: [], active: '' } });
    expect(loadPersisted().awsSessions).toEqual({ open: [], active: '' });
  });

  it('破損した awsSessions を正規化する', () => {
    setRaw({ awsSessions: { open: ['a', 42, 'a', ''], active: 'zzz' } });
    expect(loadPersisted().awsSessions).toEqual({ open: ['a'], active: 'a' });
  });

  it('非文字列の activeProfile では生成しない', () => {
    setRaw({ activeProfile: 42 });
    expect(loadPersisted().awsSessions).toBeUndefined();
  });

  it('冪等である (load → save → load で結果が変わらない)', () => {
    setRaw({
      activeProfile: 'd',
      awsSessions: { open: ['a', 'b'], active: 'b' },
    });
    const first = loadPersisted();
    savePersisted(first);
    const second = loadPersisted();
    expect(second.awsSessions).toEqual(first.awsSessions);
  });

  it("既存の view 'bigquery' → 'gcp' マイグレーションと共存する", () => {
    setRaw({ view: 'bigquery', activeProfile: 'a' });
    const state = loadPersisted();
    expect(state.view).toBe('gcp');
    expect(state.awsSessions).toEqual({ open: ['a'], active: 'a' });
  });

  it('JSON 破損時は空状態にフォールバックする', () => {
    localStorage.setItem(STORAGE_KEY, '{not json');
    expect(loadPersisted()).toEqual({});
  });
});

import { act, renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it } from 'vitest';
import { STORAGE_KEY, type PersistedState } from '../lib/storage';
import { useSessionTabs } from './useSessionTabs';

const readRaw = (): PersistedState => JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}');

describe('useSessionTabs', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('localStorage から初期状態を復元する', () => {
    localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({ awsSessions: { open: ['a', 'b'], active: 'b' } }),
    );
    const { result } = renderHook(() => useSessionTabs('awsSessions'));
    expect(result.current.open).toEqual(['a', 'b']);
    expect(result.current.active).toBe('b');
  });

  it('openSession でタブが追加されアクティブ + ミラーが保存される', () => {
    const { result } = renderHook(() => useSessionTabs('awsSessions'));
    act(() => result.current.openSession('prof-a'));
    act(() => result.current.openSession('prof-b'));

    expect(result.current.open).toEqual(['prof-a', 'prof-b']);
    expect(result.current.active).toBe('prof-b');
    const raw = readRaw();
    expect(raw.awsSessions).toEqual({ open: ['prof-a', 'prof-b'], active: 'prof-b' });
    expect(raw.activeProfile).toBe('prof-b');
  });

  it('全タブを閉じるとミラーフィールドが削除される', () => {
    const { result } = renderHook(() => useSessionTabs('awsSessions'));
    act(() => result.current.openSession('prof-a'));
    expect(readRaw().activeProfile).toBe('prof-a');

    act(() => result.current.closeSession('prof-a'));
    const raw = readRaw();
    expect(raw.awsSessions).toEqual({ open: [], active: '' });
    expect(raw.activeProfile).toBeUndefined();
  });

  it('gcpSessions スコープは gcpProject をミラーし aws と分離される', () => {
    const aws = renderHook(() => useSessionTabs('awsSessions'));
    const gcp = renderHook(() => useSessionTabs('gcpSessions'));
    act(() => aws.result.current.openSession('prof-a'));
    act(() => gcp.result.current.openSession('proj-x'));

    const raw = readRaw();
    expect(raw.awsSessions?.open).toEqual(['prof-a']);
    expect(raw.gcpSessions?.open).toEqual(['proj-x']);
    expect(raw.activeProfile).toBe('prof-a');
    expect(raw.gcpProject).toBe('proj-x');
  });

  it('他フィールド (region 等) を保存時に消さない', () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ region: 'us-east-1' }));
    const { result } = renderHook(() => useSessionTabs('awsSessions'));
    act(() => result.current.openSession('prof-a'));
    expect(readRaw().region).toBe('us-east-1');
  });
});

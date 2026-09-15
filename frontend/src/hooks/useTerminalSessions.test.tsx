import { act, renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it } from 'vitest';
import {
  activateTerminalSession,
  closeTerminalSession,
  openTerminalSession,
  resetTerminalSessionsForTest,
  setTerminalDockCollapsed,
  useTerminalSessions,
  type TerminalSession,
} from './useTerminalSessions';
import { closeSession } from '../lib/sessionTabsState';

const EC2_INPUT: Omit<TerminalSession, 'id'> = {
  kind: 'ec2',
  profile: 'test-profile',
  region: 'ap-northeast-1',
  label: 'web-01',
  wsUrl:
    'ws://127.0.0.1:8089/api/aws/profiles/test-profile/ec2/i-0001/session?region=ap-northeast-1',
};

describe('useTerminalSessions', () => {
  beforeEach(() => {
    resetTerminalSessionsForTest();
  });

  it('初期状態はセッション 0 で展開されている', () => {
    const { result } = renderHook(() => useTerminalSessions());

    expect(result.current.tabs).toEqual({ open: [], active: '' });
    expect(result.current.sessions).toEqual({});
    expect(result.current.collapsed).toBe(false);
  });

  it('openTerminalSession が返した id がアクティブになる', () => {
    const { result } = renderHook(() => useTerminalSessions());

    let id = '';
    act(() => {
      id = openTerminalSession(EC2_INPUT);
    });

    expect(result.current.tabs.open).toEqual([id]);
    expect(result.current.tabs.active).toBe(id);
    expect(result.current.sessions[id]).toEqual({ ...EC2_INPUT, id });
  });

  it('折りたたみ中に開くと展開される', () => {
    const { result } = renderHook(() => useTerminalSessions());

    act(() => {
      openTerminalSession(EC2_INPUT);
      setTerminalDockCollapsed(true);
    });
    expect(result.current.collapsed).toBe(true);

    act(() => {
      openTerminalSession(EC2_INPUT);
    });

    expect(result.current.collapsed).toBe(false);
  });

  it('同じ input で 2 回開くと異なる id の 2 セッションになる', () => {
    const { result } = renderHook(() => useTerminalSessions());

    let first = '';
    let second = '';
    act(() => {
      first = openTerminalSession(EC2_INPUT);
      second = openTerminalSession(EC2_INPUT);
    });

    expect(first).not.toBe(second);
    expect(result.current.tabs.open).toEqual([first, second]);
    expect(result.current.tabs.active).toBe(second);
    expect(Object.keys(result.current.sessions)).toHaveLength(2);
  });

  it('アクティブなセッションを閉じると sessionTabsState の closeSession の規則で次が選ばれる', () => {
    const { result } = renderHook(() => useTerminalSessions());

    let first = '';
    let second = '';
    let third = '';
    act(() => {
      first = openTerminalSession(EC2_INPUT);
      second = openTerminalSession(EC2_INPUT);
      third = openTerminalSession(EC2_INPUT);
      activateTerminalSession(second);
    });
    expect(result.current.tabs.active).toBe(second);

    act(() => {
      closeTerminalSession(second);
    });

    // 純関数側の規則 (左隣、無ければ先頭) と一致すること
    const expected = closeSession({ open: [first, second, third], active: second }, second);
    expect(result.current.tabs.open).toEqual([first, third]);
    expect(result.current.tabs.active).toBe(first);
    expect(result.current.tabs.active).toBe(expected.active);
    expect(result.current.sessions[second]).toBeUndefined();
  });

  it('非アクティブなセッションを閉じてもアクティブは変わらない', () => {
    const { result } = renderHook(() => useTerminalSessions());

    let first = '';
    let second = '';
    act(() => {
      first = openTerminalSession(EC2_INPUT);
      second = openTerminalSession(EC2_INPUT);
    });

    act(() => {
      closeTerminalSession(first);
    });

    expect(result.current.tabs.open).toEqual([second]);
    expect(result.current.tabs.active).toBe(second);
  });

  it('最後のセッションを閉じると tabs.open が空になる', () => {
    const { result } = renderHook(() => useTerminalSessions());

    let id = '';
    act(() => {
      id = openTerminalSession(EC2_INPUT);
    });

    act(() => {
      closeTerminalSession(id);
    });

    expect(result.current.tabs).toEqual({ open: [], active: '' });
    expect(result.current.sessions).toEqual({});
  });

  it('存在しない id を closeTerminalSession に渡しても状態が変わらない', () => {
    const { result } = renderHook(() => useTerminalSessions());

    let id = '';
    act(() => {
      id = openTerminalSession(EC2_INPUT);
    });
    const before = result.current;

    act(() => {
      closeTerminalSession('terminal-does-not-exist');
    });

    expect(result.current).toBe(before);
    expect(result.current.tabs.open).toEqual([id]);
    expect(result.current.sessions[id]).toEqual({ ...EC2_INPUT, id });
  });

  it('activateTerminalSession でアクティブが変わる', () => {
    const { result } = renderHook(() => useTerminalSessions());

    let first = '';
    act(() => {
      first = openTerminalSession(EC2_INPUT);
      openTerminalSession({ ...EC2_INPUT, kind: 'ecs', label: 'cluster / abc / app' });
    });

    act(() => {
      activateTerminalSession(first);
    });

    expect(result.current.tabs.active).toBe(first);
  });

  it('setTerminalDockCollapsed で折りたたみ状態が切り替わる', () => {
    const { result } = renderHook(() => useTerminalSessions());

    act(() => {
      setTerminalDockCollapsed(true);
    });
    expect(result.current.collapsed).toBe(true);

    act(() => {
      setTerminalDockCollapsed(false);
    });
    expect(result.current.collapsed).toBe(false);
  });

  it('別インスタンスの操作が全インスタンスへ即時反映される', () => {
    const dock = renderHook(() => useTerminalSessions());
    const other = renderHook(() => useTerminalSessions());

    let id = '';
    act(() => {
      id = openTerminalSession(EC2_INPUT);
    });

    expect(dock.result.current.tabs.active).toBe(id);
    expect(other.result.current.tabs.active).toBe(id);
  });
});

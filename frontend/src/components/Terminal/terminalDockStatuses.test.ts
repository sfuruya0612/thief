import { describe, expect, it } from 'vitest';
import { pruneStatuses } from './terminalDockStatuses';

describe('pruneStatuses', () => {
  it('tabs.open に無い id のエントリを削除する', () => {
    const result = pruneStatuses({ 'terminal-1': 'connected', 'terminal-2': 'closed' }, [
      'terminal-1',
    ]);

    expect(result).toEqual({ 'terminal-1': 'connected' });
  });

  it('削除対象が無ければ同じ参照を返す', () => {
    const statuses = { 'terminal-1': 'connected' as const };

    const result = pruneStatuses(statuses, ['terminal-1', 'terminal-2']);

    expect(result).toBe(statuses);
  });

  it('statuses が空でも同じ参照を返す', () => {
    const statuses = {};

    const result = pruneStatuses(statuses, ['terminal-1']);

    expect(result).toBe(statuses);
  });

  it('openIds が空なら全エントリを削除する', () => {
    const result = pruneStatuses({ 'terminal-1': 'connected', 'terminal-2': 'error' }, []);

    expect(result).toEqual({});
  });
});

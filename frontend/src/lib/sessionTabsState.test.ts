import { describe, expect, it } from 'vitest';
import {
  EMPTY_SESSIONS,
  activateSession,
  closeSession,
  moveSession,
  nextEnabledIndex,
  normalizeSessionState,
  openSession,
  swapSessionToVisible,
  type SessionTabsState,
} from './sessionTabsState';

const s = (open: string[], active: string): SessionTabsState => ({ open, active });

describe('openSession', () => {
  it('未オープンの id を末尾に追加してアクティブにする', () => {
    expect(openSession(s(['a'], 'a'), 'b')).toEqual(s(['a', 'b'], 'b'));
  });

  it('既にオープン済みの id はアクティブ化のみで重複追加しない', () => {
    expect(openSession(s(['a', 'b'], 'a'), 'b')).toEqual(s(['a', 'b'], 'b'));
  });

  it('空文字は no-op', () => {
    expect(openSession(EMPTY_SESSIONS, '')).toEqual(EMPTY_SESSIONS);
  });
});

describe('activateSession', () => {
  it('open に含まれる id をアクティブにする', () => {
    expect(activateSession(s(['a', 'b'], 'a'), 'b')).toEqual(s(['a', 'b'], 'b'));
  });

  it('open に無い id は no-op', () => {
    expect(activateSession(s(['a'], 'a'), 'zzz')).toEqual(s(['a'], 'a'));
  });
});

describe('closeSession', () => {
  it('アクティブでないタブを閉じてもアクティブは変わらない', () => {
    expect(closeSession(s(['a', 'b', 'c'], 'a'), 'b')).toEqual(s(['a', 'c'], 'a'));
  });

  it('アクティブタブを閉じると左隣がアクティブになる', () => {
    expect(closeSession(s(['a', 'b', 'c'], 'b'), 'b')).toEqual(s(['a', 'c'], 'a'));
  });

  it('先頭のアクティブタブを閉じると新しい先頭がアクティブになる', () => {
    expect(closeSession(s(['a', 'b'], 'a'), 'a')).toEqual(s(['b'], 'b'));
  });

  it('最後の 1 個を閉じると空状態になる', () => {
    expect(closeSession(s(['a'], 'a'), 'a')).toEqual(EMPTY_SESSIONS);
  });

  it('open に無い id は no-op', () => {
    expect(closeSession(s(['a'], 'a'), 'zzz')).toEqual(s(['a'], 'a'));
  });
});

describe('moveSession', () => {
  it('前方から後方へ移動する', () => {
    expect(moveSession(s(['a', 'b', 'c'], 'a'), 0, 2)).toEqual(s(['b', 'c', 'a'], 'a'));
  });

  it('後方から前方へ移動する', () => {
    expect(moveSession(s(['a', 'b', 'c'], 'a'), 2, 0)).toEqual(s(['c', 'a', 'b'], 'a'));
  });

  it('範囲外 index は no-op', () => {
    const st = s(['a', 'b'], 'a');
    expect(moveSession(st, -1, 0)).toEqual(st);
    expect(moveSession(st, 0, 2)).toEqual(st);
  });
});

describe('swapSessionToVisible', () => {
  it('隠れ側の id を右端の表示位置と入替えてアクティブにする', () => {
    // visibleCount=2 → 表示 [a, b] / 隠れ [c, d]。d を選ぶと b と入替。
    expect(swapSessionToVisible(s(['a', 'b', 'c', 'd'], 'a'), 'd', 2)).toEqual(
      s(['a', 'd', 'c', 'b'], 'd'),
    );
  });

  it('表示域内の id はアクティブ化のみ', () => {
    expect(swapSessionToVisible(s(['a', 'b', 'c'], 'a'), 'b', 2)).toEqual(s(['a', 'b', 'c'], 'b'));
  });

  it('visibleCount が open 数を超えていても clamp されて壊れない', () => {
    expect(swapSessionToVisible(s(['a', 'b'], 'a'), 'b', 99)).toEqual(s(['a', 'b'], 'b'));
  });

  it('visibleCount が 0 以下でも最低 1 に clamp される', () => {
    expect(swapSessionToVisible(s(['a', 'b', 'c'], 'a'), 'c', 0)).toEqual(s(['c', 'b', 'a'], 'c'));
  });

  it('open に無い id は no-op', () => {
    expect(swapSessionToVisible(s(['a'], 'a'), 'zzz', 1)).toEqual(s(['a'], 'a'));
  });
});

describe('normalizeSessionState', () => {
  it('重複と空文字を除去する', () => {
    expect(normalizeSessionState(s(['a', 'b', 'a', ''], 'b'))).toEqual(s(['a', 'b'], 'b'));
  });

  it('active が open に無ければ先頭に補正する', () => {
    expect(normalizeSessionState(s(['a', 'b'], 'zzz'))).toEqual(s(['a', 'b'], 'a'));
  });

  it('open が空なら active も空になる', () => {
    expect(normalizeSessionState(s([], 'zzz'))).toEqual(EMPTY_SESSIONS);
  });

  it('非文字列 (手編集・破損データ) を除去する', () => {
    const broken = { open: ['a', 42, null, 'b'], active: 42 } as unknown as SessionTabsState;
    expect(normalizeSessionState(broken)).toEqual(s(['a', 'b'], 'a'));
  });

  it('open が配列でなくても空に補正する', () => {
    const broken = { open: 'oops', active: 'a' } as unknown as SessionTabsState;
    expect(normalizeSessionState(broken)).toEqual(EMPTY_SESSIONS);
  });

  it('冪等である (2 回適用しても同じ)', () => {
    const once = normalizeSessionState(s(['a', 'a', 'b'], 'x'));
    expect(normalizeSessionState(once)).toEqual(once);
  });
});

describe('nextEnabledIndex', () => {
  const items = [{ disabled: true }, {}, { disabled: true }, {}];

  it('下方向で disabled をスキップする', () => {
    expect(nextEnabledIndex(items, 1, 1)).toBe(3);
  });

  it('上方向で disabled をスキップしてラップする', () => {
    expect(nextEnabledIndex(items, 1, -1)).toBe(3);
  });

  it('未選択 (-1) からの移動は最初の有効行を返す', () => {
    expect(nextEnabledIndex(items, -1, 1)).toBe(1);
  });

  it('全行 disabled なら -1', () => {
    expect(nextEnabledIndex([{ disabled: true }, { disabled: true }], 0, 1)).toBe(-1);
  });

  it('空配列なら -1', () => {
    expect(nextEnabledIndex([], 0, 1)).toBe(-1);
  });
});

// 分割表示の状態遷移 (lib/splitPanes.ts) の検証 (issue 0175)。
// 設計判断 1 の規則を分岐ごとに固定する。変化が無い遷移が引数と同じ参照を返すことも
// 併せて検証する (React の再描画を抑えるための性質)。
import { describe, expect, it } from 'vitest';
import { closePane, focusPane, openSplit, selectService, type SplitPanesState } from './splitPanes';

const single: SplitPanesState = { services: ['ec2'], ids: [0], focused: 0 };
const split: SplitPanesState = { services: ['ec2', 'ssm'], ids: [0, 1], focused: 0 };

describe('selectService', () => {
  it('1 ペインのときはフォーカス中のペインのサービスを置き換える', () => {
    expect(selectService(single, 'ssm')).toEqual({ services: ['ssm'], ids: [0], focused: 0 });
  });

  it('もう一方のペインが表示中のサービスを選ぶとフォーカスだけが移る', () => {
    const state = selectService(split, 'ssm');

    expect(state).toEqual({ services: ['ec2', 'ssm'], ids: [0, 1], focused: 1 });
    // サービス自体は変わらない (同じサービスの二重表示をしない)
    expect(state.services).toEqual(split.services);
    // ペインの識別子も変わらない
    expect(state.ids).toEqual(split.ids);
  });

  it('フォーカス中のペインが表示中のサービスを選んでも同じ参照を返す', () => {
    expect(selectService(split, 'ec2')).toBe(split);
  });

  it('同じサービスの二重表示にならないよう、もう一方のペインが持つサービスは置き換えない', () => {
    const state: SplitPanesState = { services: ['ec2', null], ids: [0, 1], focused: 1 };

    // フォーカスしていないペイン (0) が ec2 を表示しているため、フォーカスが 0 へ移る
    expect(selectService(state, 'ec2')).toEqual({
      services: ['ec2', null],
      ids: [0, 1],
      focused: 0,
    });
  });

  it('分割中にフォーカス中のペインのサービスを置き換える', () => {
    expect(selectService(split, 's3')).toEqual({
      services: ['s3', 'ssm'],
      ids: [0, 1],
      focused: 0,
    });
  });

  it('範囲外のフォーカスを持つ状態は同じ参照を返す', () => {
    const state: SplitPanesState = { services: ['ec2'], ids: [0], focused: 1 };

    expect(selectService(state, 'ssm')).toBe(state);
  });

  it('負のフォーカスを持つ状態は同じ参照を返す', () => {
    const state: SplitPanesState = { services: ['ec2'], ids: [0], focused: -1 };

    expect(selectService(state, 'ssm')).toBe(state);
  });
});

describe('openSplit', () => {
  it('2 つ目のペインをサービス未選択・未使用の識別子で追加し、フォーカスを 2 つ目へ移す', () => {
    expect(openSplit(single)).toEqual({ services: ['ec2', null], ids: [0, 1], focused: 1 });
  });

  it('閉じたペインの識別子を再利用せず、重ならない値を追加する', () => {
    const reopened: SplitPanesState = { services: ['ssm'], ids: [1], focused: 0 };

    expect(openSplit(reopened)).toEqual({ services: ['ssm', null], ids: [1, 2], focused: 1 });
  });

  it('識別子が空の状態では 0 を追加する (Math.max の -Infinity を使わない)', () => {
    const state: SplitPanesState = { services: ['ec2'], ids: [], focused: 0 };

    expect(openSplit(state).ids).toEqual([0]);
  });

  it('既に 2 ペインなら同じ参照を返す', () => {
    expect(openSplit(split)).toBe(split);
  });
});

describe('closePane', () => {
  it('添字 0 のペインを閉じると残ったペインが 1 つ目になり、その識別子が保たれる', () => {
    expect(closePane(split, 0)).toEqual({ services: ['ssm'], ids: [1], focused: 0 });
  });

  it('添字 1 のペインを閉じると残ったペインが 1 つ目になり、その識別子が保たれる', () => {
    expect(closePane(split, 1)).toEqual({ services: ['ec2'], ids: [0], focused: 0 });
  });

  it('1 ペインのときは同じ参照を返す', () => {
    expect(closePane(single, 0)).toBe(single);
  });

  it('範囲外の添字は同じ参照を返す', () => {
    expect(closePane(split, 2)).toBe(split);
  });
});

describe('focusPane', () => {
  it('範囲内の添字へフォーカスを移す', () => {
    expect(focusPane(split, 1)).toEqual({ services: ['ec2', 'ssm'], ids: [0, 1], focused: 1 });
  });

  it('同じペインと範囲外の添字は同じ参照を返す', () => {
    expect(focusPane(split, 0)).toBe(split);
    expect(focusPane(split, -1)).toBe(split);
    expect(focusPane(split, 2)).toBe(split);
  });
});

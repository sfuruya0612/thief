import { describe, expect, it } from 'vitest';
import { SESSION_TAB_METRICS, computeVisibleTabCount } from './sessionTabsLayout';

const { tabWidth, gap, moreButtonWidth } = SESSION_TAB_METRICS;
const per = tabWidth + gap;

describe('computeVisibleTabCount', () => {
  it('全タブが収まるなら tabCount を返す', () => {
    expect(computeVisibleTabCount(per * 5, 5)).toBe(5);
  });

  it('溢れる場合は「他 N」ボタン幅を確保した本数を返す', () => {
    // 4 本ちょうどの幅に 5 本 → moreButton を引くと 3 本
    const width = per * 4;
    expect(computeVisibleTabCount(width, 5)).toBe(Math.floor((width - moreButtonWidth) / per));
  });

  it('幅 0 でも最低 1 本は表示する', () => {
    expect(computeVisibleTabCount(0, 5)).toBe(1);
  });

  it('溢れる場合の上限は tabCount - 1 (「他 N」に最低 1 本入る)', () => {
    // 全部は入らないが moreButton 控除後は tabCount 本入る境界
    const width = per * 5 - 1;
    expect(computeVisibleTabCount(width, 5)).toBe(4);
  });

  it('タブ 0 本なら 0', () => {
    expect(computeVisibleTabCount(1000, 0)).toBe(0);
  });

  it('タブ 1 本は幅に関わらず 1', () => {
    expect(computeVisibleTabCount(0, 1)).toBe(1);
  });
});

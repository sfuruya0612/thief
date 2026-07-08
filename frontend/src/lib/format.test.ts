import { describe, expect, it } from 'vitest';
import { formatMoney, formatUptime } from './format';

describe('formatUptime', () => {
  it('空文字なら空文字を返す', () => {
    expect(formatUptime('')).toBe('');
  });

  it('不正な日時なら空文字を返す', () => {
    expect(formatUptime('not-a-date')).toBe('');
  });
});

describe('formatMoney', () => {
  it('undefined は em dash を返す', () => {
    expect(formatMoney(undefined)).toBe('—');
  });

  it('0 は em dash を返す', () => {
    expect(formatMoney(0)).toBe('—');
  });

  it('100 以下は小数 2 桁で表示する', () => {
    expect(formatMoney(12.3)).toBe('$12.30');
  });

  it('100 超は整数で表示する', () => {
    expect(formatMoney(1234)).toBe('$1,234');
  });
});

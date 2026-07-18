import { describe, expect, it } from 'vitest';
import {
  formatFetchedAt,
  formatMoney,
  formatPricingUnit,
  formatUnitPrice,
  formatUptime,
} from './format';

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

describe('formatUnitPrice', () => {
  it('0 は em dash に隠さずそのまま表示する (All Upfront RI の時間単価 $0/hr)', () => {
    expect(formatUnitPrice(0)).toBe('$0.00');
  });

  it('最大 4 桁の小数まで表示する', () => {
    expect(formatUnitPrice(0.0864)).toBe('$0.0864');
  });

  it('末尾 0 が出ない場合は 2 桁までに丸める', () => {
    expect(formatUnitPrice(0.1)).toBe('$0.10');
  });
});

describe('formatPricingUnit', () => {
  it('Hrs は /時間 になる', () => {
    expect(formatPricingUnit('Hrs')).toBe('/時間');
  });

  it('vCPU-Hours は /vCPU時間 になる', () => {
    expect(formatPricingUnit('vCPU-Hours')).toBe('/vCPU時間');
  });

  it('GB-Hours は /GB時間 になる', () => {
    expect(formatPricingUnit('GB-Hours')).toBe('/GB時間');
  });

  it('未知の unit はそのまま /<unit> にする', () => {
    expect(formatPricingUnit('Quantity')).toBe('/Quantity');
  });
});

describe('formatFetchedAt', () => {
  it('RFC3339 を MM/DD HH:mm (ローカル時刻) に整形する', () => {
    const d = new Date(2026, 6, 18, 9, 5, 0); // 2026-07-18 09:05 ローカル
    expect(formatFetchedAt(d.toISOString())).toBe('07/18 09:05');
  });

  it('不正な日時は空文字を返す', () => {
    expect(formatFetchedAt('not-a-date')).toBe('');
  });
});

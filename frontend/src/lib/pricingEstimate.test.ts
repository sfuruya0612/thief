import { describe, expect, it } from 'vitest';
import type { PriceRateRow } from '../types/aws';
import {
  effectiveHourlyRate,
  estimate,
  HOURS_PER_MONTH,
  monthlyRecurring,
  savingsPercent,
  subtotal,
  type PriceSelectionByService,
  type PriceTablesByService,
} from './pricingEstimate';

function rate(overrides: Partial<PriceRateRow> = {}): PriceRateRow {
  return {
    rateId: 'sku.6YS6EN2CT7',
    model: 'on_demand',
    group: 'On-Demand',
    label: 'm5.large / Linux / Shared',
    attributes: { instance_type: 'm5.large' },
    term: { lease: null, offeringClass: null, payment: null },
    unit: 'Hrs',
    priceUSD: 0.096,
    upfrontUSD: 0,
    currency: 'USD',
    ...overrides,
  };
}

describe('subtotal', () => {
  it('On-Demand: 継続課金のみで前払いは 0', () => {
    const s = subtotal(2, rate({ priceUSD: 0.1 }));
    expect(s.recurringMonthly).toBeCloseTo(0.1 * HOURS_PER_MONTH * 2);
    expect(s.upfrontOnce).toBe(0);
    expect(s.effectiveMonthly).toBeCloseTo(s.recurringMonthly);
  });

  it('All Upfront RI (時間単価0/前払い>0): 実効月額は前払いの月割のみになる (1yr)', () => {
    const s = subtotal(
      1,
      rate({
        model: 'reserved',
        priceUSD: 0,
        upfrontUSD: 1200,
        term: { lease: '1yr', offeringClass: 'standard', payment: 'All Upfront' },
      }),
    );
    expect(s.recurringMonthly).toBe(0);
    expect(s.upfrontOnce).toBe(1200);
    expect(s.effectiveMonthly).toBeCloseTo(1200 / 12);
  });

  it('All Upfront RI (3yr): 契約月数 36 で月割する', () => {
    const s = subtotal(
      1,
      rate({
        model: 'reserved',
        priceUSD: 0,
        upfrontUSD: 3600,
        term: { lease: '3yr', offeringClass: 'standard', payment: 'All Upfront' },
      }),
    );
    expect(s.effectiveMonthly).toBeCloseTo(3600 / 36);
  });

  it('Partial Upfront RI: 継続課金と前払いの月割を両方加算する', () => {
    const s = subtotal(
      1,
      rate({
        model: 'reserved',
        priceUSD: 0.05,
        upfrontUSD: 600,
        term: { lease: '1yr', offeringClass: 'standard', payment: 'Partial Upfront' },
      }),
    );
    const recurring = 0.05 * HOURS_PER_MONTH;
    expect(s.recurringMonthly).toBeCloseTo(recurring);
    expect(s.upfrontOnce).toBe(600);
    expect(s.effectiveMonthly).toBeCloseTo(recurring + 600 / 12);
  });

  it('No Upfront RI: 前払いが 0 なので実効月額は継続課金と一致する', () => {
    const s = subtotal(
      1,
      rate({
        model: 'reserved',
        priceUSD: 0.08,
        upfrontUSD: 0,
        term: { lease: '1yr', offeringClass: 'standard', payment: 'No Upfront' },
      }),
    );
    expect(s.effectiveMonthly).toBeCloseTo(s.recurringMonthly);
  });

  it('Savings Plan: upfront_usd は常に 0 のため実効月額は継続課金と一致する', () => {
    const s = subtotal(
      1,
      rate({
        model: 'savings_plan',
        group: 'Compute Savings Plans',
        priceUSD: 0.07,
        upfrontUSD: 0,
        term: { lease: '3yr', offeringClass: null, payment: 'No Upfront' },
      }),
    );
    expect(s.upfrontOnce).toBe(0);
    expect(s.effectiveMonthly).toBeCloseTo(s.recurringMonthly);
  });

  it('qty=0 の場合はすべて 0 になる', () => {
    const s = subtotal(
      0,
      rate({
        priceUSD: 1,
        upfrontUSD: 100,
        term: { lease: '1yr', offeringClass: null, payment: 'Partial Upfront' },
      }),
    );
    expect(s).toEqual({ recurringMonthly: 0, upfrontOnce: 0, effectiveMonthly: 0 });
  });

  it('lease が未知の値 (契約月数を導出できない) でも 0 除算にならない', () => {
    const s = subtotal(
      1,
      rate({
        priceUSD: 0,
        upfrontUSD: 100,
        term: { lease: null, offeringClass: null, payment: null },
      }),
    );
    expect(Number.isFinite(s.effectiveMonthly)).toBe(true);
    expect(s.effectiveMonthly).toBe(0);
  });
});

// issue 0057: RI 単価表に前払い/月額/実効時間単価/節減率を表示するための算出関数。
describe('effectiveHourlyRate', () => {
  it('No Upfront: 前払いが 0 のため継続時間単価と一致する', () => {
    const r = rate({
      model: 'reserved',
      priceUSD: 0.08,
      upfrontUSD: 0,
      term: { lease: '1yr', offeringClass: 'standard', payment: 'No Upfront' },
    });
    expect(effectiveHourlyRate(r)).toBeCloseTo(0.08);
  });

  it('All Upfront (1yr): 継続時間単価が 0 でも前払いの按分だけで正しく算出される', () => {
    const r = rate({
      model: 'reserved',
      priceUSD: 0,
      upfrontUSD: 876, // 876 / (730 * 12) = 0.1
      term: { lease: '1yr', offeringClass: 'standard', payment: 'All Upfront' },
    });
    expect(effectiveHourlyRate(r)).toBeCloseTo(0.1);
  });

  it('All Upfront (3yr): 契約月数 36 (26280 時間) で按分する', () => {
    const r = rate({
      model: 'reserved',
      priceUSD: 0,
      upfrontUSD: 2628, // 2628 / (730 * 36) = 0.1
      term: { lease: '3yr', offeringClass: 'standard', payment: 'All Upfront' },
    });
    expect(effectiveHourlyRate(r)).toBeCloseTo(0.1);
  });

  it('Partial Upfront: 継続時間単価と前払いの按分を加算する', () => {
    const r = rate({
      model: 'reserved',
      priceUSD: 0.05,
      upfrontUSD: 438, // 438 / (730 * 12) = 0.05
      term: { lease: '1yr', offeringClass: 'standard', payment: 'Partial Upfront' },
    });
    expect(effectiveHourlyRate(r)).toBeCloseTo(0.1);
  });
});

describe('monthlyRecurring', () => {
  it('継続時間単価 × 730 を返す', () => {
    expect(monthlyRecurring(rate({ priceUSD: 0.1 }))).toBeCloseTo(0.1 * HOURS_PER_MONTH);
  });

  it('All Upfront (継続時間単価 0) では 0 になる', () => {
    expect(monthlyRecurring(rate({ priceUSD: 0, upfrontUSD: 1200 }))).toBe(0);
  });
});

describe('savingsPercent', () => {
  it('RI が On-Demand より安い場合は正の節減率を返す', () => {
    // 実効 0.08、On-Demand 0.1 → 20% 節減
    expect(savingsPercent(0.08, 0.1)).toBeCloseTo(20);
  });

  it('RI が On-Demand より高い異常値でも隠さず負の値を返す', () => {
    expect(savingsPercent(0.12, 0.1)).toBeCloseTo(-20);
  });

  it('同一 label の On-Demand が見つからない場合は null を返す', () => {
    expect(savingsPercent(0.08, undefined)).toBeNull();
  });

  it('On-Demand 時間単価が 0 以下の場合は算出不能として null を返す', () => {
    expect(savingsPercent(0.08, 0)).toBeNull();
  });
});

describe('estimate', () => {
  const table = {
    service: 'ec2',
    region: 'ap-northeast-1',
    fetchedAt: '2026-07-18T09:00:00Z',
    licenseUnresolved: false,
    rates: [
      rate({ rateId: 'od-1', model: 'on_demand', priceUSD: 0.1 }),
      rate({
        rateId: 'ri-1',
        model: 'reserved',
        priceUSD: 0,
        upfrontUSD: 1200,
        term: { lease: '1yr', offeringClass: 'standard', payment: 'All Upfront' },
      }),
    ],
  };

  it('On-Demand + RI 混在: サービス内訳が両方の小計を合算する', () => {
    const selection: PriceSelectionByService = {
      ec2: {
        'od-1': { checked: true, qty: 2 },
        'ri-1': { checked: true, qty: 1 },
      },
    };
    const rates: PriceTablesByService = { ec2: table };

    const result = estimate(selection, rates);

    const odRecurring = 0.1 * HOURS_PER_MONTH * 2;
    expect(result.byService).toHaveLength(1);
    expect(result.byService[0].service).toBe('ec2');
    expect(result.byService[0].recurringMonthly).toBeCloseTo(odRecurring);
    expect(result.byService[0].upfrontOnce).toBe(1200);
    expect(result.byService[0].effectiveMonthly).toBeCloseTo(odRecurring + 1200 / 12);
    expect(result.totalRecurringMonthly).toBeCloseTo(odRecurring);
    expect(result.totalUpfrontOnce).toBe(1200);
    expect(result.totalEffectiveMonthly).toBeCloseTo(odRecurring + 1200 / 12);
  });

  it('複数サービスにまたがる場合は合計に両方を含める', () => {
    const rdsTable = {
      ...table,
      service: 'rds',
      rates: [rate({ rateId: 'rds-od-1', model: 'on_demand', priceUSD: 0.2 })],
    };
    const selection: PriceSelectionByService = {
      ec2: { 'od-1': { checked: true, qty: 1 } },
      rds: { 'rds-od-1': { checked: true, qty: 1 } },
    };
    const rates: PriceTablesByService = { ec2: table, rds: rdsTable };

    const result = estimate(selection, rates);

    expect(result.byService.map((b) => b.service).sort()).toEqual(['ec2', 'rds']);
    expect(result.totalRecurringMonthly).toBeCloseTo(0.1 * HOURS_PER_MONTH + 0.2 * HOURS_PER_MONTH);
  });

  it('チェックが外れている行はスキップする', () => {
    const selection: PriceSelectionByService = {
      ec2: { 'od-1': { checked: false, qty: 5 } },
    };
    const result = estimate(selection, { ec2: table });
    expect(result.byService).toEqual([]);
    expect(result.totalRecurringMonthly).toBe(0);
  });

  it('現テーブルに存在しない rate_id は安全にスキップする (リージョン切替直後を想定)', () => {
    const selection: PriceSelectionByService = {
      ec2: {
        'od-1': { checked: true, qty: 1 },
        'stale-rate-id': { checked: true, qty: 3 },
      },
    };
    const result = estimate(selection, { ec2: table });
    expect(result.byService).toHaveLength(1);
    expect(result.byService[0].recurringMonthly).toBeCloseTo(0.1 * HOURS_PER_MONTH);
  });

  it('rates にテーブル自体が無いサービスは安全にスキップする', () => {
    const selection: PriceSelectionByService = {
      elasticache: { x: { checked: true, qty: 1 } },
    };
    const result = estimate(selection, {});
    expect(result).toEqual({
      byService: [],
      totalRecurringMonthly: 0,
      totalUpfrontOnce: 0,
      totalEffectiveMonthly: 0,
    });
  });

  it('選択が空の場合は空の見積もりを返す', () => {
    const result = estimate({}, { ec2: table });
    expect(result).toEqual({
      byService: [],
      totalRecurringMonthly: 0,
      totalUpfrontOnce: 0,
      totalEffectiveMonthly: 0,
    });
  });
});

import { describe, expect, it } from 'vitest';
import type { PriceRateRaw, PriceTableRaw, PriceTermRaw } from '../types/aws';
import { priceRateFromRaw, priceTableFromRaw, priceTermFromRaw } from './normalizePricing';

function term(overrides: Partial<PriceTermRaw> = {}): PriceTermRaw {
  return {
    lease: null,
    offering_class: null,
    payment: null,
    ...overrides,
  };
}

function rate(overrides: Partial<PriceRateRaw> = {}): PriceRateRaw {
  return {
    rate_id: 'sku.6YS6EN2CT7',
    model: 'on_demand',
    group: 'On-Demand',
    label: 'm5.large / Linux / Shared',
    attributes: { instance_type: 'm5.large', os: 'Linux' },
    term: term(),
    unit: 'Hrs',
    price_usd: 0.096,
    upfront_usd: 0,
    currency: 'USD',
    ...overrides,
  };
}

describe('priceTermFromRaw', () => {
  it('on-demand の全 null な term をそのまま変換する', () => {
    expect(priceTermFromRaw(term())).toEqual({
      lease: null,
      offeringClass: null,
      payment: null,
    });
  });

  it('offering_class を offeringClass にマップする', () => {
    expect(
      priceTermFromRaw(term({ lease: '1yr', offering_class: 'standard', payment: 'All Upfront' })),
    ).toEqual({
      lease: '1yr',
      offeringClass: 'standard',
      payment: 'All Upfront',
    });
  });
});

describe('priceRateFromRaw', () => {
  it('snake_case のフィールドを camelCase に変換する', () => {
    expect(priceRateFromRaw(rate())).toEqual({
      rateId: 'sku.6YS6EN2CT7',
      model: 'on_demand',
      group: 'On-Demand',
      label: 'm5.large / Linux / Shared',
      attributes: { instance_type: 'm5.large', os: 'Linux' },
      term: { lease: null, offeringClass: null, payment: null },
      unit: 'Hrs',
      priceUSD: 0.096,
      upfrontUSD: 0,
      currency: 'USD',
    });
  });

  it('RI の term / upfront_usd をネストしたまま変換する', () => {
    const row = priceRateFromRaw(
      rate({
        model: 'reserved',
        group: 'Reserved Instance',
        term: term({ lease: '3yr', offering_class: 'convertible', payment: 'All Upfront' }),
        price_usd: 0,
        upfront_usd: 3500.5,
      }),
    );
    expect(row.model).toBe('reserved');
    expect(row.term).toEqual({
      lease: '3yr',
      offeringClass: 'convertible',
      payment: 'All Upfront',
    });
    expect(row.priceUSD).toBe(0);
    expect(row.upfrontUSD).toBe(3500.5);
  });
});

describe('priceTableFromRaw', () => {
  it('テーブル全体と rates 配列をまとめて変換する', () => {
    const raw: PriceTableRaw = {
      service: 'ec2',
      region: 'ap-northeast-1',
      fetched_at: '2026-07-18T09:00:00Z',
      license_unresolved: false,
      rates: [
        rate(),
        rate({ rate_id: 'sku2.term2', model: 'reserved', group: 'Reserved Instance' }),
      ],
    };

    const row = priceTableFromRaw(raw);
    expect(row.service).toBe('ec2');
    expect(row.region).toBe('ap-northeast-1');
    expect(row.fetchedAt).toBe('2026-07-18T09:00:00Z');
    expect(row.licenseUnresolved).toBe(false);
    expect(row.rates).toHaveLength(2);
    expect(row.rates[0].rateId).toBe('sku.6YS6EN2CT7');
    expect(row.rates[1].rateId).toBe('sku2.term2');
  });

  it('license_unresolved を素通しする (SP のライセンス区別が縮退した場合の表現)', () => {
    const raw: PriceTableRaw = {
      service: 'compute-sp',
      region: 'ap-northeast-1',
      fetched_at: '2026-07-18T09:00:00Z',
      license_unresolved: true,
      rates: [],
    };

    const row = priceTableFromRaw(raw);
    expect(row.licenseUnresolved).toBe(true);
    expect(row.rates).toEqual([]);
  });
});

import { describe, expect, it } from 'vitest';
import type { PriceRateRow } from '../types/aws';
import {
  attributeValueOptions,
  matchesAttributeSelection,
  PRICING_ATTRIBUTE_FILTERS,
} from './pricingAttributeFilters';

function rate(attributes: Record<string, string>): PriceRateRow {
  return {
    rateId: 'sku.test',
    model: 'on_demand',
    group: 'On-Demand',
    label: 'test',
    attributes,
    term: { lease: null, offeringClass: null, payment: null },
    unit: 'Hrs',
    priceUSD: 0.1,
    upfrontUSD: 0,
    currency: 'USD',
  };
}

describe('PRICING_ATTRIBUTE_FILTERS', () => {
  it('ecs はチップ絞り込み対象の属性を持たない', () => {
    expect(PRICING_ATTRIBUTE_FILTERS.ecs).toEqual([]);
  });

  it('rds は instance_family / engine / deployment_option / storage_type / license_model の 5 軸を持つ (issue 0054)', () => {
    expect(PRICING_ATTRIBUTE_FILTERS.rds.map((s) => s.key)).toEqual([
      'instance_family',
      'engine',
      'deployment_option',
      'storage_type',
      'license_model',
    ]);
  });

  it('rds の storage_type は表示用ラベルを持つ', () => {
    const spec = PRICING_ATTRIBUTE_FILTERS.rds.find((s) => s.key === 'storage_type');
    expect(spec?.valueLabels).toEqual({ standard: 'Standard', io_optimized: 'IO-Optimized' });
  });

  it('ec2 は instance_family / os / license_model の 3 軸を持つ (issue 0053, 0054)', () => {
    expect(PRICING_ATTRIBUTE_FILTERS.ec2.map((s) => s.key)).toEqual([
      'instance_family',
      'os',
      'license_model',
    ]);
  });

  it('elasticache は instance_family / engine の 2 軸を持つ (issue 0054)', () => {
    expect(PRICING_ATTRIBUTE_FILTERS.elasticache.map((s) => s.key)).toEqual([
      'instance_family',
      'engine',
    ]);
  });
});

describe('attributeValueOptions', () => {
  it('重複を除いたユニーク値を昇順で返す', () => {
    const rates = [rate({ os: 'Windows' }), rate({ os: 'Linux' }), rate({ os: 'Linux' })];
    expect(attributeValueOptions(rates, 'os')).toEqual(['Linux', 'Windows']);
  });

  it('該当キーが存在しない/空文字列の行は無視する', () => {
    const rates = [rate({ os: 'Linux' }), rate({}), rate({ os: '' })];
    expect(attributeValueOptions(rates, 'os')).toEqual(['Linux']);
  });

  it('rates が空なら空配列を返す', () => {
    expect(attributeValueOptions([], 'os')).toEqual([]);
  });

  it('instance_family の選択肢を昇順で抽出する (issue 0054)', () => {
    const rates = [
      rate({ instance_family: 'm5' }),
      rate({ instance_family: 'db.r6g' }),
      rate({ instance_family: 'm5' }),
    ];
    expect(attributeValueOptions(rates, 'instance_family')).toEqual(['db.r6g', 'm5']);
  });
});

describe('matchesAttributeSelection', () => {
  it('selected が空オブジェクトなら常に一致する', () => {
    expect(matchesAttributeSelection(rate({ os: 'Linux' }), {})).toBe(true);
  });

  it('値集合が空 (Set.size === 0) のキーは絞り込みなし扱いになる', () => {
    const selected = { os: new Set<string>() };
    expect(matchesAttributeSelection(rate({ os: 'Linux' }), selected)).toBe(true);
  });

  it('選択済みの値に一致すれば true', () => {
    const selected = { os: new Set(['Linux', 'Windows']) };
    expect(matchesAttributeSelection(rate({ os: 'Linux' }), selected)).toBe(true);
  });

  it('選択済みの値に一致しなければ false', () => {
    const selected = { os: new Set(['Windows']) };
    expect(matchesAttributeSelection(rate({ os: 'Linux' }), selected)).toBe(false);
  });

  it('複数キーは AND 条件で判定する', () => {
    const selected = {
      engine: new Set(['MySQL']),
      deployment_option: new Set(['Multi-AZ']),
    };
    expect(
      matchesAttributeSelection(
        rate({ engine: 'MySQL', deployment_option: 'Single-AZ' }),
        selected,
      ),
    ).toBe(false);
    expect(
      matchesAttributeSelection(rate({ engine: 'MySQL', deployment_option: 'Multi-AZ' }), selected),
    ).toBe(true);
  });

  it('rate がそのキー自体を持たない (Savings Plans の storage_type 等) 場合は対象外として一致させる', () => {
    const selected = { storage_type: new Set(['standard']) };
    // Savings Plans の行には storage_type が付与されないため attributes に存在しない。
    expect(matchesAttributeSelection(rate({ engine: 'MySQL' }), selected)).toBe(true);
  });

  it('instance_family の選択で絞り込める (issue 0054)', () => {
    const selected = { instance_family: new Set(['m5']) };
    expect(matchesAttributeSelection(rate({ instance_family: 'm5' }), selected)).toBe(true);
    expect(matchesAttributeSelection(rate({ instance_family: 'm5a' }), selected)).toBe(false);
  });
});

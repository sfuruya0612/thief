// wafOverviewRows の Description 行の検証 (issue 0074)。
import { describe, expect, it } from 'vitest';
import { cloudfrontOverviewRows, wafOverviewRows } from './overviewRows';
import type { CloudFrontRow, WAFRow } from '../../types/aws';

const baseRow: WAFRow = {
  region: 'ap-northeast-1',
  id: 'acl-1',
  name: 'edge-acl',
  state: 'active',
  scope: 'REGIONAL',
  description: '',
  ruleCount: 3,
  associatedCount: 1,
  tags: {},
};

describe('wafOverviewRows', () => {
  it('description が空のとき Description 行はダッシュ表示になる', () => {
    const rows = wafOverviewRows(baseRow);
    const desc = rows.find(([label]) => label === 'Description');
    expect(desc).toBeDefined();
    expect(desc?.[1]).toBe('—');
  });

  it('description が設定されていればその値を表示する', () => {
    const rows = wafOverviewRows({ ...baseRow, description: 'Protects the public API' });
    const desc = rows.find(([label]) => label === 'Description');
    expect(desc?.[1]).toBe('Protects the public API');
  });
});

const cloudfrontBaseRow: CloudFrontRow = {
  region: 'global',
  id: 'E123',
  name: 'test',
  state: 'deployed',
  domainName: 'd123.cloudfront.net',
  aliases: [],
  origins: ['origin.example.com'],
  behaviors: [],
  enabled: true,
  priceClass: 'PriceClass_All',
};

describe('cloudfrontOverviewRows', () => {
  it('Alternate domains 行が aliases のカンマ結合を表示する', () => {
    const rows = cloudfrontOverviewRows({
      ...cloudfrontBaseRow,
      aliases: ['example.com', 'www.example.com'],
    });
    const alt = rows.find(([label]) => label === 'Alternate domains');
    expect(alt?.[1]).toBe('example.com, www.example.com');
  });

  it('aliases が空配列のとき Alternate domains 行はダッシュ表示になる', () => {
    const rows = cloudfrontOverviewRows(cloudfrontBaseRow);
    const alt = rows.find(([label]) => label === 'Alternate domains');
    expect(alt?.[1]).toBe('—');
  });

  it('Enabled と Price class の行が残っている', () => {
    const rows = cloudfrontOverviewRows(cloudfrontBaseRow);
    expect(rows.some(([label]) => label === 'Enabled')).toBe(true);
    expect(rows.some(([label]) => label === 'Price class')).toBe(true);
  });
});

// components/tables/columns.tsx の列定義のテスト。
import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';
import {
  cacheColumns,
  cloudfrontBehaviorColumns,
  cloudfrontColumns,
  kinesisColumns,
  wafColumns,
} from './columns';
import type { CacheRow, CloudFrontBehaviorRow, CloudFrontRow, WAFRow } from '../../types/aws';

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

describe('wafColumns', () => {
  it('description 列が存在する', () => {
    expect(wafColumns.some((c) => c.key === 'description')).toBe(true);
  });

  it('列幅の合計が 100% になる', () => {
    const total = wafColumns.reduce((sum, c) => sum + Number.parseFloat(c.width), 0);
    expect(total).toBe(100);
  });

  it('description が空文字の行のセルはダッシュ表示になる', () => {
    const col = wafColumns.find((c) => c.key === 'description');
    if (!col) throw new Error('description column not found');
    const { container } = render(<>{col.cell(baseRow)}</>);
    expect(container).toHaveTextContent('—');
  });

  it('description が設定されていればその値を表示する', () => {
    const col = wafColumns.find((c) => c.key === 'description');
    if (!col) throw new Error('description column not found');
    const { container } = render(
      <>{col.cell({ ...baseRow, description: 'Protects the public API' })}</>,
    );
    expect(container).toHaveTextContent('Protects the public API');
  });

  it('列の並びが Web ACL, Description, State, Scope, Region, Rules, Associated の順になる', () => {
    expect(wafColumns.map((c) => c.key)).toEqual([
      'name',
      'description',
      'state',
      'scope',
      'region',
      'ruleCount',
      'associatedCount',
    ]);
    expect(wafColumns.map((c) => c.header)).toEqual([
      'Web ACL',
      'Description',
      'State',
      'Scope',
      'Region',
      'Rules',
      'Associated',
    ]);
  });

  it('各列の幅が並べ替え前と同じ対応を保つ', () => {
    const widthByKey = Object.fromEntries(wafColumns.map((c) => [c.key, c.width]));
    expect(widthByKey).toEqual({
      name: '20%',
      description: '17%',
      state: '8%',
      scope: '13%',
      region: '20%',
      ruleCount: '10%',
      associatedCount: '12%',
    });
  });

  it('ruleCount と associatedCount は右寄せのまま保たれる', () => {
    const ruleCount = wafColumns.find((c) => c.key === 'ruleCount');
    const associatedCount = wafColumns.find((c) => c.key === 'associatedCount');
    expect(ruleCount?.align).toBe('right');
    expect(associatedCount?.align).toBe('right');
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

describe('cloudfrontColumns', () => {
  it('列の並びが Distribution, State, Domain, Alternate domains, Origins の順になる', () => {
    expect(cloudfrontColumns.map((c) => c.key)).toEqual([
      'id',
      'state',
      'domainName',
      'aliases',
      'origins',
    ]);
    expect(cloudfrontColumns.map((c) => c.header)).toEqual([
      'Distribution',
      'State',
      'Domain',
      'Alternate domains',
      'Origins',
    ]);
  });

  it('列幅の合計が 100% になる', () => {
    const total = cloudfrontColumns.reduce((sum, c) => sum + Number.parseFloat(c.width), 0);
    expect(total).toBe(100);
  });

  it('aliases が複数件のとき、カンマ + 半角スペースで結合して表示する', () => {
    const col = cloudfrontColumns.find((c) => c.key === 'aliases');
    if (!col) throw new Error('aliases column not found');
    const { container } = render(
      <>{col.cell({ ...cloudfrontBaseRow, aliases: ['example.com', 'www.example.com'] })}</>,
    );
    expect(container).toHaveTextContent('example.com, www.example.com');
  });

  it('aliases が空配列のときダッシュ表示になる', () => {
    const col = cloudfrontColumns.find((c) => c.key === 'aliases');
    if (!col) throw new Error('aliases column not found');
    const { container } = render(<>{col.cell(cloudfrontBaseRow)}</>);
    expect(container).toHaveTextContent('—');
  });
});

const cloudfrontBehaviorBaseRow: CloudFrontBehaviorRow = {
  id: '1',
  order: 1,
  pathPattern: '/api/*',
  targetOriginId: 'origin-1',
  viewerProtocolPolicy: 'https-only',
  allowedMethods: ['GET', 'HEAD'],
  compress: true,
  isDefault: false,
};

describe('cloudfrontBehaviorColumns', () => {
  it('列幅の合計が 100% になる', () => {
    const total = cloudfrontBehaviorColumns.reduce((sum, c) => sum + Number.parseFloat(c.width), 0);
    expect(total).toBe(100);
  });

  it('allowedMethods が複数件のとき、カンマ + 半角スペースで結合して表示する', () => {
    const col = cloudfrontBehaviorColumns.find((c) => c.key === 'allowedMethods');
    if (!col) throw new Error('allowedMethods column not found');
    const { container } = render(<>{col.cell(cloudfrontBehaviorBaseRow)}</>);
    expect(container).toHaveTextContent('GET, HEAD');
  });

  it('allowedMethods が空配列のときダッシュ表示になる', () => {
    const col = cloudfrontBehaviorColumns.find((c) => c.key === 'allowedMethods');
    if (!col) throw new Error('allowedMethods column not found');
    const { container } = render(
      <>{col.cell({ ...cloudfrontBehaviorBaseRow, allowedMethods: [] })}</>,
    );
    expect(container).toHaveTextContent('—');
  });

  it('compress が true のときチェックマークを表示する', () => {
    const col = cloudfrontBehaviorColumns.find((c) => c.key === 'compress');
    if (!col) throw new Error('compress column not found');
    const { container } = render(<>{col.cell(cloudfrontBehaviorBaseRow)}</>);
    expect(container).toHaveTextContent('✓');
  });

  it('compress が false のときダッシュ表示になる', () => {
    const col = cloudfrontBehaviorColumns.find((c) => c.key === 'compress');
    if (!col) throw new Error('compress column not found');
    const { container } = render(
      <>{col.cell({ ...cloudfrontBehaviorBaseRow, compress: false })}</>,
    );
    expect(container).toHaveTextContent('—');
  });

  it('pathPattern の filterValue は既定ビヘイビアで Default (*) を返す', () => {
    const col = cloudfrontBehaviorColumns.find((c) => c.key === 'pathPattern');
    if (!col || !col.filterValue) throw new Error('pathPattern column or filterValue not found');
    expect(col.filterValue({ ...cloudfrontBehaviorBaseRow, isDefault: true })).toBe('Default (*)');
  });

  it('pathPattern の filterValue は追加ビヘイビアで生の pathPattern を返す', () => {
    const col = cloudfrontBehaviorColumns.find((c) => c.key === 'pathPattern');
    if (!col || !col.filterValue) throw new Error('pathPattern column or filterValue not found');
    expect(col.filterValue(cloudfrontBehaviorBaseRow)).toBe('/api/*');
  });
});

describe('kinesisColumns', () => {
  it('列の並びが Stream, State, Mode, Shards, Retention, Encryption, Region の順になる', () => {
    expect(kinesisColumns.map((c) => c.key)).toEqual([
      'name',
      'state',
      'mode',
      'shardCount',
      'retentionHours',
      'encryptionType',
      'region',
    ]);
    expect(kinesisColumns.map((c) => c.header)).toEqual([
      'Stream',
      'State',
      'Mode',
      'Shards',
      'Retention',
      'Encryption',
      'Region',
    ]);
  });

  it('列幅の合計が 100% になる', () => {
    const total = kinesisColumns.reduce((sum, c) => sum + Number.parseFloat(c.width), 0);
    expect(total).toBe(100);
  });

  it('各列の幅が Mode 列追加後の再配分と同じ対応を保つ', () => {
    const widthByKey = Object.fromEntries(kinesisColumns.map((c) => [c.key, c.width]));
    expect(widthByKey).toEqual({
      name: '20%',
      state: '10%',
      mode: '15%',
      shardCount: '9%',
      retentionHours: '12%',
      encryptionType: '13%',
      region: '21%',
    });
  });
});

const cacheBaseRow: CacheRow = {
  region: 'ap-northeast-1',
  id: 'cc-1',
  name: 'cc-1',
  state: 'available',
  engine: 'redis',
  engineVersion: '7.1.0',
  nodeType: 'cache.t4g.micro',
  numNodes: 2,
  endpoint: 'cc-1.example.cache.amazonaws.com',
  port: 6379,
  parameterGroup: 'default.redis7',
  replicationGroupId: 'rg-1',
  nodeAvailabilityZones: [],
};

describe('cacheColumns', () => {
  it('列幅の合計が 100% になる', () => {
    const total = cacheColumns.reduce((sum, c) => sum + Number.parseFloat(c.width), 0);
    expect(total).toBe(100);
  });

  it('nodeAvailabilityZones が複数件のとき、カンマ + 半角スペースで結合して表示する', () => {
    const col = cacheColumns.find((c) => c.key === 'nodeAvailabilityZones');
    if (!col) throw new Error('nodeAvailabilityZones column not found');
    const { container } = render(
      <>
        {col.cell({
          ...cacheBaseRow,
          nodeAvailabilityZones: ['ap-northeast-1a', 'ap-northeast-1c'],
        })}
      </>,
    );
    expect(container).toHaveTextContent('ap-northeast-1a, ap-northeast-1c');
  });

  it('nodeAvailabilityZones が空配列のときダッシュ表示になる', () => {
    const col = cacheColumns.find((c) => c.key === 'nodeAvailabilityZones');
    if (!col) throw new Error('nodeAvailabilityZones column not found');
    const { container } = render(<>{col.cell(cacheBaseRow)}</>);
    expect(container).toHaveTextContent('—');
  });
});

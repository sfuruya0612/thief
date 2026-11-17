// wafOverviewRows の Description 行の検証 (issue 0074)。
import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';
import {
  cacheOverviewRows,
  cloudfrontOverviewRows,
  iamOverviewRows,
  kinesisOverviewRows,
  openSearchOverviewRows,
  wafOverviewRows,
} from './overviewRows';
import type {
  CacheRow,
  CloudFrontRow,
  IAMRow,
  KinesisRow,
  OpenSearchRow,
  WAFRow,
} from '../../types/aws';

const baseRow: WAFRow = {
  region: 'ap-northeast-1',
  id: 'acl-1',
  name: 'edge-acl',
  state: 'active',
  scope: 'REGIONAL',
  description: '',
  ruleCount: 3,
  associatedCount: 1,
  associatedCountFetchFailed: false,
  tags: {},
  tagsFetchFailed: false,
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

  it('associatedCountFetchFailed が true のとき Associated resources 行に警告アイコンを表示する', () => {
    const rows = wafOverviewRows({ ...baseRow, associatedCountFetchFailed: true });
    const associated = rows.find(([label]) => label === 'Associated resources');
    const { container } = render(<>{associated?.[1]}</>);
    expect(container.querySelector('.fetch-failed-warning')).not.toBeNull();
    expect(container).toHaveTextContent('1');
  });

  it('associatedCountFetchFailed が false のとき Associated resources 行に警告アイコンを表示しない', () => {
    const rows = wafOverviewRows(baseRow);
    const associated = rows.find(([label]) => label === 'Associated resources');
    const { container } = render(<>{associated?.[1]}</>);
    expect(container.querySelector('.fetch-failed-warning')).toBeNull();
  });
});

const iamBaseRow: IAMRow = {
  region: 'global',
  id: 'user-1',
  name: 'deploy-bot',
  state: 'active',
  arn: 'arn:aws:iam::123456789012:user/deploy-bot',
  kind: 'user',
  mfaEnabled: true,
  mfaEnabledFetchFailed: false,
  lastActivity: '2026-01-01T00:00:00Z',
  groups: ['admins'],
  groupsFetchFailed: false,
  policies: ['AdministratorAccess'],
  policiesFetchFailed: false,
};

describe('iamOverviewRows', () => {
  it('mfaEnabledFetchFailed が true のとき MFA 行に警告アイコンを表示する', () => {
    const rows = iamOverviewRows({ ...iamBaseRow, mfaEnabledFetchFailed: true });
    const mfa = rows.find(([label]) => label === 'MFA');
    const { container } = render(<>{mfa?.[1]}</>);
    expect(container.querySelector('.fetch-failed-warning')).not.toBeNull();
  });

  it('policiesFetchFailed が true のとき Policies attached 行に警告アイコンを表示する', () => {
    const rows = iamOverviewRows({ ...iamBaseRow, policiesFetchFailed: true });
    const policies = rows.find(([label]) => label === 'Policies attached');
    const { container } = render(<>{policies?.[1]}</>);
    expect(container.querySelector('.fetch-failed-warning')).not.toBeNull();
    expect(container).toHaveTextContent('1');
  });

  it('groupsFetchFailed が true のとき Groups 行に警告アイコンを表示する', () => {
    const rows = iamOverviewRows({ ...iamBaseRow, groupsFetchFailed: true });
    const groups = rows.find(([label]) => label === 'Groups');
    const { container } = render(<>{groups?.[1]}</>);
    expect(container.querySelector('.fetch-failed-warning')).not.toBeNull();
    expect(container).toHaveTextContent('1');
  });

  it('フラグが false のときはいずれの行にも警告アイコンを表示しない', () => {
    const rows = iamOverviewRows(iamBaseRow);
    for (const label of ['MFA', 'Policies attached', 'Groups']) {
      const row = rows.find(([l]) => l === label);
      const { container } = render(<>{row?.[1]}</>);
      expect(container.querySelector('.fetch-failed-warning')).toBeNull();
    }
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

// kinesisOverviewRows の Capacity mode 行の検証 (issue 0097)。
const kinesisBaseRow: KinesisRow = {
  region: 'ap-northeast-1',
  id: 'events-stream',
  name: 'events-stream',
  state: 'ACTIVE',
  mode: 'on-demand',
  shardCount: 2,
  retentionHours: 24,
  encryptionType: 'KMS',
  tags: {},
};

describe('kinesisOverviewRows', () => {
  it('Resource ID の直後に Capacity mode 行があり、既存行 (Shards 以降) の順序は変わらない', () => {
    const rows = kinesisOverviewRows(kinesisBaseRow);
    expect(rows.map(([label]) => label)).toEqual([
      'Resource ID',
      'Capacity mode',
      'Shards',
      'Retention',
      'Encryption',
      'Region',
    ]);
  });

  it('Capacity mode 行の値は KinesisRow.mode をそのまま表示する', () => {
    const rows = kinesisOverviewRows({ ...kinesisBaseRow, mode: 'provisioned' });
    const mode = rows.find(([label]) => label === 'Capacity mode');
    expect(mode?.[1]).toBe('provisioned');
  });

  it('mode が空文字列でもそのまま表示し、ダッシュへ変換しない', () => {
    const rows = kinesisOverviewRows({ ...kinesisBaseRow, mode: '' });
    const mode = rows.find(([label]) => label === 'Capacity mode');
    expect(mode?.[1]).toBe('');
  });
});

// openSearchOverviewRows の Overview 項目の検証。
const openSearchBaseRow: OpenSearchRow = {
  region: 'ap-northeast-1',
  id: 'arn:aws:es:ap-northeast-1:123:domain/products',
  name: 'products',
  state: 'active',
  engineVersion: 'OpenSearch_2.19',
  dataNodeType: 'm5.large.search',
  dataNodeCount: 3,
  dedicatedMasterEnabled: true,
  dedicatedMasterType: 'm5.large.search',
  dedicatedMasterCount: 3,
  ebsVolumeSizeGB: 100,
  ebsVolumeType: 'gp3',
  endpoint: 'search-products.ap-northeast-1.es.amazonaws.com',
  vpcId: 'vpc-0123456789abcdef0',
  encryptionAtRest: true,
  nodeToNodeEncryption: true,
  enforceHTTPS: true,
  tags: {},
  tagsFetchFailed: false,
};

describe('openSearchOverviewRows', () => {
  it('Overview の項目が Web UI の並びと同じになる', () => {
    const rows = openSearchOverviewRows(openSearchBaseRow);
    expect(rows.map(([label]) => label)).toEqual([
      'Resource ID',
      'Engine version',
      'Data node type',
      'Data nodes',
      'Dedicated master type',
      'Dedicated masters',
      'EBS storage',
      'Endpoint',
      'VPC',
      'Encryption at rest',
      'Node-to-node encryption',
      'HTTPS enforcement',
      'Region',
    ]);
  });

  it('EBS storage はサイズとボリュームタイプを 1 行にまとめる', () => {
    const rows = openSearchOverviewRows(openSearchBaseRow);
    expect(rows.find(([label]) => label === 'EBS storage')?.[1]).toBe('100 GiB gp3');
  });

  it('EBS ボリュームが無いドメインはストレージと空の接続先をダッシュで表示する', () => {
    const rows = openSearchOverviewRows({
      ...openSearchBaseRow,
      ebsVolumeSizeGB: 0,
      ebsVolumeType: '',
      endpoint: '',
      vpcId: '',
      dataNodeType: '',
    });
    expect(rows.find(([label]) => label === 'EBS storage')?.[1]).toBe('—');
    expect(rows.find(([label]) => label === 'Endpoint')?.[1]).toBe('—');
    expect(rows.find(([label]) => label === 'VPC')?.[1]).toBe('—');
    expect(rows.find(([label]) => label === 'Data node type')?.[1]).toBe('—');
  });

  it('データノードとマスターノードのインスタンスタイプとノード数を別々に表示する', () => {
    const rows = openSearchOverviewRows(openSearchBaseRow);
    expect(rows.find(([label]) => label === 'Data node type')?.[1]).toBe('m5.large.search');
    expect(rows.find(([label]) => label === 'Data nodes')?.[1]).toBe(3);
    expect(rows.find(([label]) => label === 'Dedicated master type')?.[1]).toBe('m5.large.search');
    expect(rows.find(([label]) => label === 'Dedicated masters')?.[1]).toBe(3);
  });

  it('dedicated master が無効ならマスターノードの行はダッシュ表示になる', () => {
    const rows = openSearchOverviewRows({
      ...openSearchBaseRow,
      dedicatedMasterEnabled: false,
      dedicatedMasterType: 'm5.large.search',
      dedicatedMasterCount: 3,
    });
    expect(rows.find(([label]) => label === 'Dedicated master type')?.[1]).toBe('—');
    expect(rows.find(([label]) => label === 'Dedicated masters')?.[1]).toBe('—');
  });

  it('暗号化と HTTPS は enabled / disabled で表示する', () => {
    const rows = openSearchOverviewRows({
      ...openSearchBaseRow,
      encryptionAtRest: false,
      nodeToNodeEncryption: false,
      enforceHTTPS: false,
    });
    expect(rows.find(([label]) => label === 'Encryption at rest')?.[1]).toBe('disabled');
    expect(rows.find(([label]) => label === 'Node-to-node encryption')?.[1]).toBe('disabled');
    expect(rows.find(([label]) => label === 'HTTPS enforcement')?.[1]).toBe('disabled');
  });
});

const cacheBaseRow: CacheRow = {
  region: 'ap-northeast-1',
  id: 'my-redis-001',
  name: 'my-redis-001',
  state: 'available',
  engine: 'redis',
  engineVersion: '7.1.0',
  nodeType: 'cache.t4g.micro',
  numNodes: 2,
  endpoint: 'my-redis.abc123.apne1.cache.amazonaws.com',
  port: 6379,
  parameterGroup: 'default.redis7',
  replicationGroupId: 'my-redis',
  nodeAvailabilityZones: ['ap-northeast-1a', 'ap-northeast-1c'],
};

describe('cacheOverviewRows', () => {
  it('Nodes の直後に AZs 行があり、既存行の順序は変わらない', () => {
    const rows = cacheOverviewRows(cacheBaseRow);
    expect(rows.map(([label]) => label)).toEqual([
      'Resource ID',
      'Engine',
      'Engine version',
      'Replication group',
      'Node type',
      'Nodes',
      'AZs',
      'Endpoint',
      'Port',
      'Region',
    ]);
  });

  it('複数の AZ をカンマ区切りで連結して表示する', () => {
    const rows = cacheOverviewRows(cacheBaseRow);
    const azs = rows.find(([label]) => label === 'AZs');
    expect(azs?.[1]).toBe('ap-northeast-1a, ap-northeast-1c');
  });

  it('nodeAvailabilityZones が空配列のときはダッシュを表示する', () => {
    const rows = cacheOverviewRows({ ...cacheBaseRow, nodeAvailabilityZones: [] });
    const azs = rows.find(([label]) => label === 'AZs');
    expect(azs?.[1]).toBe('—');
  });
});

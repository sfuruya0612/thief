// wafOverviewRows の Description 行の検証 (issue 0074)。
import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';
import { cloudfrontOverviewRows, iamOverviewRows, wafOverviewRows } from './overviewRows';
import type { CloudFrontRow, IAMRow, WAFRow } from '../../types/aws';

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

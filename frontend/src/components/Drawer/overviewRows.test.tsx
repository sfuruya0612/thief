// wafOverviewRows の Description 行の検証 (issue 0074)。
import { describe, expect, it } from 'vitest';
import { wafOverviewRows } from './overviewRows';
import type { WAFRow } from '../../types/aws';

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

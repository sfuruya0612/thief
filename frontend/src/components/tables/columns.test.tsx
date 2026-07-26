// components/tables/columns.tsx の列定義のテスト。
import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';
import { wafColumns } from './columns';
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

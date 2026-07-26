// wafColumns の列構成の検証 (issue 0074)。
// Description 列の存在、列幅合計 100%、空の description のダッシュ表示を確認する。
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
});

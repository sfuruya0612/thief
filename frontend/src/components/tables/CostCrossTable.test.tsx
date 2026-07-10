import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';
import { CostCrossTable } from './CostCrossTable';

describe('CostCrossTable', () => {
  it('縦軸に group、横軸に categories (日付) を並べたクロス表を描画する', () => {
    const { container } = render(
      <CostCrossTable
        categories={['2026-07-01', '2026-07-02']}
        rows={[
          { group: 'AmazonEC2', amounts: [10, 20], total: 30 },
          { group: 'AmazonS3', amounts: [1, 2], total: 3 },
        ]}
      />,
    );

    const headers = Array.from(container.querySelectorAll('thead th')).map((el) => el.textContent);
    expect(headers).toEqual(['Group', 'Total', '2026-07-01', '2026-07-02']);

    const rows = Array.from(container.querySelectorAll('tbody tr')).map((tr) =>
      Array.from(tr.querySelectorAll('td')).map((td) => td.textContent),
    );
    expect(rows).toEqual([
      ['AmazonEC2', '$30', '$10', '$20'],
      ['AmazonS3', '$3', '$1', '$2'],
    ]);
  });

  it('rows が空の場合は空メッセージを表示する', () => {
    const { getByText } = render(<CostCrossTable categories={['2026-07-01']} rows={[]} />);
    expect(getByText('No cost data match current filters')).toBeInTheDocument();
  });
});

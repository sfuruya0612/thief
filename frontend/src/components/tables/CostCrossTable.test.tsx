import { describe, expect, it } from 'vitest';
import { fireEvent, render } from '@testing-library/react';
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

  it('Group 列ヘッダのハンドルをドラッグすると列幅が px 化され、Total 列の left オフセットも追従する', () => {
    const { container } = render(
      <CostCrossTable
        categories={['2026-07-01']}
        rows={[{ group: 'AmazonEC2', amounts: [10], total: 10 }]}
      />,
    );
    const handle = container.querySelector(
      'th[data-col-key="group"] .col-resize-handle',
    ) as HTMLSpanElement;

    fireEvent.pointerDown(handle, { clientX: 100 });
    fireEvent(document, new MouseEvent('pointermove', { clientX: 220 }));
    fireEvent(document, new MouseEvent('pointerup'));

    const groupCol = container.querySelector('col:nth-of-type(1)') as HTMLTableColElement;
    const totalHeader = container.querySelector('th[data-col-key="total"]') as HTMLTableCellElement;
    // jsdom の getBoundingClientRect は常に 0 を返すため、幅の絶対値ではなく
    // px 化されたこと・Total 列の left が Group 列幅と一致していることのみ検証する
    expect(groupCol.style.width.endsWith('px')).toBe(true);
    expect(totalHeader.style.left).toBe(groupCol.style.width);
  });

  it('ドラッグしても MIN_COL_WIDTH 未満には縮まない', () => {
    const { container } = render(
      <CostCrossTable
        categories={['2026-07-01']}
        rows={[{ group: 'AmazonEC2', amounts: [10], total: 10 }]}
      />,
    );
    const handle = container.querySelector(
      'th[data-col-key="2026-07-01"] .col-resize-handle',
    ) as HTMLSpanElement;

    fireEvent.pointerDown(handle, { clientX: 100 });
    fireEvent(document, new MouseEvent('pointermove', { clientX: -1000 }));
    fireEvent(document, new MouseEvent('pointerup'));

    const categoryCol = container.querySelector('col:nth-of-type(3)') as HTMLTableColElement;
    expect(categoryCol.style.width).toBe('60px');
  });
});

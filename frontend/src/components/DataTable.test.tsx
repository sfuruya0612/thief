import { describe, expect, it } from 'vitest';
import { fireEvent, render } from '@testing-library/react';
import { DataTable } from './DataTable';
import type { ColumnDef } from './tables/columns';

interface Row {
  id: string;
  name: string;
  size: number;
}

const rows: Row[] = [
  { id: '1', name: 'alpha', size: 2048 },
  { id: '2', name: 'beta', size: 512 },
];

const baseColumns: ColumnDef<Row>[] = [
  { key: 'name', header: 'Name', width: '50%', cell: (r) => r.name },
  { key: 'size', header: 'Size', width: '30%', cell: (r) => String(r.size) },
  {
    key: 'actions',
    header: '',
    width: '20%',
    cell: () => 'Download',
  },
];

function renderTable(columns: ColumnDef<Row>[] = baseColumns) {
  return render(<DataTable rows={rows} columns={columns} onSelect={() => {}} selectedId={null} />);
}

describe('DataTable', () => {
  it('列ヘッダ右端のハンドルをドラッグすると対象列が px 幅に切り替わり、table に dt-resized が付く', () => {
    const { container } = renderTable();
    const table = container.querySelector('table') as HTMLTableElement;
    const handle = container.querySelector(
      'th[data-col-key="name"] .col-resize-handle',
    ) as HTMLSpanElement;

    expect(table.className).not.toContain('dt-resized');

    fireEvent.pointerDown(handle, { clientX: 100 });
    fireEvent(document, new MouseEvent('pointermove', { clientX: 160 }));
    fireEvent(document, new MouseEvent('pointerup'));

    expect(table.className).toContain('dt-resized');
    const col = container.querySelector('col[style]:nth-of-type(2)') as HTMLTableColElement;
    // jsdom の getBoundingClientRect は常に 0 を返すため、幅の絶対値ではなく
    // 「px 数値に変換されたこと」だけを検証する
    expect(col.style.width.endsWith('px')).toBe(true);
  });

  it('ドラッグしても MIN_COL_WIDTH 未満には縮まない', () => {
    const { container } = renderTable();
    const handle = container.querySelector(
      'th[data-col-key="name"] .col-resize-handle',
    ) as HTMLSpanElement;

    fireEvent.pointerDown(handle, { clientX: 100 });
    fireEvent(document, new MouseEvent('pointermove', { clientX: -1000 }));
    fireEvent(document, new MouseEvent('pointerup'));

    const col = container.querySelector('col[style]:nth-of-type(2)') as HTMLTableColElement;
    expect(col.style.width).toBe('60px');
  });

  it('列フィルター行が常時表示され、actions 列や空 header 列には入力欄が出ない', () => {
    const { container } = renderTable();
    const filterInputs = container.querySelectorAll('tr.dt-filter-row input.dt-col-filter');
    // name, size の 2 列のみ対象 (actions 列は header 空 + key==='actions' で除外)
    expect(filterInputs.length).toBe(2);
  });

  it('列フィルターで部分一致 (case-insensitive) 絞り込みができる', () => {
    const { container } = renderTable();
    const nameFilter = container.querySelectorAll(
      'tr.dt-filter-row input.dt-col-filter',
    )[0] as HTMLInputElement;

    fireEvent.change(nameFilter, { target: { value: 'ALP' } });

    expect(container.textContent).toContain('alpha');
    expect(container.textContent).not.toContain('beta');
  });

  it('複数列のフィルターは AND で合成される', () => {
    const { container } = renderTable();
    const [nameFilter, sizeFilter] = container.querySelectorAll(
      'tr.dt-filter-row input.dt-col-filter',
    ) as NodeListOf<HTMLInputElement>;

    fireEvent.change(nameFilter, { target: { value: 'a' } });
    fireEvent.change(sizeFilter, { target: { value: '512' } });

    // name に "a" を含み size が "512" の行は beta のみ
    expect(container.textContent).not.toContain('alpha');
    expect(container.textContent).toContain('beta');
  });

  it('filterValue を指定した列は表示値でフィルタされる', () => {
    const columns: ColumnDef<Row>[] = [
      ...baseColumns.slice(0, 1),
      {
        key: 'size',
        header: 'Size',
        width: '30%',
        cell: (r) => `${r.size} B`,
        filterValue: (r) => `${r.size} B`,
      },
      baseColumns[2],
    ];
    const { container } = renderTable(columns);
    const sizeFilter = container.querySelectorAll(
      'tr.dt-filter-row input.dt-col-filter',
    )[1] as HTMLInputElement;

    // 生値の "512" ではなく filterValue が返す "512 B" でマッチすることを確認する
    fireEvent.change(sizeFilter, { target: { value: '512 B' } });

    expect(container.textContent).toContain('beta');
    expect(container.textContent).not.toContain('alpha');
  });

  it('filterable: false を指定した列には入力欄が出ない', () => {
    const columns: ColumnDef<Row>[] = [
      { ...baseColumns[0], filterable: false },
      baseColumns[1],
      baseColumns[2],
    ];
    const { container } = renderTable(columns);
    const filterInputs = container.querySelectorAll('tr.dt-filter-row input.dt-col-filter');
    expect(filterInputs.length).toBe(1);
  });

  it('フィルタ後にソートが適用される (filter -> sort の順)', () => {
    const columns: ColumnDef<Row>[] = [
      { key: 'name', header: 'Name', width: '50%', cell: (r) => r.name },
      { key: 'size', header: 'Size', width: '30%', cell: (r) => String(r.size) },
    ];
    const threeRows: Row[] = [
      { id: '1', name: 'apple', size: 3 },
      { id: '2', name: 'apricot', size: 1 },
      { id: '3', name: 'banana', size: 2 },
    ];
    const { container } = render(
      <DataTable rows={threeRows} columns={columns} onSelect={() => {}} selectedId={null} />,
    );

    const nameFilter = container.querySelectorAll(
      'tr.dt-filter-row input.dt-col-filter',
    )[0] as HTMLInputElement;
    fireEvent.change(nameFilter, { target: { value: 'ap' } });

    // size 列ヘッダをクリックして昇順ソート
    const sizeHeader = container.querySelector('th[data-col-key="size"]') as HTMLTableCellElement;
    fireEvent.click(sizeHeader);

    const bodyRows = container.querySelectorAll('tbody tr');
    // banana は "ap" にマッチせず除外され、残り 2 行が size 昇順 (apricot=1, apple=3) で並ぶ
    expect(bodyRows.length).toBe(2);
    expect(bodyRows[0].textContent).toContain('apricot');
    expect(bodyRows[1].textContent).toContain('apple');
  });

  it('全件がフィルタで除外されると空表示になる', () => {
    const { container } = renderTable();
    const nameFilter = container.querySelectorAll(
      'tr.dt-filter-row input.dt-col-filter',
    )[0] as HTMLInputElement;

    fireEvent.change(nameFilter, { target: { value: 'no-such-row' } });

    expect(container.textContent).toContain('No resources match current filters');
  });
});

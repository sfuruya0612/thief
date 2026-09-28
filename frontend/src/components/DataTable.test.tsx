import { describe, expect, it, vi } from 'vitest';
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

  describe('selection (制御化したチェックボックス列)', () => {
    function renderSelected(
      selected: ReadonlySet<string>,
      isSelectable: (row: Row) => boolean,
      extra: { rows?: Row[]; columns?: ColumnDef<Row>[] } = {},
    ) {
      const onChange = vi.fn();
      const view = render(
        <DataTable
          rows={extra.rows ?? rows}
          columns={extra.columns ?? baseColumns}
          onSelect={() => {}}
          selectedId={null}
          selection={{ selected, onChange, isSelectable }}
        />,
      );
      return { ...view, onChange };
    }

    it('isSelectable が false の行にはチェックボックスが無く、行のチェックは onChange に集合を渡す', () => {
      const { container, onChange } = renderSelected(new Set(), (r) => r.id === '1');
      const bodyRows = container.querySelectorAll('tbody tr');

      expect(bodyRows[0].querySelector('input.cb')).not.toBeNull();
      expect(bodyRows[1].querySelector('input.cb')).toBeNull();

      fireEvent.click(bodyRows[0].querySelector('input.cb') as HTMLInputElement);
      expect([...onChange.mock.calls[0][0]]).toEqual(['1']);

      // チェック済みの行を外すと、その id だけが集合から消える
      const { container: checkedContainer, onChange: onCheckedChange } = renderSelected(
        new Set(['1', '2']),
        () => true,
      );
      const checkedRows = checkedContainer.querySelectorAll('tbody tr');
      fireEvent.click(checkedRows[1].querySelector('input.cb') as HTMLInputElement);
      expect([...onCheckedChange.mock.calls[0][0]]).toEqual(['1']);
    });

    it('ヘッダは選べる行が 1 つも選ばれていなければ全選択、1 つでも選ばれていれば全解除になる', () => {
      const { container, onChange } = renderSelected(new Set(), () => true);
      const header = container.querySelector('thead input.cb') as HTMLInputElement;

      fireEvent.click(header);
      expect([...onChange.mock.calls[0][0]]).toEqual(['1', '2']);

      // indeterminate (一部だけ選択) からのクリックは全解除になる
      const partial = renderSelected(new Set(['1']), () => true);
      const partialHeader = partial.container.querySelector('thead input.cb') as HTMLInputElement;
      fireEvent.click(partialHeader);
      expect([...partial.onChange.mock.calls[0][0]]).toEqual([]);

      // 全て選択済みからのクリックも全解除になる
      const all = renderSelected(new Set(['1', '2']), () => true);
      const allHeader = all.container.querySelector('thead input.cb') as HTMLInputElement;
      fireEvent.click(allHeader);
      expect([...all.onChange.mock.calls[0][0]]).toEqual([]);
    });

    it('ヘッダは全て選ばれていれば checked、一部なら indeterminate になる', () => {
      const all = renderSelected(new Set(['1', '2']), () => true);
      const allHeader = all.container.querySelector('thead input.cb') as HTMLInputElement;
      expect(allHeader.checked).toBe(true);
      expect(allHeader.indeterminate).toBe(false);

      const partial = renderSelected(new Set(['1']), () => true);
      const partialHeader = partial.container.querySelector('thead input.cb') as HTMLInputElement;
      expect(partialHeader.checked).toBe(false);
      expect(partialHeader.indeterminate).toBe(true);

      const none = renderSelected(new Set(), () => true);
      const noneHeader = none.container.querySelector('thead input.cb') as HTMLInputElement;
      expect(noneHeader.checked).toBe(false);
      expect(noneHeader.indeterminate).toBe(false);
    });

    it('全選択は列フィルタとソートを適用した表示順の選べる行を対象にする', () => {
      const threeRows: Row[] = [
        { id: '1', name: 'alpha', size: 3 },
        { id: '2', name: 'apricot', size: 1 },
        { id: '3', name: 'banana', size: 2 },
      ];
      const columns: ColumnDef<Row>[] = [
        { key: 'name', header: 'Name', width: '50%', cell: (r) => r.name },
        { key: 'size', header: 'Size', width: '30%', cell: (r) => String(r.size) },
      ];
      const { container, onChange } = renderSelected(new Set(), (r) => r.id !== '3', {
        rows: threeRows,
        columns,
      });

      // size 列で昇順ソートすると表示順は apricot(2) → banana(3) → alpha(1) になる
      fireEvent.click(container.querySelector('th[data-col-key="size"]') as HTMLTableCellElement);

      fireEvent.click(container.querySelector('thead input.cb') as HTMLInputElement);
      // 選択できない banana (3) を除いた表示順の id になる
      expect([...onChange.mock.calls[0][0]]).toEqual(['2', '1']);
    });

    it('選べる行が 1 つも無い一覧にはヘッダのチェックボックスが出ない', () => {
      const { container } = renderSelected(new Set(), () => false);
      expect(container.querySelector('thead input.cb')).toBeNull();
      expect(container.querySelectorAll('tbody input.cb')).toHaveLength(0);

      // 選べる行が 1 つでもあればヘッダに出る
      const { container: selectableContainer } = renderSelected(new Set(), (r) => r.id === '1');
      expect(selectableContainer.querySelector('thead input.cb')).not.toBeNull();
    });

    it('列フィルタで選べる行が全て隠れるとヘッダのチェックボックスが消え、選択は変わらない', () => {
      const { container, onChange } = renderSelected(new Set(['1']), (r) => r.id === '1');
      expect((container.querySelector('thead input.cb') as HTMLInputElement).checked).toBe(true);

      // name 列のフィルタで alpha (id 1) を隠すと、選べる行が 1 つも表示されなくなる
      const nameFilter = container.querySelector(
        'tr.dt-filter-row input.dt-col-filter',
      ) as HTMLInputElement;
      fireEvent.change(nameFilter, { target: { value: 'beta' } });
      expect(container.querySelector('thead input.cb')).toBeNull();
      expect(container.querySelectorAll('tbody input.cb')).toHaveLength(0);
      // 隠れた行の選択は DataTable からは変えない (onChange を呼ばない)
      expect(onChange).not.toHaveBeenCalled();

      // フィルタを消すとヘッダが戻り、残っている選択で checked になる
      fireEvent.change(nameFilter, { target: { value: '' } });
      expect((container.querySelector('thead input.cb') as HTMLInputElement).checked).toBe(true);
    });

    it('selection を渡さない一覧は内部 state のチェックボックスのまま動く', () => {
      const { container } = renderTable();
      const header = container.querySelector('thead input.cb') as HTMLInputElement;

      fireEvent.click(header);
      const bodyRows = container.querySelectorAll('tbody tr');
      expect((bodyRows[0].querySelector('input.cb') as HTMLInputElement).checked).toBe(true);
      expect((bodyRows[1].querySelector('input.cb') as HTMLInputElement).checked).toBe(true);
      expect(header.checked).toBe(true);

      fireEvent.click(header);
      expect((bodyRows[0].querySelector('input.cb') as HTMLInputElement).checked).toBe(false);
      expect((bodyRows[1].querySelector('input.cb') as HTMLInputElement).checked).toBe(false);
    });
  });
});

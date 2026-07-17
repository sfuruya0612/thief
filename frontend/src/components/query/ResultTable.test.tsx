import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ResultTable } from './ResultTable';

// vitest の globals が無効なため RTL の自動 cleanup は効かない。明示的に実行する。
afterEach(cleanup);

function bodyRows(): HTMLElement[] {
  const table = screen.getByRole('table');
  const tbody = table.querySelector('tbody')!;
  return Array.from(tbody.querySelectorAll('tr')).map((tr) => tr as HTMLElement);
}

describe('ResultTable', () => {
  const columns = ['user_id', 'events'];
  const rows = [
    ['u_b', '2'],
    ['u_a', '10'],
    ['u_c', '1'],
  ];

  it('行番号付きで全行を表示する', () => {
    render(<ResultTable columns={columns} rows={rows} />);
    expect(screen.getByText('u_b')).toBeInTheDocument();
    expect(screen.getByText('3 行中 1–3 を表示')).toBeInTheDocument();
  });

  it('数値列はヘッダクリックで数値としてソートされる', () => {
    render(<ResultTable columns={columns} rows={rows} />);
    fireEvent.click(screen.getByText('events'));
    // 昇順: 1, 2, 10 (文字列ソートなら 1, 10, 2 になる)
    const first = bodyRows().map((tr) => within(tr).getAllByRole('cell')[2].textContent);
    expect(first).toEqual(['1', '2', '10']);
    fireEvent.click(screen.getByText('events'));
    const second = bodyRows().map((tr) => within(tr).getAllByRole('cell')[2].textContent);
    expect(second).toEqual(['10', '2', '1']);
  });

  it('列フィルターで部分一致に絞り込む', () => {
    render(<ResultTable columns={columns} rows={rows} />);
    const inputs = screen.getAllByPlaceholderText('フィルター…');
    fireEvent.change(inputs[0], { target: { value: 'u_a' } });
    expect(screen.queryByText('u_b')).not.toBeInTheDocument();
    expect(screen.getByText('u_a')).toBeInTheDocument();
    expect(screen.getByText('1 行中 1–1 を表示')).toBeInTheDocument();
  });

  it('50 行を超えるとページングされる', () => {
    const many = Array.from({ length: 120 }, (_, i) => [`user_${i}`, String(i)]);
    render(<ResultTable columns={columns} rows={many} />);
    expect(screen.getByText('120 行中 1–50 を表示')).toBeInTheDocument();
    expect(screen.getByText('1 / 3')).toBeInTheDocument();
    fireEvent.click(screen.getByText('›'));
    expect(screen.getByText('120 行中 51–100 を表示')).toBeInTheDocument();
  });

  it('結果 0 件では空メッセージを出す', () => {
    render(<ResultTable columns={columns} rows={[]} />);
    expect(screen.getByText('結果がありません')).toBeInTheDocument();
  });

  it('hasMore のとき追加読み込みボタンを表示する', () => {
    const onLoadMore = vi.fn();
    render(<ResultTable columns={columns} rows={rows} hasMore onLoadMore={onLoadMore} />);
    fireEvent.click(screen.getByText('さらに読み込む'));
    expect(onLoadMore).toHaveBeenCalledTimes(1);
  });
});

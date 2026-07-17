import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { SessionPickerItem } from '../../lib/sessionMeta';
import { AddSessionPicker } from './AddSessionPicker';

afterEach(cleanup);

const items: SessionPickerItem[] = [
  {
    id: 'opened-prof',
    name: 'opened-prof',
    searchText: 'opened-prof',
    disabled: true,
  },
  {
    id: 'sso-prof',
    name: 'sso-prof',
    meta: 'ap-northeast-1 · SSO',
    badge: { label: 'SSO 有効', tone: 'ok' },
    searchText: 'sso-prof 111111111111 administratoraccess',
  },
  {
    id: 'expired-prof',
    name: 'expired-prof',
    meta: 'ap-northeast-1 · SSO',
    badge: { label: '期限切れ', tone: 'warn' },
    searchText: 'expired-prof 222222222222',
  },
];

function renderPicker(overrides: Partial<Parameters<typeof AddSessionPicker>[0]> = {}) {
  const handlers = { onSelect: vi.fn(), onClose: vi.fn() };
  render(
    <AddSessionPicker
      items={items}
      placeholder="プロファイルを検索…"
      headerNote="~/.aws/config · 3件"
      footerHint="footer hint"
      emptyText="一致するプロファイルがありません"
      {...handlers}
      {...overrides}
    />,
  );
  return handlers;
}

const searchInput = () => screen.getByPlaceholderText('プロファイルを検索…');

describe('AddSessionPicker', () => {
  it('ヘッダー注記・フッターヒント・行の meta とバッジを表示する', () => {
    renderPicker();
    expect(screen.getByText('~/.aws/config · 3件')).toBeInTheDocument();
    expect(screen.getByText('footer hint')).toBeInTheDocument();
    expect(screen.getAllByText('ap-northeast-1 · SSO')).toHaveLength(2);
    expect(screen.getByText('SSO 有効')).toBeInTheDocument();
    expect(screen.getByText('期限切れ')).toBeInTheDocument();
  });

  it('開設済み行はグレーアウトして「開いています」を表示しクリックできない', () => {
    const handlers = renderPicker();
    expect(screen.getByText('開いています')).toBeInTheDocument();
    fireEvent.click(screen.getByText('opened-prof'));
    expect(handlers.onSelect).not.toHaveBeenCalled();
  });

  it('行クリックで onSelect が呼ばれる (期限切れバッジでも開ける)', () => {
    const handlers = renderPicker();
    fireEvent.click(screen.getByText('expired-prof'));
    expect(handlers.onSelect).toHaveBeenCalledWith('expired-prof');
  });

  it('searchText で絞り込める', () => {
    renderPicker();
    fireEvent.change(searchInput(), { target: { value: '2222' } });
    expect(screen.queryByText('sso-prof')).not.toBeInTheDocument();
    expect(screen.getByText('expired-prof')).toBeInTheDocument();
  });

  it('一致なしで空状態を表示する', () => {
    renderPicker();
    fireEvent.change(searchInput(), { target: { value: 'zzz' } });
    expect(screen.getByText('一致するプロファイルがありません')).toBeInTheDocument();
  });

  it('初期ハイライトは disabled をスキップした最初の行になり ⏎ ヒントが出る', () => {
    renderPicker();
    const options = screen.getAllByRole('option');
    expect(options[1]).toHaveClass('active');
    expect(screen.getByText('⏎ 開く')).toBeInTheDocument();
  });

  it('ArrowDown は disabled 行をスキップし Enter で確定する', () => {
    const handlers = renderPicker();
    fireEvent.keyDown(searchInput(), { key: 'ArrowDown' });
    fireEvent.keyDown(searchInput(), { key: 'Enter' });
    expect(handlers.onSelect).toHaveBeenCalledWith('expired-prof');
  });

  it('ArrowUp は末尾へラップして disabled をスキップする', () => {
    const handlers = renderPicker();
    // 初期ハイライト = index 1 (sso-prof)。ArrowUp → 末尾 expired-prof。
    fireEvent.keyDown(searchInput(), { key: 'ArrowUp' });
    fireEvent.keyDown(searchInput(), { key: 'Enter' });
    expect(handlers.onSelect).toHaveBeenCalledWith('expired-prof');
  });

  it('Enter は disabled 行しか無ければ発火しない', () => {
    const handlers = renderPicker({
      items: [{ id: 'a', name: 'a', searchText: 'a', disabled: true }],
    });
    fireEvent.keyDown(screen.getByPlaceholderText('プロファイルを検索…'), { key: 'Enter' });
    expect(handlers.onSelect).not.toHaveBeenCalled();
  });

  it('Escape で onClose が呼ばれ document へ伝播しない', () => {
    const outerKeyDown = vi.fn();
    document.addEventListener('keydown', outerKeyDown);
    const handlers = renderPicker();
    fireEvent.keyDown(searchInput(), { key: 'Escape' });
    expect(handlers.onClose).toHaveBeenCalled();
    expect(
      outerKeyDown.mock.calls.filter(([e]) => (e as KeyboardEvent).key === 'Escape'),
    ).toHaveLength(0);
    document.removeEventListener('keydown', outerKeyDown);
  });

  it('items 差し替えでハイライトがリセットされる', () => {
    const handlers = { onSelect: vi.fn(), onClose: vi.fn() };
    const props = {
      placeholder: 'プロファイルを検索…',
      headerNote: 'note',
      footerHint: 'hint',
      emptyText: 'empty',
      ...handlers,
    };
    const { rerender } = render(<AddSessionPicker items={items} {...props} />);
    // 3 行 → 1 行 (disabled でない) に差し替え。stale index が範囲外を指さない。
    rerender(
      <AddSessionPicker items={[{ id: 'only', name: 'only', searchText: 'only' }]} {...props} />,
    );
    expect(screen.getByRole('option')).toHaveClass('active');
  });
});

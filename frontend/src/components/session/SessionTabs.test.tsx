import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { SessionTabItem } from './SessionTabs';
import { SessionTabs } from './SessionTabs';

afterEach(cleanup);

const items: SessionTabItem[] = [
  { id: 'aaa', label: 'aaa', env: 'default' },
  { id: 'bbb', label: 'bbb', env: 'stg' },
  { id: 'ccc', label: 'ccc', env: 'prod' },
];

function renderTabs(overrides: Partial<Parameters<typeof SessionTabs>[0]> = {}) {
  const handlers = {
    onActivate: vi.fn(),
    onClose: vi.fn(),
    onReorder: vi.fn(),
    onSwapToVisible: vi.fn(),
  };
  render(
    <SessionTabs
      items={items}
      activeId="aaa"
      addLabel="＋ プロファイルを追加"
      picker={(close) => (
        <div data-testid="picker">
          <button onClick={close}>close picker</button>
        </div>
      )}
      {...handlers}
      {...overrides}
    />,
  );
  return handlers;
}

describe('SessionTabs', () => {
  it('タブを描画しアクティブタブに aria-selected が付く', () => {
    renderTabs();
    const tabs = screen.getAllByRole('tab');
    expect(tabs).toHaveLength(3);
    expect(tabs[0]).toHaveAttribute('aria-selected', 'true');
    expect(tabs[1]).toHaveAttribute('aria-selected', 'false');
  });

  it('タブクリックで onActivate が呼ばれる', () => {
    const handlers = renderTabs();
    fireEvent.click(screen.getByText('bbb'));
    expect(handlers.onActivate).toHaveBeenCalledWith('bbb');
  });

  it('× クリックで onClose が呼ばれ onActivate は発火しない', () => {
    const handlers = renderTabs();
    fireEvent.click(screen.getByRole('button', { name: 'bbb を閉じる' }));
    expect(handlers.onClose).toHaveBeenCalledWith('bbb');
    expect(handlers.onActivate).not.toHaveBeenCalled();
  });

  it('missingIds のタブはドットがグレーになり title で注記される', () => {
    renderTabs({ missingIds: ['bbb'] });
    expect(screen.getByTitle('bbb (一覧に見つかりません)')).toBeInTheDocument();
  });

  it('Ctrl+2 で 2 番目のタブへ onSwapToVisible が呼ばれる', () => {
    const handlers = renderTabs();
    fireEvent.keyDown(window, { code: 'Digit2', ctrlKey: true });
    expect(handlers.onSwapToVisible).toHaveBeenCalledWith('bbb', 3);
  });

  it('input フォーカス中は Ctrl+数字を奪わない', () => {
    const handlers = renderTabs();
    const input = document.createElement('input');
    document.body.appendChild(input);
    fireEvent.keyDown(input, { code: 'Digit2', ctrlKey: true });
    expect(handlers.onSwapToVisible).not.toHaveBeenCalled();
    input.remove();
  });

  it('範囲外の Ctrl+数字は no-op', () => {
    const handlers = renderTabs();
    fireEvent.keyDown(window, { code: 'Digit9', ctrlKey: true });
    expect(handlers.onSwapToVisible).not.toHaveBeenCalled();
  });

  it('visibleCountOverride で溢れたタブが「他 N ▾」に畳まれる', () => {
    renderTabs({ visibleCountOverride: 2 });
    expect(screen.getAllByRole('tab')).toHaveLength(2);
    expect(screen.getByText('他 1 ▾')).toBeInTheDocument();
  });

  it('「他 N ▾」メニューの行クリックで onSwapToVisible が呼ばれる', () => {
    const handlers = renderTabs({ visibleCountOverride: 2 });
    fireEvent.click(screen.getByText('他 1 ▾'));
    expect(screen.getByText('表示されていないセッション · 接続は維持')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('menuitem'));
    expect(handlers.onSwapToVisible).toHaveBeenCalledWith('ccc', 2);
  });

  it('アクティブタブが隠れ側にあると「他 N」がハイライトされる', () => {
    renderTabs({ visibleCountOverride: 2, activeId: 'ccc' });
    expect(screen.getByText('他 1 ▾')).toHaveClass('holds-active');
  });

  it('オーバーフロー時は ＋ ボタンがラベル無しに縮みヒントが並べ替え案内になる', () => {
    renderTabs({ visibleCountOverride: 2 });
    expect(screen.queryByText('＋ プロファイルを追加')).not.toBeInTheDocument();
    expect(screen.getByText('＋')).toBeInTheDocument();
    expect(screen.getByText('ドラッグで並べ替え')).toBeInTheDocument();
  });

  it('＋ クリックでピッカーが開き close 関数で閉じる', () => {
    renderTabs();
    fireEvent.click(screen.getByText('＋ プロファイルを追加'));
    expect(screen.getByTestId('picker')).toBeInTheDocument();
    fireEvent.click(screen.getByText('close picker'));
    expect(screen.queryByTestId('picker')).not.toBeInTheDocument();
  });

  it('タブ数に応じてショートカットヒントが変わる', () => {
    renderTabs();
    expect(screen.getByText('各タブがエディタ・履歴を保持 · ⌃1–3 で切替')).toBeInTheDocument();
  });

  it('DnD の drop で onReorder が呼ばれる', () => {
    const handlers = renderTabs();
    const tabs = screen.getAllByRole('tab');
    fireEvent.dragStart(tabs[0], { dataTransfer: { setData: vi.fn(), effectAllowed: '' } });
    fireEvent.dragOver(tabs[2], { dataTransfer: {} });
    fireEvent.drop(tabs[2], { dataTransfer: {} });
    expect(handlers.onReorder).toHaveBeenCalledWith(0, 2);
  });
});

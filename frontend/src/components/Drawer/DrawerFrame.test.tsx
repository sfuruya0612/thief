// DrawerFrame (Drawer の配置) のテスト。docs/issues/closed/0204 で Drawer.test.tsx から移した。
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render } from '@testing-library/react';
import { DrawerFrame } from './DrawerFrame';

function renderFrame(props: Partial<React.ComponentProps<typeof DrawerFrame>> = {}) {
  return render(
    <DrawerFrame open onClose={() => {}} {...props}>
      <div data-testid="body">body</div>
    </DrawerFrame>,
  );
}

function drawerElement(container: HTMLElement): HTMLElement {
  const el = container.querySelector('.drawer');
  expect(el).not.toBeNull();
  return el as HTMLElement;
}

describe('DrawerFrame のサイズクランプ', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('永続化された height が現在のウィンドウ高さの 85% にクランプされる (画面全体を覆う不具合の回帰テスト)', () => {
    localStorage.setItem('cloudlens:drawerSize', JSON.stringify({ height: 900 }));
    window.innerHeight = 600;

    const { container } = renderFrame({ position: 'bottom' });

    expect(drawerElement(container).style.height).toBe('510px'); // 600 * 0.85
  });

  it('永続化された height がウィンドウ高さの 85% 未満ならそのまま適用される', () => {
    localStorage.setItem('cloudlens:drawerSize', JSON.stringify({ height: 300 }));
    window.innerHeight = 600;

    const { container } = renderFrame({ position: 'bottom' });

    expect(drawerElement(container).style.height).toBe('300px');
  });

  it('永続化された width が現在のウィンドウ幅の 85% にクランプされる', () => {
    localStorage.setItem('cloudlens:drawerSize', JSON.stringify({ width: 2000 }));
    window.innerWidth = 1000;

    const { container } = renderFrame({ position: 'right' });

    expect(drawerElement(container).style.width).toBe('850px'); // 1000 * 0.85
  });

  it('永続値がない場合は inline の height/width を設定しない (CSS デフォルトに任せる)', () => {
    const { container } = renderFrame({ position: 'bottom' });

    const drawer = drawerElement(container);
    expect(drawer.style.height).toBe('');
    expect(drawer.style.width).toBe('');
  });
});

describe('DrawerFrame の ESC キー', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('open 中に Escape で onClose が呼ばれる', () => {
    const onClose = vi.fn();
    renderFrame({ onClose });

    fireEvent.keyDown(document, { key: 'Escape' });

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('Escape 以外のキーでは onClose が呼ばれない', () => {
    const onClose = vi.fn();
    renderFrame({ onClose });

    fireEvent.keyDown(document, { key: 'Enter' });

    expect(onClose).not.toHaveBeenCalled();
  });

  it('閉じている (open=false) 場合は Escape でも onClose が呼ばれない', () => {
    const onClose = vi.fn();
    renderFrame({ open: false, onClose });

    fireEvent.keyDown(document, { key: 'Escape' });

    expect(onClose).not.toHaveBeenCalled();
  });
});

// issue 0175: 分割表示中は Drawer をペインの中に収める (contained) ため、配置と
// リサイズの基準をビューポートから包含ブロック (ペインの .main) へ切り替える。
describe('DrawerFrame の分割表示向けの contained / closeOnEscape', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  // jsdom は offsetParent を実装しない (常に null) ため、包含ブロックを差し込んで
  // リサイズがペインの矩形を基準にすることを検証する。
  function stubContainingBlock(el: HTMLElement, rect: DOMRect) {
    const block = document.createElement('div');
    block.getBoundingClientRect = () => rect;
    Object.defineProperty(el, 'offsetParent', { value: block, configurable: true });
  }

  function rect(left: number, top: number, width: number, height: number): DOMRect {
    return {
      left,
      top,
      width,
      height,
      right: left + width,
      bottom: top + height,
      x: left,
      y: top,
      toJSON: () => ({}),
    } as DOMRect;
  }

  it('contained を渡すと .drawer と .drawer-backdrop に contained が付き、既定値では付かない', () => {
    const contained = renderFrame({ contained: true });
    expect(drawerElement(contained.container).classList.contains('contained')).toBe(true);
    expect(
      contained.container.querySelector('.drawer-backdrop')?.classList.contains('contained'),
    ).toBe(true);

    const plain = renderFrame();
    expect(drawerElement(plain.container).classList.contains('contained')).toBe(false);
    expect(plain.container.querySelector('.drawer-backdrop')?.classList.contains('contained')).toBe(
      false,
    );
  });

  it('closeOnEscape=false のとき ESC で onClose を呼ばない (フォーカスしていないペイン用)', () => {
    const onClose = vi.fn();
    renderFrame({ closeOnEscape: false, onClose });

    fireEvent.keyDown(document, { key: 'Escape' });

    expect(onClose).not.toHaveBeenCalled();
  });

  it('contained の右配置は包含ブロックの矩形を基準に幅を計算する', () => {
    const { container } = renderFrame({ contained: true, position: 'right' });
    const drawer = drawerElement(container);
    stubContainingBlock(drawer, rect(100, 50, 1000, 800));

    fireEvent.pointerDown(drawer.querySelector('.resize-handle')!);
    fireEvent(document, new MouseEvent('pointermove', { clientX: 300 }));
    fireEvent(document, new MouseEvent('pointerup'));

    // 1000 - (300 - 100) - 8。ウィンドウ幅 (jsdom 既定の 1024) 基準なら 716 になる
    expect(drawer.style.width).toBe('792px');
  });

  it('contained の右配置の上限は包含ブロック幅の 85% になる', () => {
    const { container } = renderFrame({ contained: true, position: 'right' });
    const drawer = drawerElement(container);
    stubContainingBlock(drawer, rect(0, 0, 400, 800));

    fireEvent.pointerDown(drawer.querySelector('.resize-handle')!);
    fireEvent(document, new MouseEvent('pointermove', { clientX: 1000 }));
    fireEvent(document, new MouseEvent('pointerup'));

    expect(drawer.style.width).toBe('340px'); // 400 * 0.85
  });

  it('contained の下配置は包含ブロックの矩形を基準に高さを計算する', () => {
    const { container } = renderFrame({ contained: true, position: 'bottom' });
    const drawer = drawerElement(container);
    stubContainingBlock(drawer, rect(0, 50, 800, 600));

    fireEvent.pointerDown(drawer.querySelector('.resize-handle')!);
    fireEvent(document, new MouseEvent('pointermove', { clientY: 200 }));
    fireEvent(document, new MouseEvent('pointerup'));

    // 600 - (200 - 50) - 8。ウィンドウ高さ (jsdom 既定の 768) 基準なら 610 になる
    expect(drawer.style.height).toBe('442px');
  });

  it('contained の下配置の上限は包含ブロック高さの 85% になる', () => {
    const { container } = renderFrame({ contained: true, position: 'bottom' });
    const drawer = drawerElement(container);
    stubContainingBlock(drawer, rect(0, 0, 800, 200));

    fireEvent.pointerDown(drawer.querySelector('.resize-handle')!);
    fireEvent(document, new MouseEvent('pointermove', { clientY: 0 }));
    fireEvent(document, new MouseEvent('pointerup'));

    expect(drawer.style.height).toBe('170px'); // 200 * 0.85
  });
});

describe('DrawerFrame の開閉クラスと transform の定義元', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  // 閉じ位置と開き位置は styles/features/drawer.css の .drawer / .drawer.open / .drawer.pos-bottom /
  // .drawer.pos-bottom.open だけで定義する。inline の transform を残すと、下配置の閉じ位置が
  // 参照する --drawer-lift (ターミナルドックの高さ) の加算を CSS 側だけ直す余地が生まれる
  // (issue 0174 の reopen)。
  it('閉じた下配置の Drawer は pos-bottom を持ち open を持たず、inline の transform を設定しない', () => {
    const { container } = renderFrame({ open: false, position: 'bottom' });

    const drawer = drawerElement(container);
    expect(drawer.classList.contains('pos-bottom')).toBe(true);
    expect(drawer.classList.contains('open')).toBe(false);
    expect(drawer.style.transform).toBe('');
  });

  it('閉じた右配置の Drawer は pos-bottom も open も持たず、inline の transform を設定しない', () => {
    const { container } = renderFrame({ open: false, position: 'right' });

    const drawer = drawerElement(container);
    expect(drawer.classList.contains('pos-bottom')).toBe(false);
    expect(drawer.classList.contains('open')).toBe(false);
    expect(drawer.style.transform).toBe('');
  });

  it('開いた Drawer は open を持ち、inline の transform を設定しない', () => {
    const { container } = renderFrame({ position: 'bottom' });

    const drawer = drawerElement(container);
    expect(drawer.classList.contains('open')).toBe(true);
    expect(drawer.style.transform).toBe('');
  });
});

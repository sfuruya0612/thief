import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render } from '@testing-library/react';
import { Drawer } from './Drawer';
import type { BaseRow } from '../../types/common';

const RESOURCE: BaseRow = {
  id: 'i-0123456789abcdef0',
  name: 'test-instance',
  state: 'running',
};

function renderDrawer(props: Partial<React.ComponentProps<typeof Drawer>> = {}) {
  return render(
    <Drawer
      resource={RESOURCE}
      service="ec2"
      profile="test-profile"
      region="ap-northeast-1"
      overviewRows={[]}
      onClose={() => {}}
      {...props}
    />,
  );
}

function drawerElement(container: HTMLElement): HTMLElement {
  const el = container.querySelector('.drawer');
  expect(el).not.toBeNull();
  return el as HTMLElement;
}

describe('Drawer のサイズクランプ', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('永続化された height が現在のウィンドウ高さの 85% にクランプされる (画面全体を覆う不具合の回帰テスト)', () => {
    localStorage.setItem('cloudlens:drawerSize', JSON.stringify({ height: 900 }));
    window.innerHeight = 600;

    const { container } = renderDrawer({ position: 'bottom' });

    expect(drawerElement(container).style.height).toBe('510px'); // 600 * 0.85
  });

  it('永続化された height がウィンドウ高さの 85% 未満ならそのまま適用される', () => {
    localStorage.setItem('cloudlens:drawerSize', JSON.stringify({ height: 300 }));
    window.innerHeight = 600;

    const { container } = renderDrawer({ position: 'bottom' });

    expect(drawerElement(container).style.height).toBe('300px');
  });

  it('永続化された width が現在のウィンドウ幅の 85% にクランプされる', () => {
    localStorage.setItem('cloudlens:drawerSize', JSON.stringify({ width: 2000 }));
    window.innerWidth = 1000;

    const { container } = renderDrawer({ position: 'right' });

    expect(drawerElement(container).style.width).toBe('850px'); // 1000 * 0.85
  });

  it('永続値がない場合は inline の height/width を設定しない (CSS デフォルトに任せる)', () => {
    const { container } = renderDrawer({ position: 'bottom' });

    const drawer = drawerElement(container);
    expect(drawer.style.height).toBe('');
    expect(drawer.style.width).toBe('');
  });
});

describe('Drawer の ESC キー', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('open 中に Escape で onClose が呼ばれる', () => {
    const onClose = vi.fn();
    renderDrawer({ onClose });

    fireEvent.keyDown(document, { key: 'Escape' });

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('Escape 以外のキーでは onClose が呼ばれない', () => {
    const onClose = vi.fn();
    renderDrawer({ onClose });

    fireEvent.keyDown(document, { key: 'Enter' });

    expect(onClose).not.toHaveBeenCalled();
  });

  it('閉じている (resource が null) 場合は Escape でも onClose が呼ばれない', () => {
    const onClose = vi.fn();
    renderDrawer({ resource: null, onClose });

    fireEvent.keyDown(document, { key: 'Escape' });

    expect(onClose).not.toHaveBeenCalled();
  });
});

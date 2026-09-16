import { afterEach, describe, expect, it, vi } from 'vitest';
import type { PointerEvent as ReactPointerEvent } from 'react';
import { startPanelHeightResize, startPanelResize } from './panelResize';

function fakeReactPointerEvent(): ReactPointerEvent<HTMLDivElement> {
  return { preventDefault: () => {} } as unknown as ReactPointerEvent<HTMLDivElement>;
}

function dispatchPointerMove(clientX: number) {
  document.dispatchEvent(new MouseEvent('pointermove', { clientX }) as unknown as PointerEvent);
}

function dispatchPointerMoveY(clientY: number) {
  document.dispatchEvent(new MouseEvent('pointermove', { clientY }) as unknown as PointerEvent);
}

function dispatchPointerUp() {
  document.dispatchEvent(new MouseEvent('pointerup') as unknown as PointerEvent);
}

describe('startPanelResize', () => {
  afterEach(() => {
    document.documentElement.style.removeProperty('--test-panel-w');
    document.body.style.cursor = '';
    document.body.style.userSelect = '';
  });

  it('パネル左端のオフセットを差し引いた幅を CSS 変数に反映する', () => {
    const onWidthChange = vi.fn();
    const onPointerDown = startPanelResize({
      min: 200,
      max: 480,
      cssVar: '--test-panel-w',
      getLeftEdge: () => 100,
      onWidthChange,
    });
    onPointerDown(fakeReactPointerEvent());

    dispatchPointerMove(350);

    expect(document.documentElement.style.getPropertyValue('--test-panel-w')).toBe('250px');
    expect(onWidthChange).toHaveBeenCalledWith(250);
  });

  it('下限・上限でクランプする', () => {
    const onWidthChange = vi.fn();
    const onPointerDown = startPanelResize({
      min: 200,
      max: 480,
      cssVar: '--test-panel-w',
      getLeftEdge: () => 100,
      onWidthChange,
    });
    onPointerDown(fakeReactPointerEvent());

    dispatchPointerMove(150); // 150 - 100 = 50 < min
    expect(onWidthChange).toHaveBeenCalledWith(200);

    dispatchPointerMove(1000); // 1000 - 100 = 900 > max
    expect(onWidthChange).toHaveBeenCalledWith(480);
  });

  it('pointerup 後は pointermove を無視する', () => {
    const onWidthChange = vi.fn();
    const onPointerDown = startPanelResize({
      min: 200,
      max: 480,
      cssVar: '--test-panel-w',
      getLeftEdge: () => 0,
      onWidthChange,
    });
    onPointerDown(fakeReactPointerEvent());
    dispatchPointerUp();
    onWidthChange.mockClear();

    dispatchPointerMove(300);

    expect(onWidthChange).not.toHaveBeenCalled();
  });
});

describe('startPanelHeightResize', () => {
  afterEach(() => {
    document.documentElement.style.removeProperty('--test-panel-h');
    document.body.style.cursor = '';
    document.body.style.userSelect = '';
  });

  it('パネル下端からポインタまでの距離を高さとして CSS 変数に反映する', () => {
    const onHeightChange = vi.fn();
    const onPointerDown = startPanelHeightResize({
      min: 192,
      max: 680,
      cssVar: '--test-panel-h',
      getBottomEdge: () => 800,
      onHeightChange,
    });
    onPointerDown(fakeReactPointerEvent());

    dispatchPointerMoveY(300);

    expect(document.documentElement.style.getPropertyValue('--test-panel-h')).toBe('500px');
    expect(onHeightChange).toHaveBeenCalledWith(500);
  });

  it('下限・上限でクランプする', () => {
    const onHeightChange = vi.fn();
    const onPointerDown = startPanelHeightResize({
      min: 192,
      max: 680,
      cssVar: '--test-panel-h',
      getBottomEdge: () => 800,
      onHeightChange,
    });
    onPointerDown(fakeReactPointerEvent());

    dispatchPointerMoveY(790); // 800 - 790 = 10 < min
    expect(onHeightChange).toHaveBeenCalledWith(192);

    dispatchPointerMoveY(0); // 800 - 0 = 800 > max
    expect(onHeightChange).toHaveBeenCalledWith(680);
  });

  it('pointerup 後は pointermove を無視する', () => {
    const onHeightChange = vi.fn();
    const onPointerDown = startPanelHeightResize({
      min: 192,
      max: 680,
      cssVar: '--test-panel-h',
      getBottomEdge: () => 800,
      onHeightChange,
    });
    onPointerDown(fakeReactPointerEvent());
    dispatchPointerUp();
    onHeightChange.mockClear();

    dispatchPointerMoveY(300);

    expect(onHeightChange).not.toHaveBeenCalled();
  });

  it('getBottomEdge は pointerdown 時に 1 回だけ呼ばれる', () => {
    const getBottomEdge = vi.fn(() => 800);
    const onPointerDown = startPanelHeightResize({
      min: 192,
      max: 680,
      cssVar: '--test-panel-h',
      getBottomEdge,
      onHeightChange: () => {},
    });
    onPointerDown(fakeReactPointerEvent());

    dispatchPointerMoveY(300);
    dispatchPointerMoveY(400);

    expect(getBottomEdge).toHaveBeenCalledTimes(1);
  });
});

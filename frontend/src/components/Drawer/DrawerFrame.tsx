// Drawer の「配置」だけを受け持つ枠 (docs/issues/closed/0204)。
// 右 / 下のどちらに出すか (position)、分割表示中にペインの中に収めるか (contained)、backdrop、
// ESC で閉じるか (closeOnEscape)、ドラッグでのリサイズとその永続化 (cloudlens:drawerSize) を
// ここに集め、中身 (見出し・タブ・本文) は Drawer.tsx が children として渡す。
// 開閉位置 (transform) と閉じ位置の計算は styles/features/drawer.css だけが持つ (issue 0174)。
import { useEffect, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import type { DrawerPos } from '../../types/common';

const DRAWER_SIZE_KEY = 'cloudlens:drawerSize';

interface DrawerSize {
  width?: number;
  height?: number;
}

function loadDrawerSize(): DrawerSize {
  try {
    return JSON.parse(localStorage.getItem(DRAWER_SIZE_KEY) || '{}') as DrawerSize;
  } catch {
    return {};
  }
}

function saveDrawerSize(size: DrawerSize): void {
  try {
    localStorage.setItem(DRAWER_SIZE_KEY, JSON.stringify(size));
  } catch {
    // quota / serialization エラーは無視
  }
}

export interface DrawerFrameProps {
  open: boolean;
  position?: DrawerPos;
  // 分割表示中はペインの中に収める (position: absolute + ペイン基準の % 上限)。
  // 既定値は現状と同じ position: fixed。
  contained?: boolean;
  // 分割表示中はフォーカス中のペインの Drawer だけが ESC を受け取る。既定値は true
  // (1 ペインでは分割前と同じく ESC で閉じる)。
  closeOnEscape?: boolean;
  onClose: () => void;
  children?: ReactNode;
}

export function DrawerFrame({
  open,
  position = 'right',
  contained = false,
  closeOnEscape = true,
  onClose,
  children,
}: DrawerFrameProps) {
  const [size, setSize] = useState<DrawerSize>(loadDrawerSize);
  const drawerRef = useRef<HTMLDivElement | null>(null);

  // レイアウト崩れ等で閉じるボタンに届かない場合の復旧手段として ESC でも閉じられるようにする。
  // 分割表示中は closeOnEscape を false で受け取った側 (フォーカスしていないペイン) が
  // リスナーを登録しない (両ペインの Drawer が開いていても ESC で閉じるのは 1 つだけ)。
  useEffect(() => {
    if (!open || !closeOnEscape) return;
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [open, closeOnEscape, onClose]);

  // リサイズの基準になる矩形。contained のときは包含ブロック (offsetParent = ペインの
  // .main) の矩形を使い、ドラッグ位置と上限をペイン基準で計算する。offsetParent を
  // 持たない環境 (jsdom 等) と 1 ペインではウィンドウ基準にフォールバックする。
  const resizeBounds = () => {
    const block = contained ? drawerRef.current?.offsetParent : null;
    if (block) {
      const rect = block.getBoundingClientRect();
      return { left: rect.left, top: rect.top, width: rect.width, height: rect.height };
    }
    return { left: 0, top: 0, width: window.innerWidth, height: window.innerHeight };
  };

  const startResize = (e: React.PointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    const move = (ev: PointerEvent) => {
      setSize((prev) => {
        const bounds = resizeBounds();
        let next: DrawerSize;
        if (position === 'bottom') {
          const h = Math.min(
            Math.max(bounds.height - (ev.clientY - bounds.top) - 8, 220),
            bounds.height * 0.85,
          );
          next = { ...prev, height: Math.round(h) };
        } else {
          const w = Math.min(
            Math.max(bounds.width - (ev.clientX - bounds.left) - 8, 380),
            bounds.width * 0.85,
          );
          next = { ...prev, width: Math.round(w) };
        }
        saveDrawerSize(next);
        return next;
      });
    };
    const up = () => {
      document.removeEventListener('pointermove', move);
      document.removeEventListener('pointerup', up);
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
    };
    document.addEventListener('pointermove', move);
    document.addEventListener('pointerup', up);
    document.body.style.cursor = position === 'bottom' ? 'ns-resize' : 'ew-resize';
    document.body.style.userSelect = 'none';
  };

  // 永続化されたサイズは保存時点のウィンドウサイズ基準の px 絶対値のため、
  // 現在のウィンドウサイズで再クランプする (ドラッグ時クランプと同じ 0.85 係数)。
  const sizeStyle: React.CSSProperties =
    position === 'bottom'
      ? size.height
        ? { height: Math.min(size.height, window.innerHeight * 0.85) }
        : {}
      : size.width
        ? { width: Math.min(size.width, window.innerWidth * 0.85) }
        : {};

  return (
    <>
      <div
        className={`drawer-backdrop ${open ? 'open' : ''} ${contained ? 'contained' : ''}`}
        onClick={onClose}
      />
      {/* 開閉位置 (transform) は styles/features/drawer.css の .drawer / .drawer.open /
          .drawer.pos-bottom / .drawer.pos-bottom.open だけで定義する。inline で重複させると、
          下配置の閉じ位置が参照する --drawer-lift (ターミナルドックの高さ) の加算を片方だけ
          直す余地が生まれる (issue 0174 の reopen)。 */}
      <div
        ref={drawerRef}
        className={`drawer ${position === 'bottom' ? 'pos-bottom' : ''} ${open ? 'open' : ''} ${
          contained ? 'contained' : ''
        }`}
        style={sizeStyle}
      >
        <div
          className={`resize-handle ${position === 'bottom' ? 'rh-top' : 'rh-left'}`}
          onPointerDown={startResize}
          title="Drag to resize"
        />
        {children}
      </div>
    </>
  );
}

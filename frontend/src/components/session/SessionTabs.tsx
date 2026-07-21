// セッションタブバー本体 (TopBar 直下)。AWS / GCP 共通で、ドメイン差分は
// AwsSessionTabs / GcpSessionTabs が表示用の items / picker に還元して渡す。
// タブが収まらないときはモック 7a の「他 N ▾」メニュー方式で畳む。
import { useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import type { SessionEnv } from '../../lib/sessionMeta';
import { SESSION_TAB_METRICS, computeVisibleTabCount } from '../../lib/sessionTabsLayout';

export interface SessionTabItem {
  id: string;
  label: string;
  env: SessionEnv;
}

export interface SessionTabsProps {
  items: SessionTabItem[];
  activeId: string;
  // ＋ボタンのラベル ('＋ プロファイルを追加' / '＋ プロジェクトを追加')
  addLabel: string;
  // 一覧に無い開きタブ (ドットをグレー表示し title で注記する)
  missingIds?: string[];
  // ＋ボタン直下に出すピッカー。close と現在の表示本数を渡す (選択直後の
  // swapToVisible 用。オーバーフロー中の追加タブを表示域へ入れる)。
  picker: (close: () => void, visibleCount: number) => ReactNode;
  onActivate: (id: string) => void;
  onClose: (id: string) => void;
  onReorder: (from: number, to: number) => void;
  onSwapToVisible: (id: string, visibleCount: number) => void;
  // テスト用 DI: jsdom では ResizeObserver が発火しないため表示本数を注入する
  visibleCountOverride?: number;
}

// タブ列以外 (左右 padding + ＋ボタン) に確保する幅。＋ボタンはオーバーフロー
// 時にラベル無しへ縮むが、計算は常にフルラベル幅を確保する保守値にする
// (visibleCount → ＋幅 → visibleCount の循環依存を避ける)。
const BAR_RESERVE = 160;

export function SessionTabs({
  items,
  activeId,
  addLabel,
  missingIds,
  picker,
  onActivate,
  onClose,
  onReorder,
  onSwapToVisible,
  visibleCountOverride,
}: SessionTabsProps) {
  const { t } = useTranslation('session');
  const rootRef = useRef<HTMLDivElement>(null);
  const [barWidth, setBarWidth] = useState(0);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [moreOpen, setMoreOpen] = useState(false);
  const [dragIndex, setDragIndex] = useState<number | null>(null);
  const [dropIndex, setDropIndex] = useState<number | null>(null);

  // バー幅の計測。ResizeObserver は幅変化でしか発火しないため、幅だけを
  // state に持ち、表示本数は items.length を含む useMemo の派生値にする
  // (タブ追加/削除でも再計算されることを保証する)。
  useLayoutEffect(() => {
    const el = rootRef.current;
    if (!el) return;
    setBarWidth(el.clientWidth);
    if (typeof ResizeObserver === 'undefined') return;
    const ro = new ResizeObserver((entries) => {
      const w = entries[0]?.contentRect.width ?? 0;
      setBarWidth((prev) => (prev === w ? prev : w));
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const visibleCount = useMemo(() => {
    if (visibleCountOverride !== undefined) {
      return Math.min(Math.max(0, visibleCountOverride), items.length);
    }
    // 未計測 (初回) と jsdom (clientWidth が常に 0) は全表示にフォールバック
    if (barWidth <= 0) return items.length;
    return computeVisibleTabCount(
      Math.max(0, barWidth - BAR_RESERVE),
      items.length,
      SESSION_TAB_METRICS,
    );
  }, [visibleCountOverride, barWidth, items.length]);

  const visibleTabs = items.slice(0, visibleCount);
  const hiddenTabs = items.slice(visibleCount);
  const hiddenHasActive = hiddenTabs.some((t) => t.id === activeId);
  const missing = useMemo(() => new Set(missingIds ?? []), [missingIds]);

  // ポップオーバー (ピッカー / 他 N メニュー) の外側クリック close
  useEffect(() => {
    if (!pickerOpen && !moreOpen) return;
    const onPointerDown = (e: PointerEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setPickerOpen(false);
        setMoreOpen(false);
      }
    };
    document.addEventListener('pointerdown', onPointerDown);
    return () => document.removeEventListener('pointerdown', onPointerDown);
  }, [pickerOpen, moreOpen]);

  // Ctrl+1–9 でタブ切替。入力系要素 (input / textarea / contentEditable /
  // CodeMirror / xterm) にフォーカスがあるときは奪わない。隠れ側タブは
  // activate ではなく swap して「アクティブなのに不可視」を防ぐ。
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (!e.ctrlKey || e.metaKey || e.altKey) return;
      const m = /^Digit([1-9])$/.exec(e.code);
      if (!m) return;
      // e.target は window / document のこともあるため Element 判定してから見る
      const target = e.target;
      if (
        target instanceof Element &&
        target.closest('input, textarea, [contenteditable="true"], .cm-editor, .xterm')
      ) {
        return;
      }
      const item = items[Number(m[1]) - 1];
      if (!item) return;
      e.preventDefault();
      onSwapToVisible(item.id, visibleCount);
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [items, visibleCount, onSwapToVisible]);

  const hint = (() => {
    if (hiddenTabs.length > 0) return t('sessionTabs.hintReorder');
    if (items.length === 0) return '';
    if (items.length === 1) return t('sessionTabs.hintSingle');
    return t('sessionTabs.hintMulti', { max: Math.min(items.length, 9) });
  })();

  return (
    <div className="session-tabs" ref={rootRef} role="tablist">
      {visibleTabs.map((item, i) => {
        const isMissing = missing.has(item.id);
        return (
          <div
            key={item.id}
            role="tab"
            aria-selected={item.id === activeId}
            title={isMissing ? t('sessionTabs.missingTitle', { label: item.label }) : item.label}
            className={`session-tab ${item.id === activeId ? 'active' : ''} ${
              dragIndex === i ? 'dragging' : ''
            } ${dropIndex === i && dragIndex !== null && dragIndex !== i ? 'drop-target' : ''}`}
            onClick={() => onActivate(item.id)}
            draggable
            onDragStart={(e) => {
              setDragIndex(i);
              e.dataTransfer.effectAllowed = 'move';
              e.dataTransfer.setData('text/plain', String(i));
            }}
            onDragOver={(e) => {
              e.preventDefault();
              setDropIndex((prev) => (prev === i ? prev : i));
            }}
            onDrop={(e) => {
              e.preventDefault();
              if (dragIndex !== null && dragIndex !== i) onReorder(dragIndex, i);
              setDragIndex(null);
              setDropIndex(null);
            }}
            onDragEnd={() => {
              setDragIndex(null);
              setDropIndex(null);
            }}
          >
            <span className={`session-tab-dot env-${item.env} ${isMissing ? 'missing' : ''}`} />
            <span className="session-tab-name">{item.label}</span>
            <button
              className="session-tab-close"
              aria-label={t('sessionTabs.closeTab', { label: item.label })}
              onClick={(e) => {
                e.stopPropagation();
                onClose(item.id);
              }}
            >
              ×
            </button>
          </div>
        );
      })}

      {hiddenTabs.length > 0 && (
        <div className="session-tabs-more-wrap">
          <button
            className={`session-tabs-more ${hiddenHasActive ? 'holds-active' : ''}`}
            aria-haspopup="menu"
            aria-expanded={moreOpen}
            onClick={() => {
              setMoreOpen((v) => !v);
              setPickerOpen(false);
            }}
          >
            {t('sessionTabs.more', { n: hiddenTabs.length })}
          </button>
          {moreOpen && (
            <div className="session-more-menu" role="menu">
              <div className="session-more-menu-head">{t('sessionTabs.moreMenuHead')}</div>
              <ul>
                {hiddenTabs.map((item) => {
                  const isMissing = missing.has(item.id);
                  return (
                    <li
                      key={item.id}
                      role="menuitem"
                      className="session-more-item"
                      title={
                        isMissing
                          ? t('sessionTabs.missingTitle', { label: item.label })
                          : item.label
                      }
                      onClick={() => {
                        onSwapToVisible(item.id, visibleCount);
                        setMoreOpen(false);
                      }}
                    >
                      <span
                        className={`session-tab-dot env-${item.env} ${isMissing ? 'missing' : ''}`}
                      />
                      <span className="session-more-item-name">{item.label}</span>
                      <button
                        className="session-tab-close"
                        aria-label={t('sessionTabs.closeTab', { label: item.label })}
                        onClick={(e) => {
                          e.stopPropagation();
                          onClose(item.id);
                        }}
                      >
                        ×
                      </button>
                    </li>
                  );
                })}
              </ul>
              <div className="session-more-menu-foot">{t('sessionTabs.moreMenuFoot')}</div>
            </div>
          )}
        </div>
      )}

      <div className="session-tab-add-wrap">
        <button
          className={`session-tab-add ${pickerOpen ? 'open' : ''}`}
          aria-haspopup="listbox"
          aria-expanded={pickerOpen}
          onClick={() => {
            setPickerOpen((v) => !v);
            setMoreOpen(false);
          }}
        >
          {hiddenTabs.length > 0 ? '＋' : addLabel}
        </button>
        {pickerOpen && picker(() => setPickerOpen(false), visibleCount)}
      </div>

      {hint && <span className="session-tabs-hint">{hint}</span>}
    </div>
  );
}

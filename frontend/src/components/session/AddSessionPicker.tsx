// セッション追加ピッカー (タブバーの ＋ ボタン直下に出すポップオーバーの中身)。
// 検索 + キーボード操作 (ArrowUp/Down/Enter/Escape) は旧 ProfileSelect の
// パターンを移植した。開設済み (disabled) 行はグレーアウトして選択不可にする。
// 外側クリックでの close はアンカー側 (SessionTabs) が担当する。
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import type { SessionPickerItem } from '../../lib/sessionMeta';
import { nextEnabledIndex } from '../../lib/sessionTabsState';
import { Icons } from '../icons/Icons';

export interface AddSessionPickerProps {
  items: SessionPickerItem[];
  placeholder: string;
  // ヘッダー右端の補足 ('~/.aws/config · N件' / 'gcloud projects list')
  headerNote: string;
  footerHint: string;
  emptyText: string;
  // ヘッダー右端に置く追加アクション (GCP のプロジェクト一覧 refresh ボタン)
  headerAction?: ReactNode;
  // ピッカー幅 (AWS 440px / GCP 400px。モック実測値)
  narrow?: boolean;
  // 一覧取得の失敗状態。true の間は「0件」ではなく取得エラーである旨と
  // 再試行導線を表示する (issue 0021: backend 起動前にページを開くと
  // 一覧が空のまま復旧しないバグの対策)。
  loadError?: boolean;
  onRetry?: () => void;
  onSelect: (id: string) => void;
  onClose: () => void;
}

export function AddSessionPicker({
  items,
  placeholder,
  headerNote,
  footerHint,
  emptyText,
  headerAction,
  narrow,
  loadError,
  onRetry,
  onSelect,
  onClose,
}: AddSessionPickerProps) {
  const { t } = useTranslation('session');
  const [search, setSearch] = useState('');
  const [activeIndex, setActiveIndex] = useState(-1);
  const inputRef = useRef<HTMLInputElement>(null);

  const filtered = useMemo(() => {
    const q = search.toLowerCase();
    if (!q) return items;
    return items.filter((item) => item.searchText.includes(q));
  }, [items, search]);

  // search だけでなく items の差し替え (GCP refresh 等) でもハイライトを
  // リセットする (stale な index が範囲外を指すのを防ぐ)。
  useEffect(() => {
    setActiveIndex(nextEnabledIndex(filtered, -1, 1));
  }, [filtered]);

  useEffect(() => {
    // input へのフォーカスはポップオーバー描画後に行う (ProfileSelect 踏襲)
    requestAnimationFrame(() => inputRef.current?.focus());
  }, []);

  const choose = (item: SessionPickerItem) => {
    if (item.disabled) return;
    onSelect(item.id);
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setActiveIndex((i) => {
        const next = nextEnabledIndex(filtered, i, 1);
        return next === -1 ? i : next;
      });
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setActiveIndex((i) => {
        const next = nextEnabledIndex(filtered, i, -1);
        return next === -1 ? i : next;
      });
    } else if (e.key === 'Enter') {
      e.preventDefault();
      const target = filtered[activeIndex];
      if (target) choose(target);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      // Drawer の document keydown (Escape で閉じる) と同時に発火しないよう止める
      e.stopPropagation();
      onClose();
    }
  };

  return (
    <div className={`session-picker ${narrow ? 'narrow' : ''}`}>
      <div className="session-picker-head">
        <span className="chip-search">
          <Icons.search size={12} />
          <input
            ref={inputRef}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onKeyDown={onKeyDown}
            placeholder={placeholder}
          />
        </span>
        <span className="session-picker-note">
          {headerNote}
          {headerAction}
        </span>
      </div>
      <ul className="session-picker-list" role="listbox">
        {loadError && (
          <li className="session-picker-empty session-picker-error">
            {t('addSessionPicker.loadError')}
            {onRetry && (
              <button className="btn sm ghost" onClick={onRetry}>
                {t('addSessionPicker.retry')}
              </button>
            )}
          </li>
        )}
        {!loadError && filtered.length === 0 && (
          <li className="session-picker-empty">{emptyText}</li>
        )}
        {filtered.map((item, i) => (
          <li
            key={item.id}
            role="option"
            aria-selected={i === activeIndex}
            aria-disabled={item.disabled || undefined}
            className={`session-picker-item ${i === activeIndex ? 'active' : ''} ${item.disabled ? 'disabled' : ''}`}
            onMouseEnter={() => {
              if (!item.disabled) setActiveIndex(i);
            }}
            onClick={() => choose(item)}
          >
            <span className="session-picker-item-name">{item.name}</span>
            {item.meta && <span className="session-picker-item-meta">{item.meta}</span>}
            <span className="session-picker-item-right">
              {item.disabled ? (
                <span className="session-picker-item-hint">{t('addSessionPicker.opened')}</span>
              ) : (
                <>
                  {item.badge && (
                    <span className={`session-picker-badge ${item.badge.tone}`}>
                      {item.badge.label}
                    </span>
                  )}
                  {i === activeIndex && (
                    <span className="session-picker-item-hint">
                      {t('addSessionPicker.enterHint')}
                    </span>
                  )}
                </>
              )}
            </span>
          </li>
        ))}
      </ul>
      <div className="session-picker-foot">{footerHint}</div>
    </div>
  );
}

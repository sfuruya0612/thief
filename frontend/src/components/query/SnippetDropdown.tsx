// ツールバーの「スニペット ▾」ドロップダウン。
// 一覧からの挿入と「現在のクエリをスニペットに保存」を提供する。
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { NamedQuery } from '../../types/query';

export interface SnippetDropdownProps {
  snippets: NamedQuery[];
  onInsert: (snippet: NamedQuery) => void;
  onSaveCurrent: () => void;
  onDelete: (id: string) => void;
}

export function SnippetDropdown({
  snippets,
  onInsert,
  onSaveCurrent,
  onDelete,
}: SnippetDropdownProps) {
  const { t } = useTranslation('query');
  const [open, setOpen] = useState(false);
  const wrapperRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (e: PointerEvent) => {
      if (wrapperRef.current && !wrapperRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('pointerdown', onPointerDown);
    return () => document.removeEventListener('pointerdown', onPointerDown);
  }, [open]);

  return (
    <div className="qe-snippet-dd" ref={wrapperRef}>
      <button className={`btn sm ${open ? 'active' : ''}`} onClick={() => setOpen((v) => !v)}>
        {t('snippetDropdown.button')}
      </button>
      {open && (
        <div className="qe-snippet-menu">
          <div className="qe-snippet-menu-head">{t('snippetDropdown.heading')}</div>
          <div className="qe-snippet-menu-list">
            {snippets.map((s) => (
              <div
                key={s.id}
                className="qe-snippet-item"
                onClick={() => {
                  onInsert(s);
                  setOpen(false);
                }}
              >
                <div className="qe-snippet-item-body">
                  <b>{s.name}</b>
                  <span className="qe-snippet-item-sql">{s.sql}</span>
                </div>
                <button
                  className="qe-snippet-item-delete"
                  title={t('snippetDropdown.deleteSnippet')}
                  onClick={(e) => {
                    e.stopPropagation();
                    onDelete(s.id);
                  }}
                >
                  ×
                </button>
              </div>
            ))}
            {snippets.length === 0 && (
              <div className="qe-snippet-empty">{t('snippetDropdown.empty')}</div>
            )}
          </div>
          <button
            className="qe-snippet-menu-save"
            onClick={() => {
              onSaveCurrent();
              setOpen(false);
            }}
          >
            {t('snippetDropdown.saveCurrent')}
          </button>
        </div>
      )}
    </div>
  );
}

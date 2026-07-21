// エディタパネル上部のタブバー。ダブルクリックでリネーム、× でクローズ、＋ で追加。
import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import type { QueryTab } from '../../types/query';

export interface EditorTabsBarProps {
  tabs: QueryTab[];
  activeTabId: string;
  onSelect: (id: string) => void;
  onClose: (id: string) => void;
  onAdd: () => void;
  onRename: (id: string, name: string) => void;
  right?: ReactNode;
}

export function EditorTabsBar({
  tabs,
  activeTabId,
  onSelect,
  onClose,
  onAdd,
  onRename,
  right,
}: EditorTabsBarProps) {
  const { t } = useTranslation('query');
  const [editingId, setEditingId] = useState<string | null>(null);
  const [draft, setDraft] = useState('');

  const commit = () => {
    if (editingId !== null) {
      onRename(editingId, draft);
      setEditingId(null);
    }
  };

  return (
    <div className="qe-tabs">
      {tabs.map((tab) => (
        <div
          key={tab.id}
          className={`qe-tab ${tab.id === activeTabId ? 'active' : ''}`}
          onClick={() => onSelect(tab.id)}
          onDoubleClick={() => {
            setEditingId(tab.id);
            setDraft(tab.name);
          }}
          title={tab.name}
        >
          {editingId === tab.id ? (
            <input
              className="qe-tab-rename"
              value={draft}
              autoFocus
              onChange={(e) => setDraft(e.target.value)}
              onClick={(e) => e.stopPropagation()}
              onBlur={commit}
              onKeyDown={(e) => {
                if (e.key === 'Enter') commit();
                if (e.key === 'Escape') setEditingId(null);
              }}
            />
          ) : (
            <span className="qe-tab-name">{tab.name}</span>
          )}
          <button
            className="qe-tab-close"
            title={t('editorTabsBar.closeTab')}
            onClick={(e) => {
              e.stopPropagation();
              onClose(tab.id);
            }}
          >
            ×
          </button>
        </div>
      ))}
      <button className="qe-tab-add" title={t('editorTabsBar.newTab')} onClick={onAdd}>
        ＋
      </button>
      <span className="qe-tabs-right">{right}</span>
    </div>
  );
}

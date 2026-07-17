// エディタパネル上部のタブバー。ダブルクリックでリネーム、× でクローズ、＋ で追加。
import { useState, type ReactNode } from 'react';
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
      {tabs.map((t) => (
        <div
          key={t.id}
          className={`qe-tab ${t.id === activeTabId ? 'active' : ''}`}
          onClick={() => onSelect(t.id)}
          onDoubleClick={() => {
            setEditingId(t.id);
            setDraft(t.name);
          }}
          title={t.name}
        >
          {editingId === t.id ? (
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
            <span className="qe-tab-name">{t.name}</span>
          )}
          <button
            className="qe-tab-close"
            title="タブを閉じる"
            onClick={(e) => {
              e.stopPropagation();
              onClose(t.id);
            }}
          >
            ×
          </button>
        </div>
      ))}
      <button className="qe-tab-add" title="新しいタブ" onClick={onAdd}>
        ＋
      </button>
      <span className="qe-tabs-right">{right}</span>
    </div>
  );
}

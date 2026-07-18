// ログビューア左ペインの汎用チェックボックスツリー。
// CloudWatch Logs (ロググループの prefix ツリー) と Cloud Logging (リソースタイプ別) の
// 両方で使う。親ノードは開閉、葉ノードは複数選択のチェックボックス。
import { type ReactNode, useMemo, useState } from 'react';
import type { LogTreeNode } from '../../lib/logGroupTree';

export interface LogTreeProps {
  nodes: LogTreeNode[];
  // 選択中の葉ノードの value (CloudWatch はロググループ ARN、Cloud Logging は resource.type) の集合。
  selected: Set<string>;
  onToggle: (value: string) => void;
  searchPlaceholder: string;
  footer?: ReactNode;
  emptyMessage?: string;
  loading?: boolean;
}

export function LogTree({
  nodes,
  selected,
  onToggle,
  searchPlaceholder,
  footer,
  emptyMessage,
  loading,
}: LogTreeProps) {
  const [search, setSearch] = useState('');
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());

  const query = search.trim().toLowerCase();

  // 検索語がある場合は親ラベル一致でその親配下を全表示、なければ葉ラベル一致の葉だけ残す。
  const filtered = useMemo(() => {
    if (!query) return nodes;
    return nodes
      .map((parent) => {
        if (parent.label.toLowerCase().includes(query)) return parent;
        const kids = (parent.children ?? []).filter((c) => c.label.toLowerCase().includes(query));
        return kids.length ? { ...parent, children: kids } : null;
      })
      .filter((p): p is LogTreeNode => p !== null);
  }, [nodes, query]);

  const toggleCollapse = (key: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  return (
    <div className="lv-tree-inner">
      <div className="lv-tree-search">
        <input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder={searchPlaceholder}
        />
      </div>
      <div className="lv-tree-nodes">
        {loading ? (
          <div className="lv-tree-empty">読み込み中…</div>
        ) : filtered.length === 0 ? (
          <div className="lv-tree-empty">{emptyMessage ?? '該当なし'}</div>
        ) : (
          filtered.map((parent) => {
            // 検索中は親を常に開いた状態にする。
            const isCollapsed = !query && collapsed.has(parent.key);
            return (
              <div key={parent.key} className="lv-tree-group">
                <div className="lv-tree-parent" onClick={() => toggleCollapse(parent.key)}>
                  <span className="lv-tree-caret">{isCollapsed ? '▸' : '▾'}</span>
                  <span className="lv-tree-parent-label">{parent.label}</span>
                </div>
                {!isCollapsed &&
                  (parent.children ?? []).map((leaf) => {
                    const checked = leaf.value !== undefined && selected.has(leaf.value);
                    return (
                      <label key={leaf.key} className={`lv-tree-leaf ${checked ? 'checked' : ''}`}>
                        <input
                          type="checkbox"
                          checked={checked}
                          onChange={() => leaf.value !== undefined && onToggle(leaf.value)}
                        />
                        <span className="lv-tree-leaf-label">{leaf.label}</span>
                      </label>
                    );
                  })}
              </div>
            );
          })
        )}
      </div>
      {footer && <div className="lv-tree-footer">{footer}</div>}
    </div>
  );
}

// スニペット / 保存クエリの管理と localStorage 永続化
import { useCallback, useEffect, useState } from 'react';
import type { NamedQuery } from '../../types/query';
import {
  loadNamedQueries,
  type NamedQueryKind,
  newLocalId,
  type QueryEditorService,
  saveNamedQueries,
} from '../../lib/queryEditorStorage';

export interface NamedQueriesApi {
  items: NamedQuery[];
  add: (name: string, sql: string) => void;
  remove: (id: string) => void;
}

export function useNamedQueries(
  service: QueryEditorService,
  scope: string,
  kind: NamedQueryKind,
): NamedQueriesApi {
  const [items, setItems] = useState<NamedQuery[]>(() => loadNamedQueries(service, scope, kind));

  useEffect(() => {
    saveNamedQueries(service, scope, kind, items);
  }, [service, scope, kind, items]);

  const add = useCallback(
    (name: string, sql: string) => {
      const trimmedName = name.trim();
      if (!trimmedName || !sql.trim()) return;
      setItems((prev) => [
        { id: newLocalId(kind), name: trimmedName, sql, updatedAt: new Date().toISOString() },
        ...prev,
      ]);
    },
    [kind],
  );

  const remove = useCallback((id: string) => {
    setItems((prev) => prev.filter((q) => q.id !== id));
  }, []);

  return { items, add, remove };
}

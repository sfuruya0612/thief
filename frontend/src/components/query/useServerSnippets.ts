// スニペットの管理 (backend のサービス別ファイル保存 API 経由)。
// 保存クエリ用の useNamedQueries と同じ形 (items / add / remove) を提供し、
// ビュー側の差し替えを最小にする。
import { useCallback } from 'react';
import { useDeleteSnippet, useSaveSnippet, useSnippets } from '../../api/queries';
import type { QueryEditorService } from '../../lib/queryEditorStorage';
import type { NamedQuery } from '../../types/query';

export interface ServerSnippetsApi {
  items: NamedQuery[];
  add: (name: string, sql: string) => void;
  remove: (id: string) => void;
  // 一覧取得 / 保存 / 削除のいずれかで発生した直近のエラー
  error: Error | null;
}

export function useServerSnippets(service: QueryEditorService): ServerSnippetsApi {
  const list = useSnippets(service);
  const save = useSaveSnippet(service);
  const del = useDeleteSnippet(service);
  // mutation オブジェクトは毎レンダー再生成されるが mutate は参照が安定しているため、
  // デストラクチャして useCallback の依存に使う
  const { mutate: saveMutate } = save;
  const { mutate: deleteMutate } = del;

  const add = useCallback(
    (name: string, sql: string) => {
      const trimmedName = name.trim();
      if (!trimmedName || !sql.trim()) return;
      saveMutate({ name: trimmedName, sql });
    },
    [saveMutate],
  );

  const remove = useCallback((id: string) => deleteMutate(id), [deleteMutate]);

  return {
    items: list.data ?? [],
    add,
    remove,
    error: (list.error ?? save.error ?? del.error) as Error | null,
  };
}

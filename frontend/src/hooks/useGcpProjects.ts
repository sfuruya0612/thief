// GCP のアクティブプロジェクト選択状態を管理するフック。
// 一覧取得 (useGcpProjects in api/queries.ts) と役割を分けるため、こちらは
// useActiveGcpProject という名前にしてある。
import { useCallback, useEffect, useState } from 'react';
import { useGcpProjects as useGcpProjectsQuery } from '../api/queries';
import { loadPersisted, savePersisted } from '../lib/storage';

export function useActiveGcpProject() {
  const query = useGcpProjectsQuery();
  const [activeProject, setActiveProjectState] = useState<string>(() => {
    return loadPersisted().gcpProject ?? '';
  });

  // プロジェクト一覧取得後、未選択 または 永続化された値が一覧に無い場合は先頭を採用する
  useEffect(() => {
    if (!query.data || query.data.length === 0) return;
    const exists = activeProject && query.data.some((p) => p.id === activeProject);
    if (!exists) {
      setActiveProjectState(query.data[0].id);
    }
  }, [activeProject, query.data]);

  useEffect(() => {
    if (!activeProject) return;
    const prev = loadPersisted();
    savePersisted({ ...prev, gcpProject: activeProject });
  }, [activeProject]);

  const setActiveProject = useCallback((id: string) => {
    setActiveProjectState(id);
  }, []);

  return {
    projects: query.data ?? [],
    isLoading: query.isLoading,
    error: query.error,
    activeProject,
    setActiveProject,
  };
}

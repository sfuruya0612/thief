import { useCallback, useEffect, useState } from 'react';
import { useProfiles as useProfilesQuery } from '../api/queries';
import { loadPersisted, savePersisted } from '../lib/storage';

export function useProfiles() {
  const query = useProfilesQuery();
  const [activeProfile, setActiveProfileState] = useState<string>(() => {
    return loadPersisted().activeProfile ?? '';
  });

  // 初回にプロファイルが取得できた際、activeProfile が未設定なら先頭を採用する
  useEffect(() => {
    if (!activeProfile && query.data && query.data.length > 0) {
      setActiveProfileState(query.data[0].name);
    }
  }, [activeProfile, query.data]);

  useEffect(() => {
    if (!activeProfile) return;
    const prev = loadPersisted();
    savePersisted({ ...prev, activeProfile });
  }, [activeProfile]);

  const setActiveProfile = useCallback((name: string) => {
    setActiveProfileState(name);
  }, []);

  return {
    profiles: query.data ?? [],
    isLoading: query.isLoading,
    error: query.error,
    activeProfile,
    setActiveProfile,
  };
}

import type { AppView, Tweaks } from '../types/common';

export const STORAGE_KEY = 'cloudlens:v1';

export interface PersistedState {
  activeProfile?: string;
  perProfileState?: Record<string, unknown>;
  tweaks?: Tweaks;
  region?: string;
  view?: AppView;
  sidebarWidth?: number;
  gcpProject?: string;
}

export function loadState<T>(key: string, fallback: T): T {
  try {
    const raw = localStorage.getItem(key);
    if (raw === null) return fallback;
    return JSON.parse(raw) as T;
  } catch {
    return fallback;
  }
}

export function saveState<T>(key: string, value: T): void {
  try {
    localStorage.setItem(key, JSON.stringify(value));
  } catch {
    // ignore quota / serialization errors
  }
}

export function loadPersisted(): PersistedState {
  const state = loadState<PersistedState>(STORAGE_KEY, {});
  // 旧 AppView 値 'bigquery' → 'gcp' への後方互換マイグレーション
  // (BigQuery 単独ビューは GCP 統合ビューに吸収されたため、古い localStorage 値のままだと
  // 現在の AppView 型と矛盾して空画面になる)
  if ((state.view as string | undefined) === 'bigquery') {
    state.view = 'gcp';
  }
  return state;
}

export function savePersisted(s: PersistedState): void {
  saveState<PersistedState>(STORAGE_KEY, s);
}

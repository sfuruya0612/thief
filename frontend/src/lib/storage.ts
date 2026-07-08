import type { AppView, Tweaks } from '../types/common';

export const STORAGE_KEY = 'cloudlens:v1';

export interface PersistedState {
  activeProfile?: string;
  perProfileState?: Record<string, unknown>;
  tweaks?: Tweaks;
  region?: string;
  view?: AppView;
  sidebarWidth?: number;
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
  return loadState<PersistedState>(STORAGE_KEY, {});
}

export function savePersisted(s: PersistedState): void {
  saveState<PersistedState>(STORAGE_KEY, s);
}

import type { AppView, Tweaks } from '../types/common';
import type { SessionTabsState } from './sessionTabsState';
import { normalizeSessionState, openSession } from './sessionTabsState';

export const STORAGE_KEY = 'cloudlens:v1';

export interface PersistedState {
  // 旧形式の単一選択フィールド。awsSessions / gcpSessions 導入後も
  // アクティブタブを常にミラーして書き続ける (旧バージョンへのロール
  // バック互換 + 旧バージョンで行われた選択変更の合流に使う)。
  activeProfile?: string;
  perProfileState?: Record<string, unknown>;
  tweaks?: Tweaks;
  region?: string;
  view?: AppView;
  sidebarWidth?: number;
  // Athena / CloudWatch Logs / BigQuery / Cloud Logging の左パネル (.qe-schema / .lv-tree) 幅
  resourcePanelWidth?: number;
  gcpProject?: string;
  // Cloud Logging の SUMMARY 列に先頭表示するフィールドキー (選択順 = 表示順)
  gcpLogSummaryFields?: string[];
  // セッションタブ (開いている複数セッション + アクティブ)
  awsSessions?: SessionTabsState;
  gcpSessions?: SessionTabsState;
  pricing?: PricingPersistedState;
}

// Pricing 画面の選択状態。selection は region -> service -> rate_id -> チェック状態と数量。
// リージョンを最上位のキーにするのは、リージョン切替で rate_id が変わり得るため
// (lib/pricingEstimate.ts の estimate() は現在のリージョンの選択だけを渡される想定)。
export interface PricingPersistedState {
  activeServices: string[];
  collapsed: Record<string, boolean>;
  selection: Record<string, Record<string, Record<string, { checked: boolean; qty: number }>>>;
  // 単調増加のスキーマ版。既定 active なサービスを追加するリリースごとに版を上げ、
  // 旧版の永続化データに対しては新メンバーを一度だけ activeServices へ補完する
  // (lib/pricingSelection.ts の PRICING_SCHEMA_VERSION / migratePricingState 参照)。
  // 未設定は版 0 (最初の 4 サービス構成) を意味する。
  pricingSchemaVersion?: number;
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

// 単一選択フィールド (activeProfile / gcpProject) とセッションタブの整合を取る。
// 冪等な純関数 (2 回適用しても結果が変わらない) として実装し、StrictMode の
// 二重実行や壊れた手編集データでも throw しない。
function migrateSessions(
  sessions: SessionTabsState | undefined,
  legacyActive: unknown,
): SessionTabsState | undefined {
  const legacy = typeof legacyActive === 'string' ? legacyActive : '';
  if (sessions === undefined) {
    // 旧形式のみ → 単一タブとして引き継ぐ。旧形式も無ければ未定義のまま
    // (初回起動: 一覧ロード後の自動オープンに任せる)。
    if (legacy === '') return undefined;
    return normalizeSessionState({ open: [legacy], active: legacy });
  }
  let next = normalizeSessionState(sessions);
  // 旧バージョンに戻って選択変更した後に新バージョンへ来た場合、旧フィールド
  // だけが書き換わり active と食い違う。旧側の選択をタブ集合へ合流させる。
  if (legacy !== '' && legacy !== next.active) {
    next = openSession(next, legacy);
  }
  return next;
}

export function loadPersisted(): PersistedState {
  const state = loadState<PersistedState>(STORAGE_KEY, {});
  // 旧 AppView 値 'bigquery' → 'gcp' への後方互換マイグレーション
  // (BigQuery 単独ビューは GCP 統合ビューに吸収されたため、古い localStorage 値のままだと
  // 現在の AppView 型と矛盾して空画面になる)
  if ((state.view as string | undefined) === 'bigquery') {
    state.view = 'gcp';
  }
  const awsSessions = migrateSessions(state.awsSessions, state.activeProfile);
  if (awsSessions !== undefined) state.awsSessions = awsSessions;
  const gcpSessions = migrateSessions(state.gcpSessions, state.gcpProject);
  if (gcpSessions !== undefined) state.gcpSessions = gcpSessions;
  return state;
}

export function savePersisted(s: PersistedState): void {
  saveState<PersistedState>(STORAGE_KEY, s);
}

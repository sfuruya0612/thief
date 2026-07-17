// クエリエディタの localStorage 永続化 (タブ / 保存クエリ / Athena コンテキスト)
// キーはサービスとスコープ (BigQuery: projectId, Athena: profile) で分離する
// スニペットは localStorage ではなく backend のファイル保存 API (/api/snippets) を使う
import type { NamedQuery, QueryTab } from '../types/query';
import { loadState, saveState } from './storage';

const KEY_PREFIX = 'cloudlens:qe:v1';

export type QueryEditorService = 'bigquery' | 'athena';

export interface EditorTabsState {
  tabs: QueryTab[];
  activeTabId: string;
}

// Athena のヘッダーセレクタ (Catalog / Database / Workgroup) の選択状態と
// クエリ結果の S3 出力先 (workgroup 側に設定が無い場合に必須)
export interface AthenaContext {
  catalog?: string;
  database?: string;
  workgroup?: string;
  outputLocation?: string;
}

function scopedKey(service: QueryEditorService, scope: string, kind: string): string {
  return `${KEY_PREFIX}:${service}:${scope || 'default'}:${kind}`;
}

// タブ ID / スニペット ID の生成 (localStorage 内で一意になれば十分)
export function newLocalId(prefix: string): string {
  return `${prefix}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
}

// 既存タブ名から "untitled N" の次の連番を決める
export function untitledName(tabs: QueryTab[]): string {
  let max = 0;
  for (const t of tabs) {
    const m = /^untitled (\d+)$/.exec(t.name);
    if (m) max = Math.max(max, Number(m[1]));
  }
  return `untitled ${max + 1}`;
}

// タブ状態を読み込む。空・不整合の場合は defaultSql を持つ 1 タブへフォールバックする
export function loadEditorTabs(
  service: QueryEditorService,
  scope: string,
  defaultSql: string,
): EditorTabsState {
  const stored = loadState<EditorTabsState | null>(scopedKey(service, scope, 'tabs'), null);
  if (stored && Array.isArray(stored.tabs) && stored.tabs.length > 0) {
    const tabs = stored.tabs.filter(
      (t) => t && typeof t.id === 'string' && typeof t.sql === 'string',
    );
    if (tabs.length > 0) {
      const activeTabId = tabs.some((t) => t.id === stored.activeTabId)
        ? stored.activeTabId
        : tabs[0].id;
      return { tabs, activeTabId };
    }
  }
  const tab: QueryTab = { id: newLocalId('tab'), name: 'untitled 1', sql: defaultSql };
  return { tabs: [tab], activeTabId: tab.id };
}

export function saveEditorTabs(
  service: QueryEditorService,
  scope: string,
  state: EditorTabsState,
): void {
  saveState(scopedKey(service, scope, 'tabs'), state);
}

export type NamedQueryKind = 'saved';

export function loadNamedQueries(
  service: QueryEditorService,
  scope: string,
  kind: NamedQueryKind,
): NamedQuery[] {
  const stored = loadState<NamedQuery[] | null>(scopedKey(service, scope, kind), null);
  if (!Array.isArray(stored)) return [];
  return stored.filter((q) => q && typeof q.id === 'string' && typeof q.sql === 'string');
}

export function saveNamedQueries(
  service: QueryEditorService,
  scope: string,
  kind: NamedQueryKind,
  queries: NamedQuery[],
): void {
  saveState(scopedKey(service, scope, kind), queries);
}

export function loadAthenaContext(profile: string): AthenaContext {
  return loadState<AthenaContext>(scopedKey('athena', profile, 'context'), {});
}

export function saveAthenaContext(profile: string, ctx: AthenaContext): void {
  saveState(scopedKey('athena', profile, 'context'), ctx);
}

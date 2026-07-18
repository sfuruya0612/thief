// Athena ビュー: クエリエディタ構成 (デザイン 2b/3b)。
// ヘッダー右に Catalog / Database / Workgroup セレクタ、左スキーマツリー
// (database → table → columns、パーティション列は橙表示)、エディタ + 結果パネル。
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { format as formatSql } from 'sql-formatter';
import {
  useAthenaCatalogs,
  useAthenaDatabases,
  useAthenaExecution,
  useAthenaQueryHistory,
  useAthenaResults,
  useAthenaStartQuery,
  useAthenaStopQuery,
  useAthenaTables,
  useAthenaWorkgroups,
} from '../api/queries';
import { ErrorBanner } from '../components/ErrorBanner';
import { Loading } from '../components/Loading';
import { SSOExpiredBanner } from '../components/SSOExpiredBanner';
import { formatBytes } from '../components/tables/columns';
import { EditorTabsBar } from '../components/query/EditorTabsBar';
import { NamedQueryList } from '../components/query/NamedQueryList';
import { QueryHistoryTable } from '../components/query/QueryHistoryTable';
import { ResultsPanel, type ResultsTabKey } from '../components/query/ResultsPanel';
import { ResultTable } from '../components/query/ResultTable';
import { SchemaTreePanel, SchemaTreeRow } from '../components/query/SchemaTree';
import { SnippetDropdown } from '../components/query/SnippetDropdown';
import { SqlEditor, type SqlEditorHandle } from '../components/query/SqlEditor';
import { useEditorTabs } from '../components/query/useEditorTabs';
import { useNamedQueries } from '../components/query/useNamedQueries';
import { useServerSnippets } from '../components/query/useServerSnippets';
import { athenaResultsFromPages } from '../lib/normalizeQuery';
import {
  type AthenaContext,
  loadAthenaContext,
  saveAthenaContext,
} from '../lib/queryEditorStorage';
import { formatDurationClock, s3Dir, shortId, toCsv } from '../lib/queryFormat';
import { ApiError } from '../types/common';
import type { AthenaTableRow, NamedQuery, QueryHistoryRow } from '../types/query';

const DEFAULT_CATALOG = 'AwsDataCatalog';
const DEFAULT_WORKGROUP = 'primary';

export interface AthenaViewProps {
  profile: string;
  region: string;
}

export function AthenaView({ profile, region }: AthenaViewProps) {
  // プロファイル / リージョン切替時は key で再マウントし、タブとセレクタ状態を読み直す
  return <AthenaEditor key={`${profile}:${region}`} profile={profile} region={region} />;
}

// 候補一覧の中から永続値 → 既定値 → 先頭 の優先順で選択値を決める
function pickOption(options: string[], persisted?: string, fallback?: string): string {
  if (persisted && options.includes(persisted)) return persisted;
  if (fallback && options.includes(fallback)) return fallback;
  return options[0] ?? '';
}

function isSSOExpired(...errors: unknown[]): boolean {
  return errors.some((e) => e instanceof ApiError && e.code === 'SSO_TOKEN_EXPIRED');
}

function AthenaEditor({ profile, region }: AthenaViewProps) {
  const tabsApi = useEditorTabs('athena', profile, '');
  const { activeTab } = tabsApi;
  const editorRef = useRef<SqlEditorHandle | null>(null);
  const [resultsTab, setResultsTab] = useState<ResultsTabKey>('results');
  const [execs, setExecs] = useState<Record<string, string>>({});
  const [ctx, setCtx] = useState<AthenaContext>(() => loadAthenaContext(profile));

  useEffect(() => {
    saveAthenaContext(profile, ctx);
  }, [profile, ctx]);

  const snippets = useServerSnippets('athena');
  const saved = useNamedQueries('athena', profile, 'saved');

  const catalogs = useAthenaCatalogs(profile, region);
  const workgroups = useAthenaWorkgroups(profile, region);

  const catalog = pickOption(
    (catalogs.data ?? []).map((c) => c.name),
    ctx.catalog,
    DEFAULT_CATALOG,
  );
  const databases = useAthenaDatabases(profile, region, catalog || undefined);
  const database = pickOption(
    (databases.data ?? []).map((d) => d.name),
    ctx.database,
  );
  const workgroup = pickOption(
    (workgroups.data ?? []).map((w) => w.name),
    ctx.workgroup,
    DEFAULT_WORKGROUP,
  );

  const tables = useAthenaTables(profile, region, database || undefined, catalog || undefined);

  // ユーザー指定のクエリ結果出力先。空の場合は workgroup 側の設定に委ねる
  const configuredOutput = (ctx.outputLocation ?? '').trim();

  // 補完スキーマ: 選択中 database のテーブル → カラム (パーティション列含む)
  const schema = useMemo(() => {
    const map: Record<string, string[]> = {};
    for (const t of tables.data ?? []) {
      map[t.name] = [...t.columns.map((c) => c.name), ...t.partitionKeys.map((c) => c.name)];
    }
    return map;
  }, [tables.data]);

  const start = useAthenaStartQuery(profile, region);
  const stop = useAthenaStopQuery(profile, region);

  const activeExecId = execs[activeTab.id];
  const execStatus = useAthenaExecution(profile, region, activeExecId);
  const status = execStatus.data;
  const results = useAthenaResults(profile, region, activeExecId, status?.state === 'succeeded');
  const resultData = useMemo(
    () => athenaResultsFromPages(results.data?.pages ?? []),
    [results.data],
  );

  const history = useAthenaQueryHistory(profile, region, workgroup || undefined, true);

  const runSql = useCallback(
    (tabId: string, sql: string) => {
      const text = sql.trim();
      if (!text) return;
      start.mutate(
        {
          sql: text,
          catalog: catalog || undefined,
          database: database || undefined,
          workgroup: workgroup || undefined,
          output_location: configuredOutput || undefined,
        },
        {
          onSuccess: (exec) => {
            setExecs((prev) => ({ ...prev, [tabId]: exec.id }));
            setResultsTab('results');
          },
        },
      );
    },
    [start, catalog, database, workgroup, configuredOutput],
  );

  const runActive = useCallback(() => {
    runSql(tabsApi.activeTab.id, tabsApi.activeTab.sql);
  }, [runSql, tabsApi.activeTab]);

  const doFormat = useCallback(() => {
    try {
      const formatted = formatSql(activeTab.sql, { language: 'trino', keywordCase: 'upper' });
      editorRef.current?.replaceAll(formatted);
    } catch {
      // パースできない SQL はフォーマットせずそのまま残す
    }
  }, [activeTab.sql]);

  const doCancel = useCallback(() => {
    if (!activeExecId) return;
    stop.mutate(activeExecId, { onSuccess: () => void execStatus.refetch() });
  }, [activeExecId, stop, execStatus]);

  const openFromHistory = useCallback(
    (item: QueryHistoryRow) => {
      tabsApi.addTab(item.sql);
    },
    [tabsApi],
  );

  const rerunFromHistory = useCallback(
    (item: QueryHistoryRow) => {
      const tabId = tabsApi.addTab(item.sql);
      runSql(tabId, item.sql);
    },
    [tabsApi, runSql],
  );

  const openNamedQuery = useCallback(
    (q: NamedQuery) => {
      tabsApi.addTab(q.sql, q.name);
    },
    [tabsApi],
  );

  const saveSnippet = useCallback(() => {
    const name = window.prompt('スニペット名', activeTab.name);
    if (name) snippets.add(name, activeTab.sql);
  }, [activeTab.name, activeTab.sql, snippets]);

  const saveQuery = useCallback(() => {
    const name = window.prompt('保存クエリ名', activeTab.name);
    if (name) saved.add(name, activeTab.sql);
  }, [activeTab.name, activeTab.sql, saved]);

  const copyCsv = useCallback(() => {
    void navigator.clipboard.writeText(toCsv(resultData.columns, resultData.rows));
  }, [resultData]);

  const insertText = useCallback((text: string) => {
    editorRef.current?.insertText(text);
  }, []);

  const running = status?.state === 'queued' || status?.state === 'running';
  // 結果出力先の表示: アクティブな実行 → ユーザー指定 → 履歴の先頭 の順で分かっている値を使う
  const outputLocation =
    status?.outputLocation ||
    configuredOutput ||
    (history.data ?? []).find((h) => h.outputLocation)?.outputLocation;
  const lastFinished = !running && status && status.state !== 'queued' ? status : undefined;
  const actionError = start.error as Error | null;

  const ssoExpired = isSSOExpired(
    catalogs.error,
    workgroups.error,
    databases.error,
    tables.error,
    history.error,
    execStatus.error,
    start.error,
  );
  const listError = catalogs.error ?? workgroups.error ?? databases.error ?? tables.error;

  const resultsStatus = status ? (
    <>
      <span className={`qe-status-text ${status.state}`}>● {status.stateLabel}</span>
      <span className="qe-status-mono">
        {formatDurationClock(status.elapsedMs)} · {formatBytes(status.bytes)}
      </span>
      <span className="qe-status-mono dim">{shortId(status.id)}</span>
      <button className="btn sm" onClick={copyCsv} disabled={resultData.rows.length === 0}>
        CSVコピー
      </button>
    </>
  ) : null;

  return (
    <div className="main qe-main">
      <div className="toolbar">
        <div className="title">
          <h1>Athena</h1>
          <span className="subtitle">query editor</span>
        </div>
        <div className="qe-selectors">
          <span className="qe-selector-label">Catalog</span>
          <select
            className="btn sm"
            value={catalog}
            onChange={(e) =>
              setCtx((c) => ({ ...c, catalog: e.target.value, database: undefined }))
            }
          >
            {(catalogs.data ?? []).map((c) => (
              <option key={c.name} value={c.name}>
                {c.name}
              </option>
            ))}
            {(catalogs.data ?? []).length === 0 && <option value={catalog}>{catalog}</option>}
          </select>
          <span className="qe-selector-label">Database</span>
          <select
            className="btn sm"
            value={database}
            onChange={(e) => setCtx((c) => ({ ...c, database: e.target.value }))}
          >
            {(databases.data ?? []).map((d) => (
              <option key={d.name} value={d.name}>
                {d.name}
              </option>
            ))}
            {(databases.data ?? []).length === 0 && <option value={database}>{database}</option>}
          </select>
          <span className="qe-selector-label">Workgroup</span>
          <select
            className="btn sm"
            value={workgroup}
            onChange={(e) => setCtx((c) => ({ ...c, workgroup: e.target.value }))}
          >
            {(workgroups.data ?? []).map((w) => (
              <option key={w.name} value={w.name}>
                {w.name}
              </option>
            ))}
            {(workgroups.data ?? []).length === 0 && <option value={workgroup}>{workgroup}</option>}
          </select>
          <span className="qe-selector-label">出力先</span>
          <input
            className="qe-output-input"
            placeholder="s3://bucket/prefix/ (省略時は workgroup 設定)"
            title="クエリ結果の S3 出力先。workgroup 側に設定が無い場合は指定が必須"
            value={ctx.outputLocation ?? ''}
            onChange={(e) => setCtx((c) => ({ ...c, outputLocation: e.target.value }))}
          />
        </div>
      </div>

      {ssoExpired && <SSOExpiredBanner profile={profile} />}
      {!ssoExpired && listError ? <ErrorBanner error={listError} /> : null}

      <div className="qe-layout">
        <AthenaSchemaTree
          databases={(databases.data ?? []).map((d) => d.name)}
          databasesLoading={databases.isLoading}
          selectedDatabase={database}
          onSelectDatabase={(db) => setCtx((c) => ({ ...c, database: db }))}
          tables={tables.data ?? []}
          tablesLoading={tables.isLoading}
          onInsert={insertText}
        />

        <div className="qe-editor-col">
          <div className="qe-panel qe-editor-card">
            <EditorTabsBar
              tabs={tabsApi.tabs}
              activeTabId={tabsApi.activeTabId}
              onSelect={tabsApi.setActive}
              onClose={tabsApi.closeTab}
              onAdd={() => tabsApi.addTab()}
              onRename={tabsApi.renameTab}
              right={
                outputLocation ? (
                  <span className="qe-hint">
                    結果出力: <span className="qe-status-mono">{s3Dir(outputLocation)}</span>
                    <button
                      className="qe-copy"
                      title="出力先をコピー"
                      onClick={() => void navigator.clipboard.writeText(s3Dir(outputLocation))}
                    >
                      ⧉
                    </button>
                  </span>
                ) : (
                  <span className="qe-hint">⌘Enter で実行</span>
                )
              }
            />
            <SqlEditor
              key={activeTab.id}
              ref={editorRef}
              value={activeTab.sql}
              onChange={(v) => tabsApi.updateSql(activeTab.id, v)}
              onRun={runActive}
              schema={schema}
            />
            <div className="qe-toolbar">
              <button
                className="qe-run"
                onClick={runActive}
                disabled={start.isPending || running || !activeTab.sql.trim()}
              >
                Run query
              </button>
              <button className="btn sm" onClick={doFormat}>
                Format
              </button>
              <SnippetDropdown
                snippets={snippets.items}
                onInsert={(s) => insertText(s.sql)}
                onSaveCurrent={saveSnippet}
                onDelete={snippets.remove}
              />
              <span className="qe-toolbar-right">
                {running && status ? (
                  <>
                    <span className="qe-status-text running">● {status.stateLabel}</span>
                    <span className="qe-status-mono">
                      {formatDurationClock(status.elapsedMs)} · {formatBytes(status.bytes)} scanned
                    </span>
                    <button className="qe-cancel" onClick={doCancel} disabled={stop.isPending}>
                      キャンセル
                    </button>
                  </>
                ) : start.isPending ? (
                  <span className="qe-muted">起動中…</span>
                ) : lastFinished ? (
                  <span className="qe-muted">
                    直近実行:{' '}
                    <span className="qe-status-mono">
                      {formatDurationClock(lastFinished.elapsedMs)} ·{' '}
                      {formatBytes(lastFinished.bytes)}
                    </span>{' '}
                    scanned
                  </span>
                ) : null}
              </span>
            </div>
            {actionError && <div className="qe-error">{actionError.message}</div>}
          </div>

          <ResultsPanel
            active={resultsTab}
            onChange={setResultsTab}
            historyLabel="実行履歴"
            status={
              resultsTab === 'results' ? (
                resultsStatus
              ) : resultsTab === 'history' ? (
                <span className="qe-muted">workgroup: {workgroup || '-'} · 直近50件</span>
              ) : null
            }
          >
            {resultsTab === 'results' &&
              (status?.state === 'failed' || status?.state === 'cancelled' ? (
                <div className="qe-error qe-error-block">
                  {status.stateLabel}
                  {status.errorMessage ? `: ${status.errorMessage}` : ''}
                </div>
              ) : status?.state === 'succeeded' ? (
                <ResultTable
                  columns={resultData.columns}
                  rows={resultData.rows}
                  hasMore={results.hasNextPage}
                  isFetchingMore={results.isFetchingNextPage}
                  onLoadMore={() => void results.fetchNextPage()}
                  footerRight={
                    status.outputLocation ? (
                      <span
                        className="qe-status-mono dim qe-rt-output"
                        title={status.outputLocation}
                      >
                        {status.outputLocation}
                      </span>
                    ) : null
                  }
                />
              ) : (
                <div className="qe-tab-empty">
                  {running ? 'クエリを実行中…' : 'クエリを実行すると結果がここに表示されます'}
                </div>
              ))}
            {resultsTab === 'history' && (
              <QueryHistoryTable
                items={history.data ?? []}
                bytesLabel="スキャン量"
                formatDuration={formatDurationClock}
                onOpen={openFromHistory}
                onRerun={rerunFromHistory}
                isLoading={history.isLoading}
              />
            )}
            {resultsTab === 'saved' && (
              <NamedQueryList
                items={saved.items}
                emptyText="保存クエリはまだありません"
                onOpen={openNamedQuery}
                onDelete={saved.remove}
                header={
                  <button className="btn sm" onClick={saveQuery}>
                    ＋ 現在のクエリを保存
                  </button>
                }
              />
            )}
            {resultsTab === 'snippets' && (
              <>
                {snippets.error ? <ErrorBanner error={snippets.error} /> : null}
                <NamedQueryList
                  items={snippets.items}
                  emptyText="スニペットはまだありません"
                  onOpen={openNamedQuery}
                  onInsert={(s) => insertText(s.sql)}
                  onDelete={snippets.remove}
                  header={
                    <button className="btn sm" onClick={saveSnippet}>
                      ＋ 現在のクエリをスニペットに保存
                    </button>
                  }
                />
              </>
            )}
          </ResultsPanel>
        </div>
      </div>
    </div>
  );
}

// ============================================================
// スキーマツリー (database → table → columns、パーティション列は橙)
// ============================================================
interface AthenaSchemaTreeProps {
  databases: string[];
  databasesLoading?: boolean;
  selectedDatabase: string;
  onSelectDatabase: (db: string) => void;
  tables: AthenaTableRow[];
  tablesLoading?: boolean;
  onInsert: (text: string) => void;
}

function AthenaSchemaTree({
  databases,
  databasesLoading,
  selectedDatabase,
  onSelectDatabase,
  tables,
  tablesLoading,
  onInsert,
}: AthenaSchemaTreeProps) {
  const [search, setSearch] = useState('');
  const [expandedTable, setExpandedTable] = useState<string | null>(null);

  const q = search.trim().toLowerCase();
  const visibleTables = tables.filter((t) => !q || t.name.toLowerCase().includes(q));

  return (
    <SchemaTreePanel
      search={search}
      onSearch={setSearch}
      footer={
        <>
          パーティション列は <span className="qe-partition-hint">橙</span> で表示
        </>
      }
    >
      {databasesLoading ? (
        <Loading />
      ) : (
        <>
          {databases.map((db) => {
            const selected = db === selectedDatabase;
            return (
              <div key={db}>
                <SchemaTreeRow
                  level={0}
                  label={db}
                  expandable
                  expanded={selected}
                  onClick={() => onSelectDatabase(db)}
                />
                {selected && tablesLoading && <Loading />}
                {selected &&
                  !tablesLoading &&
                  visibleTables.map((t) => {
                    const expanded = expandedTable === t.name;
                    return (
                      <div key={t.name}>
                        <SchemaTreeRow
                          level={1}
                          label={t.name}
                          badge={t.type === 'EXTERNAL_TABLE' ? 'EXTERNAL' : t.type}
                          expandable
                          expanded={expanded}
                          selected={expanded}
                          onClick={(alt) => {
                            onInsert(alt ? `SELECT *\nFROM ${t.name}\nLIMIT 100` : t.name);
                            setExpandedTable(expanded ? null : t.name);
                          }}
                        />
                        {expanded && (
                          <>
                            {t.columns.map((c) => (
                              <SchemaTreeRow
                                key={c.name}
                                level={2}
                                label={c.name}
                                badge={c.type}
                                onClick={() => onInsert(c.name)}
                              />
                            ))}
                            {t.partitionKeys.map((c) => (
                              <SchemaTreeRow
                                key={`p-${c.name}`}
                                level={2}
                                label={c.name}
                                badge="partition"
                                partition
                                onClick={() => onInsert(c.name)}
                              />
                            ))}
                          </>
                        )}
                      </div>
                    );
                  })}
                {selected && !tablesLoading && visibleTables.length === 0 && (
                  <div className="qe-tree-empty">テーブルがありません</div>
                )}
              </div>
            );
          })}
          {databases.length === 0 && <div className="qe-tab-empty">データベースがありません</div>}
        </>
      )}
    </SchemaTreePanel>
  );
}

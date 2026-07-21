// BigQuery ビュー: クエリエディタ構成 (デザイン 2a/3a)。
// 左スキーマツリー (dataset → table → columns) + エディタパネル (タブ / SQL / ツールバー)
// + 結果パネル (結果 / 履歴 / 保存クエリ / スニペット)。
// 旧 datasets & tables 一覧はスキーマツリーに統合した。
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Trans, useTranslation } from 'react-i18next';
import { format as formatSql } from 'sql-formatter';
import {
  useBQCancelJob,
  useBQDatasets,
  useBQDryRun,
  useBQQueryHistory,
  useBQQueryJob,
  useBQQueryResults,
  useBQSchema,
  useBQStartQuery,
  useBQTables,
} from '../../api/queries';
import { ErrorBanner } from '../../components/ErrorBanner';
import { Loading } from '../../components/Loading';
import { formatBytes } from '../../components/tables/columns';
import { EditorTabsBar } from '../../components/query/EditorTabsBar';
import { NamedQueryList } from '../../components/query/NamedQueryList';
import { QueryHistoryTable } from '../../components/query/QueryHistoryTable';
import { ResultsPanel, type ResultsTabKey } from '../../components/query/ResultsPanel';
import { ResultTable } from '../../components/query/ResultTable';
import { SchemaTreePanel, SchemaTreeRow } from '../../components/query/SchemaTree';
import { SnippetDropdown } from '../../components/query/SnippetDropdown';
import { SqlEditor, type SqlEditorHandle } from '../../components/query/SqlEditor';
import { useEditorTabs } from '../../components/query/useEditorTabs';
import { useNamedQueries } from '../../components/query/useNamedQueries';
import { useServerSnippets } from '../../components/query/useServerSnippets';
import { bqResultsFromPages } from '../../lib/normalizeQuery';
import {
  estimateBQCostUSD,
  formatApproxUSD,
  formatDurationSeconds,
  shortId,
  toCsv,
} from '../../lib/queryFormat';
import type { BQDatasetRow, BQTableRow } from '../../types/nonaws';
import type { NamedQuery, QueryHistoryRow } from '../../types/query';

const DEFAULT_SQL = 'SELECT 1';

export interface BigQueryViewProps {
  projectId?: string;
}

export function BigQueryView({ projectId }: BigQueryViewProps = {}) {
  // プロジェクト切替時は key で再マウントし、localStorage のタブ状態を読み直す
  return <BigQueryEditor key={projectId ?? 'default'} projectId={projectId} />;
}

// 補完スキーマ: dataset → table → columns (ツリーが読み込んだ範囲のみ)
type SchemaMap = Record<string, Record<string, string[]>>;

function BigQueryEditor({ projectId }: BigQueryViewProps) {
  const { t } = useTranslation('gcp');
  const scope = projectId ?? '';
  const tabsApi = useEditorTabs('bigquery', scope, DEFAULT_SQL);
  const { activeTab } = tabsApi;
  const editorRef = useRef<SqlEditorHandle | null>(null);
  const [resultsTab, setResultsTab] = useState<ResultsTabKey>('results');
  // タブごとの実行ジョブ / ドライラン結果 (ページリロードで消えるのは仕様。履歴タブから復帰する)
  const [jobs, setJobs] = useState<Record<string, { jobId: string; location: string }>>({});
  const [dryRuns, setDryRuns] = useState<Record<string, number>>({});
  const [schemaMap, setSchemaMap] = useState<SchemaMap>({});

  const snippets = useServerSnippets('bigquery');
  const saved = useNamedQueries('bigquery', scope, 'saved');

  const start = useBQStartQuery(projectId);
  const dryRun = useBQDryRun(projectId);
  const cancel = useBQCancelJob(projectId);

  const activeJob = jobs[activeTab.id];
  const jobStatus = useBQQueryJob(activeJob?.jobId, activeJob?.location, projectId);
  const status = jobStatus.data;
  const results = useBQQueryResults(
    activeJob?.jobId,
    activeJob?.location,
    projectId,
    status?.state === 'succeeded',
  );
  const resultData = useMemo(() => bqResultsFromPages(results.data?.pages ?? []), [results.data]);

  const history = useBQQueryHistory(projectId, resultsTab === 'history');

  const runSql = useCallback(
    (tabId: string, sql: string) => {
      const text = sql.trim();
      if (!text) return;
      start.mutate(text, {
        onSuccess: (job) => {
          setJobs((prev) => ({ ...prev, [tabId]: job }));
          setResultsTab('results');
        },
      });
    },
    [start],
  );

  const runActive = useCallback(() => {
    runSql(tabsApi.activeTab.id, tabsApi.activeTab.sql);
  }, [runSql, tabsApi.activeTab]);

  const runDryRun = useCallback(() => {
    const text = activeTab.sql.trim();
    if (!text) return;
    dryRun.mutate(text, {
      onSuccess: (bytes) => setDryRuns((prev) => ({ ...prev, [activeTab.id]: bytes })),
    });
  }, [activeTab.id, activeTab.sql, dryRun]);

  const doFormat = useCallback(() => {
    try {
      const formatted = formatSql(activeTab.sql, { language: 'bigquery', keywordCase: 'upper' });
      editorRef.current?.replaceAll(formatted);
    } catch {
      // パースできない SQL はフォーマットせずそのまま残す
    }
  }, [activeTab.sql]);

  const doCancel = useCallback(() => {
    if (!activeJob) return;
    cancel.mutate(
      { jobId: activeJob.jobId, location: activeJob.location },
      { onSuccess: () => void jobStatus.refetch() },
    );
  }, [activeJob, cancel, jobStatus]);

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
    const name = window.prompt(t('bigQueryView.snippetNamePrompt'), activeTab.name);
    if (name) snippets.add(name, activeTab.sql);
  }, [activeTab.name, activeTab.sql, snippets, t]);

  const saveQuery = useCallback(() => {
    const name = window.prompt(t('bigQueryView.savedQueryNamePrompt'), activeTab.name);
    if (name) saved.add(name, activeTab.sql);
  }, [activeTab.name, activeTab.sql, saved, t]);

  const copyCsv = useCallback(() => {
    void navigator.clipboard.writeText(toCsv(resultData.columns, resultData.rows));
  }, [resultData]);

  const insertText = useCallback((text: string) => {
    editorRef.current?.insertText(text);
  }, []);

  // スキーマツリーが読み込んだテーブル / カラムを補完候補として蓄積する
  const reportTables = useCallback((dataset: string, tables: string[]) => {
    setSchemaMap((prev) => {
      const cur = prev[dataset] ?? {};
      let changed = false;
      const next = { ...cur };
      for (const t of tables) {
        if (!(t in next)) {
          next[t] = [];
          changed = true;
        }
      }
      return changed ? { ...prev, [dataset]: next } : prev;
    });
  }, []);
  const reportColumns = useCallback((dataset: string, table: string, columns: string[]) => {
    setSchemaMap((prev) => {
      const cur = prev[dataset] ?? {};
      if (cur[table] && cur[table].length === columns.length) return prev;
      return { ...prev, [dataset]: { ...cur, [table]: columns } };
    });
  }, []);

  const running = status?.state === 'queued' || status?.state === 'running';
  const dryRunBytes = dryRuns[activeTab.id];
  const actionError = (start.error ?? dryRun.error) as Error | null;

  const resultsStatus = status ? (
    <>
      <span className={`qe-status-text ${status.state}`}>● {status.stateLabel}</span>
      <span className="qe-status-mono">
        {formatDurationSeconds(status.elapsedMs)} · {formatBytes(status.bytes)}
      </span>
      <span className="qe-status-mono dim">{shortId(status.id)}</span>
      <button className="btn sm" onClick={copyCsv} disabled={resultData.rows.length === 0}>
        {t('bigQueryView.copyCsv')}
      </button>
    </>
  ) : null;

  return (
    <div className="main qe-main">
      <div className="toolbar">
        <div className="title">
          <h1>BigQuery</h1>
          <span className="subtitle">query editor</span>
        </div>
      </div>

      <div className="qe-layout">
        <BQSchemaTree
          projectId={projectId}
          onInsert={insertText}
          onTables={reportTables}
          onColumns={reportColumns}
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
              right={<span className="qe-hint">{t('bigQueryView.runHint')}</span>}
            />
            <SqlEditor
              key={activeTab.id}
              ref={editorRef}
              value={activeTab.sql}
              onChange={(v) => tabsApi.updateSql(activeTab.id, v)}
              onRun={runActive}
              schema={schemaMap}
            />
            <div className="qe-toolbar">
              <button
                className="qe-run"
                onClick={runActive}
                disabled={start.isPending || running || !activeTab.sql.trim()}
              >
                Run query
              </button>
              <button className="btn sm" onClick={runDryRun} disabled={dryRun.isPending}>
                Dry run
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
                      {formatDurationSeconds(status.elapsedMs)}
                    </span>
                    <button className="qe-cancel" onClick={doCancel} disabled={cancel.isPending}>
                      {t('bigQueryView.cancel')}
                    </button>
                  </>
                ) : start.isPending ? (
                  <span className="qe-muted">{t('bigQueryView.starting')}</span>
                ) : dryRun.isPending ? (
                  <span className="qe-muted">{t('bigQueryView.estimating')}</span>
                ) : dryRunBytes !== undefined ? (
                  <span className="qe-muted">
                    <Trans
                      i18nKey="bigQueryView.dryRunResult"
                      ns="gcp"
                      values={{
                        bytes: formatBytes(dryRunBytes),
                        cost: formatApproxUSD(estimateBQCostUSD(dryRunBytes)),
                      }}
                      components={{ mono: <span className="qe-status-mono" /> }}
                    />
                  </span>
                ) : null}
              </span>
            </div>
            {actionError && <div className="qe-error">{actionError.message}</div>}
          </div>

          <ResultsPanel
            active={resultsTab}
            onChange={setResultsTab}
            historyLabel={t('bigQueryView.historyLabel')}
            status={resultsTab === 'results' ? resultsStatus : null}
          >
            {resultsTab === 'results' &&
              (status?.state === 'failed' ? (
                <div className="qe-error qe-error-block">{status.errorMessage}</div>
              ) : status?.state === 'succeeded' ? (
                <ResultTable
                  columns={resultData.columns}
                  rows={resultData.rows}
                  totalRows={resultData.totalRows}
                  hasMore={results.hasNextPage}
                  isFetchingMore={results.isFetchingNextPage}
                  onLoadMore={() => void results.fetchNextPage()}
                />
              ) : (
                <div className="qe-tab-empty">
                  {running ? t('bigQueryView.runningResult') : t('bigQueryView.runToSeeResults')}
                </div>
              ))}
            {resultsTab === 'history' && (
              <>
                {history.error ? <ErrorBanner error={history.error} /> : null}
                <QueryHistoryTable
                  items={history.data ?? []}
                  bytesLabel={t('bigQueryView.bytesLabel')}
                  formatDuration={formatDurationSeconds}
                  onOpen={openFromHistory}
                  onRerun={rerunFromHistory}
                  isLoading={history.isLoading}
                />
              </>
            )}
            {resultsTab === 'saved' && (
              <NamedQueryList
                items={saved.items}
                emptyText={t('bigQueryView.savedEmpty')}
                onOpen={openNamedQuery}
                onDelete={saved.remove}
                header={
                  <button className="btn sm" onClick={saveQuery}>
                    {t('bigQueryView.saveCurrentQuery')}
                  </button>
                }
              />
            )}
            {resultsTab === 'snippets' && (
              <>
                {snippets.error ? <ErrorBanner error={snippets.error} /> : null}
                <NamedQueryList
                  items={snippets.items}
                  emptyText={t('bigQueryView.snippetEmpty')}
                  onOpen={openNamedQuery}
                  onInsert={(s) => insertText(s.sql)}
                  onDelete={snippets.remove}
                  header={
                    <button className="btn sm" onClick={saveSnippet}>
                      {t('bigQueryView.saveCurrentSnippet')}
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
// スキーマツリー (dataset → table → columns、遅延読み込み)
// ============================================================
interface BQSchemaTreeProps {
  projectId?: string;
  onInsert: (text: string) => void;
  onTables: (dataset: string, tables: string[]) => void;
  onColumns: (dataset: string, table: string, columns: string[]) => void;
}

function BQSchemaTree({ projectId, onInsert, onTables, onColumns }: BQSchemaTreeProps) {
  const { t } = useTranslation('gcp');
  const { data: datasets, isLoading, error } = useBQDatasets(projectId);
  const [search, setSearch] = useState('');
  const [expandedDatasets, setExpandedDatasets] = useState<Set<string>>(new Set());
  const [expandedTable, setExpandedTable] = useState<string | null>(null);

  const toggleDataset = (id: string) => {
    setExpandedDatasets((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const q = search.trim().toLowerCase();
  const visibleDatasets = (datasets ?? []).filter((d) => !q || d.name.toLowerCase().includes(q));

  return (
    <SchemaTreePanel
      search={search}
      onSearch={setSearch}
      footer={<>{t('bigQueryView.treeFooter')}</>}
    >
      {isLoading ? (
        <Loading />
      ) : (
        <>
          {error ? <div className="qe-tab-empty">{(error as Error).message}</div> : null}
          {visibleDatasets.map((d) => (
            <BQDatasetNode
              key={d.id}
              projectId={projectId}
              dataset={d}
              expanded={expandedDatasets.has(d.id)}
              onToggle={() => toggleDataset(d.id)}
              search={q}
              expandedTable={expandedTable}
              onExpandTable={setExpandedTable}
              onInsert={onInsert}
              onTables={onTables}
              onColumns={onColumns}
            />
          ))}
          {(datasets ?? []).length === 0 && !error && (
            <div className="qe-tab-empty">{t('bigQueryView.noDatasets')}</div>
          )}
        </>
      )}
    </SchemaTreePanel>
  );
}

interface BQDatasetNodeProps {
  projectId?: string;
  dataset: BQDatasetRow;
  expanded: boolean;
  onToggle: () => void;
  search: string;
  expandedTable: string | null;
  onExpandTable: (key: string | null) => void;
  onInsert: (text: string) => void;
  onTables: (dataset: string, tables: string[]) => void;
  onColumns: (dataset: string, table: string, columns: string[]) => void;
}

function BQDatasetNode({
  projectId,
  dataset,
  expanded,
  onToggle,
  search,
  expandedTable,
  onExpandTable,
  onInsert,
  onTables,
  onColumns,
}: BQDatasetNodeProps) {
  // 展開時のみテーブル一覧を取得する (enabled は dataset 引数の有無で制御される)
  const { data: tables, isLoading } = useBQTables(expanded ? dataset.id : '', projectId);

  useEffect(() => {
    if (tables)
      onTables(
        dataset.id,
        tables.map((t) => t.name),
      );
  }, [tables, dataset.id, onTables]);

  const visibleTables = (tables ?? []).filter(
    (t) => !search || t.name.toLowerCase().includes(search),
  );

  return (
    <>
      <SchemaTreeRow
        level={0}
        label={dataset.name}
        expandable
        expanded={expanded}
        onClick={onToggle}
      />
      {expanded && isLoading && <Loading />}
      {expanded &&
        !isLoading &&
        visibleTables.map((t) => (
          <BQTableNode
            key={t.id}
            projectId={projectId}
            datasetId={dataset.id}
            table={t}
            expanded={expandedTable === `${dataset.id}.${t.id}`}
            onToggle={() =>
              onExpandTable(
                expandedTable === `${dataset.id}.${t.id}` ? null : `${dataset.id}.${t.id}`,
              )
            }
            onInsert={onInsert}
            onColumns={onColumns}
          />
        ))}
    </>
  );
}

interface BQTableNodeProps {
  projectId?: string;
  datasetId: string;
  table: BQTableRow;
  expanded: boolean;
  onToggle: () => void;
  onInsert: (text: string) => void;
  onColumns: (dataset: string, table: string, columns: string[]) => void;
}

function BQTableNode({
  projectId,
  datasetId,
  table,
  expanded,
  onToggle,
  onInsert,
  onColumns,
}: BQTableNodeProps) {
  const { data: fields } = useBQSchema(
    expanded ? datasetId : '',
    expanded ? table.id : '',
    projectId,
  );

  useEffect(() => {
    if (fields)
      onColumns(
        datasetId,
        table.id,
        fields.map((f) => f.name),
      );
  }, [fields, datasetId, table.id, onColumns]);

  const tableRef = projectId
    ? `\`${projectId}.${datasetId}.${table.id}\``
    : `\`${datasetId}.${table.id}\``;

  return (
    <>
      <SchemaTreeRow
        level={1}
        label={table.name}
        badge={table.type}
        expandable
        expanded={expanded}
        selected={expanded}
        onClick={(alt) => {
          onInsert(alt ? `SELECT *\nFROM ${tableRef}\nLIMIT 100` : tableRef);
          onToggle();
        }}
      />
      {expanded &&
        (fields ?? []).map((f) => (
          <SchemaTreeRow
            key={f.id}
            level={2}
            label={f.name}
            badge={f.type}
            onClick={() => onInsert(f.name)}
          />
        ))}
    </>
  );
}

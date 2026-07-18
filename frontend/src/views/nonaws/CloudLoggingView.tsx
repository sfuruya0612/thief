// Cloud Logging ビュー (デザイン Turn 8b)。左にリソースタイプ別ツリー (チェックで resource.type
// フィルタを構築)、右上に Logging query language 入力 + severity 積み上げヒストグラム、右下に
// JSON 展開可能なログ一覧。静的検索 (entries.list, ページング) と Live Tail (entries.tail,
// WebSocket) の 2 モード。CloudWatch Logs と共通のログビューアコンポーネントを使う。
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useGcpLogEntries } from '../../api/queries';
import { gcpLoggingTailUrl } from '../../api/terminal';
import { ErrorBanner } from '../../components/ErrorBanner';
import { LogHistogram } from '../../components/logviewer/LogHistogram';
import { LogFieldRow, LogList, SeverityBadge } from '../../components/logviewer/LogList';
import { LogToolbarActions } from '../../components/logviewer/LogToolbarActions';
import { LogTree } from '../../components/logviewer/LogTree';
import { LogViewerShell } from '../../components/logviewer/LogViewerShell';
import { SummaryFieldPicker } from '../../components/logviewer/SummaryFieldPicker';
import { useCopy } from '../../components/logviewer/useCopy';
import { useLiveTail } from '../../components/logviewer/useLiveTail';
import { formatLogClock, jsonFieldsOf, rowsToCsv, rowsToJson } from '../../lib/logFormat';
import type { LogTreeNode } from '../../lib/logGroupTree';
import { buildHistogram, rangeFromItems } from '../../lib/logHistogram';
import { availableSummaryFieldKeys, buildSummaryText } from '../../lib/logSummaryFields';
import { type PresetOption, presetToRange } from '../../lib/logTimeRange';
import { logEntryFromRaw, logSeverityLevel } from '../../lib/normalizeGcp';
import { loadPersisted, savePersisted } from '../../lib/storage';
import type { LogEntryRaw, LogEntryRow } from '../../types/gcp';

export interface CloudLoggingViewProps {
  projectId: string;
}

// GCP の主要な監視対象リソースタイプ。バックエンドにリソースタイプ列挙 API が無いため、
// 代表的なものを静的に持ち、チェックで resource.type フィルタを構築する。
const GCP_RESOURCE_TREE: LogTreeNode[] = [
  {
    key: 'compute',
    label: 'Compute',
    children: [
      { key: 'k8s_container', label: 'GKE (k8s_container)', value: 'k8s_container' },
      { key: 'gce_instance', label: 'Compute Engine (gce_instance)', value: 'gce_instance' },
    ],
  },
  {
    key: 'serverless',
    label: 'Serverless',
    children: [
      {
        key: 'cloud_run_revision',
        label: 'Cloud Run (cloud_run_revision)',
        value: 'cloud_run_revision',
      },
      { key: 'cloud_function', label: 'Cloud Functions (cloud_function)', value: 'cloud_function' },
    ],
  },
  {
    key: 'data',
    label: 'Database & Storage',
    children: [
      {
        key: 'cloudsql_database',
        label: 'Cloud SQL (cloudsql_database)',
        value: 'cloudsql_database',
      },
      { key: 'gcs_bucket', label: 'Cloud Storage (gcs_bucket)', value: 'gcs_bucket' },
      {
        key: 'bigquery_resource',
        label: 'BigQuery (bigquery_resource)',
        value: 'bigquery_resource',
      },
    ],
  },
  {
    key: 'network',
    label: 'Networking',
    children: [
      {
        key: 'http_load_balancer',
        label: 'Load Balancer (http_load_balancer)',
        value: 'http_load_balancer',
      },
    ],
  },
];

const FILTER_SNIPPETS = [
  { label: 'ERROR 以上', snippet: 'severity>=ERROR' },
  { label: 'WARNING 以上', snippet: 'severity>=WARNING' },
  { label: 'INFO のみ', snippet: 'severity=INFO' },
];

const copyRaw = (text: string) => void navigator.clipboard?.writeText(text);

// 選択された resource.type とユーザー入力フィルタを Logging query language の複数条件 (改行 = AND)
// として結合する。複数タイプは OR でまとめる。
function composeGcpFilter(types: string[], userFilter: string): string {
  const parts: string[] = [];
  if (types.length === 1) {
    parts.push(`resource.type="${types[0]}"`);
  } else if (types.length > 1) {
    parts.push(`(${types.map((t) => `resource.type="${t}"`).join(' OR ')})`);
  }
  const uf = userFilter.trim();
  if (uf) parts.push(uf);
  return parts.join('\n');
}

// trace ("projects/<proj>/traces/<id>") から Cloud Trace コンソール URL を組み立てる。
// 形式が合わなければ null。
function traceUrl(trace: string): string | null {
  const m = /^projects\/([^/]+)\/traces\/(.+)$/.exec(trace);
  if (!m) return null;
  return `https://console.cloud.google.com/traces/list?project=${encodeURIComponent(m[1])}&tid=${encodeURIComponent(m[2])}`;
}

export function CloudLoggingView({ projectId }: CloudLoggingViewProps) {
  // プロジェクト切替時はフィルタ・結果をリセットして再マウントする。
  return <CloudLoggingEditor key={projectId} projectId={projectId} />;
}

function CloudLoggingEditor({ projectId }: CloudLoggingViewProps) {
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [filterInput, setFilterInput] = useState('');
  const [appliedFilter, setAppliedFilter] = useState('');
  const [preset, setPreset] = useState<PresetOption>('1h');
  const [customStart, setCustomStart] = useState('');
  const [customEnd, setCustomEnd] = useState('');
  const [appliedRange, setAppliedRange] = useState<{ start: string; end: string } | null>(null);
  const [runToken, setRunToken] = useState(0);
  const [live, setLive] = useState(false);
  const [autoScroll, setAutoScroll] = useState(true);
  const [summaryFields, setSummaryFields] = useState<string[]>(
    () => loadPersisted().gcpLogSummaryFields ?? [],
  );
  const bodyRef = useRef<HTMLDivElement>(null);

  const jsonCopy = useCopy();
  const csvCopy = useCopy();

  const toggleSummaryField = useCallback((key: string) => {
    setSummaryFields((prev) => {
      const next = prev.includes(key) ? prev.filter((k) => k !== key) : [...prev, key];
      savePersisted({ ...loadPersisted(), gcpLogSummaryFields: next });
      return next;
    });
  }, []);

  const clearSummaryFields = useCallback(() => {
    setSummaryFields([]);
    savePersisted({ ...loadPersisted(), gcpLogSummaryFields: [] });
  }, []);

  const toggleType = useCallback((value: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(value)) next.delete(value);
      else next.add(value);
      return next;
    });
  }, []);

  const addFilterClause = useCallback((clause: string) => {
    setFilterInput((prev) => (prev.trim() ? `${prev.trim()}\n${clause}` : clause));
  }, []);

  const staticQuery = useGcpLogEntries(
    projectId,
    runToken,
    { filter: appliedFilter, start: appliedRange?.start, end: appliedRange?.end },
    !live,
  );

  const parseLive = useCallback(
    (raw: Record<string, unknown>, seq: number) =>
      logEntryFromRaw(raw as unknown as LogEntryRaw, seq),
    [],
  );
  const tailUrl = useMemo(
    () => gcpLoggingTailUrl(projectId, appliedFilter),
    [projectId, appliedFilter],
  );
  const liveTail = useLiveTail<LogEntryRow>({
    enabled: live && !!projectId,
    url: tailUrl,
    parse: parseLive,
  });

  const runSearch = useCallback(() => {
    const composed = composeGcpFilter([...selected], filterInput);
    setAppliedFilter(composed);
    if (live) {
      liveTail.clear();
      return;
    }
    const r =
      preset === 'custom'
        ? {
            start: customStart ? new Date(customStart).toISOString() : '',
            end: customEnd ? new Date(customEnd).toISOString() : '',
          }
        : presetToRange(preset);
    setAppliedRange(r);
    setRunToken((t) => t + 1);
  }, [selected, filterInput, live, preset, customStart, customEnd, liveTail]);

  const toggleLive = useCallback(() => {
    setLive((prev) => {
      const next = !prev;
      if (next) setAppliedFilter(composeGcpFilter([...selected], filterInput));
      return next;
    });
  }, [selected, filterInput]);

  const staticRows = useMemo(() => {
    const pages = staticQuery.data?.pages ?? [];
    const rows: LogEntryRow[] = [];
    let index = 0;
    for (const page of pages) {
      for (const raw of page.entries) {
        rows.push(logEntryFromRaw(raw, index));
        index += 1;
      }
    }
    return rows;
  }, [staticQuery.data]);

  const rows = live ? liveTail.lines : staticRows;

  const availableFields = useMemo(() => availableSummaryFieldKeys(rows), [rows]);

  const histItems = useMemo(
    () =>
      rows
        .map((r) => ({ tsMs: Date.parse(r.timestamp), level: logSeverityLevel(r.severity) }))
        .filter((it) => !Number.isNaN(it.tsMs)),
    [rows],
  );
  const range = useMemo(() => {
    if (!live && appliedRange?.start && appliedRange?.end) {
      return { startMs: Date.parse(appliedRange.start), endMs: Date.parse(appliedRange.end) };
    }
    return rangeFromItems(histItems);
  }, [live, appliedRange, histItems]);
  const buckets = useMemo(
    () => buildHistogram(histItems, range.startMs, range.endMs),
    [histItems, range],
  );

  const onScroll = useCallback(() => {
    const el = bodyRef.current;
    if (!el) return;
    setAutoScroll(el.scrollHeight - el.scrollTop - el.clientHeight < 24);
  }, []);
  useEffect(() => {
    if (!live || !autoScroll) return;
    const el = bodyRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [rows, live, autoScroll]);

  const listError = live ? undefined : staticQuery.error;

  const exportJson = useCallback(() => {
    jsonCopy.copy(rowsToJson(rows));
  }, [rows, jsonCopy]);
  const exportCsv = useCallback(() => {
    const csv = rowsToCsv(
      ['timestamp', 'severity', 'payload'],
      rows.map((r) => [r.timestamp, r.severity, r.payload]),
    );
    csvCopy.copy(csv);
  }, [rows, csvCopy]);

  const renderDetail = useCallback(
    (row: LogEntryRow) => {
      const fields = jsonFieldsOf(row.payload);
      const tUrl = row.trace ? traceUrl(row.trace) : null;
      return (
        <div className="lv-detail-fields">
          {fields
            ? fields.map((f) => (
                <LogFieldRow
                  key={f.key}
                  label={`jsonPayload.${f.key}`}
                  value={f.value}
                  onAddFilter={() => addFilterClause(`jsonPayload.${f.key}="${f.value}"`)}
                  onCopy={() => copyRaw(f.value)}
                />
              ))
            : row.payload && (
                <LogFieldRow
                  label="payload"
                  value={row.payload}
                  onCopy={() => copyRaw(row.payload)}
                />
              )}
          {row.resourceType && (
            <LogFieldRow
              label="resource.type"
              value={row.resourceType}
              onAddFilter={() => addFilterClause(`resource.type="${row.resourceType}"`)}
              onCopy={() => copyRaw(row.resourceType)}
            />
          )}
          {Object.entries(row.labels).map(([k, v]) => (
            <LogFieldRow
              key={`label-${k}`}
              label={`labels.${k}`}
              value={v}
              onAddFilter={() => addFilterClause(`labels.${k}="${v}"`)}
              onCopy={() => copyRaw(v)}
            />
          ))}
          {row.trace && (
            <LogFieldRow
              label="trace"
              value={row.trace}
              trace={
                tUrl
                  ? {
                      label: 'Trace を開く ↗',
                      onOpen: () => window.open(tUrl, '_blank', 'noopener'),
                    }
                  : undefined
              }
              onCopy={() => copyRaw(row.trace)}
            />
          )}
        </div>
      );
    },
    [addFilterClause],
  );

  const liveStatusText = () => {
    switch (liveTail.status) {
      case 'connecting':
        return '接続中…';
      case 'connected':
        return `受信中 (${liveTail.lines.length} 件)`;
      case 'closed':
        return `終了${liveTail.message ? `: ${liveTail.message}` : ''}`;
      case 'error':
        return `エラー${liveTail.message ? `: ${liveTail.message}` : ''}`;
      default:
        return '待機中…';
    }
  };

  const footer = live ? (
    <span className="lv-live-status">
      <span className="lv-live-dot on" />
      ライブテール中 · {liveStatusText()}
    </span>
  ) : (
    <>
      <span>{staticQuery.isFetching ? '取得中…' : `${staticRows.length} 件表示中 · 新しい順`}</span>
      {staticQuery.hasNextPage && (
        <button
          className="lv-more-btn"
          onClick={() => void staticQuery.fetchNextPage()}
          disabled={staticQuery.isFetchingNextPage}
        >
          {staticQuery.isFetchingNextPage ? '読み込み中…' : 'さらに読み込む'}
        </button>
      )}
    </>
  );

  const histogramNode =
    rows.length > 0 ? (
      <>
        <div className="lv-hist-caption">
          <span className="lv-hist-legend">
            イベント数 / <span className="lv-sev-err">ERROR</span>{' '}
            <span className="lv-sev-warn">WARNING</span> <span className="lv-sev-info">INFO</span>
          </span>
          <span>{live ? 'ライブ更新中…' : `計 ${rows.length} 件`}</span>
        </div>
        <LogHistogram buckets={buckets} mode="stacked" />
        <div className="lv-hist-axis">
          <span>{formatLogClock(new Date(range.startMs).toISOString())}</span>
          <span>{live ? 'now' : formatLogClock(new Date(range.endMs).toISOString())}</span>
        </div>
      </>
    ) : null;

  return (
    <LogViewerShell
      title="Cloud Logging"
      subtitle="log viewer"
      toolbarActions={
        <LogToolbarActions
          live={live}
          onToggleLive={toggleLive}
          preset={preset}
          onPresetChange={setPreset}
          customStart={customStart}
          customEnd={customEnd}
          onCustomStartChange={setCustomStart}
          onCustomEndChange={setCustomEnd}
          exportLabel={csvCopy.copied ? 'コピー済み' : 'エクスポート (CSV)'}
          onExport={exportCsv}
          exportDisabled={rows.length === 0}
        />
      }
      banner={listError ? <ErrorBanner error={listError} /> : undefined}
      tree={
        <LogTree
          nodes={GCP_RESOURCE_TREE}
          selected={selected}
          onToggle={toggleType}
          searchPlaceholder="リソースを検索…"
          footer={selected.size > 0 ? `${selected.size} リソース選択中` : 'リソースタイプを選択'}
        />
      }
      filterBar={
        <>
          <span className="lv-filter-icon">⌕</span>
          <textarea
            className="lv-filter-textarea"
            value={filterInput}
            onChange={(e) => setFilterInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) runSearch();
            }}
            placeholder={
              'Logging query language\n(例: severity>=ERROR AND jsonPayload.route=~"/v1/pay.*")'
            }
            rows={2}
          />
          <div className="lv-filter-actions">
            {FILTER_SNIPPETS.map((s) => (
              <button key={s.snippet} className="btn sm" onClick={() => addFilterClause(s.snippet)}>
                {s.label}
              </button>
            ))}
            <button className="lv-run-btn" onClick={runSearch}>
              実行
            </button>
          </div>
        </>
      }
      histogram={histogramNode}
      logList={
        <LogList<LogEntryRow>
          rows={rows}
          getKey={(r) => r.id}
          getLevel={(r) => logSeverityLevel(r.severity)}
          getTimestamp={(r) => r.timestamp}
          secondHeader="SEVERITY"
          secondWidth={96}
          renderSecond={(r) => (
            <SeverityBadge
              level={logSeverityLevel(r.severity)}
              label={(r.severity || 'DEFAULT').toUpperCase()}
            />
          )}
          messageHeader="SUMMARY"
          getMessage={(r) => buildSummaryText(r, summaryFields)}
          renderDetail={renderDetail}
          headerExtra={
            <SummaryFieldPicker
              available={availableFields}
              selected={summaryFields}
              onToggle={toggleSummaryField}
              onClear={clearSummaryFields}
            />
          }
          copyLabel={jsonCopy.copied ? 'コピー済み' : 'JSONコピー'}
          onCopy={exportJson}
          footer={footer}
          emptyMessage={
            live ? 'ライブテール待機中…' : '「実行」を押すとログエントリがここに表示されます'
          }
          bodyRef={bodyRef}
          onScroll={onScroll}
        />
      }
    />
  );
}

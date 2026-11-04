import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  useAccessLogQuery,
  useAccessLogResults,
  useAccessLogSources,
  useCancelAccessLogQuery,
  useRecheckAccessLogSources,
  useStartAccessLogQuery,
} from '../api/queries';
import { DataTable } from '../components/DataTable';
import { ErrorBanner } from '../components/ErrorBanner';
import { Loading } from '../components/Loading';
import { SSOExpiredBanner } from '../components/SSOExpiredBanner';
import { SetupPanel } from '../components/accesslogs/SetupPanel';
import { SourceList } from '../components/accesslogs/SourceList';
import { Button, Stat } from '../components/primitives';
import { accessLogIPColumns, accessLogPairColumns } from '../components/tables/accessLogColumns';
import { formatBytes } from '../components/tables/columns';
import { accessLogDestinationKey } from '../lib/normalizeAccessLogs';
import { isSSOExpiredError } from '../lib/ssoError';
import { ApiError } from '../types/common';
import type {
  AccessLogQuery,
  AccessLogResults,
  AccessLogSource,
  AccessLogTableMapping,
} from '../types/accesslogs';

interface AccessLogsViewProps {
  profile: string;
  region: string;
}

type Tab = 'pairs' | 'ips' | 'setup';
type Preset = '1h' | '3h' | '24h' | '7d';

const PRESETS: { key: Preset; hours: number }[] = [
  { key: '1h', hours: 1 },
  { key: '3h', hours: 3 },
  { key: '24h', hours: 24 },
  { key: '7d', hours: 168 },
];

const TABLE_STORAGE_PREFIX = 'cloudlens:accesslogs:v1:';
const JOB_STORAGE_PREFIX = 'cloudlens:accesslogs:job:';

function loadMappings(key: string): Record<string, AccessLogTableMapping> {
  try {
    const raw = localStorage.getItem(key);
    if (!raw) return {};
    const parsed = JSON.parse(raw) as Record<string, AccessLogTableMapping>;
    return parsed && typeof parsed === 'object' ? parsed : {};
  } catch {
    return {};
  }
}

function saveMappings(key: string, mappings: Record<string, AccessLogTableMapping>) {
  try {
    localStorage.setItem(key, JSON.stringify(mappings));
  } catch {
    // localStorage が使えない環境では保存しない。
  }
}

function loadJobID(key: string): string | null {
  try {
    return sessionStorage.getItem(key);
  } catch {
    return null;
  }
}

function buildRange(preset: Preset) {
  const hours = PRESETS.find((item) => item.key === preset)?.hours ?? 24;
  const to = new Date();
  const from = new Date(to.getTime() - hours * 60 * 60 * 1000);
  return { from: from.toISOString(), to: to.toISOString(), hours };
}

function isReady(source: AccessLogSource): boolean {
  return source.status.destinationReadable && source.status.loggingEnabled && source.engine !== '';
}

// defaultSelection は選択可能なソースを、共有された出力先の attribution を考慮して選ぶ。
// リソースを特定できないソースがある出力先では、その出力先から 1 つだけ選ぶ。
function defaultSelection(sources: AccessLogSource[]): Set<string> {
  const byDestination = new Map<string, AccessLogSource[]>();
  for (const source of sources.filter(isReady)) {
    const key = accessLogDestinationKey(source);
    const group = byDestination.get(key) ?? [];
    group.push(source);
    byDestination.set(key, group);
  }
  const selected = new Set<string>();
  for (const group of byDestination.values()) {
    if (group.some((source) => !source.status.attribution)) {
      const chosen = group.find((source) => source.status.attribution) ?? group[0];
      selected.add(chosen.id);
      continue;
    }
    for (const source of group) selected.add(source.id);
  }
  return selected;
}

function isOutputLocationError(err: unknown): boolean {
  return err instanceof ApiError && err.code === 'ACCESS_LOGS_OUTPUT_LOCATION_REQUIRED';
}

function stateClass(state: string): string {
  switch (state) {
    case 'succeeded':
      return 'ok';
    case 'failed':
    case 'cancelled':
      return 'err';
    case 'partial':
      return 'warn';
    case 'running':
    case 'queued':
      return 'info';
    default:
      return 'muted';
  }
}

export function AccessLogsView({ profile, region }: AccessLogsViewProps) {
  const { t } = useTranslation('accesslogs');
  const sourcesQuery = useAccessLogSources(profile, region);
  const sources = useMemo(() => sourcesQuery.data?.sources ?? [], [sourcesQuery.data]);
  const sourceNames = useMemo(() => {
    const map: Record<string, string> = {};
    for (const source of sources) map[source.id] = source.name;
    return map;
  }, [sources]);

  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const initializedSelection = useRef(false);
  useEffect(() => {
    if (!sourcesQuery.data) return;
    const readyIDs = sourcesQuery.data.sources.filter(isReady).map((source) => source.id);
    if (!initializedSelection.current) {
      initializedSelection.current = true;
      setSelected(defaultSelection(sourcesQuery.data.sources));
      return;
    }
    setSelected((current) => {
      const available = new Set(readyIDs);
      return new Set([...current].filter((id) => available.has(id)));
    });
  }, [sourcesQuery.data]);

  const tableStorageKey = `${TABLE_STORAGE_PREFIX}${profile}:${region}`;
  const jobStorageKey = `${JOB_STORAGE_PREFIX}${profile}:${region}`;
  const [mappings, setMappings] = useState<Record<string, AccessLogTableMapping>>(() =>
    loadMappings(tableStorageKey),
  );
  useEffect(() => {
    saveMappings(tableStorageKey, mappings);
  }, [tableStorageKey, mappings]);

  const [tab, setTab] = useState<Tab>('pairs');
  const [lastRunView, setLastRunView] = useState<Exclude<Tab, 'setup'>>('pairs');
  const [preset, setPreset] = useState<Preset>('24h');
  const [confirmLongRange, setConfirmLongRange] = useState(false);
  const [onlyBlocked, setOnlyBlocked] = useState(false);
  const [status4xx, setStatus4xx] = useState(false);
  const [status5xx, setStatus5xx] = useState(false);
  const [ipFilter, setIPFilter] = useState('');
  const [uriFilter, setURIFilter] = useState('');
  const [workgroup, setWorkgroup] = useState('primary');
  const [outputLocation, setOutputLocation] = useState('');
  const [actionError, setActionError] = useState<Error | null>(null);

  const [jobId, setJobID] = useState<string | null>(() => loadJobID(jobStorageKey));
  useEffect(() => {
    try {
      if (jobId) sessionStorage.setItem(jobStorageKey, jobId);
      else sessionStorage.removeItem(jobStorageKey);
    } catch {
      // sessionStorage が使えない環境では保存しない。
    }
  }, [jobId, jobStorageKey]);

  const start = useStartAccessLogQuery(profile);
  const recheck = useRecheckAccessLogSources(profile, region);
  const statusQuery = useAccessLogQuery(profile, jobId);
  const status = statusQuery.data;
  const terminal =
    status?.state === 'succeeded' ||
    status?.state === 'partial' ||
    status?.state === 'failed' ||
    status?.state === 'cancelled';
  const resultsQuery = useAccessLogResults(profile, jobId, sourceNames, terminal === true);
  const cancel = useCancelAccessLogQuery(profile, jobId);

  const results = resultsQuery.data;
  const running = status?.state === 'queued' || status?.state === 'running';
  const ssoExpired =
    isSSOExpiredError(sourcesQuery.error) ||
    isSSOExpiredError(statusQuery.error) ||
    isSSOExpiredError(resultsQuery.error) ||
    isSSOExpiredError(start.error) ||
    isSSOExpiredError(cancel.error) ||
    isSSOExpiredError(recheck.error);
  const sourceErrors = status?.sources.filter((source) => source.error) ?? [];
  const selectedSources = useMemo(
    () => sources.filter((source) => selected.has(source.id) && isReady(source)),
    [sources, selected],
  );
  const longRange = preset === '7d';
  const insightsSelected = selectedSources.some((source) => source.engine === 'logs_insights');
  const canRun =
    selectedSources.length > 0 &&
    (!longRange || (confirmLongRange && !insightsSelected)) &&
    !start.isPending;

  const handleToggle = (id: string) => {
    const source = sources.find((item) => item.id === id);
    if (!source) return;
    setSelected((current) => {
      const next = new Set(current);
      if (next.has(id)) {
        next.delete(id);
        return next;
      }
      const key = accessLogDestinationKey(source);
      // リソースを特定できないソースは、同じ出力先を共有するほかのソースと
      // 同時に選べない。片方を選ぶともう片方を外す。
      for (const other of sources) {
        if (other.id === id || accessLogDestinationKey(other) !== key) continue;
        if (!source.status.attribution || !other.status.attribution) next.delete(other.id);
      }
      next.add(id);
      return next;
    });
  };

  const handleSelectAll = () => {
    setSelected(defaultSelection(sources));
  };

  const handleRun = () => {
    setActionError(null);
    if (selectedSources.length === 0) {
      setActionError(new Error(t('run.noSources')));
      return;
    }
    const startSources = selectedSources.map((source) => {
      if (source.engine !== 'athena') return { id: source.id };
      return { id: source.id, table: mappings[accessLogDestinationKey(source)] };
    });
    const missingTable = selectedSources.find((source) => {
      if (source.engine !== 'athena') return false;
      const mapping = mappings[accessLogDestinationKey(source)];
      return !mapping?.database || !mapping.table;
    });
    if (missingTable) {
      setActionError(new Error(t('run.tableRequired', { name: missingTable.name })));
      setTab('setup');
      return;
    }
    if (longRange && !confirmLongRange) {
      setActionError(new Error(t('run.confirmLongRange')));
      return;
    }
    if (longRange && insightsSelected) {
      setActionError(new Error(t('run.insightsRange')));
      return;
    }
    const { from, to } = buildRange(preset);
    const statusClasses = [status4xx ? '4xx' : '', status5xx ? '5xx' : ''].filter(Boolean);
    start.mutate(
      {
        view: tab === 'ips' ? 'ips' : 'pairs',
        sources: startSources,
        from,
        to,
        filter: {
          only_blocked: onlyBlocked || undefined,
          status_classes: statusClasses.length > 0 ? statusClasses : undefined,
          ip: ipFilter.trim() || undefined,
          uri_prefix: uriFilter.trim() || undefined,
        },
        limit: 200,
        confirm_long_range: confirmLongRange,
        workgroup: workgroup.trim() || undefined,
        output_location: outputLocation.trim() || undefined,
      },
      {
        onSuccess: (id) => {
          setJobID(id);
          setLastRunView(tab === 'setup' ? 'pairs' : tab);
          setTab(tab === 'setup' ? 'pairs' : tab);
        },
        onError: (error) =>
          setActionError(error instanceof Error ? error : new Error(String(error))),
      },
    );
  };

  const handleCancel = () => {
    cancel.mutate(undefined, {
      onError: (error) => setActionError(error instanceof Error ? error : new Error(String(error))),
    });
  };

  return (
    <div className="main accesslogs-main">
      <div className="toolbar accesslogs-toolbar">
        <div className="title">
          <span className="accesslogs-title-mark" aria-hidden />
          Access Logs
          <span className="subtitle">{t('subtitle')}</span>
        </div>
        <div className="accesslogs-controls">
          <label>
            <span>{t('timeRange')}</span>
            <select value={preset} onChange={(event) => setPreset(event.target.value as Preset)}>
              {PRESETS.map((item) => (
                <option key={item.key} value={item.key}>
                  {t(`range.${item.key}`)}
                </option>
              ))}
            </select>
          </label>
          {longRange ? (
            <label className="accesslogs-check">
              <input
                type="checkbox"
                checked={confirmLongRange}
                onChange={(event) => setConfirmLongRange(event.target.checked)}
              />
              <span>{t('confirmLongRange')}</span>
            </label>
          ) : null}
          {longRange && insightsSelected ? (
            <span className="accesslogs-inline-warning">{t('run.insightsRange')}</span>
          ) : null}
          <Button variant="primary" onClick={handleRun} disabled={!canRun}>
            {start.isPending ? t('starting') : t('run')}
          </Button>
          {running ? (
            <Button variant="ghost" onClick={handleCancel} disabled={cancel.isPending}>
              {cancel.isPending ? t('cancelling') : t('cancel')}
            </Button>
          ) : null}
        </div>
      </div>

      {ssoExpired ? <SSOExpiredBanner profile={profile} /> : null}
      {actionError ? <ErrorBanner error={actionError} /> : null}
      {start.error && !isSSOExpiredError(start.error) && !isOutputLocationError(start.error) ? (
        <ErrorBanner error={start.error} />
      ) : null}
      {sourcesQuery.error && !isSSOExpiredError(sourcesQuery.error) ? (
        <ErrorBanner error={sourcesQuery.error} />
      ) : null}
      {statusQuery.error && !isSSOExpiredError(statusQuery.error) ? (
        <ErrorBanner error={statusQuery.error} />
      ) : null}
      {resultsQuery.error && !isSSOExpiredError(resultsQuery.error) ? (
        <ErrorBanner error={resultsQuery.error} />
      ) : null}
      {recheck.error && !isSSOExpiredError(recheck.error) ? (
        <ErrorBanner error={recheck.error} />
      ) : null}

      {(sourcesQuery.data?.warnings ?? []).map((warning) => (
        <p key={warning} className="accesslogs-warning">
          {warning}
        </p>
      ))}
      {(status?.warnings ?? []).map((warning) => (
        <p key={warning} className="accesslogs-warning">
          {warning}
        </p>
      ))}

      <div className="accesslogs-layout">
        <SourceList
          sources={sources}
          selected={selected}
          onToggle={handleToggle}
          onSelectAll={handleSelectAll}
          onRecheck={() => recheck.mutate()}
          isRefreshing={recheck.isPending || sourcesQuery.isFetching}
        />

        <section className="accesslogs-content">
          <div className="accesslogs-filterbar">
            <label className="accesslogs-check">
              <input
                type="checkbox"
                checked={onlyBlocked}
                onChange={(event) => setOnlyBlocked(event.target.checked)}
              />
              <span>{t('filter.onlyBlocked')}</span>
            </label>
            <label className="accesslogs-check">
              <input
                type="checkbox"
                checked={status4xx}
                onChange={(event) => setStatus4xx(event.target.checked)}
              />
              <span>4xx</span>
            </label>
            <label className="accesslogs-check">
              <input
                type="checkbox"
                checked={status5xx}
                onChange={(event) => setStatus5xx(event.target.checked)}
              />
              <span>5xx</span>
            </label>
            <label>
              <span>{t('filter.ip')}</span>
              <input
                value={ipFilter}
                onChange={(event) => setIPFilter(event.target.value)}
                placeholder="203.0.113.9"
              />
            </label>
            <label>
              <span>{t('filter.uri')}</span>
              <input
                value={uriFilter}
                onChange={(event) => setURIFilter(event.target.value)}
                placeholder="/admin"
              />
            </label>
          </div>

          <div className="accesslogs-tabs" role="tablist">
            <button
              type="button"
              className={`qe-tab${tab === 'pairs' ? ' active' : ''}`}
              onClick={() => setTab('pairs')}
            >
              {t('tabs.pairs')}
            </button>
            <button
              type="button"
              className={`qe-tab${tab === 'ips' ? ' active' : ''}`}
              onClick={() => setTab('ips')}
            >
              {t('tabs.ips')}
            </button>
            <button
              type="button"
              className={`qe-tab${tab === 'setup' ? ' active' : ''}`}
              onClick={() => setTab('setup')}
            >
              {t('tabs.setup')}
            </button>
          </div>

          {tab === 'setup' ? (
            <SetupPanel
              profile={profile}
              sources={sources}
              mappings={mappings}
              onChange={(key, mapping) =>
                setMappings((current) => ({ ...current, [key]: mapping }))
              }
              workgroup={workgroup}
              onWorkgroupChange={setWorkgroup}
              outputLocation={outputLocation}
              onOutputLocationChange={setOutputLocation}
            />
          ) : (
            <ResultsPanel
              tab={tab}
              status={status}
              results={results}
              sourceNames={sourceNames}
              lastRunView={lastRunView}
              isLoading={statusQuery.isLoading || (terminal === true && resultsQuery.isLoading)}
              running={running}
              sourceErrors={sourceErrors}
            />
          )}
        </section>
      </div>
    </div>
  );
}

interface ResultsPanelProps {
  tab: Exclude<Tab, 'setup'>;
  status?: AccessLogQuery;
  results?: AccessLogResults;
  sourceNames: Record<string, string>;
  lastRunView: Exclude<Tab, 'setup'>;
  isLoading: boolean;
  running: boolean;
  sourceErrors: AccessLogQuery['sources'];
}

function ResultsPanel({
  tab,
  status,
  results,
  sourceNames,
  lastRunView,
  isLoading,
  running,
  sourceErrors,
}: ResultsPanelProps) {
  const { t } = useTranslation('accesslogs');
  if (isLoading) return <Loading />;
  if (!status) {
    return <p className="accesslogs-empty-hint">{t('empty.noRun')}</p>;
  }
  if (status.state === 'cancelled') {
    return <p className="accesslogs-empty-hint">{t('empty.cancelled')}</p>;
  }
  if (status.state === 'failed' && sourceErrors.length === 0) {
    return <p className="accesslogs-empty-hint">{t('empty.failed')}</p>;
  }
  if (running) {
    return (
      <div className="accesslogs-running">
        <p>{t('running')}</p>
        <SourceStatusTable sources={status.sources} />
      </div>
    );
  }
  if (!results && sourceErrors.length > 0) {
    return (
      <div className="accesslogs-errors">
        <SourceStatusTable sources={status.sources} />
      </div>
    );
  }
  if (!results) return <p className="accesslogs-empty-hint">{t('empty.noResults')}</p>;
  const rows = tab === 'ips' ? results.ips : results.rows;
  if (rows.length === 0) {
    if (tab !== lastRunView) {
      return <p className="accesslogs-empty-hint">{t('empty.runView')}</p>;
    }
    return (
      <div className="accesslogs-empty">
        <h3>{t('empty.title')}</h3>
        <p>{t('empty.noRows')}</p>
        <ul>
          <li>{t('empty.logging')}</li>
          <li>{t('empty.delay')}</li>
          <li>{t('empty.table')}</li>
          <li>{t('empty.permissions')}</li>
        </ul>
        {sourceErrors.length > 0 ? (
          <div className="accesslogs-errors">
            <SourceStatusTable sources={status.sources} />
          </div>
        ) : null}
        <SummaryPanel results={results} sourceNames={sourceNames} />
      </div>
    );
  }
  return (
    <div className="accesslogs-results">
      {sourceErrors.length > 0 ? (
        <div className="accesslogs-errors">
          <SourceStatusTable sources={status.sources} />
        </div>
      ) : null}
      <SummaryPanel results={results} sourceNames={sourceNames} />
      <div className="accesslogs-table">
        {tab === 'ips' ? (
          <DataTable
            rows={results.ips}
            columns={accessLogIPColumns}
            onSelect={() => undefined}
            selectedId={null}
          />
        ) : (
          <DataTable
            rows={results.rows}
            columns={accessLogPairColumns}
            onSelect={() => undefined}
            selectedId={null}
          />
        )}
      </div>
    </div>
  );
}

function SummaryPanel({
  results,
  sourceNames,
}: {
  results: AccessLogResults;
  sourceNames: Record<string, string>;
}) {
  const { t } = useTranslation('accesslogs');
  const summary = results.summary;
  return (
    <div className="accesslogs-summary">
      <div className="accesslogs-stats">
        <Stat label={t('summary.records')} value={summary.recordCount.toLocaleString()} />
        <Stat
          label={t('summary.blocked')}
          value={summary.blockedCount.toLocaleString()}
          tone={summary.blockedCount > 0 ? 'neg' : undefined}
        />
        <Stat label={t('summary.challenged')} value={summary.challengedCount.toLocaleString()} />
        <Stat label="4xx" value={summary.clientErrorCount.toLocaleString()} />
        <Stat label="5xx" value={summary.serverErrorCount.toLocaleString()} />
        <Stat label={t('summary.ips')} value={summary.distinctIps.toLocaleString()} />
        <Stat label={t('summary.uris')} value={summary.distinctUris.toLocaleString()} />
      </div>
      <p className="accesslogs-muted accesslogs-top-note">{t('summary.topNote')}</p>
      {summary.duplicateWarning ? (
        <p className="accesslogs-warning">{t('summary.duplicateWarning')}</p>
      ) : null}
      {results.warnings.map((warning) => (
        <p key={warning} className="accesslogs-warning">
          {warning}
        </p>
      ))}
      <div className="accesslogs-source-summary">
        {summary.bySources.map((source) => (
          <span key={source.sourceId} className="accesslogs-source-summary-item">
            <strong>{sourceNames[source.sourceId] ?? source.sourceId}</strong>
            <span>{t('summary.sourceRecords', { count: source.records.toLocaleString() })}</span>
            <span>{t('summary.sourceScanned', { bytes: formatBytes(source.bytesScanned) })}</span>
          </span>
        ))}
      </div>
    </div>
  );
}

function SourceStatusTable({ sources }: { sources: AccessLogQuery['sources'] }) {
  return (
    <table className="dt accesslogs-status-table">
      <thead>
        <tr>
          <th>Source</th>
          <th>Engine</th>
          <th>State</th>
          <th>Records scanned</th>
          <th>Bytes scanned</th>
          <th>Error</th>
        </tr>
      </thead>
      <tbody>
        {sources.map((source) => (
          <tr key={source.sourceId}>
            <td>{source.name}</td>
            <td>{source.engine}</td>
            <td>
              <span className={`status ${stateClass(source.state)}`}>{source.state}</span>
            </td>
            <td>{source.recordsScanned.toLocaleString()}</td>
            <td>{formatBytes(source.bytesScanned)}</td>
            <td className="accesslogs-error-cell">{source.error?.message ?? ''}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

// CloudWatch Logs ビュー (デザイン Turn 8a)。左にロググループツリー (複数選択)、右上にフィルタ
// パターン + ヒストグラム、右下に JSON 展開可能なログ一覧。静的検索 (FilterLogEvents, ページング) と
// Live Tail (StartLiveTail, WebSocket) の 2 モード。Athena/Cloud Logging と同じ「専用ビュー埋め込み」
// パターンで AccountView から使う。
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useCWLogEvents, useCWLogGroups } from '../api/queries';
import { cwLogsTailUrl } from '../api/terminal';
import { ErrorBanner } from '../components/ErrorBanner';
import { SSOExpiredBanner } from '../components/SSOExpiredBanner';
import { LogHistogram } from '../components/logviewer/LogHistogram';
import { LogFieldRow, LogList } from '../components/logviewer/LogList';
import { LogToolbarActions } from '../components/logviewer/LogToolbarActions';
import { LogTree } from '../components/logviewer/LogTree';
import { LogViewerShell } from '../components/logviewer/LogViewerShell';
import { useCopy } from '../components/logviewer/useCopy';
import { useLiveTail } from '../components/logviewer/useLiveTail';
import { formatLogClock, jsonFieldsOf, rowsToCsv, rowsToJson } from '../lib/logFormat';
import { buildLogGroupTree } from '../lib/logGroupTree';
import { buildHistogram, rangeFromItems } from '../lib/logHistogram';
import { type PresetOption, presetToRange } from '../lib/logTimeRange';
import { cwSeverityFromMessage } from '../lib/logSeverity';
import { cwLogEventFromRaw } from '../lib/normalize';
import { ApiError } from '../types/common';
import type { CWLogEventRaw, CWLogEventRow } from '../types/aws';

export interface CloudWatchLogsViewProps {
  profile: string;
  region: string;
}

const FILTER_SNIPPETS = [
  { label: 'ERROR', snippet: 'ERROR' },
  { label: 'JSON level', snippet: '{ $.level = "error" }' },
];

function isSSOExpired(...errors: unknown[]): boolean {
  return errors.some((e) => e instanceof ApiError && e.code === 'SSO_TOKEN_EXPIRED');
}

const copyRaw = (text: string) => void navigator.clipboard?.writeText(text);

export function CloudWatchLogsView({ profile, region }: CloudWatchLogsViewProps) {
  // プロファイル / リージョン切替時は key で再マウントし、選択・フィルタ・結果をリセットする。
  return <CloudWatchLogsEditor key={`${profile}:${region}`} profile={profile} region={region} />;
}

function CloudWatchLogsEditor({ profile, region }: CloudWatchLogsViewProps) {
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [filterInput, setFilterInput] = useState('');
  const [appliedFilter, setAppliedFilter] = useState('');
  const [appliedGroups, setAppliedGroups] = useState<string[]>([]);
  const [preset, setPreset] = useState<PresetOption>('1h');
  const [customStart, setCustomStart] = useState('');
  const [customEnd, setCustomEnd] = useState('');
  const [appliedRange, setAppliedRange] = useState<{ start: string; end: string } | null>(null);
  const [runToken, setRunToken] = useState(0);
  const [live, setLive] = useState(false);
  const [autoScroll, setAutoScroll] = useState(true);
  const bodyRef = useRef<HTMLDivElement>(null);

  const csvCopy = useCopy();
  const jsonCopy = useCopy();

  const groupsQuery = useCWLogGroups(profile, region);
  const eventsQuery = useCWLogEvents(
    profile,
    region,
    runToken,
    {
      groups: appliedGroups,
      filter: appliedFilter,
      start: appliedRange?.start,
      end: appliedRange?.end,
    },
    !live,
  );

  const treeNodes = useMemo(
    () => buildLogGroupTree((groupsQuery.data ?? []).map((g) => ({ name: g.name, arn: g.arn }))),
    [groupsQuery.data],
  );

  const toggleGroup = useCallback((value: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(value)) next.delete(value);
      else next.add(value);
      return next;
    });
  }, []);

  // Live Tail: appliedGroups / appliedFilter が変わると URL が変わり再接続する。
  const tailUrl = useMemo(
    () => cwLogsTailUrl(profile, region, appliedGroups, appliedFilter),
    [profile, region, appliedGroups, appliedFilter],
  );
  const parseLive = useCallback(
    (raw: Record<string, unknown>, seq: number) =>
      cwLogEventFromRaw(raw as unknown as CWLogEventRaw, seq),
    [],
  );
  const liveTail = useLiveTail<CWLogEventRow>({
    enabled: live && appliedGroups.length > 0,
    url: tailUrl,
    parse: parseLive,
  });

  const runSearch = useCallback(() => {
    const groups = [...selected];
    setAppliedGroups(groups);
    setAppliedFilter(filterInput);
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
      if (next) {
        setAppliedGroups([...selected]);
        setAppliedFilter(filterInput);
      }
      return next;
    });
  }, [selected, filterInput]);

  const staticRows = useMemo(() => {
    const pages = eventsQuery.data?.pages ?? [];
    const rows: CWLogEventRow[] = [];
    let index = 0;
    for (const page of pages) {
      for (const raw of page.events) {
        rows.push(cwLogEventFromRaw(raw, index));
        index += 1;
      }
    }
    return rows;
  }, [eventsQuery.data]);

  const rows = live ? liveTail.lines : staticRows;

  // ヒストグラム: 取得済み行を severity 付きで時間バケットに集計する (クライアント集計)。
  const histItems = useMemo(
    () =>
      rows
        .map((r) => ({ tsMs: Date.parse(r.timestamp), level: cwSeverityFromMessage(r.message) }))
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

  // Live Tail 時の自動スクロール (上へスクロールしたら止める)。
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

  const ssoExpired = isSSOExpired(groupsQuery.error, eventsQuery.error);
  const listError = live ? undefined : eventsQuery.error;

  const exportCsv = useCallback(() => {
    const csv = rowsToCsv(
      ['timestamp', 'log_group', 'message'],
      rows.map((r) => [r.timestamp, r.logGroup, r.message]),
    );
    csvCopy.copy(csv);
  }, [rows, csvCopy]);

  const exportJson = useCallback(() => {
    jsonCopy.copy(rowsToJson(rows));
  }, [rows, jsonCopy]);

  const renderDetail = useCallback((row: CWLogEventRow) => {
    const fields = jsonFieldsOf(row.message);
    return (
      <div className="lv-detail-fields">
        {fields ? (
          fields.map((f) => (
            <LogFieldRow
              key={f.key}
              label={f.key}
              value={f.value}
              onAddFilter={() => setFilterInput(`{ $.${f.key} = "${f.value}" }`)}
              onCopy={() => copyRaw(f.value)}
            />
          ))
        ) : (
          <LogFieldRow label="message" value={row.message} onCopy={() => copyRaw(row.message)} />
        )}
        {row.logStream && (
          <LogFieldRow
            label="logStream"
            value={row.logStream}
            onCopy={() => copyRaw(row.logStream)}
          />
        )}
        {row.eventId && <LogFieldRow label="eventId" value={row.eventId} />}
      </div>
    );
  }, []);

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
      <span>{eventsQuery.isFetching ? '取得中…' : `${staticRows.length} 件表示中 · 新しい順`}</span>
      {eventsQuery.hasNextPage && (
        <button
          className="lv-more-btn"
          onClick={() => void eventsQuery.fetchNextPage()}
          disabled={eventsQuery.isFetchingNextPage}
        >
          {eventsQuery.isFetchingNextPage ? '読み込み中…' : 'さらに読み込む'}
        </button>
      )}
    </>
  );

  const histogramNode =
    rows.length > 0 ? (
      <>
        <div className="lv-hist-caption">
          <span>イベント数</span>
          <span>{live ? 'ライブ更新中…' : `計 ${rows.length} 件`}</span>
        </div>
        <LogHistogram buckets={buckets} mode="mono" />
        <div className="lv-hist-axis">
          <span>{formatLogClock(new Date(range.startMs).toISOString())}</span>
          <span>{live ? 'now' : formatLogClock(new Date(range.endMs).toISOString())}</span>
        </div>
      </>
    ) : null;

  return (
    <LogViewerShell
      title="CloudWatch Logs"
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
          exportLabel={jsonCopy.copied ? 'コピー済み' : 'エクスポート (JSON)'}
          onExport={exportJson}
          exportDisabled={rows.length === 0}
        />
      }
      banner={
        ssoExpired ? (
          <SSOExpiredBanner profile={profile} />
        ) : listError ? (
          <ErrorBanner error={listError} />
        ) : undefined
      }
      tree={
        <LogTree
          nodes={treeNodes}
          selected={selected}
          onToggle={toggleGroup}
          searchPlaceholder="ロググループを検索…"
          loading={groupsQuery.isLoading}
          emptyMessage="ロググループがありません"
          footer={
            selected.size > 0
              ? `${selected.size} グループ選択中${selected.size > 1 ? ' · 横断検索' : ''}`
              : 'ロググループを選択'
          }
        />
      }
      filterBar={
        <>
          <span className="lv-filter-icon">⌕</span>
          <input
            className="lv-filter-input"
            value={filterInput}
            onChange={(e) => setFilterInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') runSearch();
            }}
            placeholder='フィルタパターン (例: ERROR または { $.level = "error" })'
          />
          <div className="lv-filter-actions">
            {FILTER_SNIPPETS.map((s) => (
              <button key={s.snippet} className="btn sm" onClick={() => setFilterInput(s.snippet)}>
                {s.label}
              </button>
            ))}
            <button className="lv-run-btn" onClick={runSearch} disabled={selected.size === 0}>
              検索
            </button>
          </div>
        </>
      }
      histogram={histogramNode}
      logList={
        <LogList<CWLogEventRow>
          rows={rows}
          getKey={(r) => r.id}
          getLevel={(r) => cwSeverityFromMessage(r.message)}
          getTimestamp={(r) => r.timestamp}
          secondHeader="GROUP"
          secondWidth={130}
          renderSecond={(r) => (
            <span className="lv-group-name" title={r.logGroup}>
              {r.logGroup}
            </span>
          )}
          messageHeader="MESSAGE"
          getMessage={(r) => r.message}
          renderDetail={renderDetail}
          tintMessageByLevel
          copyLabel={csvCopy.copied ? 'コピー済み' : 'CSVコピー'}
          onCopy={exportCsv}
          footer={footer}
          emptyMessage={
            selected.size === 0
              ? 'ロググループを選択して「検索」を押してください'
              : live
                ? 'ライブテール待機中…'
                : '「検索」を押すとログイベントがここに表示されます'
          }
          bodyRef={bodyRef}
          onScroll={onScroll}
        />
      }
    />
  );
}

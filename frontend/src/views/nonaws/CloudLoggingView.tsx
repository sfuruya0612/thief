// Cloud Logging ビュー: フィルター入力 + 期間プリセット + 実行 (静的取得、ページング) /
// Live トグル (Live Tail、WebSocket) の 2 モードでログエントリを閲覧する。
// BigQueryView と同じ「GcpView から専用ビューとして埋め込む」パターンを踏襲する。
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useGcpLogEntries } from '../../api/queries';
import { gcpLoggingTailUrl } from '../../api/terminal';
import { ErrorBanner } from '../../components/ErrorBanner';
import { appendLogLines } from '../../lib/logLines';
import { type LogTimeRangePreset, presetToRange } from '../../lib/logTimeRange';
import { logEntryFromRaw, logSeverityLevel } from '../../lib/normalizeGcp';
import type { LogEntryRaw, LogEntryRow } from '../../types/gcp';

export interface CloudLoggingViewProps {
  projectId: string;
}

type PresetOption = LogTimeRangePreset | 'custom';

const PRESET_LABELS: Record<PresetOption, string> = {
  '15m': '直近15分',
  '1h': '直近1時間',
  '6h': '直近6時間',
  '24h': '直近24時間',
  '7d': '直近7日',
  custom: 'カスタム',
};

const FILTER_SNIPPETS = [
  { label: 'ERROR以上', snippet: 'severity>=ERROR' },
  { label: 'WARNING以上', snippet: 'severity>=WARNING' },
  { label: 'INFOのみ', snippet: 'severity=INFO' },
];

type LiveStatus = 'idle' | 'connecting' | 'connected' | 'closed' | 'error';

export function CloudLoggingView({ projectId }: CloudLoggingViewProps) {
  // プロジェクト切替時はフィルター・結果をリセットして再マウントする
  return <CloudLoggingEditor key={projectId} projectId={projectId} />;
}

function CloudLoggingEditor({ projectId }: CloudLoggingViewProps) {
  const [filterInput, setFilterInput] = useState('');
  const [appliedFilter, setAppliedFilter] = useState('');
  const [preset, setPreset] = useState<PresetOption>('1h');
  const [customStart, setCustomStart] = useState('');
  const [customEnd, setCustomEnd] = useState('');
  const [range, setRange] = useState<{ start: string; end: string } | null>(null);
  const [runToken, setRunToken] = useState(0);

  const [live, setLive] = useState(false);
  const [liveStatus, setLiveStatus] = useState<LiveStatus>('idle');
  const [liveMessage, setLiveMessage] = useState<string | null>(null);
  const [liveLines, setLiveLines] = useState<LogEntryRow[]>([]);

  const [autoScroll, setAutoScroll] = useState(true);
  const logBoxRef = useRef<HTMLDivElement>(null);

  const insertFilterSnippet = useCallback((snippet: string) => {
    setFilterInput((prev) => (prev.trim() ? `${prev.trim()} AND ${snippet}` : snippet));
  }, []);

  // 「実行」ボタン: Live 中はフィルターを再適用して Live Tail を再接続する。
  // Live でなければ静的取得 (期間プリセット→start/end 変換 + ページング) を開始する。
  const runQuery = useCallback(() => {
    setAppliedFilter(filterInput);
    if (live) {
      setLiveLines([]);
      setLiveMessage(null);
      return;
    }
    const r =
      preset === 'custom'
        ? {
            start: customStart ? new Date(customStart).toISOString() : '',
            end: customEnd ? new Date(customEnd).toISOString() : '',
          }
        : presetToRange(preset);
    setRange(r);
    setRunToken((t) => t + 1);
  }, [filterInput, live, preset, customStart, customEnd]);

  const toggleLive = useCallback(() => {
    setLive((prev) => {
      const next = !prev;
      if (next) {
        setAppliedFilter(filterInput);
        setLiveLines([]);
        setLiveMessage(null);
      }
      return next;
    });
  }, [filterInput]);

  const staticQuery = useGcpLogEntries(
    projectId,
    runToken,
    { filter: appliedFilter, start: range?.start, end: range?.end },
    !live,
  );

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

  const rows = live ? liveLines : staticRows;

  // Live Tail: WebSocket 接続。projectId/appliedFilter が変わると再接続する
  // (Terminal.tsx と同じ「URL が変わったら effect を再実行する」パターン)。
  useEffect(() => {
    if (!live) {
      setLiveStatus('idle');
      return;
    }
    setLiveStatus('connecting');
    setLiveMessage(null);

    let disposed = false;
    const ws = new WebSocket(gcpLoggingTailUrl(projectId, appliedFilter));

    ws.onopen = () => {
      if (disposed) return;
      setLiveStatus('connected');
    };
    ws.onmessage = (ev) => {
      if (disposed) return;
      if (typeof ev.data !== 'string') return;
      let msg: { type?: string; reason?: string };
      try {
        msg = JSON.parse(ev.data);
      } catch {
        return;
      }
      if (msg.type === 'end') {
        setLiveStatus('closed');
        if (msg.reason) setLiveMessage(msg.reason);
        return;
      }
      const row = logEntryFromRaw(msg as LogEntryRaw, Date.now());
      setLiveLines((prev) => appendLogLines(prev, [row]));
    };
    ws.onerror = () => {
      if (disposed) return;
      setLiveStatus('error');
    };
    ws.onclose = () => {
      if (disposed) return;
      setLiveStatus((prev) => (prev === 'error' ? prev : 'closed'));
    };

    return () => {
      disposed = true;
      ws.close();
    };
  }, [live, projectId, appliedFilter]);

  // 利用者が上へスクロールしたら自動スクロールを止める (Live Tail の定石)。
  const onLogBoxScroll = useCallback(() => {
    const el = logBoxRef.current;
    if (!el) return;
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
    setAutoScroll(atBottom);
  }, []);

  useEffect(() => {
    if (!autoScroll) return;
    const el = logBoxRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [rows, autoScroll]);

  const error = live ? undefined : staticQuery.error;

  return (
    <div className="main qe-main">
      <div className="toolbar">
        <div className="title">
          <h1>Cloud Logging</h1>
          <span className="subtitle">log entries</span>
        </div>
      </div>

      <div className="qe-panel qe-editor-card">
        <textarea
          className="qe-hint"
          style={{ width: '100%', minHeight: 60, resize: 'vertical' }}
          placeholder='Logging query language (例: resource.type="cloud_run_revision" AND severity>=ERROR)'
          value={filterInput}
          onChange={(e) => setFilterInput(e.target.value)}
        />

        <div className="qe-toolbar">
          {FILTER_SNIPPETS.map((s) => (
            <button
              key={s.snippet}
              className="btn sm"
              onClick={() => insertFilterSnippet(s.snippet)}
            >
              {s.label}
            </button>
          ))}

          <select
            className="btn sm"
            value={preset}
            onChange={(e) => setPreset(e.target.value as PresetOption)}
            disabled={live}
          >
            {(Object.keys(PRESET_LABELS) as PresetOption[]).map((p) => (
              <option key={p} value={p}>
                {PRESET_LABELS[p]}
              </option>
            ))}
          </select>

          {preset === 'custom' && !live && (
            <>
              <input
                type="datetime-local"
                className="btn sm"
                value={customStart}
                onChange={(e) => setCustomStart(e.target.value)}
              />
              <input
                type="datetime-local"
                className="btn sm"
                value={customEnd}
                onChange={(e) => setCustomEnd(e.target.value)}
              />
            </>
          )}

          <button className="qe-run" onClick={runQuery}>
            実行
          </button>

          <button className={`btn sm ${live ? 'active' : ''}`} onClick={toggleLive}>
            Live {live ? 'ON' : 'OFF'}
          </button>

          <span className="qe-toolbar-right qe-muted">
            {live
              ? liveStatus === 'connecting'
                ? '接続中…'
                : liveStatus === 'connected'
                  ? `受信中 (${liveLines.length}件)`
                  : liveStatus === 'closed'
                    ? `終了${liveMessage ? `: ${liveMessage}` : ''}`
                    : liveStatus === 'error'
                      ? `エラー${liveMessage ? `: ${liveMessage}` : ''}`
                      : ''
              : staticQuery.isFetching
                ? '取得中…'
                : `${staticRows.length}件`}
          </span>
        </div>
      </div>

      {Boolean(error) && <ErrorBanner error={error} />}

      <div className="logbox" ref={logBoxRef} onScroll={onLogBoxScroll} style={{ flex: 1 }}>
        {rows.length === 0 ? (
          <div className="qe-tab-empty">
            {live ? 'Live Tail 待機中…' : '「実行」を押すとログエントリがここに表示されます'}
          </div>
        ) : (
          rows.map((row) => (
            <div key={row.id} className={`lvl-${logSeverityLevel(row.severity)}`}>
              <span className="ts">{row.timestamp}</span>
              <span>[{row.severity}]</span> <span>{row.logName}</span> <span>{row.payload}</span>
            </div>
          ))
        )}
      </div>

      {!live && staticQuery.hasNextPage && (
        <div className="qe-toolbar">
          <button
            className="btn sm"
            onClick={() => void staticQuery.fetchNextPage()}
            disabled={staticQuery.isFetchingNextPage}
          >
            {staticQuery.isFetchingNextPage ? '読み込み中…' : 'さらに読み込む'}
          </button>
        </div>
      )}
    </div>
  );
}

// オブジェクト (1 件または複数) に対する SQL 検索パネル。DrawerObjectBrowser の一覧を
// 置き換えて表示し、Close で一覧に戻る。取り込み (Worker による OPFS への書き込み) とクエリ
// 実行 (DuckDB) は props のインターフェース越しに行い、テストではモックを差し込む。
//
// 複数ファイルは並列度 OBJECT_QUERY_INGEST_CONCURRENCY で取り込み、全部そろってから
// obj ビューを 1 つ作る (objectQueryViewSql)。取り込みが終わった ingestor は、OPFS の
// ファイルと Web Locks のロックを保持したまま解放まで terminate しない。
//
// 寿命: OPFS のファイルと obj ビューはアンマウント時に解放する。解放処理は useEffect の
// クリーンアップに置き、Close ボタンに限らず Drawer の X ボタン、Objects タブから他タブへの
// 切り替え、リソースやリージョンの切り替えのいずれでも動く。DuckDB 側の登録と解放は
// enqueueDb で発生順に直列化し、解放の後に登録が走ってファイルとビューが残ることを防ぐ。
//
// StrictMode の setup → cleanup → setup では、開始は useRef の世代 (generation) ごとに 1 回に
// 限定し、次の世代の開始が前の世代の teardown の完了を待つ (直列化)。中断は AbortError と
// して扱い、エラー表示しない。
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ErrorBanner } from '../ErrorBanner';
import { SSOExpiredBanner } from '../SSOExpiredBanner';
import { SqlEditor } from '../query/SqlEditor';
import { ResultTable } from '../query/ResultTable';
import { QueryResultChart } from '../charts/QueryResultChart';
import { formatBytes } from '../tables/columns';
import { isSSOExpiredError } from '../../lib/ssoError';
import {
  initialChartSelection,
  QUERY_CHART_MAX_CATEGORIES,
  queryChartCapabilities,
  queryChartOverCategoryLimit,
} from '../../lib/queryChart';
import type { QueryChartSelection, QueryResultChartType } from '../../lib/queryChart';
import {
  formatObjectQueryLimit,
  isAbortError,
  ObjectQueryIngestError,
  objectQueryFileName,
  objectQueryFormat,
  objectQueryLockName,
  objectQueryOpfsPath,
  objectQueryOverLimit,
  objectQueryStorageName,
  objectQueryViewSql,
  objectQueryExtensions,
  OBJECT_QUERY_DEFAULT_SQL,
  OBJECT_QUERY_INGEST_CONCURRENCY,
  OBJECT_QUERY_MAX_ROWS,
} from '../../lib/objectQuery';
import type {
  ObjectQueryEngine,
  ObjectQueryFormat,
  ObjectQueryResult,
  ObjectQueryTarget,
} from '../../lib/objectQuery';
import { checkObjectQueryQuota } from '../../lib/opfsIngest';
import type { ObjectIngestProgress, ObjectIngestor } from '../../lib/opfsIngest';
import { Button } from '../primitives';

export interface DrawerObjectQueryProps {
  // files は検索対象のオブジェクト。1 件でも配列で渡す。全ファイルの読み取り関数と
  // 区切り文字は一致していること (選択の可否は objectQuerySelectionDisabledReason が判定する)。
  files: ObjectQueryTarget[];
  // maxBytes は 1 回の検索で取り込む合計サイズの上限 (バイト)。設定 (GET /api/config) の
  // 取得完了後に確定する。
  maxBytes: number;
  // profile は S3 の SSO 期限切れバナーに使う。GCS には SSO が無いため渡さない。
  profile?: string;
  engine: ObjectQueryEngine;
  createIngestor: () => ObjectIngestor;
  onClose: () => void;
}

// FALLBACK_FORMAT は対象外の形式で開かれた場合の表示。呼び出し側は対象形式の行だけを
// 選択できるため通常は到達しないが、判定が変わっても安全に閉じられるようにしておく。
const FALLBACK_FORMAT: ObjectQueryFormat = { extension: '', readFunction: 'read_csv' };

export function DrawerObjectQuery(props: DrawerObjectQueryProps) {
  // ファイルごとの形式。ファイル名に元の拡張子 (.csv.gz など) を保つために使う。
  const formats = useMemo(
    () => props.files.map((file) => objectQueryFormat(file.key) ?? FALLBACK_FORMAT),
    [props.files],
  );
  return <ObjectQueryPanel {...props} formats={formats} format={formats[0] ?? FALLBACK_FORMAT} />;
}

interface ObjectQueryPanelProps extends DrawerObjectQueryProps {
  formats: ObjectQueryFormat[];
  format: ObjectQueryFormat;
}

// CHART_HEIGHT はグラフの高さ。グラフを出せないときの案内も同じ高さにして、切り替えや
// 選択のたびにレイアウトが跳ねるのを防ぐ。
const CHART_HEIGHT = 320;

// CHART_TYPE_OPTIONS は選べるグラフの種類。値は ECharts の系列型名をそのまま使う。
const CHART_TYPE_OPTIONS: { value: QueryResultChartType; labelKey: string }[] = [
  { value: 'bar', labelKey: 'drawerObjectQuery.chart.typeBar' },
  { value: 'line', labelKey: 'drawerObjectQuery.chart.typeLine' },
];

function ObjectQueryPanel({
  files,
  maxBytes,
  profile,
  engine,
  createIngestor,
  onClose,
  formats,
  format,
}: ObjectQueryPanelProps) {
  const { t } = useTranslation('drawerStorage');
  // パネルの識別子と OPFS 上のファイル名はパネルを開いたときに 1 度だけ決める
  // (再ダウンロードしても同じ名前)。ファイル名はファイルごとの元の拡張子を保つ。
  const [panelId] = useState(() => crypto.randomUUID());
  const [fileIds] = useState(() => files.map(() => crypto.randomUUID()));
  const fileNames = useMemo(
    () => fileIds.map((id, i) => objectQueryFileName(id, formats[i])),
    [fileIds, formats],
  );
  const storageNames = useMemo(() => fileNames.map(objectQueryStorageName), [fileNames]);
  const opfsPaths = useMemo(() => fileNames.map(objectQueryOpfsPath), [fileNames]);
  const lockNames = useMemo(() => fileNames.map(objectQueryLockName), [fileNames]);
  // 読み取り関数が要する DuckDB の拡張 (json / parquet)。csv / tsv は空配列。
  const extensions = useMemo(() => objectQueryExtensions(format), [format]);

  const [phase, setPhase] = useState<'ingesting' | 'ready' | 'error'>('ingesting');
  const [progress, setProgress] = useState<ObjectIngestProgress>({ written: 0, total: null });
  const [ingestError, setIngestError] = useState<unknown>(null);
  const [sql, setSql] = useState(OBJECT_QUERY_DEFAULT_SQL);
  const [result, setResult] = useState<ObjectQueryResult | null>(null);
  const [queryError, setQueryError] = useState<unknown>(null);
  const [running, setRunning] = useState(false);
  // 結果部の表示 (表 / グラフ) と、グラフの軸の選択と種類。
  const [resultView, setResultView] = useState<'table' | 'chart'>('table');
  const [chartSelection, setChartSelection] = useState<QueryChartSelection>({
    xIndex: 0,
    yIndexes: [],
  });
  const [chartType, setChartType] = useState<QueryResultChartType>('bar');

  // teardown と開始処理は依存を空にした effect から呼ぶため、最新の props を ref で参照する。
  const latestRef = useRef({
    engine,
    maxBytes,
    files,
    fileNames,
    storageNames,
    lockNames,
    opfsPaths,
    format,
    extensions,
    panelId,
  });
  latestRef.current = {
    engine,
    maxBytes,
    files,
    fileNames,
    storageNames,
    lockNames,
    opfsPaths,
    format,
    extensions,
    panelId,
  };
  const createIngestorRef = useRef(createIngestor);
  createIngestorRef.current = createIngestor;

  // ingestorsRef は作成済みの ingestor (順番が来たファイルの分だけ)。取り込みが終わった
  // ingestor もファイルとロックを保持したまま解放まで残る。
  const ingestorsRef = useRef<ObjectIngestor[]>([]);
  // generation は effect の世代。前の世代の非同期処理が状態を更新しないための識別子。
  const generationRef = useRef(0);
  const disposedRef = useRef(false);
  const teardownRef = useRef<Promise<void>>(Promise.resolve());
  // dbChainRef は DuckDB の登録と解放の順序を保証する直列化キュー。
  const dbChainRef = useRef<Promise<void>>(Promise.resolve());

  const enqueueDb = useCallback(<T,>(task: () => Promise<T>): Promise<T> => {
    const next = dbChainRef.current.then(task);
    // 失敗しても後続のタスクを止めない (エラーは呼び出し元が受け取る)。
    dbChainRef.current = next.then(
      () => undefined,
      () => undefined,
    );
    return next;
  }, []);

  const runQuery = useCallback(async (text: string, generation: number) => {
    if (generationRef.current !== generation) return;
    setQueryError(null);
    setRunning(true);
    try {
      const res = await latestRef.current.engine.run(text);
      if (generationRef.current !== generation) return;
      setResult(res);
      // 列構成が変わりうるため、クエリの実行ごとに軸の選択を初期選択 (X は先頭の列、
      // Y は先頭の数値列) に戻す。
      setChartSelection(initialChartSelection(res.columns, res.rows));
      // グラフを表示できない結果 (列が無い、数値列が無い) になったときは表へ戻す。グラフ
      // 表示のままだと、切り替えが無効なのに選択されたままになり、表もグラフも出ない。
      if (queryChartCapabilities(res.columns, res.rows).disabledReason !== null) {
        setResultView('table');
      }
    } catch (err) {
      if (isAbortError(err) || generationRef.current !== generation) return;
      setQueryError(err);
    } finally {
      if (generationRef.current === generation) setRunning(false);
    }
  }, []);

  // terminateIngestors は作成済みの ingestor をすべて終了する (ファイルの削除とロックの
  // 解放)。二重呼び出しでも安全なため、取り込みの失敗時とアンマウント時の両方から呼べる。
  const terminateIngestors = useCallback(async (): Promise<void> => {
    const created = ingestorsRef.current;
    ingestorsRef.current = [];
    for (const ingestor of created) {
      try {
        await ingestor.terminate();
      } catch (err) {
        console.warn('failed to terminate the object query worker', err);
      }
    }
  }, []);

  const startPipeline = useCallback(
    (generation: number) => {
      ingestorsRef.current = [];
      return (async () => {
        const {
          engine,
          maxBytes,
          files,
          fileNames,
          storageNames,
          lockNames,
          opfsPaths,
          format,
          extensions,
          panelId,
        } = latestRef.current;
        // obj ビューの定義。全ファイルを読み取り関数のリスト引数で 1 つの表にし、由来の
        // オブジェクトを object_key 列で区別できるようにする。組み立ては描画時ではなく
        // 取り込みの前に行い、失敗 (files が空) は取り込みの失敗と同じ経路でエラー表示する。
        const viewSql = objectQueryViewSql(
          files.map((file, i) => ({
            opfsPath: opfsPaths[i],
            fileName: fileNames[i],
            key: file.key,
          })),
          format,
        );
        // written / totals はファイルごとの最後の進捗。進捗表示と合計サイズの判定に使う。
        const written = files.map(() => 0);
        const totals = files.map<number | null>(() => null);
        const aggregateProgress = (): ObjectIngestProgress => ({
          written: written.reduce((sum, n) => sum + n, 0),
          total: totals.every((total) => total !== null)
            ? totals.reduce((sum, total) => sum + (total ?? 0), 0)
            : null,
        });
        // 取り込みの前に、選んだ全ファイルの合計サイズで空き容量を 1 回確かめる。ファイル
        // ごとの確認は OpfsIngestor.ingest が書き込みの直前に行う (書き込みが進んで空きが
        // 減った後の確認になる)。
        await checkObjectQueryQuota(files.reduce((sum, file) => sum + file.size, 0));
        if (disposedRef.current || generationRef.current !== generation) return;

        // 同時に取り込むのは OBJECT_QUERY_INGEST_CONCURRENCY 件まで。順番が来たファイルに
        // だけ ingestor を作り (待っているファイルには作らない)、1 つ失敗したら待っている
        // ファイルは始めず、作成済みの ingestor を全部止める。
        const pending = files.map((_, index) => index);
        let failure: unknown = null;
        const runWorker = async (): Promise<void> => {
          for (;;) {
            if (failure !== null || disposedRef.current || generationRef.current !== generation) {
              return;
            }
            const index = pending.shift();
            if (index === undefined) return;
            try {
              // ingestor の作成 (Worker の起動) も失敗しうるため try の中で行う。
              const ingestor = createIngestorRef.current();
              ingestorsRef.current.push(ingestor);
              await ingestor.ingest(
                {
                  url: files[index].url,
                  storageName: storageNames[index],
                  lockName: lockNames[index],
                  maxBytes,
                  size: files[index].size,
                },
                (p) => {
                  written[index] = p.written;
                  totals[index] = p.total;
                  if (disposedRef.current || generationRef.current !== generation) return;
                  setProgress(aggregateProgress());
                },
              );
            } catch (err) {
              // 最初に失敗したエラーだけを残し、作成済みの ingestor を全部止める。
              if (failure === null) {
                failure = err;
                await terminateIngestors();
              }
              return;
            }
          }
        };
        await Promise.all(
          Array.from({ length: Math.min(OBJECT_QUERY_INGEST_CONCURRENCY, files.length) }, () =>
            runWorker(),
          ),
        );
        if (failure !== null) throw failure;
        // 一覧のサイズが古い場合の保険。全ファイルの written の合計で上限を確かめ、超えて
        // いたら全体を失敗させて取り込んだファイルを解放する。
        const totalWritten = written.reduce((sum, n) => sum + n, 0);
        if (objectQueryOverLimit(totalWritten, maxBytes)) {
          await terminateIngestors();
          throw new ObjectQueryIngestError(
            'tooLarge',
            `ingested size is larger than the limit (${totalWritten} > ${maxBytes})`,
          );
        }
        if (disposedRef.current || generationRef.current !== generation) return;
        // 登録は解放 (teardown) と直列化する。解放の後に登録が走ると、OPFS のファイルと
        // DuckDB のファイル登録が残る。
        const registered = await enqueueDb(async () => {
          if (disposedRef.current) return false;
          await engine.registerView(panelId, opfsPaths, viewSql, extensions);
          return true;
        });
        if (!registered || disposedRef.current || generationRef.current !== generation) return;
        setPhase('ready');
        await runQuery(OBJECT_QUERY_DEFAULT_SQL, generation);
      })();
    },
    [enqueueDb, runQuery, terminateIngestors],
  );

  // teardown は OPFS のファイル、ロック、DuckDB の登録と obj ビューを解放する。各段階を
  // 独立に実行し、どれかが失敗しても作成済みの ingestor の終了まで必ず到達させる。
  const teardown = useCallback(async () => {
    const steps: [string, () => Promise<void>][] = [
      ['cancel the running query', () => enqueueDb(() => latestRef.current.engine.cancelSent())],
      [
        'drop the obj view',
        () => enqueueDb(() => latestRef.current.engine.dropView(latestRef.current.panelId)),
      ],
      [
        'unregister the OPFS files',
        () => enqueueDb(() => latestRef.current.engine.dropFiles(latestRef.current.opfsPaths)),
      ],
    ];
    for (const [label, step] of steps) {
      try {
        await step();
      } catch (err) {
        console.warn(`failed to ${label}`, err);
      }
    }
    await terminateIngestors();
  }, [enqueueDb, terminateIngestors]);

  useEffect(() => {
    const generation = generationRef.current + 1;
    generationRef.current = generation;
    const pendingTeardown = teardownRef.current;
    let cancelled = false;
    void (async () => {
      // 前の世代の teardown の完了を待つ (StrictMode の setup → cleanup → setup の直列化)。
      await pendingTeardown;
      if (cancelled) return;
      disposedRef.current = false;
      startPipeline(generation).catch((err: unknown) => {
        if (isAbortError(err) || generationRef.current !== generation) return;
        setIngestError(err);
        setPhase('error');
      });
    })();
    return () => {
      cancelled = true;
      disposedRef.current = true;
      teardownRef.current = teardownRef.current.then(teardown);
    };
  }, [startPipeline, teardown]);

  const displayError = (err: unknown): Error => {
    if (err instanceof ObjectQueryIngestError) {
      switch (err.kind) {
        case 'unsupported':
          return new Error(t('drawerObjectQuery.unsupported'));
        case 'tooLarge':
          return new Error(
            t('drawerObjectQuery.tooLarge', { limit: formatObjectQueryLimit(maxBytes) }),
          );
        case 'quota':
          return new Error(t('drawerObjectQuery.quotaExceeded'));
        case 'opfs':
          return new Error(`${t('drawerObjectQuery.opfsFailed')}: ${err.message}`);
      }
    }
    return err instanceof Error ? err : new Error(String(err));
  };

  const onRun = () => {
    if (phase !== 'ready' || running) return;
    void runQuery(sql, generationRef.current);
  };

  // グラフの候補列と無効の理由は結果から決まる。Chart を選べない結果 (列が無い、数値列が
  // 無い) では切り替えボタンを無効にし、理由を title に出す。
  const chartCapabilities = useMemo(
    () =>
      result === null
        ? { numericColumns: [], disabledReason: null }
        : queryChartCapabilities(result.columns, result.rows),
    [result],
  );
  const chartDisabledReason = chartCapabilities.disabledReason;
  const chartDisabledTitle =
    chartDisabledReason === null
      ? undefined
      : chartDisabledReason === 'noColumns'
        ? t('drawerObjectQuery.chart.noColumns')
        : t('drawerObjectQuery.chart.noNumericColumns');
  // 上限 (distinct な値の数) はグラフを表示するときにだけ数える。
  const chartTooManyCategories =
    result !== null &&
    resultView === 'chart' &&
    queryChartOverCategoryLimit(result.rows, chartSelection.xIndex);

  const toggleChartY = (index: number) => {
    setChartSelection((prev) => ({
      ...prev,
      yIndexes: prev.yIndexes.includes(index)
        ? prev.yIndexes.filter((i) => i !== index)
        : [...prev.yIndexes, index],
    }));
  };

  const progressLabel =
    progress.total === null
      ? t('drawerObjectQuery.ingestingUnknownTotal', { written: formatBytes(progress.written) })
      : t('drawerObjectQuery.ingesting', {
          written: formatBytes(progress.written),
          total: formatBytes(progress.total),
          percent: progress.total > 0 ? Math.floor((progress.written / progress.total) * 100) : 0,
        });

  // 題名は 1 件ならキー、複数なら件数の文言にする。
  const title =
    files.length === 1
      ? t('drawerObjectQuery.title', { fileName: files[0].key })
      : t('drawerObjectQuery.titleMultiple', { count: files.length });

  return (
    <div className="section">
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
        <h3 style={{ margin: 0 }}>{title}</h3>
        <Button size="sm" style={{ marginLeft: 'auto' }} onClick={onClose}>
          Close
        </Button>
      </div>

      {phase === 'ingesting' && <div style={{ padding: '8px 0' }}>{progressLabel}</div>}

      {phase === 'error' && (
        <>
          <div style={{ padding: '8px 0' }}>{t('drawerObjectQuery.ingestFailed')}</div>
          {isSSOExpiredError(ingestError) && profile ? (
            <SSOExpiredBanner profile={profile} />
          ) : (
            <ErrorBanner error={displayError(ingestError)} />
          )}
        </>
      )}

      {phase === 'ready' && (
        <>
          <div style={{ display: 'flex', gap: 8, margin: '8px 0' }}>
            <Button size="sm" variant="primary" onClick={onRun} disabled={running}>
              {running ? t('drawerObjectQuery.running') : t('drawerObjectQuery.run')}
            </Button>
          </div>
          <SqlEditor value={sql} onChange={setSql} onRun={onRun} />
          {queryError !== null && (
            <>
              <div style={{ padding: '8px 0' }}>{t('drawerObjectQuery.queryFailed')}</div>
              <ErrorBanner error={queryError} />
            </>
          )}
          {result !== null && (
            <>
              {result.truncated && (
                <div className="s3-truncated-notice">
                  {t('drawerObjectQuery.truncated', { max: OBJECT_QUERY_MAX_ROWS })}
                </div>
              )}
              <div className="seg" style={{ width: 200, marginBottom: 8 }}>
                <button
                  className={resultView === 'table' ? 'active' : ''}
                  onClick={() => setResultView('table')}
                >
                  {t('drawerObjectQuery.chart.table')}
                </button>
                <button
                  className={resultView === 'chart' ? 'active' : ''}
                  disabled={chartDisabledReason !== null}
                  title={chartDisabledTitle}
                  onClick={() => setResultView('chart')}
                >
                  {t('drawerObjectQuery.chart.chart')}
                </button>
              </div>
              {resultView === 'table' ? (
                <ResultTable
                  columns={result.columns}
                  rows={result.rows}
                  footerRight={
                    <span>
                      {t('drawerObjectQuery.rowCount', { rows: result.rows.length })}
                      {' · '}
                      {t('drawerObjectQuery.elapsed', { ms: Math.round(result.elapsedMs) })}
                    </span>
                  }
                />
              ) : (
                <>
                  <div
                    style={{
                      display: 'flex',
                      alignItems: 'flex-end',
                      flexWrap: 'wrap',
                      gap: 12,
                      marginBottom: 8,
                    }}
                  >
                    <label style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                      <span className="muted">{t('drawerObjectQuery.chart.xColumn')}</span>
                      <select
                        className="btn sm"
                        value={chartSelection.xIndex}
                        onChange={(e) =>
                          setChartSelection((prev) => ({
                            ...prev,
                            xIndex: Number(e.target.value),
                          }))
                        }
                      >
                        {result.columns.map((c, i) => (
                          <option key={i} value={i}>
                            {c}
                          </option>
                        ))}
                      </select>
                    </label>
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                      <span className="muted">{t('drawerObjectQuery.chart.yColumn')}</span>
                      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 12 }}>
                        {chartCapabilities.numericColumns.map((i) => (
                          <label key={i} style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                            <input
                              type="checkbox"
                              checked={chartSelection.yIndexes.includes(i)}
                              onChange={() => toggleChartY(i)}
                            />
                            {result.columns[i]}
                          </label>
                        ))}
                      </div>
                    </div>
                    <div className="seg" style={{ width: 180 }}>
                      {CHART_TYPE_OPTIONS.map((o) => (
                        <button
                          key={o.value}
                          className={chartType === o.value ? 'active' : ''}
                          onClick={() => setChartType(o.value)}
                        >
                          {t(o.labelKey)}
                        </button>
                      ))}
                    </div>
                  </div>
                  {chartSelection.yIndexes.length === 0 ? (
                    <div className="empty-hint" style={{ height: CHART_HEIGHT }}>
                      {t('drawerObjectQuery.chart.selectY')}
                    </div>
                  ) : chartTooManyCategories ? (
                    <div className="empty-hint" style={{ height: CHART_HEIGHT }}>
                      {t('drawerObjectQuery.chart.tooManyCategories', {
                        max: QUERY_CHART_MAX_CATEGORIES,
                      })}
                    </div>
                  ) : (
                    <QueryResultChart
                      columns={result.columns}
                      rows={result.rows}
                      xIndex={chartSelection.xIndex}
                      yIndexes={chartSelection.yIndexes}
                      type={chartType}
                    />
                  )}
                </>
              )}
            </>
          )}
        </>
      )}
    </div>
  );
}

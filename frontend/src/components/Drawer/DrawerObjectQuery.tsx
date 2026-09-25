// オブジェクト 1 つに対する SQL 検索パネル。DrawerObjectBrowser の一覧を置き換えて表示し、
// Close で一覧に戻る。取り込み (Worker による OPFS への書き込み) とクエリ実行 (DuckDB) は
// props のインターフェース越しに行い、テストではモックを差し込む。
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
import { formatBytes } from '../tables/columns';
import { isSSOExpiredError } from '../../lib/ssoError';
import {
  formatObjectQueryLimit,
  isAbortError,
  ObjectQueryIngestError,
  objectQueryFileName,
  objectQueryFormat,
  objectQueryLockName,
  objectQueryOpfsPath,
  objectQueryReadSql,
  objectQueryStorageName,
  objectQueryExtensions,
  OBJECT_QUERY_DEFAULT_SQL,
  OBJECT_QUERY_MAX_ROWS,
} from '../../lib/objectQuery';
import type {
  ObjectQueryEngine,
  ObjectQueryFormat,
  ObjectQueryResult,
} from '../../lib/objectQuery';
import type { ObjectIngestProgress, ObjectIngestor } from '../../lib/opfsIngest';

export interface DrawerObjectQueryProps {
  // fileName は元オブジェクトのキー (表示と形式判定に使う)。
  fileName: string;
  // url は取り込みに使うダウンロード API の URL。
  url: string;
  // maxBytes は取り込めるサイズ上限 (バイト)。設定 (GET /api/config) の取得完了後に確定する。
  maxBytes: number;
  // size は一覧が持つオブジェクトのサイズ (ブラウザの空き容量チェックに使う)。
  size: number;
  // profile は S3 の SSO 期限切れバナーに使う。GCS には SSO が無いため渡さない。
  profile?: string;
  engine: ObjectQueryEngine;
  createIngestor: () => ObjectIngestor;
  onClose: () => void;
}

// 対象外の形式で開かれた場合の表示。呼び出し側は対象形式の行だけ Query を有効にするため
// 通常は到達しないが、判定が変わっても安全に閉じられるようにしておく。
export function DrawerObjectQuery(props: DrawerObjectQueryProps) {
  const format = useMemo(() => objectQueryFormat(props.fileName), [props.fileName]);
  if (!format) {
    return <ObjectQueryPanel {...props} format={{ extension: '', readFunction: 'read_csv' }} />;
  }
  return <ObjectQueryPanel {...props} format={format} />;
}

interface ObjectQueryPanelProps extends DrawerObjectQueryProps {
  format: ObjectQueryFormat;
}

function ObjectQueryPanel({
  fileName,
  url,
  maxBytes,
  size,
  profile,
  engine,
  createIngestor,
  onClose,
  format,
}: ObjectQueryPanelProps) {
  const { t } = useTranslation('drawerStorage');
  // OPFS 上のファイル名はパネルを開いたときに 1 度だけ決める (再ダウンロードしても同じ名前)。
  const [id] = useState(() => crypto.randomUUID());
  const opfsFileName = objectQueryFileName(id, format);
  const storageName = objectQueryStorageName(opfsFileName);
  const opfsPath = objectQueryOpfsPath(opfsFileName);
  const lockName = objectQueryLockName(opfsFileName);
  const readSql = objectQueryReadSql(opfsPath, format);
  // 読み取り関数が要する DuckDB の拡張 (json / parquet)。csv / tsv は空配列。
  const extensions = useMemo(() => objectQueryExtensions(format), [format]);

  const [phase, setPhase] = useState<'ingesting' | 'ready' | 'error'>('ingesting');
  const [progress, setProgress] = useState<ObjectIngestProgress>({ written: 0, total: null });
  const [ingestError, setIngestError] = useState<unknown>(null);
  const [sql, setSql] = useState(OBJECT_QUERY_DEFAULT_SQL);
  const [result, setResult] = useState<ObjectQueryResult | null>(null);
  const [queryError, setQueryError] = useState<unknown>(null);
  const [running, setRunning] = useState(false);

  // teardown と開始処理は依存を空にした effect から呼ぶため、最新の props を ref で参照する。
  const latestRef = useRef({
    engine,
    url,
    maxBytes,
    size,
    opfsPath,
    lockName,
    readSql,
    extensions,
  });
  latestRef.current = { engine, url, maxBytes, size, opfsPath, lockName, readSql, extensions };
  const storageNameRef = useRef(storageName);
  storageNameRef.current = storageName;
  const createIngestorRef = useRef(createIngestor);
  createIngestorRef.current = createIngestor;

  const ingestorRef = useRef<ObjectIngestor | null>(null);
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
    } catch (err) {
      if (isAbortError(err) || generationRef.current !== generation) return;
      setQueryError(err);
    } finally {
      if (generationRef.current === generation) setRunning(false);
    }
  }, []);

  const startPipeline = useCallback(
    (generation: number) => {
      const ingestor = createIngestorRef.current();
      ingestorRef.current = ingestor;
      return (async () => {
        const { url, maxBytes, size, opfsPath, lockName, readSql, engine, extensions } =
          latestRef.current;
        await ingestor.ingest(
          { url, storageName: storageNameRef.current, lockName, maxBytes, size },
          (p) => {
            if (generationRef.current === generation) setProgress(p);
          },
        );
        if (disposedRef.current || generationRef.current !== generation) return;
        // 登録は解放 (teardown) と直列化する。解放の後に登録が走ると、OPFS のファイルと
        // DuckDB のファイル登録が残る。
        const registered = await enqueueDb(async () => {
          if (disposedRef.current) return false;
          await engine.registerView(opfsPath, readSql, extensions);
          return true;
        });
        if (!registered || disposedRef.current || generationRef.current !== generation) return;
        setPhase('ready');
        await runQuery(OBJECT_QUERY_DEFAULT_SQL, generation);
      })();
    },
    [enqueueDb, runQuery],
  );

  // teardown は OPFS のファイル、ロック、DuckDB の登録と obj ビューを解放する。各段階を
  // 独立に実行し、どれかが失敗しても Worker への終了指示まで必ず到達させる。
  const teardown = useCallback(async () => {
    const steps: [string, () => Promise<void>][] = [
      ['cancel the running query', () => enqueueDb(() => latestRef.current.engine.cancelSent())],
      [
        'drop the obj view',
        () => enqueueDb(() => latestRef.current.engine.dropView(latestRef.current.opfsPath)),
      ],
      [
        'unregister the OPFS file',
        () => enqueueDb(() => latestRef.current.engine.dropFile(latestRef.current.opfsPath)),
      ],
    ];
    for (const [label, step] of steps) {
      try {
        await step();
      } catch (err) {
        console.warn(`failed to ${label}`, err);
      }
    }
    try {
      await ingestorRef.current?.terminate();
    } catch (err) {
      console.warn('failed to terminate the object query worker', err);
    }
    ingestorRef.current = null;
  }, [enqueueDb]);

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

  const progressLabel =
    progress.total === null
      ? t('drawerObjectQuery.ingestingUnknownTotal', { written: formatBytes(progress.written) })
      : t('drawerObjectQuery.ingesting', {
          written: formatBytes(progress.written),
          total: formatBytes(progress.total),
          percent: progress.total > 0 ? Math.floor((progress.written / progress.total) * 100) : 0,
        });

  return (
    <div className="section">
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
        <h3 style={{ margin: 0 }}>{t('drawerObjectQuery.title', { fileName })}</h3>
        <button className="btn sm" style={{ marginLeft: 'auto' }} onClick={onClose}>
          Close
        </button>
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
            <button className="btn sm primary" onClick={onRun} disabled={running}>
              {running ? t('drawerObjectQuery.running') : t('drawerObjectQuery.run')}
            </button>
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
            </>
          )}
        </>
      )}
    </div>
  );
}

// オブジェクト SQL 検索 (S3 / GCS のオブジェクトを DuckDB Wasm に取り込んで SQL を実行する)
// の純関数群。対象拡張子の判定、読み取り SQL の生成、行アクションの活性判定、容量上限の
// 判定を 1 箇所に集約する。DuckDB や OPFS に触れないため単体テストで固定できる。
import i18n from '../i18n';

// OBJECT_QUERY_MAX_ROWS は 1 クエリで表に出す最大行数。これを超えたら残りを読まずに打ち切る。
export const OBJECT_QUERY_MAX_ROWS = 10000;

// OBJECT_QUERY_DEFAULT_SQL は SqlEditor の初期値 (完了後に自動実行するクエリ)。
export const OBJECT_QUERY_DEFAULT_SQL = 'SELECT * FROM obj LIMIT 100';

// OBJECT_QUERY_OPFS_DIR は OPFS 上の作業ディレクトリ。起動時の残骸削除とロック名でも使う。
export const OBJECT_QUERY_OPFS_DIR = 'thief-query';

// OBJECT_QUERY_OPFS_PREFIX は DuckDB に登録する OPFS パスの接頭辞。登録名と SQL 中のパスは
// どちらもこの接頭辞付きの同じ文字列にする。
export const OBJECT_QUERY_OPFS_PREFIX = 'opfs://';

// OBJECT_QUERY_LOCK_PREFIX は Web Locks のロック名の接頭辞。ロック名は
// "<接頭辞><OPFS 上のファイル名>" で、起動時の残骸削除がファイル名から再構成できる。
const OBJECT_QUERY_LOCK_PREFIX = 'thief-query:';

// OBJECT_QUERY_EXTENSION_REPOSITORY は DuckDB に設定する拡張リポジトリのパス。Worker と
// 同一オリジンの public/assets/duckdb-extensions を指す相対 URL で、
// scripts/fetch-duckdb-extensions.mjs が拡張ファイルを置く場所と同じである。
export const OBJECT_QUERY_EXTENSION_REPOSITORY = '/assets/duckdb-extensions';

export type ObjectQueryReadFunction = 'read_csv' | 'read_json_auto' | 'read_parquet';

// ObjectQueryExtension は読み取りに必要になる DuckDB の拡張名。コアの関数 (csv / tsv) は
// 拡張を要しないため、ObjectQueryFormat.duckdbExtension は undefined になる。
export type ObjectQueryExtension = 'json' | 'parquet';

// ObjectQueryFormat は対象拡張子 1 つと DuckDB の読み取り関数の対応。
export interface ObjectQueryFormat {
  // extension は一致した拡張子 (小文字。".csv.gz" のように圧縮の拡張子を含む)。
  extension: string;
  readFunction: ObjectQueryReadFunction;
  // delimiter は read_csv に渡す区切り文字 (TSV のみ)。SQL に埋め込む 2 文字のエスケープ。
  delimiter?: string;
  // duckdbExtension は読み取り関数が属する DuckDB の拡張名。コアの関数は undefined。
  duckdbExtension?: ObjectQueryExtension;
}

// OBJECT_QUERY_FORMATS は対象拡張子と読み取り関数の表。判定は末尾一致で行う
// (".json" と ".jsonl" のように接尾辞が重なるものは、末尾一致では互いに一致しない)。
const OBJECT_QUERY_FORMATS: readonly ObjectQueryFormat[] = [
  { extension: '.csv.gz', readFunction: 'read_csv' },
  { extension: '.csv', readFunction: 'read_csv' },
  // gzip は DuckDB がパスの拡張子から自動判定するため、区切り文字だけを指定する。
  { extension: '.tsv.gz', readFunction: 'read_csv', delimiter: '\\t' },
  { extension: '.tsv', readFunction: 'read_csv', delimiter: '\\t' },
  { extension: '.json.gz', readFunction: 'read_json_auto', duckdbExtension: 'json' },
  { extension: '.json', readFunction: 'read_json_auto', duckdbExtension: 'json' },
  { extension: '.jsonl.gz', readFunction: 'read_json_auto', duckdbExtension: 'json' },
  { extension: '.jsonl', readFunction: 'read_json_auto', duckdbExtension: 'json' },
  { extension: '.ndjson.gz', readFunction: 'read_json_auto', duckdbExtension: 'json' },
  { extension: '.ndjson', readFunction: 'read_json_auto', duckdbExtension: 'json' },
  { extension: '.parquet', readFunction: 'read_parquet', duckdbExtension: 'parquet' },
];

// objectQueryFormat はオブジェクトのキーから対象形式を判定する。大文字小文字は区別しない。
// 対象外の拡張子は undefined を返す。
export function objectQueryFormat(key: string): ObjectQueryFormat | undefined {
  const lower = key.toLowerCase();
  return OBJECT_QUERY_FORMATS.find((f) => lower.endsWith(f.extension));
}

// objectQueryExtensions は format の読み取りに要する DuckDB 拡張名を返す。コアの関数 (csv /
// tsv) は空配列で、拡張を読み込まない。json 系は json、parquet は parquet。
export function objectQueryExtensions(format: ObjectQueryFormat): ObjectQueryExtension[] {
  return format.duckdbExtension === undefined ? [] : [format.duckdbExtension];
}

// objectQueryExtensionInitSql は DuckDB の初期化時に実行する SET 文を返す。autoinstall と
// autoload を止めて extensions.duckdb.org への取得を起こさず、拡張は同一オリジンのリポジトリ
// (OBJECT_QUERY_EXTENSION_REPOSITORY) からだけ読む。
export function objectQueryExtensionInitSql(): string[] {
  return [
    'SET autoinstall_known_extensions = false',
    'SET autoload_known_extensions = false',
    `SET custom_extension_repository = '${OBJECT_QUERY_EXTENSION_REPOSITORY}'`,
  ];
}

// objectQueryInstallSql は拡張を同一オリジンのリポジトリから読み込む SQL を組み立てる。
// 初期化で autoinstall / autoload を止めているため、読み取りの前に明示的に INSTALL と LOAD を
// 行う (LOAD だけではインストール済みの拡張しか読めない)。
export function objectQueryInstallSql(extension: ObjectQueryExtension): string {
  return `INSTALL ${extension}; LOAD ${extension};`;
}

// objectQueryReadSql は DuckDB の読み取り関数呼び出しを組み立てる。
// 例: read_csv('opfs://thief-query/<uuid>.csv') / read_csv('opfs://...', delim='\t')
export function objectQueryReadSql(opfsPath: string, format: ObjectQueryFormat): string {
  // パスは自前で生成する uuid と拡張子だけだが、SQL 文字列として安全にするため
  // 単一引用符は二重化する (DuckDB の文字列リテラルのエスケープ規則)。
  const path = `'${opfsPath.replace(/'/g, "''")}'`;
  if (format.delimiter !== undefined) {
    return `${format.readFunction}(${path}, delim='${format.delimiter}')`;
  }
  return `${format.readFunction}(${path})`;
}

// objectQueryFileName は OPFS に置くファイル名 (ディレクトリを含まない) を組み立てる。
// 拡張子は元オブジェクトの一致した拡張子を保つ (DuckDB が形式と圧縮を判定するため)。
export function objectQueryFileName(id: string, format: ObjectQueryFormat): string {
  return `${id}${format.extension}`;
}

// objectQueryStorageName は OPFS ルートからの相対パスを返す (例: "thief-query/<uuid>.csv")。
export function objectQueryStorageName(fileName: string): string {
  return `${OBJECT_QUERY_OPFS_DIR}/${fileName}`;
}

// objectQueryOpfsPath は DuckDB に登録する OPFS パスを返す。
// registerOPFSFileName に渡す名前と SQL 中のパスはこの 1 つの文字列を共有する。
export function objectQueryOpfsPath(fileName: string): string {
  return `${OBJECT_QUERY_OPFS_PREFIX}${objectQueryStorageName(fileName)}`;
}

// objectQueryLockName は OPFS 上のファイル名に対応する Web Locks のロック名を返す。
// 起動時の残骸削除はディレクトリのエントリ名からこの関数でロック名を再構成する。
export function objectQueryLockName(fileName: string): string {
  return `${OBJECT_QUERY_LOCK_PREFIX}${fileName}`;
}

// ObjectQueryConfigState は上限設定 (GET /api/config) の取得状態。
export type ObjectQueryConfigState =
  { status: 'loading' } | { status: 'error' } | { status: 'ready'; maxBytes: number };

// isObjectQuerySupported はメインスレッドで判定できる対応ブラウザの機能がそろっているかを返す。
// 判定するのは OPFS (navigator.storage.getDirectory) と Web Locks (navigator.locks) の 2 つ。
// createSyncAccessHandle は Worker 専用 API でメインスレッドには無いため判定せず、Worker が
// 取り込みの開始時に確認する。
export function isObjectQuerySupported(): boolean {
  return (
    typeof navigator !== 'undefined' &&
    'storage' in navigator &&
    'locks' in navigator &&
    'getDirectory' in navigator.storage
  );
}

// objectQueryOverLimit は size が上限を超えているかを返す。上限ちょうどは許容する。
export function objectQueryOverLimit(size: number, maxBytes: number): boolean {
  return size > maxBytes;
}

// objectQueryQuotaExceeded はブラウザの残り容量 (quota - usage) が取り込むサイズに足りないかを
// 返す。取り込みを始める前の判定に使う。
export function objectQueryQuotaExceeded(quota: number, usage: number, size: number): boolean {
  return quota - usage < size;
}

// formatObjectQueryLimit は上限 (バイト) を i18n の文言へ埋め込む表示用の文字列に変換する。
// バイト単位は 1024 進で丸める (components/tables/columns.tsx の formatBytes と同じ考え方。
// lib から component へ依存しないよう、ここに独立して置く)。
export function formatObjectQueryLimit(maxBytes: number): string {
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let value = maxBytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${unit === 0 ? String(value) : value.toFixed(1)} ${units[unit]}`;
}

// objectQueryDisabledReason は Query ボタンが押せない理由の文言を返す (押せる場合は空文字)。
// 判定の順序はブラウザ非対応 → 設定の取得失敗 → 設定の取得中 → 対象外の拡張子 → 上限超過。
// 設定の取得中と取得失敗で押せなくするのは、上限が分からない状態で既定値により判定すると
// backend の設定と食い違う判定になるためである。
export function objectQueryDisabledReason(
  key: string,
  size: number,
  config: ObjectQueryConfigState,
  browserSupported: boolean,
): string {
  if (!browserSupported) {
    return i18n.t('drawerStorage:objectQuery.browserUnsupported');
  }
  if (config.status === 'error') {
    return i18n.t('drawerStorage:objectQuery.configFailed');
  }
  if (config.status === 'loading') {
    return i18n.t('drawerStorage:objectQuery.configLoading');
  }
  if (!objectQueryFormat(key)) {
    return i18n.t('drawerStorage:objectQuery.unsupportedFormat');
  }
  if (objectQueryOverLimit(size, config.maxBytes)) {
    return i18n.t('drawerStorage:objectQuery.tooLarge', {
      limit: formatObjectQueryLimit(config.maxBytes),
    });
  }
  return '';
}

// ObjectQueryResult は 1 回のクエリ実行の結果。
export interface ObjectQueryResult {
  columns: string[];
  rows: string[][];
  // truncated は OBJECT_QUERY_MAX_ROWS に達して残りを読まずに打ち切ったことを示す。
  truncated: boolean;
  elapsedMs: number;
}

// ObjectQueryEngine は DuckDB 側の操作。DrawerObjectQuery はこのインターフェース越しに
// エンジンを呼び、テストではモックを差し込む。
export interface ObjectQueryEngine {
  // registerView は OPFS のファイルを DuckDB に登録し、extensions を INSTALL / LOAD してから
  // obj ビューを作成する。extensions は objectQueryExtensions が返す読み取り関数の拡張。
  registerView(
    opfsPath: string,
    readSql: string,
    extensions: ObjectQueryExtension[],
  ): Promise<void>;
  // run は SQL をストリーム実行し、OBJECT_QUERY_MAX_ROWS 行で打ち切って結果を返す。
  // 打ち切った場合はエンジン側で cancelSent する。
  run(sql: string): Promise<ObjectQueryResult>;
  // cancelSent は実行中のクエリを打ち切る (DB 未初期化なら何もしない)。
  cancelSent(): Promise<void>;
  // dropView は opfsPath に対して作られた obj ビューを削除する (未作成でも失敗しない)。
  // obj という名前は 1 つしかないため、別のオブジェクトの登録が始まっていたら何もしない
  // (閉じたパネルの解放が、後から開いたパネルのビューを消さないようにする)。
  dropView(opfsPath: string): Promise<void>;
  // dropFile は DuckDB のファイル登録を解除する (未登録でも失敗しない)。
  dropFile(opfsPath: string): Promise<void>;
}

// ObjectQueryIngestErrorKind はブラウザ側 (Worker / OPFS / 容量) 由来の取り込みエラーの種別。
export type ObjectQueryIngestErrorKind = 'unsupported' | 'tooLarge' | 'quota' | 'opfs';

// ObjectQueryIngestError は ApiError に正規化できない取り込みエラー。表示する文言は
// 呼び出し側が kind から選ぶ (元の message は英語の診断用)。
export class ObjectQueryIngestError extends Error {
  readonly kind: ObjectQueryIngestErrorKind;

  constructor(kind: ObjectQueryIngestErrorKind, message: string) {
    super(message);
    this.name = 'ObjectQueryIngestError';
    this.kind = kind;
  }
}

// isAbortError はアンマウント由来の中断かを判定する。中断はエラーとして表示しない。
export function isAbortError(err: unknown): boolean {
  return err instanceof Error && err.name === 'AbortError';
}

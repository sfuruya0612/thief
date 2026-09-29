// Vite dev server のロガーから @duckdb/duckdb-wasm の eh worker の sourcemap 警告だけを落とす。
//
// dev server は worker ファイルを配信するときに末尾の sourceMappingURL を読み、sourcemap の
// sources の各要素をパッケージのルート (node_modules/@duckdb/duckdb-wasm) と突き合わせる。
// worker の sourcemap は同梱の @duckdb/apache-arrow のソース (パッケージ外) を 117 個指し、
// その 1 つごとに「points to a source file outside its package」の警告が出る (warnOnce は
// メッセージの完全一致でしか抑えないため、パスごとに 1 行出る)。警告が示すのは worker の
// sourcemap に arrow のソース本文が欠けることだけで、worker の実行には影響しない。
// vite build では worker は dist/assets へそのまま複製され、この警告は出ない (dev server
// だけの事象)。
//
// 抑止の条件は 3 つの部分文字列をすべて含むメッセージに限る。1 つでも欠ける同種の警告
// (他の worker ファイル、@duckdb/apache-arrow 以外のパス) は従来どおり出す。Vite は
// メッセージを picocolors で色付け (ANSI エスケープ) して渡すため、判定は部分文字列の
// 包含で行う (先頭一致や完全一致にしない)。

// OUTSIDE_PACKAGE_MESSAGE は Vite が sourcemap のパッケージ外参照に使う文言。
const OUTSIDE_PACKAGE_MESSAGE = 'points to a source file outside its package';
// DUCKDB_WORKER_PATH は抑止の対象とする worker の配信パス (先頭の / を含む)。
const DUCKDB_WORKER_PATH = '/@duckdb/duckdb-wasm/dist/duckdb-browser-eh.worker.js';
// DUCKDB_ARROW_PATH は worker の sourcemap が指す同梱 arrow のソースの配信パス。
const DUCKDB_ARROW_PATH = '/@duckdb/apache-arrow/';

// isSuppressedViteWarning は msg が抑止対象の sourcemap 警告かを返す。
export function isSuppressedViteWarning(msg: string): boolean {
  return (
    msg.includes(OUTSIDE_PACKAGE_MESSAGE) &&
    msg.includes(DUCKDB_WORKER_PATH) &&
    msg.includes(DUCKDB_ARROW_PATH)
  );
}

// WarnLogger は warn と warnOnce を持つロガーの構造的な型。viteLogger は src の型検査と
// vitest だけのスコープに収めるため、src/ から vite の型を import せず、必要なメソッドだけを
// 構造で受ける。
interface WarnLogger {
  warn(msg: string, options?: unknown): void;
  warnOnce(msg: string, options?: unknown): void;
}

// withSuppressedWarnings は logger の warn と warnOnce を差し替え、isSuppressed が真を返す
// メッセージを落とす。Vite の createLogger の warnOnce は内部の出力関数を直接呼び warn を
// 経由しないため、warn だけでなく warnOnce も差し替える。差し替えは渡されたオブジェクトの
// メソッドを書き換え、同じオブジェクトを返す (スプレッドで写しを作ると、元の warn が
// hasWarned を元のオブジェクトに書き、Vite が写しから読む値と食い違う)。
export function withSuppressedWarnings<T extends WarnLogger>(
  logger: T,
  isSuppressed: (msg: string) => boolean,
): T {
  const target: WarnLogger = logger;
  const warn = target.warn.bind(target);
  const warnOnce = target.warnOnce.bind(target);
  target.warn = (msg, options) => {
    if (isSuppressed(msg)) return;
    warn(msg, options);
  };
  target.warnOnce = (msg, options) => {
    if (isSuppressed(msg)) return;
    warnOnce(msg, options);
  };
  return logger;
}

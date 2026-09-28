// DuckDB Wasm の初期化と OPFS のファイル登録。AsyncDuckDB は遅延初期化して 1 つの
// インスタンスを共有し、DrawerObjectQuery を閉じても破棄しない (Wasm の読み込みは重い)。
//
// Wasm と Worker は同一オリジンから配信する。@duckdb/duckdb-wasm の dist を Vite の
// ?url import で取り込み、ビルド成果物 (dist/) に含める。CDN (jsDelivr) の
// getJsDelivrBundles() は使わない (実行時の外部ホスト依存を増やさない)。
// COI バンドルは SharedArrayBuffer を使うため COOP / COEP ヘッダが要るので使わない。
//
// 使うのは eh バンドル (duckdb-eh.wasm + duckdb-browser-eh.worker.js)。csv / tsv の読み取り
// 関数は Wasm に静的リンクされているが、json と parquet の機能は別ファイルの拡張
// (json.duckdb_extension.wasm / parquet.duckdb_extension.wasm) で、初回使用時に取得される。
// そのため初期化で autoinstall / autoload を止め、同一オリジンの
// public/assets/duckdb-extensions (scripts/fetch-duckdb-extensions.mjs が配置する) を
// custom_extension_repository に指定する。extensions.duckdb.org へは取得に行かない。
// 拡張は registerView が読み取りの直前に INSTALL / LOAD する (csv / tsv は読み込まない)。
//
// 対応ブラウザ: 必要な機能は次の 4 つ。値は 2026-09-25 に mdn/browser-compat-data で確認した。
// | 機能 | Chrome | Firefox | Safari |
// | --- | --- | --- | --- |
// | StorageManager.getDirectory (OPFS) | 86 | 111 | 15.2 |
// | FileSystemFileHandle.createSyncAccessHandle (Worker 内) | 102 | 111 | 15.2 |
// | Web Locks (navigator.locks) | 69 | 96 | 15.4 |
// | Wasm の例外処理 (eh バンドル) | 95 | 100 | 15.2 |
// 全機能がそろう最低バージョンは Chrome 102 / Firefox 111 / Safari 15.4。メインスレッドで
// 検出するのは OPFS と Web Locks の 2 つ (objectQuery.ts の isObjectQuerySupported) で、
// createSyncAccessHandle は Worker 専用 API のため Worker が取り込みの開始時に確認する。
import * as duckdb from '@duckdb/duckdb-wasm';
import duckdbEhWasmUrl from '@duckdb/duckdb-wasm/dist/duckdb-eh.wasm?url';
import duckdbEhWorkerUrl from '@duckdb/duckdb-wasm/dist/duckdb-browser-eh.worker.js?url';
import { collectQueryResult } from './duckdbResult';
import {
  objectQueryExtensionInitSql,
  objectQueryInstallSql,
  OBJECT_QUERY_MAX_ROWS,
} from './objectQuery';
import type { ObjectQueryEngine, ObjectQueryExtension, ObjectQueryResult } from './objectQuery';

// 接続は 1 つだけ作って共有し、ビューの名前も obj の 1 つだけを使う。そのため、パネルを閉じて
// すぐ別のオブジェクトを開くと、古いパネルの解放が新しいパネルのビューを消しうる。これは
// registeredViewPath で防ぐ。
let dbPromise: Promise<duckdb.AsyncDuckDB> | null = null;
let connectionPromise: Promise<duckdb.AsyncDuckDBConnection> | null = null;
// registeredViewPath は obj ビューが今どの OPFS パスに対して作られているか。ビューの名前は
// obj の 1 つだけなので、パネルを閉じてすぐ別のオブジェクトを開くと、閉じた側の解放が開いた
// 側のビューを消しうる。解放はこの値と一致するときだけ行う。
let registeredViewPath: string | null = null;

async function instantiateDuckDB(): Promise<duckdb.AsyncDuckDB> {
  const logger = new duckdb.ConsoleLogger(duckdb.LogLevel.WARNING);
  // Worker はクラシックワーカーとして生成する (パッケージの Worker スクリプトは IIFE)。
  const worker = new Worker(duckdbEhWorkerUrl);
  const db = new duckdb.AsyncDuckDB(logger, worker);
  try {
    await db.instantiate(duckdbEhWasmUrl, null);
  } catch (err: unknown) {
    // instantiate は失敗しても Worker を片付けないため、ここで終了させる (再試行のたびに
    // Worker が増えることを防ぐ)。後始末の失敗で元のエラーを隠さない。
    await db.terminate().catch((terminateErr: unknown) => {
      console.warn('failed to terminate duckdb worker after instantiate failure', terminateErr);
    });
    throw err;
  }
  // 拡張の取得先を同一オリジンに固定する (SET は DB 全体の設定のため、専用の接続で 1 度だけ
  // 実行して閉じる)。SET に失敗した状態で検索を続けると extensions.duckdb.org へ取得に
  // 行きうるため、エラーは握り潰さずに呼び出し元 (getDuckDB) へ伝播させて取り込みエラーに
  // する。初期化そのものが失敗した場合は、開いているパネルの ErrorBanner に出る。
  const conn = await db.connect();
  try {
    for (const sql of objectQueryExtensionInitSql()) {
      await conn.query(sql);
    }
  } finally {
    await conn.close();
  }
  return db;
}

// getDuckDB は共有の AsyncDuckDB を返す。初回呼び出しで Wasm の読み込みと初期化を行う。
// 初期化に失敗した Promise を保持し続けるとパネルを開き直しても再試行できないため、
// 失敗したら保持を解いて次の呼び出しでやり直せるようにする。
export function getDuckDB(): Promise<duckdb.AsyncDuckDB> {
  dbPromise ??= instantiateDuckDB().catch((err: unknown) => {
    dbPromise = null;
    throw err;
  });
  return dbPromise;
}

// initializedDuckDB は初期化済み (または初期化中) の AsyncDuckDB を返す。未初期化なら null。
// 解放系の処理で DuckDB を新たに初期化しないために使う (取り込み前のアンマウントで
// Wasm の読み込みを始めてしまわない)。
function initializedDuckDB(): Promise<duckdb.AsyncDuckDB> | null {
  return dbPromise;
}

async function getConnection(): Promise<duckdb.AsyncDuckDBConnection> {
  connectionPromise ??= getDuckDB()
    .then((db) => db.connect())
    .catch((err: unknown) => {
      connectionPromise = null;
      throw err;
    });
  return connectionPromise;
}

// runQuery は SQL をストリーム実行し、OBJECT_QUERY_MAX_ROWS 行で打ち切る。行の収集と
// 打ち切りの判定は duckdbResult.ts の collectQueryResult が行う (単体テストで固定する)。
// send() が返すリーダーは open() されておらず schema を持たない。open() も collectQueryResult
// が行う (docs/issues/closed/0206)。
async function runQuery(sql: string): Promise<ObjectQueryResult> {
  const conn = await getConnection();
  const startedAt = performance.now();
  const reader = await conn.send(sql);
  const collected = await collectQueryResult(reader, OBJECT_QUERY_MAX_ROWS, async () => {
    await conn.cancelSent();
  });
  return { ...collected, elapsedMs: performance.now() - startedAt };
}

// objectQueryEngine は DrawerObjectQuery が使う共有エンジン。
export const objectQueryEngine: ObjectQueryEngine = {
  async registerView(
    opfsPath: string,
    readSql: string,
    extensions: ObjectQueryExtension[],
  ): Promise<void> {
    // パスの記録は登録の開始時に行う。CREATE の応答を待ってから記録すると、その往復の間に
    // 別のパネルの解放が「まだ自分のビューだ」と判定して DROP VIEW を出し、作られた直後の
    // ビューを消す。登録に失敗した場合はビューが無いままこの値が残るが、解放の DROP VIEW は
    // IF EXISTS なので害は無い。
    registeredViewPath = opfsPath;
    const db = await getDuckDB();
    await db.registerOPFSFileName(opfsPath);
    const conn = await getConnection();
    // 読み取り関数が拡張を要する形式 (json / parquet) だけ、CREATE VIEW の直前に INSTALL と
    // LOAD を行う。INSTALL は custom_extension_repository (同一オリジン) から取得する。
    // 取得できない場合は DuckDB のエラーがそのまま ErrorBanner に出る。
    for (const extension of extensions) {
      await conn.query(objectQueryInstallSql(extension));
    }
    // 解放が届く前に次の登録が走ることがあるため、既存のビューは置き換える。
    await conn.query(`CREATE OR REPLACE VIEW obj AS SELECT * FROM ${readSql}`);
  },

  run: runQuery,

  async cancelSent(): Promise<void> {
    if (!initializedDuckDB()) return;
    const conn = await getConnection();
    await conn.cancelSent();
  },

  async dropView(opfsPath: string): Promise<void> {
    if (!initializedDuckDB()) return;
    // 別のオブジェクトのビューに置き換わっていたら、その持ち主の解放に任せる。
    if (registeredViewPath !== opfsPath) return;
    registeredViewPath = null;
    const conn = await getConnection();
    await conn.query('DROP VIEW IF EXISTS obj');
  },

  async dropFile(opfsPath: string): Promise<void> {
    const db = initializedDuckDB();
    if (!db) return;
    await (await db).dropFile(opfsPath);
  },
};

#!/usr/bin/env node
// DuckDB Wasm の json / parquet 拡張をダウンロードし、同一オリジン配信のために
// public/assets/duckdb-extensions/<REVISION>/wasm_eh/ へ配置するセットアップスクリプト。
//
// 使い方: node scripts/fetch-duckdb-extensions.mjs
//
// 拡張は @duckdb/duckdb-wasm の Wasm (duckdb-eh.wasm) とコアのリビジョンが一致している
// 必要がある。取得先のリビジョンとファイルの SHA-256 を固定し、package.json の version が
// 対応する版と違えば書き出さずにエラーで終了する (版の上げ忘れと取得先の差し替え・破損の検出)。

import { createHash } from 'node:crypto';
import { mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const SCRIPT_DIR = dirname(fileURLToPath(import.meta.url));
const FRONTEND_DIR = join(SCRIPT_DIR, '..');
const OUTPUT_DIR = join(FRONTEND_DIR, 'public', 'assets', 'duckdb-extensions');

// DUCKDB_WASM_VERSION はこの拡張ファイルが対応する @duckdb/duckdb-wasm の版。
const DUCKDB_WASM_VERSION = '1.32.0';

// DUCKDB_EXTENSION_REVISION は DuckDB コアのリビジョン。1.32.0 の duckdb-eh.wasm は
// コア v1.4.3 で、拡張は https://extensions.duckdb.org/v1.4.3/wasm_eh/ に置かれている。
const DUCKDB_EXTENSION_REVISION = 'v1.4.3';

// DUCKDB_EXTENSION_PLATFORM は Wasm のバンドル名 (eh バンドルを使うため wasm_eh)。
const DUCKDB_EXTENSION_PLATFORM = 'wasm_eh';

// EXTENSIONS は拡張名 -> SHA-256。ハッシュは 2026-09-25 に取得したファイルの値。
const EXTENSIONS = [
  { name: 'json', sha256: 'b997276c8e15cc3ebdeda340d73d15dc1c4f4755ad281280451cb0a2f79302e9' },
  { name: 'parquet', sha256: '22765c8f7dc741cda2b571a66ac7bb355295d7d69a6c37e5315b265672984f55' },
];

function sha256(buffer) {
  return createHash('sha256').update(buffer).digest('hex');
}

// assertWasmVersion は node_modules の @duckdb/duckdb-wasm が対応する版かを確認する。
function assertWasmVersion() {
  const pkgPath = join(FRONTEND_DIR, 'node_modules', '@duckdb', 'duckdb-wasm', 'package.json');
  let version;
  try {
    version = JSON.parse(readFileSync(pkgPath, 'utf8')).version;
  } catch (err) {
    console.error(`failed to read ${pkgPath}: run "npm install" in frontend/ first`);
    process.exit(1);
  }
  if (version !== DUCKDB_WASM_VERSION) {
    console.error(
      `@duckdb/duckdb-wasm is ${version}, but these extensions are for ${DUCKDB_WASM_VERSION}`,
    );
    console.error(
      'update DUCKDB_WASM_VERSION, DUCKDB_EXTENSION_REVISION and the SHA-256 values in this script',
    );
    process.exit(1);
  }
}

async function fetchExtension(name) {
  const url = `https://extensions.duckdb.org/${DUCKDB_EXTENSION_REVISION}/${DUCKDB_EXTENSION_PLATFORM}/${name}.duckdb_extension.wasm`;
  const res = await fetch(url);
  if (!res.ok) {
    throw new Error(`GET ${url} returned ${res.status}`);
  }
  return new Uint8Array(await res.arrayBuffer());
}

async function main() {
  assertWasmVersion();

  // すべてのダウンロードと検証が通ってから書き出す (途中で失敗しても半端なファイルを残さない)。
  const outputs = [];
  for (const ext of EXTENSIONS) {
    const filename = `${ext.name}.duckdb_extension.wasm`;
    const body = await fetchExtension(ext.name);
    const digest = sha256(body);
    if (digest !== ext.sha256) {
      console.error(`SHA-256 mismatch for ${ext.name}: expected ${ext.sha256}, got ${digest}`);
      process.exit(1);
    }
    outputs.push([
      join(OUTPUT_DIR, DUCKDB_EXTENSION_REVISION, DUCKDB_EXTENSION_PLATFORM, filename),
      body,
    ]);
    console.log(`ok: ${ext.name}.duckdb_extension.wasm (${body.byteLength} bytes)`);
  }

  // 出力先は .gitignore 対象のため、初回セットアップ時は存在しない。
  for (const [path, body] of outputs) {
    mkdirSync(dirname(path), { recursive: true });
    writeFileSync(path, body);
  }
  removeOtherRevisions(OUTPUT_DIR, DUCKDB_EXTENSION_REVISION);
}

// removeOtherRevisions は outputDir から current 以外のエントリを削除する (ディレクトリと
// ファイルを区別しない。出力先はこのスクリプトが作るリビジョンのディレクトリだけを持つ)。
// 版を上げたときに古いバイナリが public/ とビルド成果物 (dist/) に残り続けるのを防ぐ。
// 削除を伴うためテストから直接呼べるよう、対象ディレクトリと残すリビジョンを引数で受け取る。
export function removeOtherRevisions(outputDir, current) {
  let entries;
  try {
    entries = readdirSync(outputDir, { withFileTypes: true });
  } catch {
    return;
  }
  for (const entry of entries) {
    if (entry.name === current) continue;
    rmSync(join(outputDir, entry.name), { recursive: true, force: true });
    console.log(`removed stale revision: ${entry.name}`);
  }
}

// テストからの import で取得を始めないよう、直接実行されたときだけ main を呼ぶ。
if (process.argv[1] === fileURLToPath(import.meta.url)) {
  main().catch((err) => {
    console.error(`failed to fetch DuckDB extensions: ${err.message}`);
    process.exit(1);
  });
}

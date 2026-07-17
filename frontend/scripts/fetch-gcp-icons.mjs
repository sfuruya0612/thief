#!/usr/bin/env node
// Google Cloud 公式アイコンパッケージ (zip) から未改変の SVG を抽出し、
// public/assets/gcp-icons/<service>.svg に配置するセットアップスクリプト。
//
// 使い方: node scripts/fetch-gcp-icons.mjs <path-to-gcp-icons.zip>
//
// Google Cloud の公式アイコンパッケージ (Unique Icons を含む zip) の構成に対応するため、
// ファイル名一致で検索する (ディレクトリ階層には依存しない)。

import { execFileSync } from 'node:child_process';
import { mkdtempSync, rmSync, readdirSync, statSync, copyFileSync, mkdirSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, dirname, basename } from 'node:path';
import { fileURLToPath } from 'node:url';

const SCRIPT_DIR = dirname(fileURLToPath(import.meta.url));
const OUTPUT_DIR = join(SCRIPT_DIR, '..', 'public', 'assets', 'gcp-icons');

// サービスキー -> Google Cloud 公式パッケージ内のファイル名 (Unique Icons/<Product>/SVG/*.svg)
const ICON_FILENAMES = {
  cloudrun: 'CloudRun-512-color-rgb.svg',
  bigquery: 'BigQuery-512-color.svg',
  gcs: 'Cloud_Storage-512-color.svg',
  // 実ファイル名は Google Cloud 公式アイコンパッケージ (Unique Icons) の命名規則からの
  // 推測であり、このセッションでは zip が入手できず実際の展開・検証を行えていない。
  // mise run frontend:fetch-gcp-icons 実行時にファイル名不一致で失敗した場合は、
  // パッケージ内の実際のファイル名に合わせて修正すること。
  cloudlogging: 'Cloud_Logging-512-color.svg',
};

function extractZip(zipPath, destDir) {
  execFileSync('unzip', ['-q', '-o', zipPath, '-d', destDir]);
}

function extractNestedZips(dir) {
  for (const entry of walk(dir)) {
    if (entry.toLowerCase().endsWith('.zip')) {
      extractZip(entry, dirname(entry));
    }
  }
}

function* walk(dir) {
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    const st = statSync(full);
    if (st.isDirectory()) {
      yield* walk(full);
    } else {
      yield full;
    }
  }
}

function buildFilenameIndex(dir) {
  const index = new Map();
  for (const full of walk(dir)) {
    const name = basename(full);
    if (!index.has(name)) {
      index.set(name, full);
    }
  }
  return index;
}

function main() {
  const zipPath = process.argv[2];
  if (!zipPath) {
    console.error('使い方: node scripts/fetch-gcp-icons.mjs <path-to-gcp-icons.zip>');
    process.exit(1);
  }

  const tmpDir = mkdtempSync(join(tmpdir(), 'gcp-icons-'));
  try {
    extractZip(zipPath, tmpDir);
    extractNestedZips(tmpDir);

    const index = buildFilenameIndex(tmpDir);
    const missing = [];

    // 出力先は .gitignore 対象のため、初回セットアップ時は存在しない。
    mkdirSync(OUTPUT_DIR, { recursive: true });

    for (const [key, filename] of Object.entries(ICON_FILENAMES)) {
      const src = index.get(filename);
      if (!src) {
        missing.push(`${key} (${filename})`);
        continue;
      }
      copyFileSync(src, join(OUTPUT_DIR, `${key}.svg`));
      console.log(`ok: ${key}.svg <- ${filename}`);
    }

    if (missing.length > 0) {
      console.error(`見つからなかったアイコン: ${missing.join(', ')}`);
      process.exit(1);
    }
  } finally {
    rmSync(tmpDir, { recursive: true, force: true });
  }
}

main();

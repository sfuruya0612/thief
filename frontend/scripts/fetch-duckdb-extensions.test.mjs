// 取得スクリプトのうち、削除を伴う removeOtherRevisions だけを一時ディレクトリで固定する。
// 取得本体 (ネットワークと SHA-256 の検証) はテストの対象にしない。
import { afterEach, describe, expect, it } from 'vitest';
import { existsSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { removeOtherRevisions } from './fetch-duckdb-extensions.mjs';

let dir = null;

afterEach(() => {
  if (dir) rmSync(dir, { recursive: true, force: true });
  dir = null;
});

function makeRevision(root, revision) {
  const path = join(root, revision, 'wasm_eh');
  mkdirSync(path, { recursive: true });
  writeFileSync(join(path, 'json.duckdb_extension.wasm'), 'stub');
}

describe('removeOtherRevisions', () => {
  it('現行のリビジョンを残し、それ以外のディレクトリを削除する', () => {
    dir = mkdtempSync(join(tmpdir(), 'duckdb-ext-'));
    makeRevision(dir, 'v1.4.3');
    makeRevision(dir, 'v1.3.2');
    makeRevision(dir, 'v1.2.0');

    removeOtherRevisions(dir, 'v1.4.3');

    expect(existsSync(join(dir, 'v1.4.3', 'wasm_eh', 'json.duckdb_extension.wasm'))).toBe(true);
    expect(existsSync(join(dir, 'v1.3.2'))).toBe(false);
    expect(existsSync(join(dir, 'v1.2.0'))).toBe(false);
  });

  it('出力先が存在しない場合は何もしない', () => {
    expect(() =>
      removeOtherRevisions(join(tmpdir(), 'duckdb-ext-missing'), 'v1.4.3'),
    ).not.toThrow();
  });
});

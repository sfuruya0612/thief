/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 8088,
    host: 'localhost',
    strictPort: true,
  },
  preview: {
    port: 8088,
    host: 'localhost',
    strictPort: true,
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/setupTests.ts'],
    // Vitest は Node 条件でパッケージを解決するため、@duckdb/duckdb-wasm の bare import が
    // Node 用ビルド (dist/duckdb-node.cjs) に解決され、Worker の生成に失敗する。jsdom の
    // テストではブラウザ用ビルドを使う。サブパスの import (dist/*.wasm?url など) は
    // 正規表現で一致させない (文字列のエイリアスは前方一致するため)。
    alias: [
      {
        find: /^@duckdb\/duckdb-wasm$/,
        replacement: '@duckdb/duckdb-wasm/dist/duckdb-browser.mjs',
      },
    ],
    // 既定の `pool: 'forks'` ではテストファイルごとに jsdom を作り直すため、実行時間の 74% を
    // その生成が占めていた (95 ファイル 987 テストで 71.98 秒)。`vmThreads` はワーカーごとに
    // VM コンテキストを用意しつつファイルごとの分離を保ち、同じ 987 テストが 9.58 秒 (再実行時
    // 8.82 秒) ですべて成功した。もう一方の候補である `isolate: false` は 18.33 秒と遅いうえ、
    // 実行順に依存して 10〜11 ファイル 71 テストが失敗するため採らない。なお `isolate` は
    // vmThreads プールでは効果が無いため、ここでは指定しない。
    pool: 'vmThreads',
    // vitest は既定で CSS の import を空文字列に差し替える。その判定は `app.css?raw` のような
    // raw import にも一致するため、規則のテキストを検証するテスト (src/app.css.test.ts) が
    // 中身を読めない。raw import は文字列でありスタイルを注入しないので、そこだけ通す。
    css: { include: [/\.css\?raw$/] },
  },
});

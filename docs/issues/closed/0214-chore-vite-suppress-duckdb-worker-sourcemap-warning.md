# Vite dev server で duckdb-wasm の worker の sourcemap 警告を抑止する

Created: 2026-09-29
Model: Claude Fable 5.1
Completed: 2026-09-29

## 背景

`docs/issues/TODO.md` の「frontend の dev server (mise run frontend:run) で duckdb-wasm の worker の sourcemap 警告 (points to a source file outside its package) がコンソールに出続けるので抑止したい」に対応する。

`frontend/src/lib/duckdb.ts` は DuckDB Wasm の worker を `@duckdb/duckdb-wasm/dist/duckdb-browser-eh.worker.js?url` で import し (issue 0196)、`new Worker(duckdbEhWorkerUrl)` で起動する。
Vite 8.3.0 の dev server はこの worker ファイルを配信するときに `loadAndTransform` (`frontend/node_modules/vite/dist/node/chunks/node.js` の 19573 行) で読み込み、末尾の `//# sourceMappingURL=duckdb-browser-eh.worker.js.map` に従って sourcemap を取り込む。
`injectSourcesContent` (同 17067 行) は sourcemap の `sources` の各要素をパッケージのルート (`node_modules/@duckdb/duckdb-wasm`) と突き合わせ、外を指す要素は `sourcesContent` を `null` にして `logger.warnOnce` で次の警告を出す (同 17084 行)。

```text
Sourcemap for "/.../node_modules/@duckdb/duckdb-wasm/dist/duckdb-browser-eh.worker.js" points to a source file outside its package: "/.../node_modules/@duckdb/apache-arrow/util/util/buffer.ts"
```

`@duckdb/duckdb-wasm` 1.32.0 の `dist/duckdb-browser-eh.worker.js.map` は `sources` を 138 個持ち、そのうち 117 個が `../../apache-arrow/...` (同梱の `@duckdb/apache-arrow` のソース) を指していて `sourcesContent` を持たない。
上の引用は `sources` の順に処理されて最初に出る 1 行 (index 5 の `util/util/buffer.ts`) で、以下 `Arrow.ts` などの残り 116 個について同じ形の行が続く。
`warnOnce` は同じメッセージを 1 回に抑えるが、メッセージはパスごとに異なる。
このため dev server を起動するたびに、DuckDB を初期化する操作で最大 117 行が出る。
DuckDB を初期化する操作は、S3 または GCS の Drawer の Objects タブで対象行の Query を押す操作である。
`DrawerObjectBrowser` が `DrawerObjectQuery` を表示し、`frontend/src/lib/duckdb.ts` の `getDuckDB()` が `new Worker(duckdbEhWorkerUrl)` を実行して worker が取得される。

警告が示すのは、worker の sourcemap から arrow のソース本文が欠けることだけで、worker の実行には影響しない。
`vite build` では worker が `dist/assets/duckdb-browser-eh.worker-<hash>.js` としてそのまま複製され、この警告は出ない (2026-09-29 に確認)。
したがって dev server だけの事象である。
抑止しないと、この警告に紛れて他の警告 (依存更新や設定不備で新しく出るもの) を見落としやすい。

## 対応方針

`frontend/vite.config.ts` の `customLogger` で、Vite 既定のロガー (`createLogger()`) の `warn` と `warnOnce` を差し替え、この警告だけを落とす。

- 判定は純関数 `isSuppressedViteWarning(msg: string): boolean` として新設の `frontend/src/lib/viteLogger.ts` に置く。
  3 つの部分文字列 (`points to a source file outside its package`、`/@duckdb/duckdb-wasm/dist/duckdb-browser-eh.worker.js`、`/@duckdb/apache-arrow/`) をすべて含むメッセージだけを真にする。
  Vite はメッセージを picocolors で色付け (ANSI エスケープ) して渡すため、判定は部分文字列の包含で行い、先頭一致や完全一致にしない。
  3 条件にすることで、他の worker や他のパッケージを指す同種の警告は従来どおり出る。
- ロガーの差し替えは同じファイルの `withSuppressedWarnings(logger, isSuppressed)` で行い、`warn` と `warnOnce` の両方を差し替える。
  Vite の `createLogger` が返すロガーの `warnOnce` は内部の出力関数を直接呼び `warn` を経由しない (`node.js` の 14225 行からの `createLogger`) ため、`warn` だけを差し替えてもこの警告は止まらない。
  差し替えは渡されたオブジェクトのメソッドを書き換えて同じオブジェクトを返す。
  スプレッドで写しを作ると、元の `warn` が `hasWarned` を元のオブジェクトに書き、Vite が写しから読む値と食い違う。
- 引数の型は `warn` と `warnOnce` を持つ構造的な型 (ジェネリック) で受け、`src/` から `vite` の型を import しない。
- `customLogger` を渡すと Vite の `createLogger` はそのオブジェクトをそのまま返す (`node.js` の 14226 行) ため、`config.logger` が差し替え後のロガーになり、`injectSourcesContent` の `logger.warnOnce` に届く。
  同じ理由で、`customLogger` を渡すと `createLogger(level, options)` の `level` と `options.allowClearScreen` は捨てられ、`vite.config.ts` の `logLevel` / `clearScreen` と CLI の `--logLevel` / `--clearScreen` は効かなくなる (`resolveConfig` は 36944〜36947 行で `allowClearScreen: config.clearScreen` と `customLogger` を同時に渡す)。
  このリポジトリの `frontend/package.json` の `dev` / `build` / `preview` スクリプトと `vite.config.ts` はどちらも `logLevel` と `clearScreen` を使っていないため、この副作用を受け入れる。
  `logLevel` や `clearScreen` を使いたくなったときは、`createLogger()` の引数 (`level`、`options.allowClearScreen`) に渡す。

採らなかった案。

- `server.sourcemapIgnoreList`: ブラウザの DevTools に伝える無視リストの設定で、`injectSourcesContent` はこの設定を参照しないため、警告は止まらない。
- `logLevel: 'error'`: Vite の全警告が消え、依存更新や設定不備の警告も見えなくなる。
- worker ファイルを `public/` に複製して sourcemap の参照を外す: 依存更新のたびに複製を追従する作業が要り、`?url` import で `node_modules` から直接配信している現状 (issue 0196) より保守が重い。
- `@duckdb/duckdb-wasm` の sourcemap を修正する upstream 対応: 取り込まれる時期が読めない。
  取り込まれた後にこの抑止を外せばよい。
- 判定を `vite.config.ts` に直接書く: `frontend/tsconfig.json` の検査対象は `src` だけなので `tsc --noEmit` の対象外になり (eslint の対象には入る)、vitest から読み込みにくい (config の副作用込みで import することになる)。
- `--logLevel` を保ったまま抑止する: Vite が config 関数に渡す `ConfigEnv` は `command` / `mode` / `isSsrBuild` / `isPreview` だけで CLI の `--logLevel` を含まないため、`customLogger` を使う限り保てない。
  上記のとおり副作用として受け入れる。
- `vite.config.ts` を vitest から import して `customLogger` の配線を検証する: config を import すると `react()` プラグインの生成などの副作用込みで評価され、`tsc --noEmit` の対象にも `vite.config.ts` が入る。
  配線は下の手動確認で検証する。

## 完了条件

- `frontend/src/lib/viteLogger.ts` に `isSuppressedViteWarning` と `withSuppressedWarnings` があり、`frontend/vite.config.ts` が `customLogger: withSuppressedWarnings(createLogger(), isSuppressedViteWarning)` を渡している。
  `vite.config.ts` の他の設定 (`plugins` / `server` / `preview` / `test`) は変えない。
- `frontend/src/lib/viteLogger.test.ts` が次を検証する。
  - 背景に引用した形の警告 (ANSI エスケープで色付けしたものを含む) で `isSuppressedViteWarning` が真を返す。
  - 3 条件のいずれか 1 つを欠くメッセージ (別の worker ファイル、`/@duckdb/apache-arrow/` 以外のパス、別の文言) で偽を返す。
  - `withSuppressedWarnings` が、判定が真のメッセージでは元の `warn` / `warnOnce` を呼ばず、偽のメッセージでは同じ引数で元の `warn` / `warnOnce` を呼び、渡したオブジェクトと同じオブジェクト (同一性) を返す。
- 手動確認を次の手順で行い、手順と結果を「## 解決方法」に書く。
  - backend (`example/` の floci に対して `HOME="$(pwd)/example/home"` と `THIEF_S3_PATH_STYLE=true` で起動したものでよい) と `mise run frontend:run` を起動する。
  - S3 の Drawer の Objects タブで、形式が対応しているオブジェクト (csv など) 1 件の Query を押し、SQL を 1 回実行する。
  - DevTools の Network で `duckdb-browser-eh.worker.js` が取得されたことを確認する (worker が取得されなければこの検査は空振りになる)。
  - 変更前 (`customLogger` を渡さない状態) では Vite のコンソールに `points to a source file outside its package` を含む行が出ること、変更後では出ないことを確認する。
- `CHANGES.md` の `## develop` の `### misc` にエントリを追加する。
- `mise run check` が通る。

## 関連

- issue 0196: worker を `?url` import で `node_modules` から配信する現状の経緯。

## 解決方法

`frontend/vite.config.ts` の `customLogger` に、新設の `frontend/src/lib/viteLogger.ts` で `warn` と `warnOnce` を差し替えた Vite 既定のロガーを渡し、duckdb-wasm の eh worker の sourcemap 警告だけを落とす。

- `frontend/src/lib/viteLogger.ts` (新規): `isSuppressedViteWarning(msg)` は 3 つの部分文字列 (`points to a source file outside its package`、`/@duckdb/duckdb-wasm/dist/duckdb-browser-eh.worker.js`、`/@duckdb/apache-arrow/`) をすべて含むメッセージだけを真にする。`withSuppressedWarnings(logger, isSuppressed)` は渡されたオブジェクトの `warn` と `warnOnce` をその場で書き換え、判定が真のメッセージを捨て、偽のメッセージは同じ引数で元のメソッドに渡し、渡されたオブジェクトをそのまま返す。引数は `warn` と `warnOnce` を持つ構造的な型 `WarnLogger` のジェネリックで受け、`vite` の型は import しない。
- `frontend/vite.config.ts`: `vite` から `createLogger` を追加で import し、`customLogger: withSuppressedWarnings(createLogger(), isSuppressedViteWarning)` を渡す。`plugins` / `server` / `preview` / `test` は変えていない。
- `frontend/src/lib/viteLogger.test.ts` (新規): vitest のテスト 9 件。

完了条件の検証。

| 完了条件 | 検証 |
| --- | --- |
| `viteLogger.ts` の 2 関数と `vite.config.ts` の `customLogger`、他の設定は不変 | 上記の変更。`vite.config.ts` の差分は import の 2 行、コメントの 3 行、`customLogger` の 1 行だけ |
| `viteLogger.test.ts` の検証項目 | `isSuppressedViteWarning`: 背景に引用した形の警告で真、ANSI エスケープで色付けした警告で真、別の worker (mvp) で偽、`/@duckdb/apache-arrow/` 以外のパスで偽、別の文言で偽、無関係な警告で偽の 6 件。`withSuppressedWarnings`: 真のメッセージで元の `warn` / `warnOnce` を呼ばない、偽のメッセージで同じ引数 (`{ timestamp: true }`) で元を呼ぶ、渡したオブジェクトと同じオブジェクトを返す (`toBe`) の 3 件。`mise run check` の frontend:test で 118 ファイル 1347 件が通過 (変更前は 117 ファイル 1338 件) |
| 手動確認 | 下記 |
| `CHANGES.md` の `### misc` のエントリ | 追記した |
| `mise run check` | 通過 (exit 0。backend:test は 18 パッケージすべて ok、frontend:lint は 0 errors / 9 warnings で変更前と同数、govulncheck は No vulnerabilities found) |

手動確認 (2026-09-29)。

完了条件の手順から次の 2 点を変えた。ホストの 8088 と 8089 を別のプロセスが使っていたため、frontend は `mise run frontend:run` の代わりに `frontend/` で `VITE_API_BASE=http://127.0.0.1:18089 npx vite --port 18088 --strictPort` を実行し、backend は HEAD 5202e60 を `go build ./cmd/thief` したバイナリを `HOME="$(pwd)/example/home" THIEF_S3_PATH_STYLE=true THIEF_LISTEN_ADDR=127.0.0.1:18089 thief server` で起動した (floci は `mise run example:up` と `mise run example:seed`)。ブラウザの操作と DevTools の Network の確認は、headless の Google Chrome (`--headless=new --remote-debugging-port=19222`、専用の `--user-data-dir`) に対して DevTools Protocol (`Network.enable` の `requestWillBeSent` / `responseReceived` と `Runtime.evaluate` によるクリック) で行った。操作の経路は、サイドバーの S3、バケット `thief-example-data` の行、Drawer の Objects タブ、フォルダ `reports/`、`sample.csv` の Query、取り込み後の自動実行の結果表示を待って 実行 を 1 回押す、の順である。変更前は HEAD 5202e60 の作業ツリー、変更後はこの変更を適用した作業ツリーで、それぞれ Vite を新しく起動して同じ経路を実行した。

| 項目 | 変更前 (HEAD 5202e60、`customLogger` なし) | 変更後 |
| --- | --- | --- |
| クエリ結果 | 取り込み直後の自動実行と 実行 ボタンの両方で `3 行` | 同じ |
| Network の worker 取得 | `/node_modules/@duckdb/duckdb-wasm/dist/duckdb-browser-eh.worker.js?url` の requestWillBeSent と responseReceived (200、text/javascript)、続いて `?url` なしの同パスの requestWillBeSent (type Script、Worker の起動) | 同じ |
| Vite のコンソールの `points to a source file outside its package` を含む行 | 117 行 (最初の行は `.../node_modules/@duckdb/apache-arrow/util/util/buffer.ts` を指す) | 0 行 (ログは起動メッセージの 5 行だけ) |
| ブラウザのコンソール | Vite の接続メッセージ 2 行と React DevTools の案内 1 行。例外なし | 同じ |

方針セクションからの乖離は無い。

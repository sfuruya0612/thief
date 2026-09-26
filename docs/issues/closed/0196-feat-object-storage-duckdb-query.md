# S3 / GCS のオブジェクトを DuckDB Wasm と OPFS でブラウザ内 SQL 検索する

Created: 2026-09-24
Model: Claude Fable 5.1
Completed: 2026-09-25

## 背景

TODO.md の次の項目に由来する。

> Frontend で Object Storage の Object を DuckDB Wasm + OPFS で検索、可視化できるようにしたい。
> - 検索可能なデータ容量は 1 GB までとしておき、これは環境変数で調整可能。
> - 対象とすファイルは csv,  parquet, jsonl, json など、AWS, Google Cloud の Managed サービスが出力する拡張子で代表的なものとする

本 issue は「検索」(オブジェクトの取り込み、SQL の実行、結果の表表示) と容量上限の環境変数を扱い、「可視化」(クエリ結果のグラフ表示) は docs/issues/0197 で扱う。

現状、S3 / GCS のオブジェクトに対してブラウザからできる操作は一覧、プレビュー、ダウンロード、アップロードの 4 つである。

- オブジェクトブラウザは `frontend/src/components/Drawer/DrawerObjectBrowser.tsx` の `DrawerObjectBrowser` で S3 / GCS 共通になっており、Actions 列に Preview ボタンと Download リンクを持つ。S3 側の呼び出しは `DrawerS3Objects.tsx`、GCS 側は `DrawerGCSObjects.tsx` である。
- プレビューは `frontend/src/components/Drawer/DrawerObjectPreview.tsx` の `DrawerObjectPreview` が担い、`.csv` は `lib/parseCsv.ts` の `parseCsv` と `components/query/ResultTable.tsx` の `ResultTable` で表にし、それ以外はテキストとして表示する。中身は backend の `handleS3ObjectPreview` (`backend/internal/api/handlers_s3_object.go`) と `handleGCPGCSObjectPreview` (`handlers_gcp.go`) が `PreviewResponse` の JSON エンベロープで全量を文字列として返す。
- プレビューの上限は `backend/internal/api/handlers_object_preview.go` の `maxPreviewSize = 5 << 20` (5 MiB) で固定されており、`.parquet` と `.gz` は同ファイルの `previewBinaryExtensions` (frontend 側は `lib/objectPreview.ts` の `PREVIEW_BINARY_EXTENSIONS`) に含まれるためプレビューできない。
- ダウンロードは `handleS3ObjectDownload` と `handleGCPGCSObjectDownload` が `io.Copy` でストリーミングし、`Content-Length` をオブジェクトのメタデータから設定する。サイズ上限は無い。Range リクエストには対応していない (`backend/internal/aws/s3_object.go` の `GetS3Object` は `Bucket` と `Key` だけを渡し、`backend/internal/gcp/gcs.go` の `GetObject` は `NewReader` を使う)。エラー応答は `backend/internal/api/models.go` の `ErrorResponse` (`{"error": "...", "code": "...", "details": ...}`) の形で、SSO の期限切れは `code` が `SSO_TOKEN_EXPIRED` の 401 になる。
- backend の環境変数は `backend/internal/config/config.go` の `applyEnv` が読む。アプリ固有の変数は `THIEF_` 接頭辞 (`THIEF_LISTEN_ADDR`、`THIEF_SNIPPETS_DIR`、`THIEF_S3_PATH_STYLE` など) で、`AWS_PROFILE` や `GOOGLE_CLOUD_PROJECT` などクラウド側の慣習に従う変数も同じ関数で読む。`config.Load()` は `backend/internal/cli/helper.go` の `loadConfig` 経由で CLI サブコマンドが呼ぶ (`internal/cli` 配下の `loadConfig` 呼び出しは定義を除いて 39 箇所。うち 1 箇所は一覧系サブコマンドの共通ヘルパー `runList` にあり、apigw / dynamo / elb / rds などの一覧コマンドはこれを経由して呼ぶ)。`backend/internal/cli/server.go` の `thief server` も直接呼ぶ。frontend が読む環境変数は `frontend/src/api/client.ts` の `VITE_API_BASE` だけである (型宣言は `frontend/src/vite-env.d.ts`)。
- SQL エディタ (`components/query/SqlEditor.tsx` の `SqlEditor`、props は `value` / `onChange` / `onRun` / `schema`) と結果表 (`ResultTable`、props は `columns: string[]` と `rows: string[][]`) は Athena / BigQuery ビューで使われており流用できる。
- backend と frontend の型契約は `backend/internal/contract/contract.go` の `Registry` (64 型) とゴールデン JSON で保ち、`backend/internal/contract/contract_test.go` が `Registry` の件数を 64 に固定し、`frontend/src/types/contract.check.ts` の `ContractChecks` が 64 対の型検査を並べている。
- frontend が実行時に外部ホストから自動で読み込むサブリソースは `frontend/index.html` の Google Fonts (`fonts.googleapis.com` / `fonts.gstatic.com`) だけである (`PricingToolbar.tsx` の `<a href>` と `CloudLoggingView.tsx` の `window.open` が外部サイトを開くが、どちらも利用者の操作で開く遷移であり自動の読み込みではない)。AWS / Google Cloud のアイコンは `public/assets/` から同一オリジンで配信する。
- DuckDB と OPFS に関する実装はリポジトリに無い。`frontend/package.json` に `@duckdb/duckdb-wasm` は無く、`frontend/vite.config.ts` に Worker や COOP / COEP ヘッダの設定も無い。

要望が満たされていない理由は 3 つある。オブジェクトに SQL を実行する入口が無いこと、プレビューが 5 MiB 未満のテキストに限られ parquet を読めないこと、プレビュー API が JSON で全量を返す設計のため 1 GB 級のオブジェクトに使えないことである。

## 目的

S3 / GCS のオブジェクトブラウザからオブジェクトを 1 つ選び、ブラウザ内で SQL を実行して結果を表で確認できるようにする。
取り込めるオブジェクトのサイズ上限は既定 1 GiB とし、backend の環境変数で変更できるようにする。

## pending にした理由

todo-to-issue スキルの多観点レビューを 3 ラウンド行い、最終ラウンドでも優先度「高」の指摘が 1 件出た (完了条件と設計判断の観点: `createSyncAccessHandle` は Worker 専用 API で、メインスレッドの Query ボタンの活性判定には使えない)。スキルの規約では、3 ラウンド目に「高」が出た場合はドラフトを確定せず、指摘と反映内容を添えてユーザーの確認を求める。ユーザーが不在 (就寝中) で「判断が必要になったら pending にしておく」と指示されていたため、指摘を反映した状態で pending に置く。

確認してほしい点は次の 3 つである。いずれも本文には反映済みで、確認が取れれば `issues/` に戻してそのまま実装に着手できる。

- 設計判断 9: メインスレッドの機能検出を OPFS (`navigator.storage.getDirectory`) と Web Locks の 2 つに減らし、`createSyncAccessHandle` の有無は Worker が取り込み開始時に確認して `ErrorBanner` に出す方式。probe 用の Worker を起動時に生成する案は却下した。
- 設計判断 8: 解放処理を `useEffect` のクリーンアップに置き、各段階を独立に実行して Worker への終了指示まで必ず到達させる方式と、`StrictMode` の二重実行への対処 (開始の `useRef` ガード、teardown の直列化、`AbortError` の非表示)。
- 設計判断 2: parquet / json の機能が Wasm に静的リンクされている前提で進め、別ファイルの拡張が要る場合は同一オリジン配信の方法を追記してから実装する方針。

レビュー 3 ラウンドの指摘の総数は 高 6 / 中 19 / 低 29 (観点ごとに数えた延べ数) で、却下した指摘は無い。すべて本文に反映済みである。

## pending 論点の決着 (2026-09-25)

「## pending にした理由」の 3 点について、2026-09-25 に利用者の回答を得た。いずれも本文の方針のままで決着した。

- 設計判断 9：承認。メインスレッドの機能検出は OPFS (`navigator.storage.getDirectory`) と Web Locks (`navigator.locks`) の 2 つとし、`createSyncAccessHandle` の有無は Worker が取り込みの開始時 (`fetch` の前) に確認して `ErrorBanner` に出す。probe 用の Worker は作らない。
- 設計判断 8：承認。解放処理を `useEffect` のクリーンアップに置き、各段階を独立に実行して Worker への終了指示まで必ず到達させる。`StrictMode` への対処 (開始の `useRef` ガード、teardown の直列化、アンマウント由来の `AbortError` の非表示) も本文どおりとする。
- 設計判断 2：承認。parquet と json の機能が Wasm に静的リンクされている前提で進め、別ファイルの拡張が要ると分かった場合は同一オリジンから配信する方法を本 issue に追記してから実装する。CDN (jsDelivr、`extensions.duckdb.org`) からの実行時の取得は許容しない。

### 設計判断 9 と 3 の事実の訂正

設計判断 9 の却下理由にある「OPFS の `getDirectory` に対応する版のブラウザ (Chromium 102、Firefox 111、Safari 15.2 以降) は全て Worker 内の `createSyncAccessHandle` にも対応しており」は、Chromium について誤りである。2026-09-25 に MDN の browser-compat-data (`mdn/browser-compat-data` の main ブランチ) で確認した各機能の最低バージョンは次のとおり。

| 機能 | Chrome | Firefox | Safari |
| --- | --- | --- | --- |
| `StorageManager.getDirectory` (OPFS) | 86 | 111 | 15.2 |
| `FileSystemFileHandle.createSyncAccessHandle` (Worker 内) | 102 | 111 | 15.2 |
| `FileSystemFileHandle.createWritable` | 86 | 111 | 26 |
| Web Locks (`navigator.locks`) | 69 | 96 | 15.4 |
| Wasm の例外処理 (eh バンドル) | 95 | 100 | 15.2 |

メインスレッドの 2 つの検出が真になり、Worker 内の `createSyncAccessHandle` が無い組み合わせは Chrome 86 から 101 だけである。Firefox は 111 で `getDirectory` と `createSyncAccessHandle` が同時に入り、Safari は Web Locks (15.4) の検出が偽になる 15.2 と 15.3 ではメインスレッドの時点で Query ボタンが無効になる。したがって Chrome 86 から 101 では Query ボタンが押せ、押すと Worker が `fetch` の前に `unsupported` のエラーを返して `ErrorBanner` に非対応のメッセージが出る。

probe 用の Worker を却下する結論は変えない。理由は、差が出るのは 2020-10 から 2022-04 に公開された Chrome の版に限られ、その版でもダウンロードを始める前にエラーで止まるだけで壊れないこと、probe を足すと起動時の Worker と「判定中」の状態がボタンに増え、その分岐とテストが増えることである。

全機能がそろう最低バージョンは Chrome 102、Firefox 111、Safari 15.4 である。設計判断 9 で `lib/duckdb.ts` のコメントに記録する値はこの表を実装時に MDN で再確認したものとする。

設計判断 3 の却下理由にある「`createWritable()` は Chromium と Firefox で使えるが、WebKit の対応状況を確認できていない」は、上の表のとおり Safari 26 からの対応と確認できた。メインスレッドの `createWritable()` を却下する結論は、対応ブラウザが Safari 26 以降に狭まるため変えない。

## 設計判断

リポジトリ外の事実 (npm と GitHub のバージョン、DuckDB-Wasm のソース) は 2026-09-24 に `npm view @duckdb/duckdb-wasm` と GitHub の `duckdb/duckdb-wasm` リポジトリの v1.32.0 タグのソースで確認した。実装時に同じ方法で再確認する。

### 1. 実行エンジンは `@duckdb/duckdb-wasm` を frontend に追加する (新規依存)

csv / tsv / json / jsonl / parquet を 1 つの SQL エンジンで読める。標準 Web API と既存の依存には SQL エンジンが無く、代替できない。

- 却下: backend に Go 用の DuckDB バインディングを組み込む。CGO が必須で、リポジトリ規約の `CGO_ENABLED=0` ビルドに反する。TODO も Wasm を指定している。
- 却下: sql.js や alasql などの JavaScript 製 SQL エンジン。parquet を読めない。

バージョンは `^` を付けず 1.32.0 に固定する。npm の `latest` タグは開発版 `1.33.1-dev57.0` を指しており、GitHub の最新リリース v1.33.0 は npm に公開されていない。npm にある最新の安定版が 1.32.0 である。OPFS 用の `registerOPFSFileName` は v1.32.0 の `packages/duckdb-wasm/src/bindings/bindings_base.ts` にある。

結果の読み出しに使う `apache-arrow` は `@duckdb/duckdb-wasm` 1.32.0 の依存 (`^17.0.0`) だが、`lib/duckdbResult.ts` (後述) が Arrow の型 (`Table`、`DataType` など) を直接 import するため、`frontend/package.json` にも `apache-arrow` を `^17.0.0` で直接依存として追加する。理由は、間接依存への直接 import は npm の hoisting に依存し、依存木の変化で壊れるためである。範囲を `@duckdb/duckdb-wasm` の要求と同じにして 1 つの版に解決させる。

### 2. Wasm と Worker は同一オリジンから配信し、eh バンドルを使う

`@duckdb/duckdb-wasm/dist/duckdb-eh.wasm` と `dist/duckdb-browser-eh.worker.js` を Vite の `?url` import で取り込み、`AsyncDuckDB` の初期化に渡す。設計判断 5 の読み取り関数が使う parquet と json の機能は DuckDB-Wasm の Wasm に静的リンクされている前提で進めるが、実装時に v1.32.0 のビルド構成で確認する。別ファイルの拡張として `extensions.duckdb.org` から遅延ロードされる機能があれば、`SET autoinstall_known_extensions = false` と `SET autoload_known_extensions = false` で外部からの取得を止め、必要な拡張ファイルを同一オリジンから配信する方法を本 issue に追記してから実装する。

- 却下: `getJsDelivrBundles()` による CDN 配信。実行時の外部ホスト依存を Google Fonts 以外に増やす。フォントは取得に失敗しても機能は動くが、Wasm は失敗すると機能が動かない。この 2 点で却下の理由は足りる (`getJsDelivrBundles()` はパッケージのバージョン定数から URL を組むため、バージョンの二重管理にはならない)。
- 却下: COI バンドル (`duckdb-coi.wasm`)。SharedArrayBuffer を使うため `Cross-Origin-Opener-Policy` と `Cross-Origin-Embedder-Policy: require-corp` の応答ヘッダが必須になる。COEP を付けると `index.html` の Google Fonts のスタイルシート (no-cors のサブリソース) が CORP ヘッダまたは `crossorigin` 属性を要求されるようになり、フォント読み込みの構成変更が要る。得られる並列化の利点は本機能では要らない。
- 却下: mvp バンドルへのフォールバック。対応ブラウザ (設計判断 9) は全て Wasm の例外処理に対応しており、フォールバックの分岐は保守対象を増やすだけである。

### 3. オブジェクトは Worker が既存のダウンロード API から OPFS に書き、DuckDB は OPFS から読む

取り込みは専用の Web Worker (`frontend/src/lib/opfsWriter.worker.ts`、新規) で行う。Worker は既存の `s3DownloadUrl` / `gcsDownloadUrl` (`frontend/src/api/endpoints.ts`) が組み立てた URL を `fetch` し、レスポンスの `ReadableStream` を読みながら OPFS のファイルに `createSyncAccessHandle()` の `write()` で書く。ファイルは `navigator.storage.getDirectory()` 配下の `thief-query/<uuid>.<拡張子>` に置く。`<拡張子>` は設計判断 5 の表で一致した元オブジェクトの拡張子 (`.csv.gz` のように圧縮の拡張子を含む) で、DuckDB が形式と圧縮を拡張子から判定できるようにする。

書き終えてハンドルを閉じたら、メインスレッドが DuckDB に `registerOPFSFileName('opfs://thief-query/<uuid>.<拡張子>')` で登録し、`CREATE VIEW obj AS SELECT * FROM <読み取り関数>('opfs://thief-query/<uuid>.<拡張子>')` を実行する。v1.32.0 の `runtime_browser.ts` は `opfs://` に続くパスを `/` で分割し、`getDirectoryHandle(folder, { create: true })` でサブディレクトリを辿るため、`thief-query/` のディレクトリ階層を使える。登録名と SQL 中のパスはどちらも `opfs://` 付きの同じ文字列である。

- 理由: JavaScript のヒープに全量を持たない。DuckDB は OPFS の sync access handle から必要な範囲だけ読む。
- 却下: `registerFileBuffer` に `Uint8Array` で全量を渡す。1 GiB の配列を Wasm のメモリ (上限 4 GiB) とタブのメモリに同時に置くことになる。
- 却下: `registerFileURL` でダウンロード URL を DuckDB に直接読ませる。DuckDB-Wasm の HTTP 読み取りは Range リクエストで必要な範囲だけを取る仕組みで、サーバが Range に対応していないときは全量を 1 回の GET で読んでメモリ上のバッファに載せる (v1.32.0 の `runtime_browser.ts` の full HTTP read へのフォールバック)。backend のダウンロードハンドラは Range に対応していないため、このフォールバックに入り `registerFileBuffer` と同じ問題になる。backend に Range 対応を足すと S3 / GCS の取得層とハンドラの 4 箇所に変更が及ぶ。
- 却下: プレビュー API (`PreviewResponse`) の拡張。JSON 文字列で全量を返す設計を 1 GiB に広げることになる。
- 却下: メインスレッドで `FileSystemFileHandle.createWritable()` を使って書く。`createWritable()` は Chromium と Firefox で使えるが、WebKit の対応状況を確認できていない。`createSyncAccessHandle()` は OPFS を実装する全ブラウザで Worker 内から使える。
- 却下: メインスレッドで `fetch` し、`ReadableStream` を Worker に transfer する。transferable streams の対応状況がブラウザごとに異なり、判定する機能が 1 つ増える。Worker 内の `fetch` は Worker が使える全ブラウザで使え、CORS の扱いはメインスレッドと同じである。

Worker は進捗 (書き込み済みバイト数と `Content-Length`) と完了、エラーを `postMessage` で返す。HTTP エラー (非 2xx) のときは `ErrorResponse` の JSON を読み、`status` / `code` / `error` をメインスレッドに返す。メインスレッドはこれを `types/common.ts` の `ApiError` に組み直し、`lib/ssoError.ts` の `isSSOExpiredError` が真なら `components/SSOExpiredBanner.tsx` の `SSOExpiredBanner` (S3 のみ。GCS には SSO が無い)、それ以外は `components/ErrorBanner.tsx` の `ErrorBanner` で表示する。ネットワーク到達不能は `client.ts` と同じく `ApiError(0, 'network_error', ...)` に正規化する。`client.ts` の `doFetch` は export されておらず Worker から使えないため、Worker 側で同じ形に正規化する。

`SSOExpiredBanner` は `profile: string` を必須の props に持つ。`DrawerObjectBrowser` の props には `profile` が無いため、省略可能な `profile?: string` を追加し、`DrawerS3Objects` だけが渡す (`DrawerGCSObjects` は渡さない)。`DrawerObjectBrowser` はこれを `DrawerObjectQuery` へ渡し、`DrawerObjectQuery` は `profile` があり `isSSOExpiredError` が真のときだけ `SSOExpiredBanner` を出し、それ以外のエラーは `ErrorBanner` に出す。

OPFS への書き込み中は DuckDB 側でファイルを開かない (sync access handle は同時に 1 つしか開けない)。

ダウンロード経路の backend は変更しない。AWS / Google Cloud API の追加呼び出しと権限は要らない。

### 4. 容量上限は backend の環境変数で持ち、新しい設定 API で frontend に渡す

既定値は 1 GiB (`1 << 30` バイト) とする。環境変数 `THIEF_OBJECT_QUERY_MAX_BYTES` (10 進の整数、単位はバイト) で上書きする。設定ファイル (YAML) からは読まない (`yaml:"-"`)。TODO が環境変数を指定しており、YAML の項目を増やすと設定経路が 2 つになる。

- `config.Config` に `ObjectQueryMaxBytes int64` を追加し、`applyEnv` で読む。パースできない値と 0 以下の値は `config.Load()` がエラーを返す。`config.Load()` は一覧系 (`runList` 経由) を含む大部分の CLI サブコマンドが呼ぶため、不正な値を設定したシェルでは `thief server` だけでなく `thief ec2 ls` などの CLI も起動しなくなる (`thief sso logout` のように `loadConfig` を通らないサブコマンドは影響を受けない)。これを受け入れる。理由は、設定の読み込みと検証を `config` パッケージに集約するリポジトリ規約 (起動時に設定の検証を行い、不正なら終了) に従い、不正な値を `Config` に載せたまま他のコードに渡さないためである。
- 却下: `thief server` の起動時 (`cli/server.go`) だけで検証する。検証の場所が `config` パッケージの外に分かれ、`Config` が不正な値を保持する状態ができる。
- 却下: `THIEF_S3_PATH_STYLE` のようにパース失敗を無視して既定値のままにする。設定の打ち間違いが黙って無視され、上限が意図と違うまま動く。
- backend に `GET /api/config` を追加し、`ClientConfigResponse{ObjectQueryMaxBytes int64 \`json:"object_query_max_bytes"\`}` を返す (`handlers_config.go`、新規)。認証もクラウド呼び出しも伴わない。
- 契約: `ClientConfigResponse` を `contract.go` の `Registry` に追加し、`contract_test.go` の件数の固定値を 64 から 65 に更新し、`contract.go` と `contract_test.go` と `contract.check.ts` の「64」のコメントも 65 に更新する。ゴールデン JSON を再生成し、frontend は `types/common.ts` に `ClientConfigRaw` (`object_query_max_bytes: number`) と `ClientConfigRow` (`objectQueryMaxBytes: number`) を追加し、`lib/normalize.ts` に `clientConfigFromRaw` を置く (既存の `ObjectPreviewRaw` → `ObjectPreviewRow` と同じ Raw / Row の対。コンポーネントに snake_case を渡さない規約に従う)。`contract.check.ts` の `ContractChecks` に `Expect<Contract<typeof clientConfig, ClientConfigRaw>>` の対を追加する。対を追加しないと `ClientConfigRaw` は検査されず、既存のテストではその漏れを検出できない。
- frontend は `api/endpoints.ts` に `getClientConfig` (`apiGet<ClientConfigRaw>('/api/config')`) を、`api/queries.ts` に `useClientConfig` (`queryKey: ['config']`、`staleTime: Infinity`、`select` で `clientConfigFromRaw` を適用して `ClientConfigRow` を返す) を追加し、`DrawerObjectBrowser` で参照する。
- 設定の取得中と取得失敗時 (旧版の backend が 404 を返す場合を含む) は上限が分からないため、Query ボタンを無効化し `title` に理由 (取得中、取得失敗) を出す。既定値 1 GiB で仮に判定することはしない (backend の設定と食い違う判定になる)。
- 却下: `VITE_*` の環境変数。ビルド時に埋め込まれるため、値を変えるには dist を作り直す必要がある。`mise run frontend:serve` で配信する運用と合わない。
- 却下: `/api/health` に載せる。`/api/health` は `App.tsx` の起動待ちポーリング (`useHealthCheck`) 専用で、意味が変わる。

上限の強制は frontend が 3 段階で行う。

1. 一覧の `size` が上限を超える行は Query ボタンを無効化し、`title` に理由を出す (`lib/objectPreview.ts` の `isPreviewEligible` / `previewDisabledReason` と同じ構造の純関数を `lib/objectQuery.ts` に置く)。
2. `fetch` のレスポンスの `Content-Length` が上限を超えていれば読み始めずに中断する。
3. ストリームの累積バイト数が上限を超えたら `AbortController` で中断し、OPFS のファイルを削除する。

backend では強制しない。ダウンロードハンドラは上限の無い通常のダウンロードにも使うためである。docs/issues/closed/0025 が backend で上限を強制したのは、サーバが JSON 化のために全量を読む設計だったからで、本 issue ではサーバは転送するだけであり、frontend が中断すれば転送も止まる。

### 5. 対象拡張子と読み取り関数

大文字小文字を区別せず、次の拡張子を対象にする。判定は `lib/objectQuery.ts` の純関数 `objectQueryFormat(key)` (一致した拡張子と読み取り関数を返す。対象外は `undefined`) に置く。

| 拡張子 | 読み取り関数 |
| --- | --- |
| `.csv`、`.csv.gz` | `read_csv(path)` (自動判定) |
| `.tsv`、`.tsv.gz` | `read_csv(path, delim='\t')` |
| `.json`、`.jsonl`、`.ndjson` と各 `.gz` | `read_json_auto(path)` |
| `.parquet` | `read_parquet(path)` |

gzip は DuckDB がパスの拡張子から自動判定する。OPFS 上のファイル名が元の拡張子を保つ (設計判断 3) のはこのためである。`.json` は単一の配列またはオブジェクト、`.jsonl` / `.ndjson` は改行区切りで、`read_json_auto` は両方の形式を自動判定する。

選定理由: AWS CloudTrail は `.json.gz`、AWS Cost and Usage Report は `.csv.gz` または `.parquet`、BigQuery のテーブルエクスポートは csv / 改行区切り json / parquet / avro、Cloud Logging のシンクは改行区切り json で出力する。この中で DuckDB-Wasm に同梱される拡張 (parquet、json、httpfs) で読めるものを対象にした。

- 除外: `.avro`、`.orc`。DuckDB-Wasm に同梱されていない拡張が必要になる。
- 除外: `.log`、`.log.gz` (ALB アクセスログ、VPC Flow Logs)。空白区切りでヘッダ行が無く、`read_csv` の自動判定では列名が `column0` 以降の連番になり、形式ごとの列定義が要る。本 issue では扱わない。
- プレビューの `previewBinaryExtensions` / `PREVIEW_BINARY_EXTENSIONS` は変更しない。プレビューと SQL 検索は別機能で、対象の判定も別に持つ。

### 6. UI はオブジェクトブラウザの Actions 列に Query ボタンを追加する

`DrawerObjectBrowser` の Actions 列に、Preview と Download の間に Query ボタンを置く。押下で `DrawerObjectQuery` (`frontend/src/components/Drawer/DrawerObjectQuery.tsx`、新規) を `DrawerObjectPreview` と同じくブラウザ本体を置き換える形で表示し、Close で一覧に戻る。

`DrawerObjectQuery` は次の要素で構成する。

- 読み込み進捗 (書き込み済みバイト数 / `Content-Length`)。`Content-Length` が無い場合は書き込み済みバイト数だけを出す。
- `SqlEditor` (初期値 `SELECT * FROM obj LIMIT 100`) と Run ボタン。`onRun` (Ctrl+Enter / Cmd+Enter) でも実行する。
- `ResultTable` による結果表示。行数と実行時間を表の下に出す。
- エラー表示 (`ErrorBanner`、SSO 期限切れは `SSOExpiredBanner`)。DuckDB のエラーメッセージはそのまま英語で出す。

文言は次のとおり分ける。Query ボタンのラベル `Query` は Preview / Download と同じく英語のハードコードとする (Drawer のタブ名を英語ハードコードにする docs/issues/closed/0066 の方針に、既存のアクション名が従っている)。それ以外 (Query ボタンの無効理由、進捗、Run、行数と実行時間、打ち切りの案内、取り込みエラーの見出し) は `drawerStorage` ネームスペースに `objectQuery.*` (無効理由) と `drawerObjectQuery.*` (パネル内) のキーを追加し、`frontend/src/i18n/locales/ja/drawerStorage.json` と `locales/en/drawerStorage.json` の両方に載せる。

- 却下: 新しいトップレベルビューやサービスの追加。`AppView` (`types/common.ts`) の追加は `PersistedState` と `TopBar` の変更を伴い、リポジトリ規約で事前に質問が要る。対象オブジェクトを選ぶ UI は Objects タブに既にある。
- 却下: Athena / BigQuery ビューのエディタタブ、履歴、スニペット。1 オブジェクトに対するアドホックな検索に要らない。

### 7. 結果の変換と行数上限

クエリは `AsyncDuckDBConnection.send()` でストリーム (`AsyncRecordBatchStreamReader`) として実行し、レコードバッチを読みながら行を数え、10,000 行に達したら残りを読まずに `cancelSent()` で打ち切る。打ち切ったことと `LIMIT` の利用を促す文を表の上に出す。

- 却下: `query()` で全結果を Arrow の `Table` に受けてから 10,000 行に切る。1 GiB 級のオブジェクトを `SELECT *` した結果が全てヒープに載り、設計判断 3 の「全量を持たない」方針に反する。
- 却下: 利用者の SQL をサブクエリで包んで `LIMIT 10001` を足す。`CREATE TABLE` や `PRAGMA` など SELECT 以外の文が壊れる。

読んだレコードバッチは `lib/duckdbResult.ts` (新規) の純関数で `string[][]` に変換し、`ResultTable` に渡す。変換規則は次のとおり。

- `null` は空文字列。
- boolean は `true` / `false`。
- 整数 (BigInt を含む) と Decimal は 10 進の文字列。
- Date / Timestamp は ISO 8601 の文字列 (UTC)。
- それ以外は `String(value)`。

### 8. OPFS ファイルと DuckDB インスタンスの寿命

- 取り込み前に `navigator.storage.estimate()` の `quota - usage` がオブジェクトの `size` 未満なら開始せずエラーを出す。書き込み中の `QuotaExceededError` はエラーを出して OPFS のファイルを削除する。
- OPFS のファイルは `DrawerObjectQuery` のアンマウント時に削除する。解放処理は `useEffect` のクリーンアップに置き、Close ボタンに限らず、Drawer 本体の X ボタン (`Drawer.tsx` の `onClose`)、Objects タブから他タブへの切り替え (`Drawer.tsx` が `tab === 'Objects'` で条件描画しているため `DrawerObjectBrowser` ごとアンマウントされる)、選択中のリソースが外れる経路 (`AccountView.tsx` は `data ?? []` から選択を引く。region の切り替えで queryKey が変わり `data` が `undefined` になる場合、profile の切り替えで `App.tsx` の `key={activeProfile}` により再マウントされる場合、再取得後に選択中のリソースが一覧から消えた場合に起きる。`invalidateQueries` による再取得では `data` が保持されるため起きない) のいずれでも動くようにする。順序は、実行中のクエリがあれば `cancelSent()` で止め、`DROP VIEW obj`、`dropFile`、Worker への終了指示 (Worker はロックを解放して `removeEntry` する) とする。アンマウントは取り込み開始前、ダウンロード中、`registerOPFSFileName` 後で `CREATE VIEW obj` 前、クエリ実行中のどの段階でも起きるため、各段階を独立に実行し (未作成のビューの `DROP VIEW IF EXISTS obj`、未登録のファイルの `dropFile` の失敗は捕まえて次に進む)、どの段階が失敗しても Worker への終了指示まで必ず到達させる。ダウンロード中なら Worker が `AbortController` で `fetch` を中断してからファイルを削除する。

`main.tsx` は `StrictMode` で描画しており、開発時は effect が setup → cleanup → setup と二重に走る。取り込みの開始を effect に置くと 1 回目が cleanup で中断され 2 回目が再ダウンロードするため、開始は `useRef` で 1 回に限定し、cleanup の teardown は Promise を保持して次の開始が前の teardown の完了を待つ (直列化) ようにする。アンマウント由来の中断 (`AbortError`) はエラーとして表示しない。却下: Close ボタンの `onClick` だけに解放を書く。X ボタンとタブ切り替えでファイルとロックが残り、Worker が生きている間はロックが解放されないため起動時の削除でも消えず、開き直すごとに uuid が変わってファイルが累積する。
- 同じオブジェクトを再度開いたときは再ダウンロードする。却下: ETag (GCS は generation) で一致を確認して OPFS のファイルを再利用する。一致確認のためのメタデータ保存と、再利用するファイルを残すための容量管理 (上限と削除順) が要る。本 issue は最初の版として取り込みごとに転送する動作を取り、再利用は「扱わない範囲」に置く。
- OPFS は同一オリジンの全タブで共有される。他タブが使っているファイルを消さないため、取り込みからアンマウントまでの間、Worker が `navigator.locks.request('thief-query:<uuid>', ...)` でロックを保持する。ロックは OPFS のファイルを作成する前に取得する (作成後に取ると、書き込み開始直後のファイルが他タブの起動時の削除に消されうる)。frontend の起動時 (`main.tsx` の描画前) に `thief-query/` 配下を走査し、`navigator.locks.request(name, { ifAvailable: true }, ...)` でロックを取れたファイルだけを残骸として削除する。この起動時処理は設計判断 9 のメインスレッドの機能検出が 1 つでも偽なら何もしない (OPFS 非対応ブラウザで `navigator.storage.getDirectory` を呼ぶと例外になる)。走査と削除の失敗は `console.warn` に留め、描画を止めない。却下: 起動時に `thief-query/` 配下を無条件に削除する。別タブで取り込み中またはクエリ中のファイルを消す。
- `AsyncDuckDB` は `lib/duckdb.ts` (新規) の `getDuckDB()` で遅延初期化し、1 つのインスタンスを共有する。`DrawerObjectQuery` を閉じてもインスタンスは破棄しない。

### 9. 対応ブラウザ

必要な機能は OPFS (`navigator.storage.getDirectory`)、Web Locks (`navigator.locks`)、Worker 内の `FileSystemFileHandle.createSyncAccessHandle`、Wasm の例外処理 (eh バンドル) の 4 つである。

メインスレッドで機能検出するのは前の 2 つ (`'storage' in navigator && 'getDirectory' in navigator.storage` と `'locks' in navigator`) で、1 つでも偽なら Query ボタンを無効化して理由を `title` に出し、設計判断 8 の起動時処理も何もしない。`createSyncAccessHandle` は Worker 専用の API でメインスレッドの `FileSystemFileHandle.prototype` には存在しないため (TypeScript の `lib.webworker.d.ts` にだけ宣言がある)、メインスレッドでは検出しない。Worker が取り込みの開始時に `'createSyncAccessHandle' in FileSystemFileHandle.prototype` を確認し、偽なら `unsupported` のエラーをメインスレッドに返して `ErrorBanner` に出す。Wasm の例外処理も実行前に検出せず、`AsyncDuckDB` の初期化が失敗したときは同じ `ErrorBanner` に失敗のメッセージを出す。

- 却下: メインスレッドで `createSyncAccessHandle` の有無を判定する。常に偽になり、全ブラウザで Query ボタンが無効のままになる。
- 却下: 起動時に probe 用の Worker を生成して `createSyncAccessHandle` の有無を調べ、結果を保持する。OPFS の `getDirectory` に対応する版のブラウザ (Chromium 102、Firefox 111、Safari 15.2 以降。実装時に MDN で再確認する) は全て Worker 内の `createSyncAccessHandle` にも対応しており、判定のためだけに Worker を 1 つ増やす価値が無い。各機能の最低バージョン (Chromium、Firefox、Safari) は実装時に MDN で確認し、`lib/duckdb.ts` のコメントに記録する。

### 10. テスト

Vitest の jsdom 環境では Wasm、OPFS、Web Worker が動かない。

- 純関数 (`lib/objectQuery.ts` の `objectQueryFormat` と活性判定と読み取り SQL の生成、`lib/duckdbResult.ts` の変換、上限判定) は単体テストで固定する。
- `DrawerObjectQuery` はエンジンと取り込み処理をインターフェースとして props から受け取り、モックで読み込み中、上限超過、取り込みエラー (SSO 期限切れとそれ以外)、クエリエラー、結果表示、行数打ち切りの分岐と、アンマウント時にエンジン (`cancelSent` / `DROP VIEW` / `dropFile`) と Worker の終了指示が呼ばれることをテストする。
- `DrawerObjectBrowser` 単体のテストファイルは無く、`components/Drawer/DrawerS3Objects.test.tsx` が S3 経由で描画している (GCS 側にテストは無い)。このテストは `fetch` のモックの呼び出し回数 (`toHaveBeenCalledTimes(1)` が 5 箇所) と呼び出し順 (`mock.calls[1]` / `mock.calls[2]` で upload / preview / save のリクエストを取る) を前提にしており、`DrawerObjectBrowser` が `/api/config` を取得するようになると回数と添字がずれて落ちる。`fetch` のモックを URL で分岐させ (`/api/config` には固定の応答を返す)、回数と添字の期待値を更新する。そのうえで、設定の取得中と取得失敗時に Query ボタンが無効になる分岐を同じファイルに追加する。
- backend は `handleClientConfig` を `httptest` でテストし、`config.Load()` は正常値、パース不能、0 以下の 3 系統をテストする。
- 実ファイルでの読み取りは `example/` の floci 環境で手動確認する (GCS はエミュレータが無いため S3 のみ)。

## 実装中の調査結果と設計判断 2 の追記 (2026-09-25)

### 調査結果: parquet と json は Wasm に静的リンクされていない

設計判断 2 の前提 (parquet / json の機能は Wasm に静的リンクされている) は誤りだった。implement-issues の Step 4 の実装エージェントが、`@duckdb/duckdb-wasm` 1.32.0 (DuckDB コア v1.4.3) の `duckdb-eh.wasm` を Node 上の blocking ビルドで動かし、XHR を記録するスタブで次を実測した。

- `read_csv` はコアの関数で、外部への取得は起きない。
- `read_json_auto` と `json_extract` は初回使用時に `GET https://extensions.duckdb.org/v1.4.3/wasm_eh/json.duckdb_extension.wasm` を同期 XHR で送る。
- `read_parquet` は初回使用時に `GET https://extensions.duckdb.org/v1.4.3/wasm_eh/parquet.duckdb_extension.wasm` を同期 XHR で送る。
- `SET autoload_known_extensions = false` を実行すると XHR は起きず、`Catalog Error: ... exists in the json extension` になる。
- `SET custom_extension_repository = '/assets/duckdb-extensions'` を実行すると、要求先が `GET /assets/duckdb-extensions/v1.4.3/wasm_eh/json.duckdb_extension.wasm` (Worker と同一オリジンの相対パス) に変わる。
- 拡張ファイルのサイズは json が 820,646 バイト、parquet が 3,045,039 バイトで、ライセンスは MIT である。

Wasm 内に `read_parquet` などの文字列があるのは autoload 用の既知拡張の一覧であり、関数が静的リンクされていることを示すものではなかった。

### 設計判断 2 の追記: 拡張ファイルは取得スクリプトで同一オリジンに置く (ユーザー確認済み)

設計判断 2 が定めた「別ファイルの拡張が要る場合は同一オリジンから配信する方法を追記してから実装する」に従い、配信方法を次のとおり定める。2026-09-25 にユーザーが取得スクリプト方式を選んだ。

- `frontend/scripts/fetch-duckdb-extensions.mjs` (新規) が `https://extensions.duckdb.org/<REVISION>/wasm_eh/json.duckdb_extension.wasm` と `parquet.duckdb_extension.wasm` をダウンロードし、`frontend/public/assets/duckdb-extensions/<REVISION>/wasm_eh/` に置く。取得は Node 標準の `fetch` で行い、新規依存を増やさない。
  - `<REVISION>` (`v1.4.3`) と各ファイルの SHA-256 はスクリプト内の定数に固定し、ダウンロードしたファイルのハッシュが一致しなければ書き出さずにエラーで終了する。取得先の差し替えや破損を検出するためである。
  - スクリプトは `node_modules/@duckdb/duckdb-wasm/package.json` の `version` が、定数が対応する版 (`1.32.0`) と一致することを確認し、一致しなければエラーで終了する。`@duckdb/duckdb-wasm` を上げたのに拡張の版を更新し忘れる事故を防ぐためである。
- `mise.toml` に `frontend:fetch-duckdb-extensions` タスク (引数なし) を追加し、`mise run setup` から実行されるようにする。
- `frontend/.gitignore` に `/public/assets/duckdb-extensions/` を追加する。拡張ファイルはリポジトリに含めない。
- DuckDB の初期化時 (`lib/duckdb.ts`) に `SET autoinstall_known_extensions = false`、`SET autoload_known_extensions = false`、`SET custom_extension_repository = <同一オリジンの /assets/duckdb-extensions>` を実行し、`extensions.duckdb.org` への取得を止める。
- 拡張は、読み取り関数が拡張を要する形式 (json 系は json、parquet は parquet) に限り、`CREATE VIEW obj` の直前に `INSTALL <拡張>; LOAD <拡張>;` で読み込む。csv と tsv は拡張を読み込まない。拡張ファイルを取得していない環境でも csv / tsv は動き、json / parquet の拡張の読み込みに失敗したときは DuckDB のエラーメッセージを `ErrorBanner` に出す。
- 却下: 拡張ファイルを `frontend/public/` にコミットする。約 3.9 MB のバイナリを git で管理し、版を上げるたびに差し替えることになる (ユーザーが取得スクリプト方式を選んだ)。
- 却下: 初期化時に json と parquet を常に読み込む。csv だけを検索する場合にも約 3.9 MB を取得し、拡張を取得していない環境では csv の検索まで失敗する。
- 却下: `autoload_known_extensions` を有効のまま `custom_extension_repository` だけを差し替える。設計判断 2 が autoinstall と autoload を止めると定めており、どの拡張をいつ読み込むかを DuckDB の暗黙の判定に委ねることになる。

## 実装詳細の乖離と実環境での確認 (2026-09-25)

### 方針の方式を保った実装の詳細

設計判断の方式は変えていない。実装で決めた細部を記録する。

- OPFS の読み書きは `lib/opfs.ts` と `lib/opfsIngest.ts` に分けた。設計判断 3 は Worker が OPFS に書くことだけを定めており、モジュールの分割は定めていない。
- Web Locks のロック名は `thief-query:<ファイル名>` の形式にした (`objectQueryLockName`)。起動時の残骸削除がディレクトリのエントリ名からロック名を再構成できる形にするためである。
- 10,000 行の打ち切りは 1 行先読みで判定する。10,000 行ちょうどのときに打ち切りの表示を出さないためである。
- 取り込みの進捗は 256 KiB ごとに間引いて通知する。1 チャンクごとの再描画を避けるためである。
- Arrow 17 の Date 値は ISO 文字列へ変換して `string[][]` に載せる。`ResultTable` が文字列を前提にするためである。
- DuckDB への問い合わせは `enqueueDb` のプロミス連鎖で直列化する。接続を共有するため、並行実行を避ける。
- `tsconfig.json` の `lib` に `DOM.AsyncIterable` を足した。`ReadableStream` の `for await` を型検査に通すためである。
- Vitest には alias と Worker のスタブを足した。jsdom が Worker と OPFS を持たないためである。
- `mise.toml` の取得タスクは `node --use-system-ca scripts/fetch-duckdb-extensions.mjs` にした。TLS 傍受のある環境では素の `node` の `fetch` が証明書の検証に失敗するためで、`--use-system-ca` は pin している Node 26 の機能であり依存は増えない。
- `INSTALL <拡張>; LOAD <拡張>;` は 1 文で実行する (`objectQueryInstallSql`)。DuckDB は 1 回の `query()` で複数文を実行できる。
- 初期化の SET 文は `objectQueryExtensionInitSql()` として純関数に切り出し、単体テストで固定した。
- SET 文は初期化直後の専用接続で実行して閉じる。SET の適用前に拡張の取得が走る余地をなくすためである。

### floci 環境での確認 (完了条件の 11 形式の行)

`id,name,amount` の 50 行から 11 形式のサンプルを作り、floci の S3 (`thief-example-data` バケットの `qs/` 配下) に置いて確認した。backend は `HOME=example/home`、`THIEF_S3_PATH_STYLE=true`、`THIEF_LISTEN_ADDR=127.0.0.1:8090` で起動した。

確認できたことは次のとおりである。

- `GET /api/config` が `{"object_query_max_bytes":1073741824}` を返す。
- ダウンロード API (`GET /api/aws/profiles/floci/s3/thief-example-data/objects/download`) が 11 形式すべてを HTTP 200 と元と同じバイト数で返す。
- 11 形式すべてで、`objectQueryFormat` が対象と判定し、`objectQueryReadSql` が生成する読み取り関数の呼び出しに対して `SELECT count(*)` が 50 を返す。csv / tsv の 4 形式 (`.csv`、`.csv.gz`、`.tsv`、`.tsv.gz`) は DuckDB Wasm の実バイナリに、ダウンロード API から取得したバイト列を登録して実行した。json / parquet の 7 形式は、同じ SQL を DuckDB 本体に対して、ダウンロード API から取得したファイルに対して実行した。
- 本番ビルド (`npm run build`) の `dist/assets/` に `duckdb-eh.wasm` (34,242.58 kB)、`duckdb-browser-eh.worker.js` (772.75 kB)、`duckdb-extensions/v1.4.3/wasm_eh/` の json と parquet の拡張が含まれる。

拡張の `INSTALL` / `LOAD` の経路は、実装時に Node 上で実 `duckdb-eh.wasm` と XHR のスタブによる同一オリジン配信で確認している。取得先が `/assets/duckdb-extensions/v1.4.3/wasm_eh/` の 2 ファイルだけで `extensions.duckdb.org` への要求が 0 件であること、拡張を 404 にすると json 系が `IO Error` になり csv は通ることを含む。今回の floci での確認では、Node 向けビルドでこの経路を再現できなかったため、json / parquet の 7 形式は DuckDB 本体に同じ SQL を当てる形に代えた。

確認できていないことは次のとおりである。自動テストで担保していない範囲として記録する。

- OPFS への書き込みと `opfs://` パスでの読み取り、Web Locks による排他、起動時の残骸削除、Drawer の UI 操作。いずれも OPFS と Web Locks を持つブラウザでしか動かない。単体テストとコンポーネントテストは、これらをスタブに置き換えて呼び出しの順序と引数を固定している。
- ブラウザの自動操作による確認は、ブラウザの起動が承認されなかったため行っていない。

## レビュー指摘の反映と再検証 (2026-09-25)

close 前の多観点レビュー (完了条件、テスト、規約の 3 観点) の指摘を反映した。

### 記録の訂正

「## 実装詳細の乖離と実環境での確認 (2026-09-25)」に、実装と一致しない記述が 3 件あった。以下を正とする。

- 「単体テストとコンポーネントテストは、これらをスタブに置き換えて呼び出しの順序と引数を固定している」は成立しない。`DrawerObjectQuery.test.tsx` は `cancelSent` / `dropView` / `dropFile` / `terminate` の呼び出し回数を個別に検証するだけで、呼び出しの順序は検証していない。引数を固定しているのは `registerView` と `engine.run` と `dropFile` の一部だけである。
- 「DuckDB への問い合わせは `enqueueDb` のプロミス連鎖で直列化する」は範囲が広すぎる。直列化するのは `registerView` / `cancelSent` / `dropView` / `dropFile` であり、クエリ実行の `engine.run` は `runQuery` から直接呼ばれてキューを通らない。アンマウント時は実行中の `run` と `cancelSent` を意図的に並行させるためである。
- 「10,000 行の打ち切りは 1 行先読みで判定する」は誤りで、正しくは 1 バッチ先読みである。`collectQueryResult` は上限ちょうどで止まったとき次のバッチを 1 つ引いて、その有無で打ち切りを決める。

また、close の根拠から「`dist/` に `extensions.duckdb.org` の文字列が無い」を外した。`rg` が既定で走査しないだけで、`dist/assets/duckdb-eh-*.wasm` には DuckDB の既定リポジトリの文字列が実際に埋め込まれている。文字列の有無は実行時の取得先の証明にならないため、根拠は初期化の SET 文 (`autoinstall_known_extensions` / `autoload_known_extensions` を false、`custom_extension_repository` を同一オリジンに固定) と、次の再検証で観測した要求先 2 件に置き換えた。

### 11 形式の再検証 (完了条件の 11 形式の行)

前回の確認は csv / tsv の 4 形式だけが DuckDB Wasm を通り、json / parquet の 7 形式は DuckDB 本体に同じ SQL を当てる代替で、`obj` ビューは 1 形式も作っていなかった。Node 上に実 Wasm の検証環境を組み直し、11 形式すべてを `obj` ビュー経由で確認した。

確認した経路は次のとおりである。

- エンジンは実 `duckdb-eh.wasm` (`@duckdb/duckdb-wasm` 1.32.0 の `duckdb-node-blocking`)。
- SQL は `lib/objectQuery.ts` の `objectQueryExtensionInitSql` / `objectQueryInstallSql` / `objectQueryReadSql` / `objectQueryOpfsPath` / `objectQueryFileName` が生成したものをそのまま実行する。
- 拡張は `public/assets/duckdb-extensions/v1.4.3/wasm_eh/` の実ファイルを、同一オリジン配信を模した `XMLHttpRequest` のスタブから読ませる。
- ファイルは `opfs://thief-query/<id><拡張子>` の名前で `registerFileBuffer` に登録する。

結果は 11 形式すべてで `CREATE VIEW obj` が成功し、`SELECT count(*) FROM obj` が 50 を返した。スタブが観測した要求先は `/assets/duckdb-extensions/v1.4.3/wasm_eh/json.duckdb_extension.wasm` と `.../parquet.duckdb_extension.wasm` の 2 件だけで、`extensions.duckdb.org` への要求は 0 件である。

この確認が通っていない部分は 2 つある。バイト列の取得元が S3 のダウンロード API ではなくローカルのファイルであること (ダウンロード API が 11 形式を元と同じバイト数で返すことは前節で別途確認している) と、ファイルの登録が `registerOPFSFileName` ではなく `registerFileBuffer` であることである。つまり OPFS への書き込みと `opfs://` パスでの読み取りは、この確認でも通っていない。

再現手順は次のとおりである。

1. サンプルを作る。`duckdb :memory:` で次を実行し、`sample.jsonl` を `sample.ndjson` にコピーし、`.gz` の 5 形式を `gzip -k` で作る (計 11 ファイル)。

   ```sql
   CREATE TABLE sample AS
     SELECT i AS id, 'name-' || i AS name, i * 1.5 AS value
     FROM range(1, 51) t(i);
   COPY sample TO 'sample.csv' (FORMAT csv, HEADER);
   COPY sample TO 'sample.tsv' (FORMAT csv, HEADER, DELIMITER '\t');
   COPY sample TO 'sample.json' (FORMAT json, ARRAY true);
   COPY sample TO 'sample.jsonl' (FORMAT json);
   COPY sample TO 'sample.parquet' (FORMAT parquet);
   ```

2. `lib/objectQuery.ts` を Node から読めるようにする。`import i18n from '../i18n';` の行だけを `const i18n = { t: (k: string) => k };` に差し替えて `.mts` として保存する (他の import は無いため、Node の型剥がしでそのまま読める)。
3. 検証スクリプトを書く。`globalThis.XMLHttpRequest` を、要求 URL を記録しつつ `frontend/public/` 配下の同名ファイルを返すスタブに差し替える。`duckdb-node-blocking` の `createDuckDB` に `node_modules/@duckdb/duckdb-wasm/dist/duckdb-eh.wasm` を渡して `instantiate` し、2 の SQL 生成関数で組み立てた SET 文、`INSTALL` / `LOAD`、`CREATE OR REPLACE VIEW obj AS SELECT * FROM <読み取り関数>`、`SELECT count(*) FROM obj` を 11 形式について順に実行する。
4. `cd frontend && node --experimental-strip-types <スクリプト>` で実行する。

### この差し戻しで直した不具合

レビューが挙げた実装の欠陥を修正した。再現するテストを足したものと、テストの無いものがある (各項に記す)。

- `OpfsIngestor.terminate()` が `this.worker` の有無だけで早期 return するため、`navigator.storage.estimate()` の待機中にアンマウントされると終了指示が空振りし、その後 `ingest()` が Worker を起動して取り込みを完走していた。`terminated` フラグを Worker の有無と切り離して立て、`ingest()` がクォータ確認の前後で見るようにした。`opfsIngest.test.ts` の 2 テストで固定した。
- `opfsWriter.worker.ts` が `createSyncAccessHandle()` の待機中に終了指示を受けても `abortController` がまだ無く fetch を中断できず、メインスレッドの 5 秒の強制終了まで転送が続いていた。`abortController` の設定直後に終了指示を見て、fetch を始めずに中断するようにした。Worker の本体は jsdom で動かせないため、この分岐のテストは無い。
- `runQuery` の打ち切り判定が、1 バッチが上限を超える場合に残り行を捨てたうえで次バッチの有無だけで打ち切りを決めていた。判定を `duckdbResult.ts` の `collectQueryResult` に切り出し、バッチ内で上限に達した場合は先読みせず打ち切りとするようにした。切り出しにより、打ち切りの判定を単体テストで固定できるようになった。
- `cleanupStaleObjectQueryFiles` の `catch` がループの外にあり、1 ファイルの削除失敗で残りの走査が止まっていた。エントリごとの `try` / `catch` にした。`opfs.test.ts` の 1 テストで固定した。
- `getDuckDB` / `getConnection` が失敗したプロミスを保持し続けるため、Wasm の読み込みが一時的に失敗するとページを再読み込みするまで再試行できなかった。失敗時にキャッシュを消すようにした。`duckdb.test.ts` の 1 テストで固定した。
- `scripts/fetch-duckdb-extensions.mjs` が版の更新時に古い `<revision>/` を消さず、旧版のバイナリが `public/` と `dist/` に残り続けていた。取得時に他の版のディレクトリを削除するようにした。取得スクリプトのテストは無い。

### この差し戻しで追加したテスト

- `lib/opfsWriterLimits.ts` (新規) に Worker の `Content-Length` の解釈、上限判定、3 種の例外判定を純関数として切り出し、`opfsWriterLimits.test.ts` の 9 テストで固定した。Worker は i18n を読む `objectQuery.ts` を import できないため、共有する純関数を別モジュールに置いた。
- `opfsIngest.test.ts` (新規) の 14 テストで、`Worker` のスタブによる取り込みの往復、クォータ確認中の終了指示、終了済みのインジェスタへの依頼、`ApiError` への変換、`ObjectQueryIngestError` の 4 種を固定した。
- `duckdbResult.test.ts` に `collectQueryResult` のテストを足し、打ち切りなし、バッチ途中での超過、上限ちょうどで次バッチあり / なし、空バッチ、0 バッチについて、cancel コールバックの呼び出しと先読みの回数を検証した。
- `opfs.test.ts` に 1 件の削除失敗で走査が止まらないテストを足した。
- `DrawerObjectQuery.test.tsx` に `unsupported` / `quota` / `opfs` の表示と、`Content-Length` の無い取り込みの表示の 4 テストを足した。
- `DrawerS3Objects.test.tsx` に Query ボタンから SQL 検索のパネルへ切り替わるテストを足した。`DrawerGCSObjects.test.tsx` (新規) に GCS 側でも押下可否が S3 と同じ条件で決まるテストを足した。
- `handlers_config_test.go` を `registerRoutes` と `mux.ServeHTTP` を通す形に変え、`GET /api/config` の経路の登録と、それ以外のメソッドが 405 になることを検証するようにした。

### 2 ラウンド目のレビューで直したもの

- SQL 検索のパネルを開いたまま region を切り替えると、解放されないままになっていた。`selectedId` は service の切り替えでしかリセットされず、バケットの一覧は region を問わず同じ内容を返すため、選択も Drawer もそのまま残る。切り替え先の一覧がキャッシュにあると `DrawerObjectQuery` はアンマウントされず、OPFS のファイル、Web Locks のロック、`obj` ビューが残っていた。完了条件の「region の切り替えのいずれでも解放される」を満たしていなかったため、`DrawerS3Objects` が `DrawerObjectBrowser` に `key={region}` を渡して作り直すようにした。`DrawerS3Objects.test.tsx` の 1 テストで固定した。
- `collectQueryResult` の先読みが次のバッチの行数を見ておらず、上限ちょうどで終わるクエリの末尾に 0 行のバッチが続くと、打ち切っていないのに打ち切りの表示が出ていた。行のあるバッチが出るまで読み進めるようにした。`duckdbResult.test.ts` の 2 テストで固定した。
- 完了条件が求める「`cancelSent()` が呼ばれることをモックで検証するテスト」が無かった。`collectQueryResult` に渡すコールバックの検証にとどまり、`runQuery` から `conn.cancelSent()` への配線は通っていなかった。`duckdb.test.ts` を新設し、`@duckdb/duckdb-wasm` をスタブに差し替えて `objectQueryEngine.run` が打ち切り時に接続の `cancelSent` を 1 回呼ぶこと、上限以下では呼ばないこと、初期化に失敗しても次の呼び出しでやり直せることを固定した。


### 3 ラウンド目のレビューで直したもの

追加レビュー 2 回目の指摘を反映した。実装を変えたものは次のとおりである。

- `obj` ビューの名前がパネルをまたいで共有されており、閉じたパネルの解放が後から開いたパネルのビューを消しうる状態だった。`DrawerObjectQuery` の直列化キューは `useRef` でインスタンスごとに独立している一方、接続と `obj` ビューは `duckdb.ts` のモジュール共有である。旧パネルの解放は実行中クエリのキャンセルの完了を待ってから `DROP VIEW` を出すため、その待ちの間に次のパネルが `CREATE VIEW obj` を終えると、遅れて届いた `DROP VIEW` が新しいビューを消す。次の Run が `Table with name obj does not exist` で失敗する。`ObjectQueryEngine.dropView` に OPFS パスを渡す形に変え、`duckdb.ts` が最後に登録したパスと一致するときだけ `DROP VIEW` を出すようにした。`registerView` は `CREATE OR REPLACE VIEW` のままとし、登録時にパスを記録する。
- `checkQuota` が `estimate.quota ?? 0` としており、`StorageEstimate` に値が入らない環境で「空き容量 0」と判定して取り込みを止めていた。`estimate` 自体が失敗した場合と同じく、値が欠けていればチェックを飛ばすようにした。
- `getDuckDB` の再試行で、初期化に失敗した `AsyncDuckDB` の Worker を終了させていなかった。`@duckdb/duckdb-wasm` の `instantiate` は失敗時に後始末をしないため、Wasm の読み込みが失敗し続けるとパネルを開くたびに Worker が増える。`instantiateDuckDB` で捕捉して `db.terminate()` を呼ぶようにした。
- 本番コードから呼ばれていない `isObjectQueryEligible` を削除した。Query ボタンの活性判定は `objectQueryDisabledReason` の空文字判定を直接使っている。

記録の訂正である。

- 「## レビュー指摘の反映と再検証 (2026-09-25)」の「正しくは 1 バッチ先読みである」は、2 ラウンド目の修正で成立しなくなった。現在の `collectQueryResult` は上限ちょうどで止まったとき、行のあるバッチが出るまで読み進める (0 行のバッチが続く場合は複数バッチを引く)。
- 同節の「引数を固定しているのは `registerView` と `engine.run` と `dropFile` の一部だけ」は、本ラウンドで `ingestor.ingest` への要求 (`url` / `storageName` / `lockName` / `maxBytes` / `size`) の検証を追加したため、現在は当たらない。
- 11 形式の再検証と実際のブラウザの差は 2 点である。バイト列の取得元がダウンロード API ではなくローカルのファイルであること、登録が `registerOPFSFileName` ではなく `registerFileBuffer` であること。Worker が OPFS へ書き、`opfs://` パスで DuckDB に読ませる本番の経路は、自動テスト、Node の再検証、実ブラウザのいずれでも通っていない。
- `DrawerS3Objects` の `key={region}` は `DrawerObjectBrowser` を作り直すため、region の切り替えで prefix の入力、選択中のアップロードファイル、開いている Preview もリセットされる。region が変われば一覧の内容も変わるため、意図した挙動として記録する。

### 3 ラウンド目で追加したテスト

- `opfsWriter.worker.test.ts` (新設、6 テスト)。jsdom で `navigator.storage` / `navigator.locks` / `FileSystemFileHandle` / `fetch` / `postMessage` / `close` をスタブし、Worker 本体の分岐を固定した。ロックを OPFS のファイル作成より前に取ること、終了指示でファイルを削除して閉じること、`createSyncAccessHandle` の待機中に終了指示を受けたら fetch を始めずに中断すること (1 ラウンド目で直した分岐)、HTTP エラーを status と code 付きで返すこと、`Content-Length` 超過では本文を読まないこと、累積超過でファイルを削除すること、`createSyncAccessHandle` の無いブラウザで `unsupported` を返して閉じることを含む。
- `duckdb.test.ts` に 3 テストを追加した。別のオブジェクトのビューに置き換わっていたら `dropView` が何もしないこと、`registerView` が `CREATE OR REPLACE VIEW` を出すこと、接続の取得に失敗しても次の呼び出しでやり直せること。`instantiate` の失敗時に `terminate` を呼ぶことも既存のテストに追加した。
- `DrawerObjectQuery.test.tsx` に 2 テストを追加した。`ingestor.ingest` へ渡す要求の中身 (URL、OPFS パス、ロック名、上限、サイズ) と、解放の途中の段階が失敗しても後続の段階と Worker の終了指示まで到達すること。
- `opfsIngest.test.ts` に 1 テストを追加した。`estimate` に `quota` / `usage` が無い場合に取り込みを続けること。
- `scripts/fetch-duckdb-extensions.test.mjs` (新設、2 テスト)。`removeOtherRevisions` を一時ディレクトリに対して実行し、現行のリビジョンを残して他を削除することを固定した。テストから import しても取得が始まらないよう、スクリプトは直接実行されたときだけ `main` を呼ぶ形に変えた。

追加した 14 テストは、いずれも対応する修正を元に戻すと失敗することを確認した。

### 3 ラウンド目でも残る未検証の範囲

- Worker が OPFS へ書き、`registerOPFSFileName` で DuckDB に読ませる経路。`opfsWriter.worker.test.ts` は `createSyncAccessHandle` をスタブに置き換えているため、実際の OPFS への書き込みは通っていない。
- Web Locks による他タブとの排他、起動時の残骸削除の実動作、Drawer の UI 操作。
- ブラウザの自動操作による確認は、ブラウザの起動が承認されなかったため行っていない。

## 3 ラウンド目のレビュー指摘の反映 (2026-09-25)

3 ラウンド目 (追加レビュー 2 回目、本 issue で最後のラウンド) の 3 観点の指摘を反映した。本スキルの追加レビューの上限に達しているため、以降のレビューは行わない。

### 直した不具合

- `dropView` のパス照合が `CREATE OR REPLACE VIEW` の実行中に効かず、他のパネルのビューを消しうる欠陥があった。`registeredViewPath` は CREATE の応答を受け取ってから更新していたため、旧パネルの解放がその往復の間に照合を通り、作られた直後のビューに `DROP VIEW` を出していた。記録の対象は登録の開始時に移した。`duckdb.test.ts` に、CREATE を保留させたまま旧パネルの `dropView` を呼ぶテストを追加した (記録の位置を元に戻すと失敗する)。

### 直したテストと記述

- `duckdb.test.ts` に「後始末の `terminate` が失敗しても、初期化のエラーをそのまま伝える」を追加した。`terminate` の失敗を握り潰して元のエラーを再送出する分岐は、これまで実行されていなかった。
- `opfsWriter.worker.test.ts` に 2 テストを追加した。書き込み中の `QuotaExceededError` を `quota` として返してファイルを削除すること、取り込み中の終了指示で fetch を中断してファイルを削除し閉じること。あわせてロックの検証を「ロックの取得が最初の操作である」ことまで固定した (以前は OPFS の操作より前であることしか見ていなかった)。
- `DrawerObjectQuery.test.tsx` の解放のテストで、`dropView` に渡す OPFS パスが `registerView` に渡したものと一致することを固定した。以前は `dropFile` の引数しか固定しておらず、`dropView` に別の文字列を渡しても落ちなかった。
- `fetch-duckdb-extensions.mjs` の `removeOtherRevisions` のコメントが「ディレクトリを削除する」と書いていたが、`rmSync` はエントリの種別を問わない。コメントを実装に合わせた。

### 記録の訂正

- 「この差し戻しで直した不具合」の「Worker の本体は jsdom で動かせないため、この分岐のテストは無い」(`createSyncAccessHandle` の待機中の終了指示) は、3 ラウンド目に追加した `opfsWriter.worker.test.ts` で成立しなくなった。同じ節の「取得スクリプトのテストは無い」も、`fetch-duckdb-extensions.test.mjs` の追加で成立しなくなった。
- 「3 ラウンド目で追加したテスト」の「追加した 14 テストは、いずれも対応する修正を元に戻すと失敗することを確認した」は、対象の範囲が読み取れない書き方だった。実際に元に戻して失敗を確認したのは、3 ラウンド目に変更した実装に対応する 8 項目 (`dropView` のパス照合、`instantiateDuckDB` の `terminate`、接続キャッシュの破棄、`checkQuota` の欠損値スキップ、`DrawerS3Objects.test.tsx` の取り込みモックによる隔離、`removeOtherRevisions` の絞り込み、`CREATE OR REPLACE VIEW`、取り込み要求の `lockName`) と、本節で追加した Worker の中断分岐および `dropView` の実行中の照合である。残りのテストは 1 ラウンド目と 2 ラウンド目の変更、または初期実装を対象としており、3 ラウンド目の検証の対象ではない。
- 3 ラウンド目の変更一覧に `DrawerS3Objects.test.tsx` への `vi.mock('../../lib/opfsIngest')` の追加が漏れていた。Query ボタンのテストで実インジェスタを動かさないようにした変更であり、これにより同ファイルの Query 関連テストは取り込み処理を通らなくなった (取り込み後の表示は `DrawerObjectQuery.test.tsx` が担う)。
- 「3 ラウンド目で追加したテスト」の件数は本節の追加で変わった。`opfsWriter.worker.test.ts` は 8 テスト、`duckdb.test.ts` の追加は 5 テストとなる。
- 「この差し戻しで追加したテスト」の「`opfsIngest.test.ts` (新規) の 14 テストで」は、3 ラウンド目の 1 テスト追加で 15 テストになった (検証する対象そのものは変わらない)。
- `lib/duckdb.ts` の冒頭のコメントと `DrawerS3Objects.tsx` の `key={region}` のコメントを書き直した。前者は「クエリと obj ビューの名前も 1 つだけ」と書いていたがクエリに名前は無く、共有しているのは接続 1 つとビュー名 `obj` である。`registeredViewPath` が防ぐのも競合全般ではなく、古いパネルの解放が新しいパネルのビューを消すことだけである。後者は「region が変われば一覧の内容も変わる」がバケット一覧とオブジェクト一覧を同じ語で指していたため、region がオブジェクト一覧の取得条件 (`queryKey`) に入ることを根拠とする書き方に改めた。

### なお残る未検証の範囲

- `fetch-duckdb-extensions.mjs` の `main()` から `removeOtherRevisions(OUTPUT_DIR, DUCKDB_EXTENSION_REVISION)` を呼ぶ経路。関数自体はテストがあるが、`main()` が正しい引数で呼ぶことは、取得が実ネットワークを伴うため固定していない。
- `OpfsIngestor.terminate()` の 5 秒の強制終了と、その後の `removeObjectQueryFile` による後始末。`DrawerS3Objects.test.tsx` がインジェスタをモックしたため、この経路を通る偶発的なカバレッジも無くなった。
- `readErrorBody` の JSON 解析失敗時のフォールバックと、`removeEntry` の `NotFoundError` の分岐。

## 完了条件

- `frontend/package.json` の `dependencies` に `@duckdb/duckdb-wasm` が `1.32.0` (範囲指定なし) と `apache-arrow` が `^17.0.0` で追加され、この 2 つ以外の新規依存が増えていない。
- `THIEF_OBJECT_QUERY_MAX_BYTES` が未設定のとき `config.Config.ObjectQueryMaxBytes` は `1073741824` になる。整数以外または 0 以下の値を設定すると `config.Load()` がエラーを返す。
- `GET /api/config` が `{"object_query_max_bytes": <ObjectQueryMaxBytes>}` を返す。
- `ClientConfigResponse` が `contract.go` の `Registry` に登録され、`contract_test.go` の件数の固定値が 65 になり、ゴールデン JSON `frontend/src/types/__contract__/ClientConfigResponse.json` が生成され、`backend/internal/contract/testdata/tags.golden` が再生成され、`contract.check.ts` の `ContractChecks` に `ClientConfigRaw` との対が追加されている。`types/common.ts` に `ClientConfigRow`、`lib/normalize.ts` に `clientConfigFromRaw` がある。
- `api/endpoints.ts` に `getClientConfig`、`api/queries.ts` に `useClientConfig` がある。
- S3 と GCS の Objects タブの各行に Query ボタンがあり、`objectQueryFormat` が対象と判定し、`size` が上限以下で、設定の取得が完了し、設計判断 9 のメインスレッドの 2 つの機能検出が両方真の行だけ押せる。押せない行は `title` に理由 (対象外の拡張子、上限超過、設定の取得中、設定の取得失敗、ブラウザ非対応のいずれか) を出す。
- Query ボタンを押すと取り込みが始まり、進捗が表示され、完了後に `SELECT * FROM obj LIMIT 100` の結果が `ResultTable` に表示される。
- 設計判断 5 の表の全形式 (csv、csv.gz、tsv、tsv.gz、json、json.gz、jsonl、jsonl.gz、ndjson、ndjson.gz、parquet の 11 種) について、floci 環境の S3 に置いたサンプルに対して `SELECT count(*) FROM obj` がサンプルの行数と一致する件数を返す。
- レスポンスの `Content-Length` が上限を超える場合、および累積バイト数が上限を超えた場合は取り込みが中断され、エラーが表示され、OPFS に `thief-query/` 配下のファイルが残らない。
- `DrawerObjectBrowser` に省略可能な `profile` props があり、`DrawerS3Objects` だけが渡す。ダウンロード API が 401 `SSO_TOKEN_EXPIRED` を返したとき (S3) は `SSOExpiredBanner` が表示され、それ以外の HTTP エラーとネットワーク到達不能は `ErrorBanner` に `ApiError` のメッセージが表示される。
- `DrawerObjectQuery` の Close ボタン、Drawer の X ボタン、Objects タブから他タブへの切り替え、region の切り替えのいずれでも、OPFS のファイルが削除され、Web Locks のロックが解放され、DuckDB のファイル登録と `obj` ビューが解除される。frontend の起動時に、他タブがロックを保持していない `thief-query/` 配下のファイルが削除され、ロックを保持しているファイルは残る。設計判断 9 のメインスレッドの機能検出が偽のブラウザでは起動時処理が何もせず例外も出ない。Worker 内で `createSyncAccessHandle` が無い場合は `ErrorBanner` に非対応のメッセージが出る。
- 取り込み前の `navigator.storage.estimate()` で `quota - usage` がオブジェクトの `size` 未満のときは取り込みを開始せずエラーが表示される。書き込み中の `QuotaExceededError` ではエラーが表示され、`thief-query/` 配下にファイルが残らない。
- 結果が 10,000 行を超えるクエリでは 10,000 行で打ち切られ、打ち切りの表示が出る。`cancelSent()` が呼ばれることをモックで検証するテストがある。
- `duckdb-eh.wasm` と `duckdb-browser-eh.worker.js` (実装時の確認で別ファイルの拡張が必要と分かった場合はそのファイルも) が frontend のビルド成果物 (`dist/`) に含まれ、起動から SQL 実行までの間に Google Fonts 以外の外部ホストからサブリソースを読み込まない。
- 新規の UI 文言 (Query ボタンのラベルを除く) が `locales/ja/drawerStorage.json` と `locales/en/drawerStorage.json` の両方にある。
- 設計判断 10 の単体テストとコンポーネントテスト、backend のテストが追加されている。
- `mise run check` が通過する。
- 扱わない範囲: クエリ結果のグラフ表示 (docs/issues/0197)、複数オブジェクトの横断検索、`.log` / `.avro` / `.orc`、取り込んだファイルの再利用 (ETag / generation による一致確認)、設定ファイル (YAML) からの上限の指定、プレビュー機能の対象拡張子の変更、CLI への同等機能の追加。

## 関連

- docs/issues/0197: 同じ TODO 項目の「可視化」を扱う。本 issue の `DrawerObjectQuery` と結果の `string[][]` を前提にする。
- docs/issues/closed/0025: プレビュー機能。オブジェクトブラウザのアクション追加と対象判定の純関数の構造を踏襲する。
- docs/issues/closed/0039: プレビューからの編集。`DrawerObjectPreview` によるブラウザ本体の置き換え表示の前例。
- docs/issues/closed/0066: Drawer のタブ名を英語ハードコードにする方針。

## 解決方法

S3 / GCS のオブジェクトを、ブラウザ内の DuckDB Wasm と OPFS で SQL 検索できるようにした。方式は設計判断 1 から 10 と「## 実装中の調査結果と設計判断 2 の追記 (2026-09-25)」「## 実装詳細の乖離と実環境での確認 (2026-09-25)」「## レビュー指摘の反映と再検証 (2026-09-25)」のとおりである。

### 変更内容

backend:

- `internal/config/config.go`: `ObjectQueryMaxBytes` を追加し、既定値 `DefaultObjectQueryMaxBytes = 1 << 30` (1073741824) を置いた。`THIEF_OBJECT_QUERY_MAX_BYTES` で上書きでき、整数以外と 0 以下は `Load()` がエラーを返す。
- `internal/api/handlers_config.go` (新規): `GET /api/config` が `{"object_query_max_bytes": <値>}` を返す。`internal/api/routes.go` に経路を足した。
- `internal/contract/contract.go`: `ClientConfigResponse` を `Registry` に登録した (65 件)。`testdata/tags.golden` とゴールデン JSON `frontend/src/types/__contract__/ClientConfigResponse.json` を再生成した。

frontend:

- `package.json`: `@duckdb/duckdb-wasm` を `1.32.0` (範囲指定なし)、`apache-arrow` を `^17.0.0` で追加した。この 2 つ以外の新規依存は無い。
- `lib/objectQuery.ts` (新規): 対象拡張子と読み取り関数の表 (11 形式)、SQL の生成、パスとロック名の組み立て、上限とクォータの判定、行アクションの活性判定を置いた。`isObjectQuerySupported` が `navigator` の機能検出を行い、`ObjectQueryIngestError` を定義するほかは純関数である。
- `lib/opfsWriterLimits.ts` (新規): Worker が使う `Content-Length` の解釈と上限判定、例外の種別判定。Worker は i18n を読む `objectQuery.ts` を import できないため別モジュールにした。
- `lib/opfs.ts` / `lib/opfsIngest.ts` / `lib/opfsWriter.worker.ts` (新規): OPFS への書き込みを Worker で行い、Web Locks (`thief-query:<ファイル名>`) で排他する。起動時に、ロックを保持していない `thief-query/` 配下のファイルを削除する。
- `lib/duckdb.ts` / `lib/duckdbResult.ts` (新規): DuckDB Wasm の初期化、拡張の読み込み、クエリ実行、Arrow から `string[][]` への変換を行う。10,000 行の打ち切りと `cancelSent` の判定は `collectQueryResult` に切り出した。`obj` ビューは `registeredViewPath` で登録元の OPFS パスを持ち、`dropView(opfsPath)` は一致するときだけ `DROP VIEW` を出す (閉じたパネルの解放が、後から開いたパネルのビューを消さないようにする)。
- `components/Drawer/DrawerObjectQuery.tsx` (新規): 取り込みの進捗、SQL の入力と実行、結果の表、エラー表示を行う。
- `components/Drawer/DrawerObjectBrowser.tsx`: Actions 列に Query ボタンを足し、`objectQueryDisabledReason` で押せない理由を `title` に出す。省略可能な `profile` props を足し、`DrawerS3Objects.tsx` だけが渡す。
- `main.tsx`: 起動時に `cleanupStaleObjectQueryFiles()` を呼び、前回のタブが残した `thief-query/` のファイルを削除する (ロックを保持しているファイルは残す)。
- `components/Drawer/DrawerS3Objects.tsx`: `DrawerObjectBrowser` に `key={region}` を渡し、region が変わったら作り直す。バケットの一覧は region を問わず同じため選択が残り、SQL 検索のパネルが開いたままだと解放されないためである。作り直しにより prefix の入力、選択中のアップロードファイル、開いている Preview もリセットされる (region はオブジェクト一覧の取得条件に入るため、選択とパネルを残さないことを意図した挙動とする)。
- `api/endpoints.ts` の `getClientConfig`、`api/queries.ts` の `useClientConfig`、`types/common.ts` の `ClientConfigRow`、`lib/normalize.ts` の `clientConfigFromRaw`、`types/contract.check.ts` の対を足した。
- `scripts/fetch-duckdb-extensions.mjs` (新規) と `mise.toml` の `frontend:fetch-duckdb-extensions`: json と parquet の拡張を `public/assets/duckdb-extensions/v1.4.3/wasm_eh/` に取得する。取得時に他の版のディレクトリを削除する (`removeOtherRevisions`)。テストから import しても取得が始まらないよう、直接実行されたときだけ `main` を呼ぶ。`mise run setup` の依存に入れ、`frontend/.gitignore` で取得物を除外した。
- `i18n/locales/{ja,en}/drawerStorage.json`: `objectQuery.*` と `drawerObjectQuery.*` を両方に足した。
- `tsconfig.json` に `DOM.AsyncIterable`、`vite.config.ts` に Worker の alias、`setupTests.ts` に Worker のスタブを足した。

テスト (新規): `lib/objectQuery.test.ts`、`lib/opfsWriterLimits.test.ts`、`lib/opfs.test.ts`、`lib/opfsIngest.test.ts`、`lib/opfsWriter.worker.test.ts`、`lib/duckdb.test.ts`、`lib/duckdbResult.test.ts`、`components/Drawer/DrawerObjectQuery.test.tsx`、`components/Drawer/DrawerGCSObjects.test.tsx`、`scripts/fetch-duckdb-extensions.test.mjs`。既存の `DrawerS3Objects.test.tsx` にも追加した。backend は `internal/api/handlers_config_test.go` (新規) と `internal/config/config_test.go` に追加した。

### 完了条件の検証

- 依存: `package.json` の差分は 2 行で、`@duckdb/duckdb-wasm@1.32.0` と `apache-arrow@17.0.0` だけが増えた。
- 環境変数と既定値: `config_test.go` の `TestObjectQueryMaxBytes` (既定 / 上書き / 整数以外 / 0 / 負値) で検証した。
- `GET /api/config`: `handlers_config_test.go` の `TestHandleClientConfig` (`registerRoutes` と `mux.ServeHTTP` を通す) と `TestHandleClientConfigMethodNotAllowed` で検証した。floci 環境の実サーバでも `{"object_query_max_bytes":1073741824}` を確認した。
- 契約: `contract_test.go` の件数の固定値が 65 になり、`ClientConfigResponse.json` と `tags.golden` を再生成した。`contract.check.ts` の `ContractChecks` に対を足し、`tsc --noEmit` が通る。
- API 経路: `endpoints.ts` の `getClientConfig` と `queries.ts` の `useClientConfig` がある。
- Query ボタンの押下可否と理由: `DrawerS3Objects.test.tsx` の 4 テスト (5 ケース。対象外の拡張子と上限超過が 1 テストに同居する) で検証した。GCS 側でも同じ条件で出し分けることを `DrawerGCSObjects.test.tsx` の 1 テストで検証した。
- 取り込みから結果表示まで: `DrawerObjectQuery.test.tsx` の「取り込み中の進捗を Content-Length 付きで表示する」「取り込み完了後に obj ビューを作成して初期クエリの結果を表に表示する」と、Query ボタンから SQL 検索のパネルへ切り替わる `DrawerS3Objects.test.tsx` のテストで検証した。
- 11 形式の `SELECT count(*)`: 50 行のサンプル 11 形式について、実 `duckdb-eh.wasm` に `lib/objectQuery.ts` が生成する SQL をそのまま流し、`CREATE VIEW obj` の後の `SELECT count(*) FROM obj` が 11 形式すべてで 50 を返すことを確認した。拡張は `public/assets/duckdb-extensions/v1.4.3/wasm_eh/` の実ファイルを同一オリジン配信のスタブから読ませた。実際のブラウザとの違いは 2 つで、バイト列の取得元がローカルのファイルであること (ダウンロード API が 11 形式を元と同じバイト数で返すことは別途確認した) と、登録が `registerOPFSFileName` ではなく `registerFileBuffer` であることである。OPFS への書き込みと `opfs://` パスでの読み取りはこの確認でも通っていない。経路と再現手順は「## レビュー指摘の反映と再検証 (2026-09-25)」に書いた。
- 上限とクォータ: `objectQuery.test.ts` の `objectQueryOverLimit` / `objectQueryQuotaExceeded` と、`opfsWriterLimits.test.ts` の 9 テスト (`Content-Length` の解釈、上限ちょうどの境界、`QuotaExceededError` を含む 3 種の例外判定)、`opfsIngest.test.ts` のクォータ不足で Worker を起動しないテストで検証した。
- SSO バナー: `DrawerObjectBrowser` の `profile` は `DrawerS3Objects` だけが渡し、401 `SSO_TOKEN_EXPIRED` は `SSOExpiredBanner`、それ以外は `ErrorBanner` に出ることを 3 テストで検証した。`opfsIngest.test.ts` でも 401 と到達不能が `ApiError` に変換されることを検証した。
- 解放と起動時の残骸削除: `DrawerObjectQuery.test.tsx` のアンマウントの 2 テストで `cancelSent` / `dropView` / `dropFile` / Worker の終了指示が呼ばれることを、`opfsIngest.test.ts` の 2 テストでクォータ確認中と終了後の依頼で Worker を起動せず中断することを、`opfs.test.ts` の 7 テストで起動時の削除とロック保持時の残置、1 件の失敗で走査が止まらないことを検証した。機能検出が偽のときに何もせず例外も出ないこと、Worker の `unsupported` / `quota` / `opfs` が `ErrorBanner` に出ることを含む。region の切り替えでも解放されることは、`DrawerS3Objects.test.tsx` の「SQL 検索のパネルを開いたまま region を変えると一覧に戻る」で検証した (`DrawerObjectBrowser` を `key={region}` で作り直す)。`ingestor.ingest` へ渡す要求 (`url` / `storageName` / `lockName` / `maxBytes` / `size`) は `DrawerObjectQuery.test.tsx` の 1 テストで固定した。解放の途中の段階が失敗しても後続の段階と Worker の終了指示まで到達することも 1 テストで固定した。呼び出しの順序そのものは検証していない (回数と引数のみ)。`obj` ビューが別のオブジェクトのものに置き換わっていたら `dropView` が何もしないことは `duckdb.test.ts` で固定した (登録の完了後だけでなく、`CREATE OR REPLACE VIEW` の実行中も何もしないことを含む)。`dropView` に渡す OPFS パスが `registerView` に渡したものと一致することも固定した。`DrawerS3Objects.test.tsx` では、Query ボタンのテストが実インジェスタを動かさないよう `lib/opfsIngest` をモックしている (取り込み後の表示は `DrawerObjectQuery.test.tsx` が担う)。
- Worker 本体の分岐: `opfsWriter.worker.test.ts` の 8 テストで、ロックの取得が最初の操作であること、終了指示でファイルを削除して閉じること、`createSyncAccessHandle` の待機中に終了指示を受けたら fetch を始めずに中断すること、HTTP エラーを status と code 付きで返すこと、`Content-Length` 超過では本文を読まないこと、累積超過でファイルを削除すること、`createSyncAccessHandle` の無いブラウザで `unsupported` を返して閉じること、書き込み中の `QuotaExceededError` を `quota` として返してファイルを削除すること、取り込み中の終了指示で fetch を中断してファイルを削除し閉じることを固定した。
- 10,000 行の打ち切り: `duckdb.test.ts` (8 テスト) のうち 2 テストで、`@duckdb/duckdb-wasm` をスタブに差し替えて `objectQueryEngine.run` が打ち切り時に接続の `cancelSent` を 1 回呼び、上限以下では呼ばないことをモックで検証した。残りの 6 テストはビューの登録と解放、接続と初期化の再試行、初期化の失敗時の `terminate` を対象としている。行の収集と打ち切りの判定そのものは `duckdbResult.test.ts` の `collectQueryResult` の 8 テスト (上限ちょうど、バッチ途中での超過、末尾の 0 行バッチを含む) で固定した。表示は `DrawerObjectQuery.test.tsx` の「10000 行で打ち切ったときは打ち切りの案内を出す」で検証した。
- ビルド成果物と外部読み込み: `npm run build` の `dist/assets/` に `duckdb-eh.wasm`、`duckdb-browser-eh.worker.js`、`opfsWriter.worker.js`、`duckdb-extensions/v1.4.3/wasm_eh/{json,parquet}.duckdb_extension.wasm` が入る。`dist/index.html` の外部読み込みは Google Fonts だけである。実行時に外部ホストへ取りに行かないことの根拠は、初期化の SET 文 (`autoinstall_known_extensions` と `autoload_known_extensions` を false、`custom_extension_repository` を同一オリジンに固定) と、上の 11 形式の確認で観測した要求先が `/assets/duckdb-extensions/v1.4.3/wasm_eh/` の 2 ファイルだけで `extensions.duckdb.org` への要求が 0 件だったことである。
- i18n: `locales/ja/drawerStorage.json` と `locales/en/drawerStorage.json` の両方に新規の文言がある。
- `mise run check`: exit 0。frontend は 110 ファイル / 1191 件が pass し、backend は `-race` 付きで全パッケージが ok。lint は 0 errors / 9 warnings で、warning は既存の行だけである。ベースライン (実装前の作業ツリーで 104 ファイル / 1131 件、失敗なし) から新たな失敗は無い。今回の追加テストは、いずれも対応する修正を元に戻すと失敗することを確認した。
- 扱わない範囲: クエリ結果のグラフ表示 (docs/issues/0197)、複数オブジェクトの横断検索、`.log` / `.avro` / `.orc`、取り込んだファイルの再利用、設定ファイルからの上限指定、プレビュー機能の対象拡張子の変更、CLI への同等機能の追加は実装していない。

### 自動テストで担保していない範囲

OPFS への実際の書き込みと `opfs://` パスでの読み取り (`registerOPFSFileName` 経由)、Web Locks による排他、起動時の残骸削除、Drawer の UI 操作は、jsdom が OPFS と Web Locks を持たないためスタブで置き換えている。ブラウザの自動操作による確認は、ブラウザの起動が承認されなかったため行っていない。11 形式の確認も、エンジンと SQL と拡張は実物だが、バイト列の取得元がローカルのファイルであり、登録もバッファ登録に代えている。`opfsWriter.worker.test.ts` は `createSyncAccessHandle` をスタブに置き換えているため、Worker が OPFS へ書いて `registerOPFSFileName` で読ませる本番の経路は、自動テスト、Node の再検証、実ブラウザのいずれでも通っていない。詳細は「## 実装詳細の乖離と実環境での確認 (2026-09-25)」と「## レビュー指摘の反映と再検証 (2026-09-25)」に書いた。

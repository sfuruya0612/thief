# オブジェクトの SQL 検索で、複数のオブジェクトを 1 つのビューにまとめて検索する

Created: 2026-09-28
Model: Claude Fable 5.1
Completed: 2026-09-29

## 背景

TODO.md の次の項目に由来する。

> Object Storage の DuckDB によるクエリを複数ファイルに対してかけられるようにしたい

現状のオブジェクト SQL 検索 (issue 0196、ADR 0018) は、1 回の検索で 1 つのオブジェクトだけを扱う。

- `frontend/src/components/Drawer/DrawerObjectBrowser.tsx` の Query ボタンは行ごとに 1 つで、`setQueryRow(r)` で 1 行だけを `DrawerObjectQuery` に `fileName`、`url`、`size` として渡す。
- `frontend/src/components/Drawer/DrawerObjectQuery.tsx` は `crypto.randomUUID()` で OPFS のファイル名を 1 つ決め、`createIngestor()` で Worker を 1 つ起動し、`engine.registerView(opfsPath, readSql, extensions)` で `obj` ビューを作る。
  解放 (`teardown`) は `cancelSent`、`dropView`、`dropFile`、`ingestor.terminate()` の順で 1 ファイル分を行う。
- `frontend/src/lib/duckdb.ts` の `objectQueryEngine.registerView` は `db.registerOPFSFileName(opfsPath)` を 1 回呼び、`CREATE OR REPLACE VIEW obj AS SELECT * FROM <readSql>` を実行する。
  `registeredViewPath` は `obj` ビューの持ち主を OPFS パス 1 つで記録し、`dropView` は一致するときだけ DROP する。
  `frontend/src/lib/duckdb.test.ts` はこの持ち主の判定 (別のパスで `registerView` した後の `dropView` が DROP しない、CREATE の完了前の `dropView` が DROP しない) を OPFS パス 1 つの引数でテストしている。
- `frontend/src/lib/objectQuery.ts` の `objectQueryReadSql(opfsPath, format)` はパス 1 つを受け取る。
  `objectQueryDisabledReason(key, size, config, browserSupported)` は、ブラウザ非対応、設定の取得失敗、取得中、対象外の形式、上限超過の順に判定し、1 オブジェクトのサイズを `maxBytes` と比べる (`objectQueryOverLimit` は `size > maxBytes` で、ちょうどは可)。
- `frontend/src/lib/opfsIngest.ts` の `OpfsIngestor` は Worker 1 つ、ファイル 1 つ、Web Locks のロック 1 つを持つ。
  `ingest` は `ObjectIngestRequest.size` (一覧のサイズ) で空き容量を確かめてから (`checkQuota`) Worker を起動する。
  `frontend/src/lib/opfsWriter.worker.ts` は書き込み完了後もロックと OPFS のファイルを保持して終了指示を待ち、終了指示でファイルを削除して `self.close()` する。
  取り込みが終わった Worker を `terminate` すると、ファイルが消えて検索できなくなる。
  `frontend/src/lib/opfs.ts` の `cleanupStaleObjectQueryFiles` はファイル名からロック名を再構成し、ロックを取れたファイルだけを残骸として消す (ファイル単位で完結している)。
- backend の上限 `THIEF_OBJECT_QUERY_MAX_BYTES` は `backend/internal/config/config.go` の `ObjectQueryMaxBytes` で、`backend/internal/api/handlers_config.go` の `GET /api/config` が「1 オブジェクトのサイズ上限」として frontend に渡す。
  frontend 側の `frontend/src/types/common.ts` の `ClientConfigRaw` のコメントも「1 オブジェクトのサイズ上限」である。
- 共有部品 `frontend/src/components/DataTable.tsx` は、全行の先頭にチェックボックスの列を描き、チェック状態を内部 state `checked` に持つ。
  ヘッダのチェックボックスは列フィルタを通った全行を対象に全選択する。
  外から制御する props は無く、16 ファイル 18 か所の一覧が共有している (`DrawerELBTargets.tsx` と `DrawerELBListeners.tsx` は 1 ファイルに 2 か所ずつ)。
  Objects タブではこの列は何にも使われていない。

要望が満たされていない理由は、出力を複数のオブジェクトに分割するサービスが多いことにある。
Athena の CTAS や UNLOAD、BigQuery のエクスポート、Cloud Logging のシンクは、`part-00000.parquet`、`part-00001.parquet` のように出力を分割する。
1 つずつしか検索できないと、分割された出力の全体に対する集計ができない。

DuckDB 側の事実は次のとおりである。

- 同一オリジンで配信している拡張は v1.4.3 (`frontend/public/assets/duckdb-extensions/v1.4.3`) で、`@duckdb/duckdb-wasm` は 1.32.0 である。
- DuckDB の `read_csv`、`read_json_auto`、`read_parquet` は、ファイルのパスのリスト (`read_parquet(['a', 'b'])`) を受け取って 1 つの表として読む。
  `union_by_name = true` で列を名前で揃え、`filename = true` で由来のパスを `filename` 列として足す。
- `AsyncDuckDB.registerOPFSFileName(name)` はファイルを 1 つずつ登録し、`dropFile(name)` と `dropFiles(names)` で解除する (`node_modules/@duckdb/duckdb-wasm/dist/types/src/parallel/async_bindings.d.ts`)。

既存のテストは次の 4 つで、いずれも 1 オブジェクトが前提である。

- `DrawerObjectQuery.test.tsx` (エンジンと取り込みのモック)。
- `duckdb.test.ts` (`registerView` と `dropView` の持ち主の判定)。
- `DrawerS3Objects.test.tsx` と `DrawerGCSObjects.test.tsx` (Query ボタンの活性判定)。

## 目的

Objects タブで複数のオブジェクトを選び、1 回の SQL 検索に掛けられるようにする。
選んだ全オブジェクトが 1 つの `obj` ビューになり、由来のオブジェクトを列で区別できる。

## 設計判断

### 複数のファイルは読み取り関数のリスト引数で 1 つのビューにする

`obj` ビューの定義を、読み取り関数にパスのリストを渡す形にする。

```sql
CREATE OR REPLACE VIEW obj AS
SELECT * EXCLUDE (filename),
  CASE
    WHEN filename LIKE '%<uuid1>.parquet' THEN '<key1>'
    WHEN filename LIKE '%<uuid2>.parquet' THEN '<key2>'
  END AS object_key
FROM read_parquet(['opfs://thief-query/<uuid1>.parquet', 'opfs://thief-query/<uuid2>.parquet'], union_by_name = true, filename = true)
```

- 1 件の選択でも同じ形 (1 要素のリスト) にし、`object_key` 列を付ける。
- 由来の判定は `filename` の値の末尾が OPFS のファイル名 (`<uuid><拡張子>`) に一致するかで行う。
  DuckDB が `filename` 列に返す値の先頭部分 (スキームやディレクトリ) が登録した文字列と一致するかに依存しない。
  uuid はファイル間で一意なので誤一致しない。
- `object_key` の値は元のオブジェクトのキーで、SQL の文字列リテラルとして単一引用符を二重化する (`objectQueryReadSql` と同じ規則)。
- `union_by_name = true` により、分割された出力の一部で列が欠けていても読める。

採らなかった案。

- オブジェクトごとにビュー (`obj_1`、`obj_2`) を作り、利用者が `UNION ALL` を書く。
  分割された出力を 1 つの表として扱う要望に反し、既定の SQL (`SELECT * FROM obj LIMIT 100`) が使えない。
- `SELECT * FROM read_x('a') UNION ALL BY NAME SELECT * FROM read_x('b')` を組み立てる。
  parquet の並列スキャンと述語のプッシュダウンがファイルごとに分かれ、リスト引数に対する利点が無い。
- glob (`opfs://thief-query/*.parquet`) で読む。
  他のパネルや他のタブが同じディレクトリに置いたファイルまで読む。
- 由来の列を出さない。
  分割された出力でどのオブジェクトの行かを絞る手段が無くなる。
- `filename` の値と登録した文字列の完全一致で由来を判定する。
  DuckDB が返す値の形を確認していないため、一致しないと `object_key` が全て NULL になる。

### 同じ形式のオブジェクトだけを組み合わせられる

選べる組み合わせは、`ObjectQueryFormat` の `readFunction` と `delimiter` が全て一致するものだけとする。
`.csv` と `.csv.gz` の混在は許す (同じ `read_csv`)。
`.csv` と `.tsv`、`.csv` と `.parquet` の混在は拒否し、理由を表示する。

採らなかった案。

- 混在を許し、形式ごとに読んで `UNION ALL BY NAME` で結合する。
  csv の型推論と parquet の型が食い違い、エラーの説明が難しくなる。
  要望の主眼 (分割された同一形式の出力) から外れる。

### サイズの上限は選んだオブジェクトの合計に掛ける

`THIEF_OBJECT_QUERY_MAX_BYTES` の意味を「1 回の検索で取り込むオブジェクトの合計サイズの上限」に変える。
1 件の選択では合計がそのサイズなので、現状の判定と同じ結果になる。
判定は 4 か所で行う。

1. 選択の可否は、一覧が持つサイズの合計が `maxBytes` を超えるとき (`合計 > maxBytes`、ちょうどは可) に不可とする。
2. 取り込みを始める前に、`DrawerObjectQuery` が合計サイズで空き容量を 1 回確かめる (`opfsIngest.ts` に、`OpfsIngestor.checkQuota` と同じ判定を公開する関数を足す)。
   `OpfsIngestor.ingest` のファイルごとの確認 (`ObjectIngestRequest.size`) はそのまま残す (書き込みが進んで空きが減った後の確認になる)。
3. Worker には合計の上限 `maxBytes` をそのまま渡す (`OpfsWriterRequest.maxBytes` の意味は変えない)。
   1 ファイルが合計の上限を超えたら現状どおり `tooLarge` で止まる。
4. 全ファイルの取り込みが終わった時点で `written` の合計が `maxBytes` を超えていたら、`tooLarge` として全体を失敗させる (一覧のサイズが古い場合の保険)。

`config.go`、`handlers_config.go`、`types/common.ts` の `ClientConfigRaw`、`docs/prd/thief.md` の FR-17 の説明を合計の意味に直す。
JSON のフィールド名 `object_query_max_bytes` は変えない。

採らなかった案。

- 1 ファイルごとに上限を掛ける。
  1 GiB のオブジェクトを 10 個選べてしまい、OPFS の quota を超えて途中で失敗する。
- 上限の意味を変えるため `THIEF_OBJECT_QUERY_MAX_BYTES` を別の名前にする。
  1 件の検索では意味が変わらず、設定している利用者の環境変数を無効にする理由が無い。

### 選べる件数は 50 件まで

1 回の検索で選べるオブジェクトは `OBJECT_QUERY_MAX_FILES` (50) までとする。
取り込みが終わった Worker もファイルとロックを保持したまま解放まで生きるため (背景の `opfsWriter.worker.ts`)、50 件の検索では最大 50 個の Worker とロックが同時に存在する。
DuckDB の Worker も読み取り時にファイルごとに OPFS のハンドルを開く。
どちらもファイル数に比例して増えるため、件数を無制限にはしない。

採らなかった案。

- Worker を 1 つにして全ファイルを順に取り込む。
  取り込み 1 回で `self.close()` する Worker のメッセージ規約と、ロックの寿命 (ロック → ファイル削除 → `closed`) を作り直すことになる。
  件数の上限で足りる。
- 取り込みが終わった Worker を順に `terminate` する。
  ファイルが消えて検索できない。

### 取り込みは並列度 4 で行い、1 つ失敗したら全体を止める

- 同時に取り込むファイルは `OBJECT_QUERY_INGEST_CONCURRENCY` (4) までとし、残りは順番を待つ。
  取り込みの順番が来たファイルにだけ `createIngestor()` で ingestor を作る (待っているファイルには ingestor が無い)。
  backend は 1 リクエストにつき 1 ストリームを中継するだけなので、並列にしても backend の負荷は同時に開くストリームの数の分しか増えない。
- 取り込みが終わった ingestor は解放まで `terminate` しない。
- 進捗は全ファイルの `written` の合計と `total` の合計で表示する。
  1 つでも `Content-Length` が無いファイルがあれば、合計不明の表示 (`ingestingUnknownTotal`) にする。
- 1 つでも失敗したら、待っているファイルの取り込みを始めず、作成済みの全 ingestor を `terminate` し、最初に失敗したエラーを 1 つ表示する。
- アンマウント時の解放は作成済みの全 ingestor に対して行う。
  DuckDB 側の解放は `dropView` を 1 回、ファイルの登録解除を全ファイル分行い、その後に全 ingestor を `terminate` する。
  `enqueueDb` による直列化と、StrictMode の世代管理は現状の仕組みを保つ。

採らなかった案。

- 逐次 (並列度 1) にする。
  100 MiB のオブジェクト 20 件で待ち時間が線形に増える。

### インターフェースの変更

- `DrawerObjectQuery` の props を `files: ObjectQueryTarget[]` (`{ key, url, size }`) に変え、`fileName`、`url`、`size` を削除する。
  1 件でも配列で渡す。
  題名は 1 件なら `Query: <key>` (`drawerObjectQuery.title`)、複数なら件数の文言 (`drawerObjectQuery.titleMultiple`) にする。
- `ObjectQueryEngine.registerView(id, opfsPaths, viewSql, extensions)` は、全ファイルを `registerOPFSFileName` で登録し、拡張を INSTALL / LOAD してから `viewSql` を実行する。
  `obj` ビューの持ち主の記録 (`registeredViewPath`) はパネルの ID (`id`) に置き換え、`dropView(id)` は一致するときだけ DROP する。
  ファイルの登録解除は `dropFiles(opfsPaths)` の形にする。
- `objectQuery.ts` に次の純関数と定数を足す。
  - `objectQueryViewSql(files: { opfsPath, fileName, key }[], format)`: 上のビューの SQL を組み立てる。
  - `objectQuerySelectionDisabledReason(targets, config, browserSupported)`: 「選択した N 件に Query」ボタンが押せない理由を返す。
    判定の順序はブラウザ非対応 → 設定の取得失敗 → 取得中 → 0 件 → 形式の混在 → 件数超過 → 合計サイズ超過で、押せるときは空文字。
  - `clampObjectQuerySelection(ids, max)`: 集合の挿入順の先頭 `max` 件だけを残した新しい集合を返す。
  - `OBJECT_QUERY_MAX_FILES`、`OBJECT_QUERY_INGEST_CONCURRENCY`。

### 選択の UI は `DataTable` の既存のチェックボックス列を制御化して使う

`DataTable` に任意の props `selection` を足す。

```ts
selection?: {
  selected: ReadonlySet<string>;
  onChange: (next: Set<string>) => void;
  // false の行にはチェックボックスを描かない
  isSelectable: (row: T) => boolean;
};
```

- `selection` を渡した一覧では、行のチェックボックスは `selected` を映し、変更は `onChange` に新しい集合を渡す。
  `isSelectable` が false の行にはチェックボックスを描かない。
  ヘッダのチェックボックスは、列フィルタとソートを適用した表示順の `isSelectable` な行 (以下「選べる行」) を対象にする。
  選べる行が全て選ばれていれば checked、一部なら indeterminate にする。
  クリックは、選べる行のうち選ばれている行が 1 つも無ければ全選択 (表示順の id を持つ集合を `onChange` に渡す)、1 つでもあれば全解除 (空集合を渡す) にする。
  indeterminate からのクリックが全解除になるため、後述の打ち切りで全件を選べない一覧でも全解除に到達できる。
- `selection` を渡さない一覧 (Objects タブ以外の 17 か所) は現状の内部 state の挙動のままにする。
- `DrawerObjectBrowser` は行の選択を `selectedIds: Set<string>` の state に持ち (アップロード用の既存の state `selected` (`File | null`) は名前も役割も変えない)、`isSelectable` はオブジェクト行で `objectQueryDisabledReason` が空文字の行だけを true にする (フォルダ行と Query 不可の行は false)。
  設定の取得中と取得失敗、ブラウザ非対応では全行が false になる。
- `DrawerObjectBrowser` は `onChange` で受け取った集合を毎回 `clampObjectQuerySelection` に通し、`selectedIds` の要素数を常に 50 以下に保つ。
  打ち切りは集合の挿入順の先頭 50 件を残す (全選択では `DataTable` が渡す表示順、1 行のチェックでは既存の選択の後ろに新しい id が足された順)。
  全選択で 51 件以上のときは表示順の先頭 50 件が選ばれ、50 件選択中に 51 件目をチェックしても選択は変わらない (チェックボックスは付かない)。
  ツールバーに選択数を出す。50 件未満では `selectedCount` (「N 件選択中」)、50 件では `selectionClamped` (「N 件選択中 (上限 50 件)」) の文言にする。
- `objectQuerySelectionDisabledReason` の件数超過の判定は、打ち切りにより UI からは到達しない。
  ボタンの可否を決める純関数を打ち切りの有無に依存させないために残し、テストは純関数で行う。
- 選択は表示中のオブジェクト行の `id` との積で解釈し、一覧の変化で消えた `id` は選択から外れる。
  prefix の確定、モードの切り替え、フォルダの移動のイベントハンドラで `selectedIds` を空にする。
- ツールバーに「選択した N 件に Query」ボタンを置く。
  `objectQuerySelectionDisabledReason` が空文字でないときは無効にし、title に理由を出す (既存の Query ボタンと同じ形)。
- 行ごとの Query ボタンは残し、その行 1 件の `files` で開く。

採らなかった案。

- 2 つ目のチェックボックスの列を足す。
  既存の列と 2 列並び、既存の列は何にも使われないまま残る。
- Objects タブだけ `DataTable` を使わない別の表にする。
  ソート、列フィルタ、列幅の変更を作り直すことになる。
- 全選択で 50 件超を選ばせ、ボタンを無効にして理由だけ出す。
  一覧は 1000 件まで返るため 51 件以上は普通に起こり、全選択のたびに押せないボタンが残る。
- 行ごとの Query ボタンを消してチェックボックスだけにする。
  1 件の操作が 2 クリックになる。

### 文言

`frontend/src/i18n/locales/{ja,en}/drawerStorage.json` に次のキーを足す。

- `objectQuery.noneSelected`、`objectQuery.mixedFormats`、`objectQuery.tooManyFiles` (`{{max}}`)、`objectQuery.totalTooLarge` (`{{limit}}`)。
- `drawerObjectBrowser.querySelected` (`{{count}}`)、`drawerObjectBrowser.selectedCount` (`{{count}}`)、`drawerObjectBrowser.selectionClamped` (`{{count}}` と `{{max}}`)。
- `drawerObjectQuery.titleMultiple` (`{{count}}`)。

### ドキュメント

- `docs/prd/thief.md` の FR-17 に、複数のオブジェクトの検索、`object_key` 列、同じ形式の制約、合計サイズの上限、件数の上限を書く。
- 判断 (リスト引数で 1 つのビューにする、同じ形式に限る、上限を合計に掛ける、`DataTable` の選択を制御化する) を ADR として `docs/adr/0030` に書く。
  ADR 0018 の決定 (DuckDB Wasm、11 形式、上限の既定 1 GiB と `THIEF_OBJECT_QUERY_MAX_BYTES`、拡張の同一オリジン配信) はどれも覆さず、上限の掛かる対象を広げるだけなので、ADR 0018 の `Status` は `Accepted` のままにする。
  `docs/adr/0030` の「状況」に ADR 0018 を拡張する旨を書く。

## 未確定論点

- DuckDB が `filename` 列に返す値の末尾が OPFS のファイル名 (`<uuid><拡張子>`) と一致することは、リポジトリ内に根拠が無い。
  実装時にブラウザで 3 形式 (csv、json、parquet) を試して確かめる。
  一致しなければ、`filename` 列の値をそのまま `object_key` として出し、キーへの写像は諦める。
  調査結果 (2026-09-28、解消済み): 一致する。(a) を採る。
  duckdb-wasm 1.32.0 (DuckDB v1.4.3) の Node 向け同期バインディング (`frontend/node_modules/@duckdb/duckdb-wasm/dist/duckdb-node-blocking.cjs`) で、`registerFileText` に `opfs://thief-query/<uuid>.csv` の名前で登録した 2 ファイルを `read_csv([...], union_by_name = true, filename = true)` で読むと、`filename` 列の値は登録した文字列そのもの (`opfs://thief-query/<uuid>.csv`) だった。
  json と parquet は Node では `LOAD json` が返ってこず wasm で試せなかったため、DuckDB CLI v1.5.5 で同じ形を試した。3 形式とも `filename` 列の値は読み取り関数に渡したパス文字列そのものだった。
  ブラウザでの実行は「## 動作確認 (2026-09-28)」で確かめた (3 形式とも `object_key` 列が元のキーになった)。由来の判定は末尾の後方一致で行うため、先頭のスキームやディレクトリの形には依存しない。
- DuckDB 1.4 の `filename` オプションは列名の文字列も受け取る (`filename = 'object_key'`) と DuckDB のドキュメントにあるが、同一オリジンで配信している v1.4.3 の 3 つの読み取り関数で動くかは確認していない。
  動けば `EXCLUDE (filename)` と `AS object_key` の付け替えを省ける。
  実装時にブラウザで試し、動かない形式が 1 つでもあれば `EXCLUDE` の形に統一する。
  調査結果 (2026-09-28、解消済み): 3 つの読み取り関数すべてで動く。`filename = 'object_key'` の形を採る。
  csv は wasm (DuckDB v1.4.3)、json と parquet は CLI v1.5.5 で、`filename = 'object_key'` を渡すと `object_key` 列が足され、`filename` 列は現れなかった。
  その後、配信版と同じ wasm v1.4.3 と同梱の json / parquet 拡張でも 3 形式で動くことをブラウザで確かめた (「## 動作確認 (2026-09-28)」)。
  このオプションの処理は 3 つの読み取り関数が共有する本体側にある (次の論点のエラー文言が、本体に組み込みの csv でも拡張の json / parquet でも同一であることから判断した)。
  `object_key` の値の写像は `EXCLUDE` と `AS` の付け替えではなく `SELECT * REPLACE (CASE ... END AS object_key)` で行う。次の形が 3 形式で動き、wasm では `CREATE OR REPLACE VIEW obj AS ...` にしてからの `SELECT` も動いた。

  ```sql
  SELECT * REPLACE (
    CASE
      WHEN object_key LIKE '%<uuid1>.parquet' THEN '<key1>'
      WHEN object_key LIKE '%<uuid2>.parquet' THEN '<key2>'
      ELSE object_key
    END AS object_key
  )
  FROM read_parquet(['opfs://thief-query/<uuid1>.parquet', 'opfs://thief-query/<uuid2>.parquet'], union_by_name = true, filename = 'object_key')
  ```

  `ELSE object_key` は、どの `WHEN` にも当たらないときに NULL ではなくパスをそのまま出すためのものである。
  「## 完了条件」の `objectQueryViewSql` の行にある `filename = true` は、同じ行の末尾の規則によりこの形 (`filename = 'object_key'`) に読み替える。
  「## 設計判断」の SQL 例 (`SELECT * EXCLUDE (filename)` と `filename = true`) も同じくこの形に読み替え、同節の `objectQueryReadSql` への参照は `objectQueryViewSql` に読み替える (`objectQueryReadSql` は削除した。「## 実装詳細の乖離 (2026-09-28)」の 1 番目)。
- データが `filename` という列を持つとき、`filename = true` との衝突で DuckDB がどう振る舞うかは確認していない。
  実装時に確かめ、エラーになるなら `filename` オプションに別名を渡す形 (上の論点) に寄せる。
  調査結果 (2026-09-28、解消済み): エラーになる。上の論点のとおり `filename = 'object_key'` に寄せる。
  データに `filename` 列があるファイルを `filename = true` で読むと、3 形式とも `Binder Error: Option filename adds column "filename", but a column with this name is also in the file. Try setting a different name: filename='<filename column name>'` になった (csv は wasm v1.4.3、json と parquet は CLI v1.5.5)。
  `filename = 'object_key'` にすると、データの `filename` 列は残ったまま `object_key` 列が足された。
  データに `object_key` 列があるファイルでは同じ Binder Error が `object_key` について出る。この場合は他の SQL エラーと同じくエラー文言をそのまま画面に出し、列名の付け替えなどの回避は行わない (どの列名を選んでもデータ側の列と衝突しうるため)。

## 完了条件

純関数と単体テスト (`frontend/src/lib/objectQuery.ts`、`objectQuery.test.ts`)。

- `objectQueryViewSql` が、複数の OPFS パスをリスト引数にした読み取り関数 (`union_by_name = true`、`filename = true`) と、`object_key` 列を含む SQL を返す。
  `object_key` 列の作り方は「未確定論点」の確認結果で次のどちらかに定まり、採った方をテストの期待値にする。
  (a) `filename` の値の末尾が OPFS のファイル名に一致する場合: `filename LIKE '%<ファイル名>'` の後方一致で元のキーを返す `CASE` 式にし、キーの単一引用符を二重化する。
  (b) 一致しない場合: `filename` の値をそのまま `object_key` 列として出す (キーへの写像は行わない)。
  `filename = 'object_key'` の形は、3 つの読み取り関数すべてで動くと確かめたときだけ採ってよく、その場合も列名は `object_key`、値は上の (a) または (b) と同じにする。
  1 件でもリストの形である。
  tsv では `delim` が付く。
- `objectQuerySelectionDisabledReason` が、ブラウザ非対応、設定の取得失敗、取得中、0 件、`readFunction` または `delimiter` の不一致、`OBJECT_QUERY_MAX_FILES` 超過、合計サイズの上限超過 (`合計 > maxBytes`) のそれぞれで、対応する文言を返す。
  `.csv` と `.csv.gz` の組み合わせは可、合計がちょうど `maxBytes` は可と判定する。
- `clampObjectQuerySelection` が、集合の挿入順の先頭 `max` 件だけを残した新しい集合を返す (`max` 件以下なら同じ要素)。
- `OBJECT_QUERY_MAX_FILES` (50) と `OBJECT_QUERY_INGEST_CONCURRENCY` (4) が定数として定義されている。

`DataTable` (`DataTable.test.tsx`)。

- `selection` を渡すと、`isSelectable` が false の行にチェックボックスが無く、行のチェックが `onChange` に集合を渡す。
  ヘッダのチェックボックスは、選べる行 (列フィルタとソートを適用した表示順の `isSelectable` な行) が 1 つも選ばれていなければ全選択 (表示順の id の集合を渡す)、1 つでも選ばれていれば全解除 (空集合を渡す) になり、全て選ばれていれば checked、一部なら indeterminate になる。
- `selection` を渡さないと、現状の内部 state の挙動のままである。

エンジン (`frontend/src/lib/duckdb.ts`、`duckdb.test.ts`)。

- `registerView(id, opfsPaths, viewSql, extensions)` が全ファイルを `registerOPFSFileName` で登録し、拡張を INSTALL / LOAD してから `viewSql` を実行する。
- `obj` ビューの持ち主をパネルの ID で記録し、別のパネルの登録が始まった後の解放でビューを消さない。
  `duckdb.test.ts` の持ち主の判定のテストが新しいシグネチャ (パネルの ID) で書き直されている。
- ファイルの登録解除が `dropFiles` で全ファイル分行われる。

`DrawerObjectQuery` (`DrawerObjectQuery.test.tsx`)。

- `files` の配列を受け取り、並列度 4 で取り込む (5 件渡したとき、同時に `ingest` が呼ばれているのは 4 件までで、1 件の完了後に 5 件目が始まる)。
- 取り込みの前に合計サイズで空き容量を 1 回確かめ、足りなければ `quotaExceeded` で止まり、`createIngestor` を呼ばない。
- 全ファイルの取り込みが終わった時点で `written` の合計が `maxBytes` を超えていたら、`tooLarge` のエラーになり、全ファイルが解放される。
- 進捗が全ファイルの合計で表示され、1 つでも `total` が null なら合計不明の表示になる。
- 1 つの取り込みが失敗すると、待っているファイルの `createIngestor` が呼ばれず、作成済みの全 ingestor が `terminate` され、エラーが 1 つ表示される (5 件で 1 件目が失敗したとき、`terminate` は 4 回、5 件目の `createIngestor` は呼ばれない)。
- 取り込みが終わった ingestor は、解放まで `terminate` されない。
- アンマウントで `dropView` が 1 回、`dropFiles` が全ファイル分、`terminate` が作成済みの ingestor の数だけ行われる。
  StrictMode の setup → cleanup → setup で、ファイルとロックが残らない。
- 題名が 1 件では `Query: <key>`、複数では件数の文言になる。

`DrawerObjectBrowser` (`DrawerS3Objects.test.tsx`、`DrawerGCSObjects.test.tsx`)。

- Query 可能なオブジェクト行にだけチェックボックスがあり、フォルダ行と Query 不可の行には無い。
- ヘッダのチェックボックスで表示中の Query 可能な行が全選択 / 全解除される。
  Query 可能な行が 51 件以上あるとき、全選択で選ばれるのは `DataTable` が渡す表示順の先頭 50 件で、「50 件選択中 (上限 50 件)」の文言が出る。
  その状態で 51 件目の行をチェックしても、選択は元の 50 件のままで (51 件目の行にはチェックが付かず、元の 50 件のチェックはどれも外れない)、ヘッダのクリックで全解除される。
  50 件未満では「N 件選択中」の文言が出る。
- 「選択した N 件に Query」ボタンが、0 件、形式の混在、合計サイズの超過で無効になり、title に理由が出る。
- ボタンを押すと、選んだ行の `files` で `DrawerObjectQuery` が開く。
- 行ごとの Query ボタンが残り、その行 1 件で開く。
- prefix の確定、モードの切り替え、フォルダの移動で選択が空になる。
- `DrawerGCSObjects.test.tsx` の Query ボタンの活性判定のテストが通る。

設定とドキュメントと検証。

- `config.go`、`handlers_config.go`、`types/common.ts` の `ClientConfigRaw` のコメントが「合計サイズの上限」の意味になっている。
- 「文言」の節のキーが ja と en の `drawerStorage.json` にある。
- `docs/prd/thief.md` の FR-17 が「設計判断」の「ドキュメント」のとおりに更新されている。
- `docs/adr/0030` が書かれ、`docs/adr/README.md` の表に行がある。
  ADR 0018 は書き換えない。
- `CHANGES.md` の `## develop` に追記されている (`SELECT *` の結果に `object_key` 列が増えるため、種別は `[CHANGE]`)。
- `mise run check` が通る。

扱わない範囲。

- 異なる形式 (csv と parquet など) の混在。
- フォルダ全体の再帰的な選択 (選べるのは一覧に表示されている行だけ。階層をまたぐときはフラットモードで一覧に出す)。
- backend の API の変更 (エンドポイントと応答は変えず、設定の説明のコメントだけを直す)。

## 関連

- issue 0207 (フォルダの階層表示) を先に実装する。
  本 issue のチェックボックスは issue 0207 の行の型 (`ObjectBrowserRow`、フォルダ行とオブジェクト行) の上に足し、階層をまたいだ選択は issue 0207 のフラットモードに依存する。
- issue 0196 (1 オブジェクトの SQL 検索) と ADR 0018 の設計を拡張する。

## 実装詳細の乖離 (2026-09-28)

方針の方式は保ったまま、実装の細部を次のとおりにした。

- `objectQueryReadSql` は削除した。1 ファイル専用の読み取り関数の組み立ては `objectQueryViewSql` に置き換わり、呼び出し元が無くなったため。テストも `objectQueryViewSql` のものに置き換えた。
- `ObjectQueryEngine.dropFile(opfsPath)` は `dropFiles(opfsPaths)` に置き換えた (`AsyncDuckDB.dropFiles` を使う)。
- `OpfsIngestor` のプライベートな `checkQuota` は公開関数 `checkObjectQueryQuota(size)` (`opfsIngest.ts`) に置き換え、`OpfsIngestor.ingest` もこれを呼ぶ。空き容量の判定の実装は 1 つに保つ。
- `DrawerObjectQuery` はファイルごとの形式 `formats` を内部の `ObjectQueryPanel` に渡す。`.csv` と `.csv.gz` の混在で OPFS のファイル名に元の圧縮拡張子を保つため (形式 1 つだけでは `.csv.gz` のファイルを `.csv` の名前にしてしまう)。ビューの SQL の組み立てには先頭のファイルの形式を使う (読み取り関数と区切り文字は全ファイルで一致する)。
- 取り込み後の `written` の合計が上限を超えたときの表示は、既存の `drawerObjectQuery.tooLarge` の文言を使う。「文言」の節に合計用のキーが無いため。合計用の言い回しにする件は、close のコミットの後に `docs/issues/TODO.md` へ追記して個別にコミットする。
- 進捗の合計不明の表示は、まだ取り込みを始めていないファイルがあるときにも出る (そのファイルの `total` はまだ無いため)。全ファイルの取り込みが始まり、全ファイルに `Content-Length` があれば合計の表示になる。
- StrictMode のテストは、`dropView` と `dropFiles` の呼び出し回数ではなく、最後の呼び出しの引数と、作成済みの ingestor の終了の完了で検証する (1 回目の cleanup でも解放が走るため呼び出しは 2 回になり、2 回目の解放は持ち主の判定で何もしない)。
- 選択の件数と「選択した N 件に Query」ボタンの行 (`.s3-selection-row`) は、選択が 0 件でも表示する。
- `obj` ビューの SQL (`objectQueryViewSql`) は描画時ではなく取り込みの開始時 (空き容量の確認の前) に組み立てる。`files` が空のときの例外を描画の失敗ではなく取り込みの失敗と同じ経路 (エラー表示) で扱うため。`objectQueryViewSql` は `files` が空なら英語のメッセージの `Error` を投げる。
- `DataTable` のヘッダのチェックボックスは、`selection` を渡した一覧で選べる行 (列フィルタとソートを適用した表示順の `isSelectable` な行) が 1 つも表示されていなければ描画しない (ヘッダの対象が無いため)。列フィルタで選べる行が全て隠れたときも消えるが、隠れた行の選択は残る (選択の件数と「選択した N 件に Query」ボタンはそのまま)。その選択は、フィルタを消してヘッダで全解除するか、prefix の確定、モードの切り替え、フォルダの移動で空にできる。
- 「選択した N 件に Query」ボタンは、押せるときも行ごとの Query ボタンと同じく `title` を持つ (`drawerObjectBrowser.openQuerySelected`)。

## 動作確認 (2026-09-28)

`objectQueryViewSql` が返す形の SQL を、配信版と同じ duckdb-wasm 1.32.0 (DuckDB v1.4.3) の eh バンドルと、リポジトリの `frontend/public/assets/duckdb-extensions/v1.4.3/wasm_eh/` の json / parquet 拡張で、ヘッドレス Chrome (`--headless=new`) から実行して確かめた。

- プローブ: duckdb-wasm を Worker で起動し、`custom_extension_repository` を `/assets/duckdb-extensions` にして `INSTALL` / `LOAD` してから SQL を実行し、結果を DOM に書くページを Vite でビルドし、GET だけを扱う静的サーバで配信した。結果は Chrome の `--dump-dom` で取り出した。ファイルは `registerFileText` / `registerFileBuffer` で `thief-query/<uuid><拡張子>` の名前で登録した (OPFS の実ファイルではない。この登録では `filename` の値の末尾が登録名の形 `<uuid><拡張子>` になった)。
- 結果: 20 ケースすべて期待どおり (`version` は `v1.4.3`)。csv 2 ファイル (片方にだけある列を `union_by_name = true` で NULL 埋め)、json 2 ファイル (`.json` と `.jsonl`、キーに `'` を含む) と 1 ファイル、parquet 2 ファイル (DuckDB CLI で書いたもの) と 1 ファイルと DuckDB 自身が `COPY ... TO` で書いたもの、tsv (`delim='\t'`) で、`CREATE OR REPLACE VIEW obj AS ...` の後の `SELECT *` で `object_key` 列が元のキーになり、`filename` 列は現れず、`GROUP BY object_key` も動いた。
- 登録したファイル (登録名は `thief-query/<uuid><拡張子>`) とキー:
  - csv: `11111111-1111-4111-8111-111111111111.csv` = `id,name` / `1,foo` (キー `out/part-00000.csv`)、`22222222-2222-4222-8222-222222222222.csv` = `id,name,extra` / `2,bar,x` (キー `out/part-00001.csv`)。
  - json: `44444444-4444-4444-8444-444444444444.json` = `{"id":1,"name":"foo"}` (キー `logs/a'b.json`)、`55555555-5555-4555-8555-555555555555.jsonl` = `{"id":2,"name":"bar","extra":"x"}` と `{"id":3,"name":"baz"}` の 2 行 (キー `logs/b.jsonl`)。
  - parquet: DuckDB CLI で書いた `66666666-6666-4666-8666-666666666666.parquet` (`id` 1、`name` foo。キー `athena/part-0.parquet`) と `77777777-7777-4777-8777-777777777777.parquet` (`id` 2、`name` bar。キー `athena/part-1.parquet`)、`COPY (SELECT 10 AS id, 'gen' AS name) TO 'thief-query/gen.parquet' (FORMAT PARQUET)` で DuckDB 自身が書いた `gen.parquet` (キー `gen/x.parquet`)。
  - tsv: `88888888-8888-4888-8888-888888888888.tsv` = `id<TAB>name` / `5<TAB>tab` (キー `data.tsv`)。
- 各ケースの SQL: `... view` は `objectQueryViewSql` と同じ形の `CREATE OR REPLACE VIEW obj AS SELECT * REPLACE (CASE WHEN object_key LIKE '%<uuid><拡張子>' THEN '<キー>' ... ELSE object_key END AS object_key) FROM <読み取り関数>(['thief-query/<uuid><拡張子>', ...], union_by_name = true, filename = 'object_key')` (tsv は `delim='\t'` 付き。`single` は 1 要素のリスト、`gen` は `gen.parquet` と `66666666-...parquet` の 2 要素)。`... select` は `SELECT * FROM obj ORDER BY id`、`... group` は `SELECT object_key, count(*) AS n FROM obj GROUP BY object_key ORDER BY object_key`、`... install` は `INSTALL <拡張>; LOAD <拡張>;`、`parquet copy` は上の `COPY` 文。
- `--dump-dom` で取り出した `pre#out` の全文 (転記):

  ```text
  version v1.4.3
  csv view OK []
  csv select OK [{"id":1,"name":"foo","extra":null,"object_key":"out/part-00000.csv"},{"id":2,"name":"bar","extra":"x","object_key":"out/part-00001.csv"}]
  csv group OK [{"object_key":"out/part-00000.csv","n":1},{"object_key":"out/part-00001.csv","n":1}]
  json install OK []
  json view OK []
  json select OK [{"id":1,"name":"foo","extra":null,"object_key":"logs/a'b.json"},{"id":2,"name":"bar","extra":"x","object_key":"logs/b.jsonl"},{"id":3,"name":"baz","extra":null,"object_key":"logs/b.jsonl"}]
  json group OK [{"object_key":"logs/a'b.json","n":1},{"object_key":"logs/b.jsonl","n":2}]
  json view single OK []
  json select single OK [{"id":1,"name":"foo","object_key":"logs/a'b.json"}]
  parquet install OK []
  parquet view OK []
  parquet select OK [{"id":1,"name":"foo","object_key":"athena/part-0.parquet"},{"id":2,"name":"bar","object_key":"athena/part-1.parquet"}]
  parquet group OK [{"object_key":"athena/part-0.parquet","n":1},{"object_key":"athena/part-1.parquet","n":1}]
  parquet view single OK []
  parquet select single OK [{"id":1,"name":"foo","object_key":"athena/part-0.parquet"}]
  parquet copy OK [{"Count":1}]
  parquet view gen OK []
  parquet select gen OK [{"id":1,"name":"foo","object_key":"athena/part-0.parquet"},{"id":10,"name":"gen","object_key":"gen/x.parquet"}]
  tsv view OK []
  tsv select OK [{"id":5,"name":"tab","object_key":"data.tsv"}]
  ```
- サーバのログで、wasm 本体と json / parquet の拡張ファイルが `/assets/duckdb-extensions/v1.4.3/wasm_eh/` から取得されたことを確かめた。
- Node 向けブロッキングバインディングでは json / parquet の `LOAD` が返ってこない (Emscripten の動的ライブラリの読み込みが非同期のため) ので、ブラウザで確かめた。

アプリ本体 (`DrawerObjectQuery` の画面) のブラウザでの動作確認は行っていない。アプリが使う `registerOPFSFileName` と OPFS の実ファイルの経路 (アプリが登録する文字列は `objectQueryOpfsPath` が返す `opfs://thief-query/<uuid><拡張子>`) もこのプローブでは通していないため、その経路でも `filename` の値の末尾が `<uuid><拡張子>` で終わることは前提のままである。

## 解決方法

- frontend (純関数): `lib/objectQuery.ts` に `objectQueryViewSql` (`CREATE OR REPLACE VIEW obj AS SELECT * REPLACE (CASE ... ELSE object_key END AS object_key) FROM <readFunction>([...], union_by_name = true, filename = 'object_key')`。tsv では `delim` が付く)、`objectQuerySelectionDisabledReason`、`clampObjectQuerySelection`、`OBJECT_QUERY_MAX_FILES` (50)、`OBJECT_QUERY_INGEST_CONCURRENCY` (4)、`ObjectQueryTarget` を足し、`objectQueryReadSql` を削除した。`objectQueryViewSql` は `files` が空なら `Error` を投げる。`ObjectQueryEngine` は `registerView(id, opfsPaths, viewSql, extensions)` / `dropView(id)` / `dropFiles(opfsPaths)` にした。
- frontend (エンジン): `lib/duckdb.ts` の `registerView` は全パスを `registerOPFSFileName` で登録し、拡張を INSTALL / LOAD してから `viewSql` を実行する。`obj` ビューの持ち主はパネルの ID (`registeredViewOwner`) で記録し、`dropView` は一致するときだけ DROP する。`dropFiles` は `AsyncDuckDB.dropFiles` でまとめて解除する。
- frontend (取り込み): `lib/opfsIngest.ts` に `checkObjectQueryQuota(size)` を公開し、`DrawerObjectQuery` が取り込みの前に合計サイズで 1 回、`OpfsIngestor.ingest` がファイルごとに呼ぶ。
- frontend (パネル): `DrawerObjectQuery.tsx` の props を `files: ObjectQueryTarget[]` にし、ファイルごとに uuid の OPFS ファイル名を決め、取り込みの開始時に `obj` ビューの SQL を組み立ててから、並列度 4 で取り込む (順番が来たファイルにだけ ingestor を作り、ingestor の作成と取り込みのどちらかが 1 つ失敗したら残りを始めず、作成済みの ingestor を全部止める)。全ファイルの取り込み後に `written` の合計が上限を超えていたら `tooLarge` で失敗させる。進捗は全ファイルの合計で表示し、1 つでも `total` が null なら合計不明の表示にする。解放は `cancelSent` → `dropView(panelId)` → `dropFiles(opfsPaths)` → 全 ingestor の `terminate` の順。題名は 1 件なら `Query: <key>`、複数なら件数の文言。
- frontend (一覧): `DataTable.tsx` に任意の props `selection` (`selected` / `onChange` / `isSelectable`) を足し、渡した一覧ではチェックボックス列を制御化する (ヘッダは列フィルタとソートを適用した表示順の選べる行を対象に、1 つも選ばれていなければ全選択、1 つでもあれば全解除。全て選ばれていれば checked、一部なら indeterminate。選べる行が 1 つも表示されていなければヘッダのチェックボックスを出さない。列フィルタで隠れた行の選択は残る)。`DrawerObjectBrowser.tsx` は `selectedIds` を持ち、Query 可能なオブジェクト行だけを選べるようにし、`clampObjectQuerySelection` で 50 件に打ち切り、ツールバーに選択数と「選択した N 件に Query」ボタン (押せないときは理由、押せるときは `openQuerySelected` を `title` に出す) を置く。prefix の確定、モードの切り替え、フォルダの移動で選択を空にする。行ごとの Query ボタンはその行 1 件の `files` で開く。ツールバーの行 (`.s3-selection-row`) のスタイルは `styles/features/storage.css` に足した。
- 設定のコメント: `backend/internal/config/config.go`、`backend/internal/api/handlers_config.go`、`frontend/src/types/common.ts` の `ClientConfigRaw` を「1 回の検索で取り込む合計サイズの上限」に直した。
- 文言: ja / en の `drawerStorage.json` に `objectQuery.noneSelected` / `mixedFormats` / `tooManyFiles` / `totalTooLarge`、`drawerObjectBrowser.querySelected` / `openQuerySelected` / `selectedCount` / `selectionClamped`、`drawerObjectQuery.titleMultiple` を足し、en の `drawerObjectBrowser.querySelected` を「Query {{count}} selected objects」に、`drawerObjectQuery.ingestFailed` を「Ingest failed」に直した。
- 動作確認: `objectQueryViewSql` の SQL の形 (`filename = 'object_key'` と CASE 式の写像) を、配信版と同じ duckdb-wasm 1.32.0 (DuckDB v1.4.3) と同梱の json / parquet 拡張でヘッドレス Chrome から csv / tsv、json、parquet の 3 形式で確かめた (「## 動作確認 (2026-09-28)」。各ケースの入力と `pre#out` の出力を同節に転記した)。アプリが使う `registerOPFSFileName` と OPFS の実ファイルの経路は確かめていない。
- ドキュメント: `docs/prd/thief.md` の FR-17 を更新し、`docs/adr/0030` を書いて `docs/adr/README.md` に行を足した。`CHANGES.md` の `## develop` に `[CHANGE]` で追記した。
- 方針の方式を保った実装の細部は「## 実装詳細の乖離 (2026-09-28)」に記録した。
- `mise run check` の通過を確認した (frontend 117 ファイル 1310 テスト、backend 全パッケージ ok、lint はエラー 0 で警告 9 件は既存分)。

完了条件の検証は次のとおりである。

- 純関数: `objectQuery.test.ts` の「複数の OPFS パスをリスト引数にした読み取り関数と object_key 列でビューを組み立てる」「1 件でもリストの形にし、由来のキーを CASE 式で写す」「tsv では delim が付く」「キーの単一引用符を二重化する」「files が空なら組み立てずに例外を投げる」(`objectQueryViewSql`。`filename = 'object_key'` の形と (a) の写像)、「対象形式がそろっていれば押せる」「ブラウザ非対応 → 設定の取得失敗 → 取得中 → 0 件 の順に理由を返す」「readFunction または delimiter が食い違う組み合わせは形式の混在として拒否する」「件数が OBJECT_QUERY_MAX_FILES を超えると拒否する」「合計サイズが上限を超えると拒否し、ちょうどは許す」(`objectQuerySelectionDisabledReason`)、「集合の挿入順の先頭 max 件だけを残す」「max 件以下なら同じ要素の集合を返す」(`clampObjectQuerySelection`)、「件数の上限は 50、並列度は 4」(定数)。
- `DataTable`: `DataTable.test.tsx` の `selection` の describe の「isSelectable が false の行にはチェックボックスが無く、行のチェックは onChange に集合を渡す」「ヘッダは選べる行が 1 つも選ばれていなければ全選択、1 つでも選ばれていれば全解除になる」「ヘッダは全て選ばれていれば checked、一部なら indeterminate になる」「全選択は列フィルタとソートを適用した表示順の選べる行を対象にする」「選べる行が 1 つも無い一覧にはヘッダのチェックボックスが出ない」「列フィルタで選べる行が全て隠れるとヘッダのチェックボックスが消え、選択は変わらない」「selection を渡さない一覧は内部 state のチェックボックスのまま動く」。
- エンジン: `duckdb.test.ts` の「registerView は全ファイルを登録し、拡張を読み込んでから viewSql を実行する」「別のパネルのビューに置き換わっていたら dropView は何もしない」「別のパネルの登録が始まっていれば、CREATE の完了前でも dropView は何もしない」「dropFiles は全ファイルの登録をまとめて解除する」。
- `DrawerObjectQuery`: `DrawerObjectQuery.test.tsx` の「5 件を並列度 4 で取り込み、1 件の完了後に 5 件目を始める」「取り込みの前に合計サイズで空き容量を 1 回確かめ、足りなければ quotaExceeded で止まる」「取り込みの written の合計が上限を超えたら tooLarge になり全ファイルを解放する」「進捗は全ファイルの合計で表示する」「1 つでも Content-Length が無いファイルがあれば合計不明の進捗表示にする」「1 つの取り込みが失敗すると、待っているファイルは始めず、作成済みの全 ingestor を止める」「ingestor の作成に失敗しても、作成済みの ingestor を止めてエラーを表示する」「files が空なら取り込みを始めず、ビューの組み立ての失敗をエラーとして表示する」「取り込みが終わった ingestor は解放まで terminate しない」「アンマウントすると cancelSent / DROP VIEW / dropFiles と Worker の終了指示を呼ぶ」「StrictMode の二重実行でも取り込みは 1 回だけで、解放でファイルとロックが残らない」「題名は 1 件ならキー、複数なら件数の文言にする」「複数ファイルを 1 つの obj ビューにし、ファイルごとの OPFS パスと由来のキーを SQL に入れる」。
- `DrawerObjectBrowser`: `DrawerS3Objects.test.tsx` の「Query 可能なオブジェクト行にだけチェックボックスが出る」「ヘッダのチェックボックスで表示中の Query 可能な行が全選択 / 全解除される」「Query 可能な行が 51 件以上のときは表示順の先頭 50 件で打ち切り、51 件目のチェックでは変わらない」「「選択した N 件に Query」ボタンは 0 件・形式の混在・合計サイズの超過で無効になる」「「選択した N 件に Query」ボタンを押すと選んだ行の files で SQL 検索が開く」「prefix の確定、フォルダの移動、モードの切り替えで選択が空になる」と、行ごとの Query ボタンの既存テスト (1 件だけ取り込む検証を足した)。`DrawerGCSObjects.test.tsx` の「Query 可能な行にだけチェックボックスが出て、ヘッダで全選択 / 全解除できる」と、既存の「Query ボタンを S3 と同じ条件で出し分ける」。
- 設定とドキュメントと検証: 上の「設定のコメント」「文言」「ドキュメント」の項と、`mise run check` の通過。
- 扱わない範囲: 異なる形式の混在は `objectQuerySelectionDisabledReason` が拒否する。フォルダ全体の再帰的な選択は無い (`isSelectable` はオブジェクト行だけを true にする)。backend の変更はコメントだけである (`git diff --cached --stat -- backend/`)。

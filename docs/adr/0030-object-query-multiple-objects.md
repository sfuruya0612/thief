# 0030. オブジェクトの SQL 検索は、同じ形式の複数のオブジェクトを読み取り関数のリスト引数で 1 つのビューにまとめる

Created: 2026-09-28
Model: Claude Fable 5.1
Status: Accepted
Decided: 2026-09-28

## 状況

ADR 0018 のオブジェクト SQL 検索は、1 回の検索で 1 つのオブジェクトだけを `obj` ビューにする。
Athena の CTAS や UNLOAD、BigQuery のエクスポート、Cloud Logging のシンクは出力を `part-00000.parquet`、`part-00001.parquet` のように分割するため、1 つずつしか検索できないと分割された出力の全体に対する集計ができない (`docs/issues/TODO.md`)。

本 ADR は ADR 0018 を拡張する。
ADR 0018 の決定 (DuckDB Wasm、11 形式、上限の既定 1 GiB と `THIEF_OBJECT_QUERY_MAX_BYTES`、拡張の同一オリジン配信) はどれも覆さず、上限の掛かる対象を広げるだけなので、ADR 0018 の Status は Accepted のままにする。

## 決定

- 選んだ全オブジェクトを、読み取り関数にパスのリストを渡す 1 つの `obj` ビューにする (`read_parquet(['opfs://...', 'opfs://...'], union_by_name = true, filename = 'object_key')`)。
  1 件の選択でも同じ形にする。
- 由来のオブジェクトを `object_key` 列で区別する。
  `filename` オプションに列名 `object_key` を渡し、値は `SELECT * REPLACE (CASE ... END AS object_key)` で、OPFS のファイル名との後方一致により元のオブジェクトのキーへ写像する。
- 組み合わせられるのは同じ形式 (`readFunction` と `delimiter` が一致するもの) だけとする。
  `.csv` と `.csv.gz` は同じ `read_csv` なので混在を許す。
- `THIEF_OBJECT_QUERY_MAX_BYTES` の意味を「1 回の検索で取り込むオブジェクトの合計サイズの上限」に変える。
  設定の名前と `GET /api/config` のフィールド名 `object_query_max_bytes` は変えない。
- 1 回の検索で選べるオブジェクトは 50 件 (`OBJECT_QUERY_MAX_FILES`) までとし、取り込みは並列度 4 (`OBJECT_QUERY_INGEST_CONCURRENCY`) で行い、1 つ失敗したら全体を止める。
- 選択の UI は、共有部品 `DataTable` の既存のチェックボックス列を任意の props `selection` で制御化して使う。
  `selection` を渡さない一覧の挙動は変えない。

## 検討した代替案

issue 0208 は次の案を採らなかった。

- オブジェクトごとにビュー (`obj_1`、`obj_2`) を作り、利用者が `UNION ALL` を書く。
  分割された出力を 1 つの表として扱う要望に反し、既定の SQL が使えない。
- 読み取り関数を `UNION ALL BY NAME` でつなぐ。
  parquet の並列スキャンと述語のプッシュダウンがファイルごとに分かれ、リスト引数に対する利点が無い。
- glob (`opfs://thief-query/*.parquet`) で読む。
  他のパネルや他のタブが同じディレクトリに置いたファイルまで読む。
- 形式の混在を許し、形式ごとに読んで結合する。
  csv の型推論と parquet の型が食い違い、エラーの説明が難しくなる。
- サイズの上限を 1 ファイルごとに掛ける。
  1 GiB のオブジェクトを 10 個選べてしまい、OPFS の quota を超えて途中で失敗する。
- 上限の意味を変えるため `THIEF_OBJECT_QUERY_MAX_BYTES` を別の名前にする。
  1 件の検索では意味が変わらず、設定している利用者の環境変数を無効にする理由が無い。
- Worker を 1 つにして全ファイルを順に取り込む。
  取り込み 1 回で終了する Worker のメッセージ規約とロックの寿命を作り直すことになり、件数の上限で足りる。
- 選択の UI に 2 つ目のチェックボックスの列を足す、または Objects タブだけ別の表にする。
  前者は使われない既存の列が残り、後者はソート、列フィルタ、列幅の変更を作り直すことになる。

## 結果

- 分割された出力の全体に対する集計が 1 回の SQL でできる。
- `SELECT *` の結果に `object_key` 列が増える (後方互換の無い変更として `CHANGES.md` に記録した)。
- 同じ名前の列 (`object_key`) を持つデータは DuckDB の Binder Error になる。
  他の SQL エラーと同じくエラー文言をそのまま画面に出す。
- 1 回の検索で最大 50 個の取り込み Worker と Web Locks のロックが解放まで同時に存在する。
- `DataTable` のチェックボックス列に外から制御する経路ができた。

## 根拠資料

- `docs/issues/closed/0208`
- `docs/adr/0018`
- `frontend/src/lib/objectQuery.ts`、`frontend/src/lib/duckdb.ts`
- `frontend/src/components/Drawer/DrawerObjectQuery.tsx`、`frontend/src/components/Drawer/DrawerObjectBrowser.tsx`
- `frontend/src/components/DataTable.tsx`

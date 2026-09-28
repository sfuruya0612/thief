# Object Storage のクエリが Cannot read properties of undefined (reading 'fields') で失敗する

Created: 2026-09-28
Model: Claude Fable 5.1
Completed: 2026-09-28

## 症状

Web の Object Storage (S3 / GCS) の Drawer で Query を押すと、取り込みの完了後に自動実行される既定の SQL が失敗し、`ErrorBanner` に次のメッセージが出る。
SQL を書き換えて Run を押しても同じメッセージになる。

```
Cannot read properties of undefined (reading 'fields')
```

このメッセージは DuckDB のエラーではなく、frontend の `lib/duckdbResult.ts` の `columnsOf` が投げる TypeError である (根拠は「## 原因」に書く)。
結果表には列も行も表示されない。

## 再現手順

1. `mise run backend:run` と `mise run frontend:run` で起動し、S3 または GCS のバケットの Objects タブで CSV などのオブジェクトを選び、Query を押す。
2. 取り込みが終わると既定の SQL (`OBJECT_QUERY_DEFAULT_SQL`) が実行され、結果表の代わりに上記のメッセージが `ErrorBanner` に出る。
3. SQL を書き換えて Run を押しても同じメッセージが出る。
4. 期待する結果: 結果表に列名と行が出る。

ブラウザを使わずに再現することもできる。
`frontend/` の apache-arrow 17.0.0 で、`tableToIPC(table, 'stream')` のバイト列を `Uint8Array` のチャンクの非同期イテラブルとして `RecordBatchReader.from()` に渡すと、戻り値は `AsyncRecordBatchStreamReader` で、その `schema` は `undefined` である。
この状態で `schema.fields` を読むと、上と同じメッセージの TypeError になる。
`await reader.open()` の後は `schema.fields` からフィールド名が取れる (2026-09-28 に Node 上で実測)。

## 原因

スキーマを読む前にリーダーを `open()` していない。
要因を呼び出しの順に挙げる。

- `lib/duckdb.ts` の `runQuery` は `conn.send(sql)` の戻り値を `open()` せずに `collectQueryResult` へ渡す。
- `@duckdb/duckdb-wasm` 1.32.0 の `AsyncDuckDBConnection.send()` は、`arrow.RecordBatchReader.from(AsyncResultStreamIterator)` の戻り値をそのまま返す。`from()` は先頭 8 バイトを覗いて file 形式か stream 形式かを判定するだけで、スキーマのメッセージは読まない。
- apache-arrow 17.0.0 の `AsyncRecordBatchStreamReader.schema` は内部実装の `schema` をそのまま返す。この値は、`open()` がスキーマのメッセージを読むか、最初の `next()` がスキーマのメッセージに出会うまで `undefined` である。
- `lib/duckdbResult.ts` の `collectQueryResult` は反復を始める前に `columnsOf(reader.schema)` を呼ぶため、`undefined.fields` で TypeError になる。
- 単体テスト (`duckdbResult.test.ts` と `duckdb.test.ts`) の `makeReader` スタブは `schema` を最初から持つため、この順序の誤りを検出できなかった。`QueryBatchReader` の `schema: Schema` という型も、実物より強い契約を宣言している。
- docs/issues/closed/0196 の設計判断 7 は `send()` でストリーム実行すると決めたが、`open()` を呼ぶ必要は書いていない。

## 修正方針

`collectQueryResult` が列名を読む前にリーダーを `open()` し、`QueryBatchReader` の型を Arrow の契約に合わせる。

- `QueryBatchReader` に `open(): Promise<unknown>` を加え、`schema` を `Schema | undefined` にする。`AsyncRecordBatchStreamReader` の `open(options?): Promise<this>` と `get schema(): Schema` はこの形を満たす。
- `collectQueryResult` の先頭で `await reader.open()` を呼び、その後に `columnsOf(reader.schema)` で列名を読む。`open()` の後も `schema` が `undefined` のとき (スキーマのメッセージが無いストリームで、Arrow は `open()` でリーダーを閉じる) は、列も行も無い結果を返す。
- `duckdbResult.test.ts` に、実物の Arrow の `AsyncRecordBatchStreamReader` を `open()` する前の状態で `collectQueryResult` に渡し、列名と行を検証するテストを追加する。リーダーは `tableToIPC(table, 'stream')` のバイト列を非同期イテラブルで流して `RecordBatchReader.from()` に渡して作る。`send()` が返すリーダーと同じ作り方であり、スタブの仮定に依存しない。
- 両テストファイルの `makeReader` スタブを、`open()` を呼ぶまで `schema` が `undefined` になる形にし、`open()` が最初の `next()` より前に 1 回呼ばれることを検証する。

採らなかった案と却下の理由は次のとおりである。

- `runQuery` (`lib/duckdb.ts`) 側で `open()` してから渡す案は採らない。`schema` を読む当事者は `collectQueryResult` であり、前提を呼び出し側に置くと同じ抜けが再発しうる。`duckdb.test.ts` は配線 (打ち切り時の `cancelSent` の呼び出し) だけを見ており行の収集は検証しないため、その抜けをテストで捕まえられない。
- 最初のレコードバッチの `batch.schema` から列名を取る案は採らない。バッチが 0 個のときに列名を決められず、Arrow がスキーマだけのストリームで返す内部のプレースホルダーバッチ (`_InternalEmptyPlaceholderRecordBatch`) に依存することになる。
- `query()` で全結果を `Table` に受ける案は、docs/issues/closed/0196 の設計判断 7 で却下済み (全結果がヒープに載る) であり、再検討しない。

新しい API と権限は不要で、backend の変更も無い。

## 完了条件

- Object Storage の Query パネルで、既定の SQL と任意の SELECT が結果表に列名と行を表示する。
- 実物の Arrow の `AsyncRecordBatchStreamReader` を `open()` する前の状態で `collectQueryResult` に渡す単体テストが通る。
- `mise run check` が通過する。

## 解決方法

`collectQueryResult` が列名を読む前にリーダーを `open()` するようにし、`QueryBatchReader` の型を Arrow の契約に合わせた。

- `lib/duckdbResult.ts` の `QueryBatchReader` に `open(): Promise<unknown>` を加え、`schema` の宣言を `readonly schema: Schema | undefined` にした。
- `collectQueryResult` の先頭で `await reader.open()` を呼び、その後に `columnsOf(reader.schema)` で列名を読むようにした。`open()` の後も `schema` が `undefined` のときは、列も行も無い結果を返す。
- `lib/duckdb.ts` の `runQuery` は処理を変えず、`send()` が返すリーダーは `open()` されておらず `open()` は `collectQueryResult` が行う、というコメントを足した。
- `duckdbResult.test.ts` に `describe('collectQueryResult (Arrow の AsyncRecordBatchStreamReader)')` の 3 テストを追加した。リーダーは `tableToIPC(table, 'stream')` のバイト列を 64 バイトのチャンクの非同期イテラブルで流し、`RecordBatchReader.from()` に渡して作る。`open()` する前のリーダーから列名と行を読むこと (渡す前に `reader.schema` が `undefined` であることも確かめる)、上限を超えたら打ち切って cancel を呼ぶこと、バイト列が空でスキーマの無いストリームでは列も行も無い結果を返すことを検証する。
- 同ファイルの `makeReader` スタブを、`open()` を呼ぶまで `schema` が `undefined` になる形にし、`open()` と `next()` の呼び出し順を記録するようにした。`open()` が最初の `next()` より前に 1 回だけ呼ばれることと、`open()` の後も `schema` が無いスタブで列も行も無い結果を返すことのテストを追加した。
- `duckdb.test.ts` の `makeReader` スタブも同じく `open()` を持ち、`open()` を呼ぶまで `schema` を持たない形にした。

追加したテストとスタブを変えた既存のテストが不具合を捕まえることは、`lib/duckdbResult.ts` だけを修正前の内容に戻して確かめた。
`duckdbResult.test.ts` と `duckdb.test.ts` の 16 テスト (実物の Arrow のリーダーを使う 3 テストを含む) が、症状と同じ `TypeError: Cannot read properties of undefined (reading 'fields')` で失敗した。
修正後は 16 テストすべてが通る。

完了条件の検証は次のとおりである。

- 「Object Storage の Query パネルで、既定の SQL と任意の SELECT が結果表に列名と行を表示する」: ブラウザでは確認していない。代わりに、`send()` が返すリーダーと同じ作り方の Arrow のリーダーを `collectQueryResult` に渡す単体テストで、列名と行が取れることを確認した。`runQuery` は `send()` の戻り値をそのまま `collectQueryResult` に渡すため、リーダーの作り方が同じであればブラウザでも同じ経路を通る。
- 「実物の Arrow の `AsyncRecordBatchStreamReader` を `open()` する前の状態で `collectQueryResult` に渡す単体テストが通る」: 通過した。
- 「`mise run check` が通過する」: 終了コード 0 で通過した。frontend は Test Files 117 passed / Tests 1257 passed、lint はエラー 0 (警告 9 件は変更前からある)。backend は 18 パッケージすべて ok。

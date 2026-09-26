# 0028 フッターの CLI コマンドサンプル表示を全て削除する

Created: 2026-07-18
Completed: 2026-07-18
Model: Claude Fable 5 claude-fable-5

## 解決方法

`StatusBar.tsx` / `useCliHint.ts` を削除し、`App.tsx` / `AccountView.tsx` からの参照、`BigQueryView.tsx` / `AthenaView.tsx` の `setCliHint` 呼び出しと `cliHintSql` 利用、`queryFormat.ts` の `cliHintSql` とそのテスト、`app.css` の `.statusbar` 一式 (未使用だった `.live`/`.pulse`/`@keyframes pulse` を含む) を全て削除した。
`mise run frontend:lint` (0 errors) と `mise run frontend:test` (274 件全通過) で確認済み。

## 背景 / 根拠

全ビュー共通フッター (`frontend/src/components/StatusBar.tsx`) は、表示中サービスに対応する模擬 CLI コマンド (`aws ec2 describe-instances` 等) を表示する専用コンポーネントで、件数などの他の情報は表示していない。
クエリエディタ (BigQuery / Athena) は操作のたびに `setCliHint` で「直近の操作と等価なコマンド」に文言を上書きする。

この表示は保守されておらず、既に実態と乖離している。
コマンド文言はハードコードされた `COMMANDS` レコードで管理されているが、後から追加されたサービス (cfn / costexplorer / GCP 系) のエントリがなく、該当サービスを選ぶとフォールバックの `aws iam list-users` という無関係なコマンドが表示される。
サービスを追加するたびに `COMMANDS` への追記が必要で、忘れても検知する仕組みがない以上、この乖離は今後も繰り返される。
維持コストだけが残る表示であり、全削除の要望が出ている (docs/issues/TODO.md)。

フッターを含め、CLI コマンドサンプルを表示する箇所を全て削除する。

## 対応内容

CLI コマンドサンプル表示に関わる一式を削除する。
`StatusBar` は CLI コマンド表示だけのコンポーネントなので、フッター領域ごと削除する (代替の表示は置かない)。

## 実装方針

削除対象の全箇所 (調査済み):

1. `frontend/src/components/StatusBar.tsx`: ファイルごと削除する (`COMMANDS` レコードと描画本体)
2. `frontend/src/hooks/useCliHint.ts`: ファイルごと削除する (`setCliHint` / `useCliHint` / `resetCliHintForTest` の共有ストア)
3. `frontend/src/App.tsx`: import (`:13`)、`footerService` の導出 (`:84` 付近のコメントとロジック)、`<StatusBar service={footerService} />` (`:159`) を削除する
4. `frontend/src/views/nonaws/BigQueryView.tsx`: `setCliHint` の import (`:31`) と呼び出し 2 箇所 (`:93` の実行時、`:111` の dry run 時)、`cliHintSql` の import (`:34`) を削除する
5. `frontend/src/views/AthenaView.tsx`: `setCliHint` の import (`:31`) と呼び出し 2 箇所 (`:131` の結果取得時、`:142` の実行時)、`cliHintSql` の import (`:38`) を削除する
6. `frontend/src/lib/queryFormat.ts:61` の `cliHintSql`: 利用箇所が上記 2 ビューのみであることを確認して削除し、`frontend/src/lib/queryFormat.test.ts` の該当テストも削除する
7. `frontend/src/app.css:1401` からの `.statusbar` 一式 (`.cmd` / `.sep` / `.spacer` / `.live` を含む配下セレクタ全部) を削除する
8. `frontend/src/views/AccountView.tsx:98` の StatusBar への言及コメント (フッター移管の履歴コメント) を削除する

### レイアウトへの影響

`.statusbar` はルート flex カラム最下段の高さ 28px / `flex-shrink: 0` の要素で、削除するとメインコンテンツ領域が 28px 分広がる。
他の要素の高さ計算が statusbar の高さを前提にしていないか (`app.css` 内の `calc()` 等) を確認し、依存があれば合わせて解消する。

### テストへの影響

- `StatusBar` / `useCliHint` 自体の専用テストは存在しない (確認済み)
- `queryFormat.test.ts` の `cliHintSql` テストを削除する
- BigQueryView / AthenaView のテストが `setCliHint` の副作用に依存していないかを実行して確認する

## スコープ外

- フッター領域の代替コンテンツ (件数表示の復活等)。必要になったら別 issue とする
- backend / CLI 本体 (`thief` コマンド) への変更 (削除するのは Web UI 上のサンプル表示のみ)

## 検証

- `mise run check` を通す
- 全ビュー (AWS 各サービス、GCP、Datadog、TiDB、クエリエディタ) でフッターが消えてレイアウト崩れがないこと、BigQuery / Athena のクエリ実行が従来どおり動くことを確認する

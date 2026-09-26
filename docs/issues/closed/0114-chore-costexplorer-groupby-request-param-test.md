# Cost Explorer の明細取得に渡す GroupBy を検証するテストを整備する

Created: 2026-08-07
Model: Claude Fable 5
Completed: 2026-08-07

## 背景

issue 0102 (ElastiCache のリクエストパラメータ検証) の棚卸しで見つかった。

`backend/internal/aws/costexplorer.go` の `getCostDetails` (57 行) は、groupBy 引数が非空のとき `GetCostAndUsageInput.GroupBy` を設定して呼び出す。
呼び出し元の `GetCostByService` / `GetCostByAccount` / `GetCostByUsageType` はそれぞれ SERVICE / LINKED_ACCOUNT / USAGE_TYPE の `GroupDefinition` を渡し、`GetCostForPeriod` は nil を渡す。
`input.GroupBy = groupBy` の行を削除してもコンパイルと API 呼び出しは成功するが、サービス別、アカウント別、使用タイプ別の 3 関数が全て「グルーピングなしの期間合計」に静かに縮退し、内訳表示の機能そのものが壊れる。

costexplorer_test.go というテストファイル自体が存在せず、このファイルは何のテストにも守られていない。
なお同じパッケージの cost.go は `costExplorerAPI` インターフェースを持ち、`GetCostAndUsageInput` の Filter / GroupBy / Metrics を検証するテストが既にあるが、costexplorer.go は別経路で `*costexplorer.Client` を直接使っている。

## 対応方針

- `getCostDetails` が cost.go の既存インターフェース `costExplorerAPI` を受け取るように抽出し、受け取った Input を記録する手書きモックで検証する。`costExplorerAPI` は `GetCostAndUsage` だけを持ち、本 issue が必要とするメソッドセットと一致する。`GetCostAndUsage` だけを持つ狭いインターフェースを costexplorer.go に新設する案は、同一パッケージ内に同じメソッドセットのインターフェースを重複定義することになるため採らない。
- 4 つの呼び出し元 (SERVICE / LINKED_ACCOUNT / USAGE_TYPE / グルーピングなし) それぞれについて、`GroupBy` の Key と Type、`Metrics`、`Granularity` が期待どおり設定されることを検証する。
- テストは costexplorer_test.go を新設して置く。既存の cost_test.go に追記する案は、対象ファイルとテストファイルの対応を崩すため採らない。

## 完了条件

- `getCostDetails` の `GetCostAndUsage` 呼び出しが、インターフェースを受け取る形に抽出されている。
- SERVICE / LINKED_ACCOUNT / USAGE_TYPE の各経路で `GroupBy` が対応する `GroupDefinition` で送られ、`GetCostForPeriod` の経路で `GroupBy` が空であることを検証するテストが存在する。
- `input.GroupBy = groupBy` の行を削除するとテストが失敗する (実装時に一時的に削除して確認する)。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0102: 起票元の棚卸し。検証手法の先例。

## 解決方法

`getCostDetails` が `costExplorerAPI` を受け取る形に抽出し、4 つの取得経路がそれぞれの集計軸で `GetCostAndUsage` を呼ぶことを検証するテストを追加した。挙動は変えていない。

### 実装 (backend/internal/aws/costexplorer.go)

`getCostDetails` の第 2 引数を `client costExplorerAPI` に変え、クライアントの生成を呼び出し元へ移した。対応方針のとおり、cost.go の既存の `costExplorerAPI` (`GetCostAndUsage` 1 メソッド) をそのまま使い、同じメソッドセットのインターフェースを costexplorer.go に重複定義することはしていない。

4 つの公開関数 (`GetCostByService` / `GetCostByAccount` / `GetCostByUsageType` / `GetCostForPeriod`) は、クライアントを生成して同名の内部関数 (`getCostByService` など) へ委譲する形にした。集計軸のディメンション名 (`SERVICE` / `LINKED_ACCOUNT` / `USAGE_TYPE`) と、期間合計の経路が `nil` を渡すことは、この内部関数に置いてある。

公開関数へ直接 `costGroupByDefinition("SERVICE")` を書いたままにすると、ディメンション名がクライアント生成と同じ関数に閉じ込められ、単体テストから到達できない。完了条件は「SERVICE / LINKED_ACCOUNT / USAGE_TYPE の各経路で `GroupBy` が対応する `GroupDefinition` で送られる」ことの検証を求めており、経路とディメンション名の対応そのものを固定する必要がある。そのため経路ごとに内部関数を置いた。クライアント生成の 4 行が 4 箇所に重複するが、これを避けるためのクロージャ受け渡しなどの抽象化は入れていない (YAGNI)。

### テスト (backend/internal/aws/costexplorer_test.go)

対応方針のとおり costexplorer_test.go を新設した。

- `recordingCostExplorer` — 受け取った `GetCostAndUsageInput` を値としてコピーし、呼び出し順に記録する手書きモック。cost_test.go の `fakeCostExplorer` は Input のポインタを 1 つだけ保持する形で呼び出し回数を確かめられないため、この用途には使わない。値でコピーするのは、`getCostDetails` がページングで同一の Input を使い回して `NextPageToken` を書き換えるためである。
- `TestGetCostDetailsSendsGroupBy` — 4 経路をテーブル駆動で検証する。各経路について `GroupBy` (Key と Type)、`Metrics`、`Granularity` を固定し、期間合計の経路では `GroupBy` が空のままであることを固定する。期待値のディメンション名は実装の `costGroupByDefinition` を通さず、AWS API の文字列で直接書き下している。

`metric` と `granularity` は経路ごとに値を変える (BlendedCost / Monthly、NetUnblendedCost / Hourly、AmortizedCost / Daily、NormalizedUsageAmount / Monthly)。全経路で同じ値を使うと、引数を無視してその値を固定で送る実装に変えてもテストが通ってしまうためである。`metric` は 4 経路とも別の値とし、実装が既定値に使う `UnblendedCost` は期待値に選んでいない。`granularity` は API が 3 値 (DAILY / MONTHLY / HOURLY) しか持たないため 1 組だけ重複するが、どの値を固定で送る実装に変えても 2 経路以上が落ちる。

この値の割り当てはレビューの指摘で 2 度直した。初版は全経路が同じ値で、下表の `Metrics` と `Granularity` の行がいずれも 0 件だった。2 版は 1 経路の値の組が実装の既定値と一致しており、その経路だけ引数無視のミューテーションを検出できなかった。

### 検出力の確認

完了条件が指定する `input.GroupBy = groupBy` の削除に加え、各経路のディメンション名と対応方針が挙げた `Metrics` / `Granularity` を 1 つずつ壊し、そのたびにテストが失敗することを実測した。

| 壊した箇所 | 失敗したサブテスト数 |
| --- | --- |
| `if len(groupBy) > 0 { input.GroupBy = groupBy }` の削除 | 3 |
| `Metrics` を引数の metric ではなく `UnblendedCost` 固定に変更 | 4 |
| `Metrics` を引数の metric ではなく `NetAmortizedCost` 固定に変更 | 4 |
| `Granularity` を引数ではなく `GranularityDaily` 固定に変更 | 3 |
| `Granularity` を引数ではなく `GranularityMonthly` 固定に変更 | 2 |
| `Granularity` を引数ではなく `GranularityHourly` 固定に変更 | 3 |
| `getCostByService` の `SERVICE` を `LINKED_ACCOUNT` に変更 | 1 |
| `getCostByAccount` の `LINKED_ACCOUNT` を `USAGE_TYPE` に変更 | 1 |
| `getCostByUsageType` の `USAGE_TYPE` を `SERVICE` に変更 | 1 |
| `getCostForPeriod` の `nil` を `SERVICE` に変更 | 1 |
| `GroupDefinitionTypeDimension` を `GroupDefinitionTypeTag` に変更 | 3 |

`Metrics` の 2 行は、実装の既定値である `UnblendedCost` と、テストが以前どの経路にも期待していた `NetAmortizedCost` の両方を選んでいる。どちらに固定しても 4 経路すべてが落ちる。`Granularity` は API の 3 値すべてについて固定化を試し、最も検出力が低い `GranularityMonthly` でも 2 経路が落ちることを確かめた。確認後、実装は元に戻し、`git diff` で復元を確認した。

### 今回のテストで検証していない範囲

モックは `NextPageToken` を返さないため、ページングのループと `input.NextPageToken` の更新は 1 周で抜ける。`recordingCostExplorer` が Input を値でコピーするのはこの使い回しに対して正しい形だが、1 ページで完結するため多ページでの記録の正しさそのものは検証していない。レスポンスも空のため、`ResultsByTime` から `CostDetail` への変換 (Groups が空のときに Total を使う分岐、`group.Metrics` にメトリクスが無いときの読み飛ばし、`group.Keys` の本数による `GroupKey` と `ServiceName` の振り分け) も通らない。`metric` が空文字のときに `UnblendedCost` へフォールバックする既定値の処理も検証していない。

いずれも本 issue の完了条件と対応方針が対象とするリクエストパラメータ (`GroupBy` / `Metrics` / `Granularity`) の検証ではないためスコープ外とした。レビューでレスポンス解析の未検証を指摘されたが、同じ理由で今回は対象に含めない。

`mise run check` の通過を確認済み。CHANGES.md の `## develop` の `### misc` に記載済み。

# ECS の一覧取得に渡すタグ取得指定とタスク絞り込みを検証するモックテストを整備する

Created: 2026-08-07
Model: Claude Fable 5
Completed: 2026-08-08

## 背景

issue 0102 (ElastiCache のリクエストパラメータ検証) の棚卸しで見つかった。ECS には同種の必須リクエストパラメータが 3 ファイルにまたがって 3 箇所ある。

- `backend/internal/aws/ecs.go` の `ListECSResources` (49 行): `DescribeClustersInput.Include: []ecstypes.ClusterField{ecstypes.ClusterFieldTags}`。削除しても API 呼び出しは成功するが、レスポンスに `Tags` が返らなくなり `ecsFromCluster` が組み立てるクラスタのタグ表示が静かに空になる。ShowCacheNodeInfo (issue 0102) と同型のパターンである。
- `backend/internal/aws/ecs_exec.go` の `ListECSTasks` (126-129 行): service 引数が空でないとき `ListTasksInput.ServiceName` を設定する。削除するとサービス指定によるタスク絞り込みが静かに効かなくなり全タスクが返る。
- `backend/internal/aws/ecs_cli.go` の `ListECSTaskInfos` (207-209 行): desiredStatus 引数が空でないとき `ListTasksInput.DesiredStatus` を設定する。削除するとステータス絞り込みが静かに効かなくなる。

`ecs_test.go` / `ecs_cli_test.go` は変換関数 (`ecsFromCluster` / `ecsTaskInfosFromSDK` 等) のテストのみで、リクエスト構築は検証していない。ecs_exec.go に対応するテストファイルは無い。

## 対応方針

- docs/issues/closed/0102 と同じ方式を採る。必要な API メソッド (`DescribeClusters` / `ListTasks`) を持つ狭いインターフェースを定義し、3 箇所の呼び出しロジックをインターフェースを受け取る内部関数に抽出する。
- 受け取った Input を記録する手書きモックで次を検証する。
  - `ListECSResources`: `Include` に `ClusterFieldTags` が含まれること。
  - `ListECSTasks`: service 指定時に `ServiceName` が設定され、空のとき nil であること。
  - `ListECSTaskInfos`: desiredStatus 指定時に `DesiredStatus` が設定され、空のとき空値であること。
- 3 箇所は同じ ECS サービスの同型の問題であるため 1 issue にまとめた。実装時にファイルごとの分割が必要になれば分割して起票し直してよい。

## 完了条件

- 上記 3 箇所の呼び出しが、狭いインターフェースを受け取る関数に抽出されている。
- 3 箇所それぞれのリクエスト構築を検証するテストが存在する。
- `Include` / `ServiceName` / `DesiredStatus` の設定行を削除するとテストが失敗する (実装時に一時的に削除して確認する)。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0102: 起票元の棚卸し。検証手法の先例。

## 解決方法」

## 解決方法

ECS の 3 箇所 (`ListECSResources` の `Include`、`ListECSTasks` の `ServiceName`、`ListECSTaskInfos` の `DesiredStatus`) の呼び出しを狭いインターフェースを受け取る内部関数へ抽出し、それぞれのリクエスト構築を検証するテストを追加した。挙動は変えていない。

### 実装

インターフェースは 2 つ定義し、いずれも ecs.go に置いた。3 ファイルにまたがる関数が同じ型を共有するためである。

- `ecsClusterListClient` — SDK の `ecs.ListClustersAPIClient` を埋め込み `DescribeClusters` を加えたもの。`ListECSResources` は ARN の列挙と詳細取得の 2 段構えのため両方を要する。
- `ecsTaskListClient` — SDK の `ecs.ListTasksAPIClient` を埋め込み `DescribeTasks` を加えたもの。`ListECSTasks` (ecs_exec.go) と `ListECSTaskInfos` (ecs_cli.go) が同じ 2 段構えで使うため 1 つを共有する。

`ListClusters` 単体のインターフェースは自前で定義せず、SDK が公開する `ecs.ListClustersAPIClient` をそのまま使う。ページネータ (`ecs.NewListClustersPaginator`) が要求する型がこれであり、同じメソッドセットを自前で書き直す重複を避けられる。既存の共通コアである `listECSClusterArnsWith` は `ListClusters` しか使わないため、引数をこの型に絞ってある。`ecsClusterListClient` はこれを埋め込んでいるので、`listECSResources` からはそのまま渡せる。

当初は `ListClusters` 単体のインターフェース `ecsListClustersClient` を自前で定義していたが、`ecsClusterListClient` と語順が入れ替わっただけの紛らわしい名前になること、および 1 つの issue で 3 つの型を新設するのは過剰であることをレビューで指摘され、SDK の型を使う形に改めた。これで型は 2 つになり、`listECSClusterArnsWith` が使うメソッドだけを要求する状態は保たれている。

公開関数 3 つはクライアントを生成して同名の内部関数 (`listECSResources` / `listECSTasks` / `listECSTaskInfos`) へ委譲する形にした。Input の構築、ページング、バッチ分割はすべて内部関数側に置いてある。

対応方針が「実装時にファイルごとの分割が必要になれば分割して起票し直してよい」としていたが、分割はしていない。インターフェースの共有によって 3 箇所が 1 つの変更としてまとまり、分けると `ecsTaskListClient` の定義が 2 つの issue にまたがるためである。

### テスト

テストファイルは対象ファイルに対応させ、モックは共有した。

- `internal/aws/ecs_test.go` — `mockECSClusterListClient` / `mockECSTaskListClient` / `ecsListTasksPages` と `TestListECSResourcesSendsIncludeTags`。モックとページのヘルパは、インターフェースを定義している ecs.go に対応するこのファイルに置き、他 2 ファイルのテストから使う。
- `internal/aws/ecs_exec_test.go` — `TestListECSTasksSendsServiceName`。
- `internal/aws/ecs_cli_test.go` — `TestListECSTaskInfosSendsDesiredStatus`。

いずれも期待する Input 全体を `cmp.Diff` で比較する。0117 で採った方式に揃えたもので、検証対象のフィールドに加えて `Clusters` / `Cluster` / `NextToken` の引き継ぎを同時に固定でき、「期待していないフィールドが新たに設定される」変更も検出できる。`ecs.DescribeClustersInput` / `ecs.ListTasksInput` / `ecs.DescribeTasksInput` はいずれも unexported フィールドを持つため `cmpopts.IgnoreUnexported` は方式によらず必要になる。

期待値は実装の定数式を経由せず AWS API の値 (`"TAGS"` / `"RUNNING"`) で直接書き下している。

`ListTasks` の 2 本は 2 ページ構成のレスポンスを使い、ページ送り後の呼び出しでも絞り込みが維持されることを確かめる。`ListECSResources` の側は ARN の列挙を 2 ページに分け、両ページの ARN が 1 回の `DescribeClusters` にまとめて渡ることを確かめる。

3 本とも 2 段目の呼び出しの Input も検証する。`DescribeClustersInput` は当初から比較対象に含めていたが、`DescribeTasksInput` の `Cluster` と `Tasks` はレビューの指摘で追加した。`ListTasksInput` の比較だけでは、`Cluster` を落として実 API で必須パラメータ不足になる変更も、`Tasks` を落としてタスクが 1 件も返らなくなる変更も検出できない。同じ issue の中で 2 段目を片方だけ検証しない状態も避けた。`DescribeTasksInput` はスライス全体を比較しており、呼び出し回数も同時に固定される。

`ServiceName` は `*string`、`DesiredStatus` は値型という違いがある。未指定時の期待値はそれぞれ nil と空文字であり、この差は下表の検出力にも現れている (条件反転のミューテーションで前者は 2 ケース、後者は 1 ケースが落ちる)。`DesiredStatus` は空文字と未設定を型として区別できないため、未指定時の検証は「空のままであること」までしか固定できない。

### 検出力の確認

完了条件が指定する `Include` / `ServiceName` / `DesiredStatus` の設定行の削除に加え、値の差し替え、条件判定、ARN のバッチとページ集約、2 段目の `DescribeTasksInput` の構築を 1 つずつ壊し、そのたびにテストが失敗することを実測した。`TestListECSResourcesSendsIncludeTags` はサブテストを持たないため失敗の有無で、他 2 本は全 2 サブテスト中の失敗数で示す。

| 壊した箇所 | 結果 |
| --- | --- |
| ecs.go: `Include` の設定行を削除 | 失敗する |
| ecs.go: `ClusterFieldTags` を `ClusterFieldSettings` に変更 | 失敗する |
| ecs.go: `Clusters: arns[i:end]` を先頭 1 件に変更 | 失敗する |
| ecs.go: ARN の集約を最後のページのみに変更 | 失敗する |
| ecs_exec.go: `ServiceName` の設定を削除 | 2 サブテスト中 1 件 |
| ecs_exec.go: `service != ""` の条件を反転 | 2 サブテスト中 2 件 |
| ecs_exec.go: `ServiceName` を引数ではなく固定値に変更 | 2 サブテスト中 1 件 |
| ecs_exec.go: `DescribeTasksInput` の `Cluster` を削除 | 2 サブテスト中 2 件 |
| ecs_exec.go: `DescribeTasksInput` の `Tasks` を先頭 1 件に変更 | 2 サブテスト中 2 件 |
| ecs_cli.go: `DesiredStatus` の設定を削除 | 2 サブテスト中 1 件 |
| ecs_cli.go: `desiredStatus != ""` の条件を反転 | 2 サブテスト中 1 件 |
| ecs_cli.go: `DesiredStatus` を `DesiredStatusStopped` 固定に変更 | 2 サブテスト中 1 件 |
| ecs_cli.go: `DescribeTasksInput` の `Cluster` を削除 | 2 サブテスト中 2 件 |
| ecs_cli.go: `DescribeTasksInput` の `Tasks` を先頭 1 件に変更 | 2 サブテスト中 2 件 |

`Clusters` を先頭 1 件にするミューテーションは、当初 `arns[i:1]` と書いたところ `end` が未使用になりビルドが通らず、失敗 0 件と誤って記録された。`arns[i:end][:1]` に直して測り直している。ビルド失敗を失敗 0 件と数えないよう、測定はビルド結果を判定してから失敗数を数えている。確認後、実装は元に戻し、`git diff` で復元を確認した。

### 今回のテストで検証していない範囲

`DescribeTasks` / `DescribeClusters` のバッチ分割 (100 件ごと) は、テストデータが 2 件のため 1 バッチで完結し、境界の分割そのものは検証していない。各 API が失敗した場合のエラー経路も検証していない。

いずれも本 issue の完了条件が対象とする `Include` / `ServiceName` / `DesiredStatus` の構築の検証ではないためスコープ外とした。バッチ分割の境界はレビューで指摘されたが、101 件以上のテストデータを組む変更は、この issue が対象とする絞り込みパラメータの検証とは別の関心事であり、同種の issue 群 (0102 および 0112 から 0117) のどれもバッチ境界を扱っていない。エラー経路の未検証も同じく issue 群すべてに共通する範囲であり、0116 でこの issue 群では対象に含めないと決めている。

`mise run check` の通過を確認済み。CHANGES.md の `## develop` の `### misc` に記載済み。

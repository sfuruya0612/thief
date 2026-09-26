# ECS クラスタ ARN 列挙の重複とバッチ分割の手書き clamp を整理する

Created: 2026-07-16
Completed: 2026-07-16
Model: Claude Fable 5 claude-fable-5

## 根拠

1. クラスタ ARN のページネーション取得が 2 実装ある。
   - `backend/internal/aws/ecs.go` `ListECSResources` 内 (41-48 行)
   - `backend/internal/aws/ecs_cli.go` `ListECSClusterArns` (116-132 行)

   どちらも「NewListClustersPaginator でループし page.ClusterArns を append」する同一ロジック。

2. Describe 系 API のバッチ分割で、スライス上限の clamp が 2 方式混在している。
   - 手書き分岐 (`end := i + N; if end > len(arns) { end = len(arns) }`): `ecs.go` 55-59 行、`ecs_exec.go` 87-91 行 / 147-151 行
   - Go 組込み `min()`: `ecs_cli.go` 143 / 186 / 236 行

   同じパッケージ内で新旧の書き方が混在しており、片方だけ修正される事故を招きやすい。

## 対応方針

- クラスタ ARN 列挙のコアを「クライアントを引数で受け取る非公開関数」に分離し、`ListECSResources` と `ListECSClusterArns` の両方から再利用する (クライアント生成を 2 回に増やさないため、公開関数のシグネチャは変えない)。
- `ecs.go` / `ecs_exec.go` の手書き clamp を `min()` に統一する。

## API レスポンス / 画面表示への影響

なし。取得内容・順序・バッチ境界は同一。

## 解決方法

- `backend/internal/aws/ecs.go`: クラスタ ARN のページネーション取得を `listECSClusterArnsWith(ctx, client)` に抽出し、`ListECSResources` から使うようにした。
- `backend/internal/aws/ecs_cli.go`: `ListECSClusterArns` は `listECSClusterArnsWith` へのデリゲートにした (公開シグネチャは不変、クライアント生成回数も不変)。
- `ecs.go` / `ecs_exec.go` の手書き上限 clamp (`end := i + N; if end > len { ... }`) 3 箇所を `min()` に統一し、マジックナンバー (10 / 100) を既存のバッチサイズ定数 (`ecsDescribeServicesBatchSize` / `ecsDescribeTasksBatchSize` / `ecsDescribeClustersBatchSize`) に置き換えた。
- `mise run check` 全通過を確認した。

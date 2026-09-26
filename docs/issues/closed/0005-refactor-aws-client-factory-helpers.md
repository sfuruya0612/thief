# AWS クライアント生成クロージャの重複をサービス別ヘルパに集約する

Created: 2026-07-16
Completed: 2026-07-16
Model: Claude Fable 5 claude-fable-5

## 根拠

`backend/internal/aws/` では、同一サービスのクライアント生成クロージャが関数ごとにインラインでコピペされている。

```go
client, err := NewClient(ctx, profile, region, func(cfg aws.Config) *rds.Client {
    return rds.NewFromConfig(cfg)
})
if err != nil {
    return nil, err
}
```

同一ファイル内で複数回インライン展開している例: `ssm.go` (5)、`elb.go` (5)、`ecs_exec.go` (4)、`ecr.go` (4)、`rds.go` (3)、`dynamo.go` (3)、ほか 2 箇所ずつが多数。

一方で `cfn.go` の `newCfnClient`、`ecs_cli.go` の `newECSClient`、`s3_object.go` の `newS3ClientForBucket` は「サービスごとに 1 つのクライアント生成ヘルパ」パターンを既に確立している。特に `ecs.go` / `ecs_exec.go` は既存の `newECSClient` を使わずに同じクロージャをインライン展開しており、既存の共通化に追随していない。

## 対応方針

- サービスごとに `newXxxClient(ctx, profile, region)` ヘルパを定義し (既存の `newCfnClient` / `newECSClient` パターンに揃える)、インラインのクロージャをすべて置き換える。
- `ecs.go` / `ecs_exec.go` は既存の `newECSClient` を再利用する。
- Cost Explorer のようにリージョン固定 (us-east-1) のものはヘルパ内にその知識を寄せる。

## API レスポンス / 画面表示への影響

なし。生成されるクライアントと呼び出しは同一。

## 解決方法

- インラインの `NewClient` クロージャ 54 箇所をサービス別ヘルパ呼び出しに置換し、各サービスの主ファイルに `newXxxClient` を定義した (newEC2Client / newRDSClient / newElastiCacheClient / newLambdaClient / newECRClient / newS3Client / newIAMClient / newSSOClient / newSSMClient / newKinesisClient / newCloudFrontClient / newELBClient / newDynamoClient / newAPIGatewayClient / newAPIGatewayV2Client / newSQSClient / newWAFClient / newSecretsManagerClient / newCostExplorerClient / newSTSClient)。
- グローバルサービスはリージョン知識をヘルパに埋め込んだ: `newIAMClient(ctx, profile)` / `newCloudFrontClient(ctx, profile)` は us-east-1 固定、`newSTSClient(ctx, profile)` は SDK のデフォルト解決。
- `ecs.go` / `ecs_exec.go` は既存の `newECSClient` を、`cfn.go` の ListCFNStacks は既存の `newCfnClient` を再利用するようにした。
- `sso_oidc.go` に既存だったプロファイルなしの `newSSOClient(ctx, region)` は、sso.go の `newSSOClient(ctx, profile, region)` に統合し、呼び出し側で profile に空文字を渡す形にした (挙動は同一)。
- `mise run check` 全通過を確認した。

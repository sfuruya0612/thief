# QueryDynamoItems が同一リクエスト内で AWS config を二重ロードする無駄を解消する

Created: 2026-07-16
Completed: 2026-07-16
Model: Claude Fable 5 claude-fable-5

## 根拠

`backend/internal/aws/dynamo.go` の `QueryDynamoItems` は、冒頭 (227 行) で dynamodb クライアントを生成する (`NewClient` → `GetSession` → `config.LoadDefaultConfig`。`~/.aws/config` と認証情報の読み込みを伴う)。

PK 指定検索の場合、256 行で `DescribeDynamoTable(ctx, profile, region, table)` を呼ぶが、この関数 (142-147 行) は内部でもう一度 `NewClient` を呼んでクライアントを作り直すため、PK 検索 1 回につき config のロードと認証情報解決が 2 回走る。最初に作ったクライアントは末尾の `Query` (295 行) でしか使われない。

```go
client, err := NewClient(...)              // config ロード 1 回目
...
schema, err := DescribeDynamoTable(...)    // 内部で config ロード 2 回目 (同一 profile/region)
...
out, err := client.Query(ctx, input)       // 1 回目のクライアントを使用
```

## 対応方針

`DescribeDynamoTable` のコアを「クライアントを引数で受け取る非公開関数」に分離し、`QueryDynamoItems` は冒頭で生成したクライアントをそのまま渡す。公開 API の `DescribeDynamoTable(ctx, profile, region, table)` は薄いラッパとして残す。

## API レスポンス / 画面表示への影響

なし。取得されるスキーマ・アイテムは同一。

## 解決方法

- `backend/internal/aws/dynamo.go`: `DescribeDynamoTable` のコアを `describeDynamoTableWith(ctx, client, table)` に分離した。公開 API の `DescribeDynamoTable(ctx, profile, region, table)` はクライアントを生成してコアへ委譲する薄いラッパとして維持。
- `QueryDynamoItems` は冒頭で生成したクライアントを `describeDynamoTableWith` にそのまま渡すようにし、PK 指定検索での config ロードと認証情報解決を 2 回から 1 回に削減した。
- `mise run check` 全通過を確認した。

# SDK Tag 型ごとに重複する tagsToMap 実装をジェネリクスで統合する

Created: 2026-07-16
Completed: 2026-07-16
Model: Claude Fable 5 claude-fable-5

## 根拠

`backend/internal/aws/` に、本体が完全同一で型だけが異なる Tag → map 変換が 4 実装 + インライン 1 箇所ある。

- `ec2.go` `tagsToMap([]ec2types.Tag)`
- `rds.go` `rdsTagsToMap([]rdstypes.Tag)`
- `ecs.go` `ecsTagsToMap([]ecstypes.Tag)`
- `dynamo.go` `dynamoTagsToMap([]dynamodbtypes.Tag)`
- `waf.go` の `TagInfoForResource.TagList` を map 化するインラインループ

いずれも以下と同型:

```go
m := make(map[string]string, len(tags))
for _, t := range tags {
    m[ptrStr(t.Key)] = ptrStr(t.Value)
}
return m
```

サービス追加のたびに同じ関数が複製され続ける。

## 対応方針

各 SDK の Tag 型は別 struct のため、キー/値のアクセサを受け取るジェネリックヘルパへ集約する。

```go
func tagsToMapFunc[T any](tags []T, kv func(T) (key, value *string)) map[string]string
```

既存 4 関数と WAF のインラインループをこのヘルパ呼び出しに置き換える。

## API レスポンス / 画面表示への影響

なし。生成される map の内容は同一。

## 解決方法

- `backend/internal/aws/resource.go`: `tagsToMapFunc[T any](tags []T, kv func(T) (key, value *string)) map[string]string` を追加した。
- `ec2.go` の `tagsToMap` / `rds.go` の `rdsTagsToMap` / `ecs.go` の `ecsTagsToMap` / `dynamo.go` の `dynamoTagsToMap` は 1 行のデリゲートに置き換えた (呼び出し側と既存テストは無変更)。
- `waf.go` のインラインループは `tagsToMapFunc` の直接呼び出しに置き換えた。
- `resource_test.go` に tagsToMapFunc のテーブル駆動テスト (空 / 単一 / 複数 / nil キー / 重複キー last-wins) を追加した。
- `mise run check` 全通過を確認した。

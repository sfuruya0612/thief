# Datadog cost レスポンスの CostInfo 組み立てループを共通化する

Created: 2026-07-16
Completed: 2026-07-16
Model: Claude Fable 5 claude-fable-5

## 根拠

`backend/internal/datadog/usage.go` の `GetHistoricalCost` (44-58 行) と `GetEstimatedCost` (85-99 行) が、レスポンスから `CostInfo` を組み立てる二重ループを完全同一コードで持っている。

```go
result := []CostInfo{}
for _, item := range resp.GetData() {
    attrs := item.GetAttributes()
    for _, charge := range attrs.GetCharges() {
        result = append(result, CostInfo{
            Month:       attrs.GetDate().Format("2006-01"),
            ...
        })
    }
}
```

`GetHistoricalCostByOrg` と `GetEstimatedCostByOrg` の戻り値はどちらも `datadogV2.CostByOrgResponse` (datadog-api-client-go v2.55.0 で確認) であり、型の差異なく関数抽出できる。

## 対応方針

`costInfosFromResponse(resp datadogV2.CostByOrgResponse) []CostInfo` を抽出し、両関数から呼ぶ。

## CLI 出力 / API レスポンス / 画面表示への影響

なし。マッピング内容・順序・フォーマットは同一。

## 解決方法

- `backend/internal/datadog/usage.go`: `costInfosFromResponse(resp datadogV2.CostByOrgResponse) []CostInfo` を抽出し、`GetHistoricalCost` / `GetEstimatedCost` の重複ループを置換した。
- マッピングの正しさは既存のテーブル駆動テスト (TestGetHistoricalCost / TestGetEstimatedCost / 空データ 2 ケース) を無変更で通すことで担保した。
- `mise run check` 全通過を確認した。

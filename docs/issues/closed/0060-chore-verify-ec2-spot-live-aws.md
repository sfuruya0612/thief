# EC2 Spot (issue 0056) の実 AWS 環境での挙動確認が未了

Created: 2026-07-21
Completed: 2026-07-21
Model: Claude Opus 4.8

## 背景

issue 0056 で EC2 Spot 単価を独立サービス `ec2-spot` として実装したが、この開発環境には実 AWS 認証情報が無く、`DescribeSpotPriceHistory` への実呼び出しによる検証ができなかった。
実装は手書きフェイクによる自動テストのみで検証済みであり、以下の 2 点は実 AWS のレスポンス特性に依存するため、テストコードだけでは正しさを保証できない。

## 未検証の点

1. **`ec2SpotLookbackWindow` (現在 1 時間) の妥当性**：`backend/internal/aws/pricing.go` の `fetchEC2SpotRates` は `StartTime = now - 1h` で `DescribeSpotPriceHistory` を呼ぶ。Spot 価格の履歴エントリは価格が変化したときのみ記録されるため、変化頻度の低いインスタンスタイプ/ゾーンの組では直近 1 時間にエントリが無く、その組が単価表から欠落する可能性がある。実 AWS でリージョンを指定し、主要なインスタンスタイプ (m5/c5/r5 系等) がこの窓で取得できているか確認し、必要なら窓を広げる (例: 3〜24 時間) ことを検討する。
2. **`spotOSFromProductDescription` の網羅性**：`Linux/UNIX` / `Windows` の 2 系統 4 値は SDK の enum (`ec2types.RIProductDescription`) で確認済みだが、`Red Hat Enterprise Linux` / `SUSE Linux` とその `(Amazon VPC)` 変種は API ドキュメントの記載のみに基づく実装であり、実データでの確認ができていない。実 AWS で RHEL/SUSE インスタンスの Spot 価格を取得し、`os` 属性が On-Demand 側の語彙 (`RHEL`/`SUSE`) と一致することを確認する。

## 対応方針

- 実 AWS 環境 (認証情報が使えるマシン) で `mise run backend:run` を起動し、Pricing 画面の EC2 Spot カードで上記 2 点を確認する。
- 窓が狭すぎて欠落するインスタンスタイプが見つかった場合は `ec2SpotLookbackWindow` の値を調整する。
- OS 語彙のマッピング漏れが見つかった場合は `spotOSFromProductDescription` (`backend/internal/aws/pricing.go`) の switch 分岐を修正する。
- 確認が取れ、修正が不要であればこの issue はそのまま closed に移動し、確認結果を「## 解決方法」に記載する。修正が必要であれば修正後に closed へ移動する。

## 完了条件

- 実 AWS 環境で EC2 Spot カードの表示内容を確認し、上記 2 点について妥当性を確認または修正する。

## 解決方法

実 AWS 環境 (`example-dev` / `ap-northeast-1`) で `DescribeSpotPriceHistory` を直接呼び、未検証だった 2 点を確認した。

### 1. `ec2SpotLookbackWindow` (1 時間) の妥当性: 修正不要

直近 1 時間窓で 18111 件・1141 種のインスタンスタイプが取得でき、主要 family (m5/c5/r5 系) だけで 153 種が含まれていた。1 時間窓で主要インスタンスタイプの欠落は発生しないことを確認したため、窓は 1 時間のまま据え置く。確認結果を `ec2SpotLookbackWindow` のコメントに追記した。

### 2. `spotOSFromProductDescription` の網羅性: 修正あり

ProductDescription の distinct 値は以下だった。

```
Linux/UNIX
Red Hat Enterprise Linux
SUSE Linux
Ubuntu Pro Linux
Windows
```

ドキュメント記載の 4 系統 (Linux/UNIX, Red Hat Enterprise Linux, SUSE Linux, Windows) に加え、`Ubuntu Pro Linux` が実際に返ることが判明した。この値は switch の default に落ち、os チップが `Ubuntu Pro Linux` のまま表示されていた。On-Demand 側 (Pricing API の `operatingSystem`) は同 OS を `Ubuntu Pro` と表記するため、チップを揃えるべく `spotOSFromProductDescription` に `case "Ubuntu Pro Linux": return "Ubuntu Pro"` を追加した。`(Amazon VPC)` 変種は既存の suffix ストリップで同様に正規化される。単体テスト (`TestSpotOSFromProductDescription`) に Ubuntu Pro のケースを追加した。

- 変更ファイル: `backend/internal/aws/pricing.go` (`spotOSFromProductDescription` / `ec2SpotLookbackWindow` コメント)、`backend/internal/aws/pricing_test.go`
- `mise run backend:test` / `mise run backend:lint` 通過を確認

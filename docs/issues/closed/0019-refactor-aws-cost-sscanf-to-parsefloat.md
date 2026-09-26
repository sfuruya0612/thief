# cost.go の金額パースを fmt.Sscanf から strconv.ParseFloat に統一する

Created: 2026-07-16
Completed: 2026-07-16
Model: Claude Fable 5 claude-fable-5

## 根拠

`backend/internal/aws/cost.go` は Cost Explorer が返す金額文字列の float64 変換に `fmt.Sscanf(*m.Amount, "%f", &x)` を使っている (152 / 158 / 212 行)。

- `fmt.Sscanf` はフォーマット解析器を経由する重い経路であり、`ResultsByTime × Groups` の二重ループ内で呼ばれる。
- 同一パッケージの `dynamo.go` (325 行) は `strconv.ParseFloat` を使っており、数値パースの方法が不統一。
- `Sscanf` はエラーを握り潰しても部分一致 (例: `"12.34abc"` → 12.34) を受理するため、`ParseFloat` (全体が数値でなければエラー) の方が堅牢性の原則にも沿う。

## 対応方針 (対応保留)

3 箇所を `strconv.ParseFloat(*m.Amount, 64)` に置換する (パース失敗時は現状同様 0 のままとする)。

## 保留の理由

この変換結果は API レスポンス (`unblended_amount` / `net_amortized_amount` / forecast の `amount`) の値そのものを算出する箇所である。Cost Explorer が実際に返す 10 進文字列では両者の解釈は一致するが、不正入力 (数値 + 後続ゴミ等) に対しては解釈が異なりうるため、「API レスポンスに関わる箇所は書き換えない」という本タスクの制約に従い issue 起票のみとする。

## 解決方法

ユーザーから対応の明示指示を受けて実装した。

- `cost.go` に `costAmount(s *string) float64` ヘルパを追加した。内部は `strconv.ParseFloat` で、nil またはパース不能な文字列は 0 として扱う。`fmt.Sscanf` と異なり部分一致 (例: `"12.34abc"`) は受理せず 0 になる (堅牢性優先の厳密パース)。
- `GetCost` の UnblendedCost / NetAmortizedCost と `GetForecast` の MeanValue の 3 箇所を `costAmount` 呼び出しに置換した。nil チェックはヘルパ内に集約され、呼び出し側の分岐が消えた。
- `cost_test.go` に `TestCostAmount` (テーブル駆動 8 ケース: 10 進 / 0 / 負数 / 指数表記 / nil / 空文字 / 後続ゴミ / 非数値) を追加した。
- Cost Explorer が実際に返す 10 進文字列に対する挙動は従来と同一。挙動が変わるのは不正入力時のみ (部分一致受理 → 0)。
- `mise run check` 通過を確認した。

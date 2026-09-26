# RDS Savings Plans の Oracle/Db2 レートで Operation 違いの完全重複行が残る

Created: 2026-07-19
Completed: 2026-07-19
Model: Claude Opus 4.8

## 症状

issue 0049 (rate_id に productDescription を含めて一意性を確保する修正) をデプロイし直した後も、
RDS の Savings Plans で `rate_id` の重複が残る。issue 0049 が対象にしていた「同じ instance_type
で OS/engine 違い」のパターンは解消されたが、別の原因による重複が Oracle / Db2 エンジンに限って
残っている。

`ap-northeast-1` の RDS 単価を `refresh=true` で再取得し集計すると、100 件の重複 `rate_id`
グループが残っており、すべて `engine` が `Oracle` (64件) または `Db2` (36件) だった。

重複している行は `rate_id` だけでなく `label` / `model` / `attributes` / `term` / `price_usd`
まで完全に同一 (以下は実データ、`price_usd` も同じ 4.7424 usd/hr)。

```json
{"rate_id": "8870e805-fb04-4245-820e-231a54e5121b#APN1-InstanceUsage:db.m7i.12xl#Db2", "label": "db.m7i.12xl / Db2", "model": "savings_plan", "attributes": {"engine": "Db2", "instance_type": "db.m7i.12xl"}, "term": {"lease": "1yr", "offering_class": null, "payment": "No Upfront"}, "price_usd": 4.7424}
{"rate_id": "8870e805-fb04-4245-820e-231a54e5121b#APN1-InstanceUsage:db.m7i.12xl#Db2", "label": "db.m7i.12xl / Db2", "model": "savings_plan", "attributes": {"engine": "Db2", "instance_type": "db.m7i.12xl"}, "term": {"lease": "1yr", "offering_class": null, "payment": "No Upfront"}, "price_usd": 4.7424}
```

## 再現手順

1. `mise run backend:run` でバックエンドを起動する (issue 0049 修正後のバイナリであること)。
2. 実 AWS プロファイルで RDS の単価をキャッシュ再取得する。

   ```
   curl "http://127.0.0.1:8089/api/aws/profiles/<profile>/pricing?service=rds&region=ap-northeast-1&refresh=true" -o rds-fresh.json
   ```

3. `rate_id` の重複を集計する。

   ```python
   import json
   from collections import Counter
   with open('rds-fresh.json') as fp:
       data = json.load(fp)
   ids = [r['rate_id'] for r in data['rates']]
   dupe = {k: v for k, v in Counter(ids).items() if v > 1}
   print(len(dupe))  # 100 件
   engines = Counter()
   for rid in dupe:
       for r in data['rates']:
           if r['rate_id'] == rid:
               engines[r['attributes'].get('engine', '?')] += 1
               break
   print(engines)  # Counter({'Oracle': 64, 'Db2': 36})
   ```

4. ブラウザで Pricing 画面の RDS カードを開くと、`Encountered two children with the same key`
   警告が (Oracle/Db2 の重複分だけ) 出続ける。

## 原因

`backend/internal/aws/pricing.go` の `fetchSavingsPlans` (L551-582) は
`DescribeSavingsPlansOfferingRates` の `SearchResults` を `savingsPlanRateFrom` /
`instanceSavingsPlanRate` でそのまま `PriceRate` に変換するだけで、変換後の重複除去を行わない。

実際に AWS API から返る `SavingsPlanOfferingRate` を直接確認したところ (一時的なデバッグコードで
`CT Audit` プロファイルの実データを取得)、Oracle/Db2 の重複ペアは次のように **`Operation`
フィールドのみが異なり、他の全フィールド (`Rate`, `Properties`, `UsageType`,
`SavingsPlanOffering` 等) が完全一致**していた。

```json
{"Operation":"CreateDBInstance:0035","ProductType":"RDS","Properties":[{"Name":"productDescription","Value":"Db2"},{"Name":"instanceType","Value":"M7i"},{"Name":"region","Value":"ap-northeast-1"}],"Rate":"4.7424000000","SavingsPlanOffering":{"OfferingId":"8870e805-fb04-4245-820e-231a54e5121b", ...},"UsageType":"APN1-InstanceUsage:db.m7i.12xl"}
{"Operation":"CreateDBInstance:0029","ProductType":"RDS","Properties":[{"Name":"productDescription","Value":"Db2"},{"Name":"instanceType","Value":"M7i"},{"Name":"region","Value":"ap-northeast-1"}],"Rate":"4.7424000000","SavingsPlanOffering":{"OfferingId":"8870e805-fb04-4245-820e-231a54e5121b", ...},"UsageType":"APN1-InstanceUsage:db.m7i.12xl"}
```

Oracle でも同じパターンを確認した (`CreateDBInstance:0005` vs `CreateDBInstance:0019`、他は全項目一致)。

`Operation` は恐らく AWS 側の内部的な API オペレーションのバリエーション (Oracle/Db2 は BYOL や
複数のライセンス関連オペレーションを持つ商用エンジンのため、同じ実効レートに複数の `Operation`
コードが紐づいている可能性がある) だが、現在のどの正規化ロジック (`props`,
`instanceSavingsPlanRate` の `attrs`) もこれを見ておらず、ユーザーに表示される情報
(`label`/`price_usd`/`term`) に何の差異ももたらさない。

## 対応方針

issue 0049 (productDescription を rate_id に追加) とは異なるアプローチが必要:
`Operation` を `rate_id` に含めて一意にするだけでは、「全く同じ価格・内容の行が 2 行表示される」
というユーザーから見た問題 (Operation 違いはユーザーにとって意味のある区別ではない) が解決しない。

以下のいずれかで対応する。実装時に最も堅牢な方式を選定する。

- `fetchSavingsPlans` (または `instanceSavingsPlanRate` の呼び出し元) で、生成した `PriceRate`
  のうち `label` + `attributes` + `term` + `price_usd` + `unit` が完全一致する行を 1 つに
  デデュープする (`Operation` 違いはユーザーに見せる情報を一切変えないため、実質的に同じ行として
  扱ってよい)。
- デデュープの判定キーは `RateID` (productDescription 込みの現行仕様) ではなく、表示・計算に
  実際に使う値の組み合わせにする (`RateID` 自体に `Operation` を含めると、意味のない差異で
  再び一意な行が量産されてしまい、根本解決にならない)。
- Oracle/Db2 以外のエンジン・EC2/ElastiCache の Savings Plans でも同じ `Operation` 違いの完全
  重複が起きていないか、実データで確認する (今回は RDS のみ確認済み)。

## 根拠

issue 0051 (Pricing 見積もり明細への期間/購入タイプ表示) の実ブラウザ検証のため backend
プロセスを再起動し、issue 0049 の修正が実機に反映されたことを確認する過程で発見した。
`curl` で `refresh=true` を指定して RDS の単価を直接取得し、Python で `rate_id` の重複を
集計したところ、issue 0049 の修正後も 100 件の重複が残っており、原因調査のため一時的な
デバッグテスト (本コミットには含めない) で実際の `DescribeSavingsPlansOfferingRates` の
生レスポンスを確認し、上記の `Operation` 差異を特定した。EC2 (`ap-northeast-1`) についても
同様の重複が起きていないかは未確認。

## 解決方法

`backend/internal/aws/pricing.go` の `fetchSavingsPlans` に `dedupeSavingsPlanRates` を追加し、
ページ収集後の `PriceRate` スライスに対して適用した。判定キーは `RateID` 単体ではなく
`PriceRate` 全体 (`RateID`/`Model`/`Group`/`Label`/`Attributes`/`Term`/`Unit`/`PriceUSD`/
`UpfrontUSD`/`Currency`) を `json.Marshal` した文字列にした。対応方針に記載の通り、`RateID`
に `Operation` を追加する方式は採らなかった (それでは一意な React key は得られても「見た目に
は全く同じ内容の行が複数表示される」という UX 上の重複が残るため)。`json.Marshal` は Go の
map→JSON 変換がキーを常にソートするため、`Attributes` (map) の順序に左右されず安定する。

```go
func dedupeSavingsPlanRates(rates []PriceRate) []PriceRate {
	seen := make(map[string]bool, len(rates))
	out := make([]PriceRate, 0, len(rates))
	for _, r := range rates {
		key, err := json.Marshal(r)
		if err != nil {
			out = append(out, r) // Marshal 失敗時は安全側 (行を残す) に倒す
			continue
		}
		if seen[string(key)] {
			continue
		}
		seen[string(key)] = true
		out = append(out, r)
	}
	return out
}
```

既存の `TestFetchSavingsPlansPagination` は、ページネーションの検証のために意図的に「同じ内容
のレートを 2 ページ分返す」フェイクを使っており、このデデュープ追加によって (正しく) 1 件に
まとめられてしまい壊れた。テストの意図 (2 ページ分のデータが両方結果に含まれること) を保った
まま、2 ページ目のレートを 1 ページ目と異なる `instanceType` にする形に修正した。

### 実施した検証

- `backend/internal/aws/pricing_test.go` に `TestDedupeSavingsPlanRates` (純関数の単体テスト:
  完全一致は 1 件にまとまる、`RateID` が同じでも価格が異なれば別行として残る、互いに異なる
  複数行は残る、空スライス) と `TestFetchSavingsPlansDedupesOperationVariants` (issue の実データ
  形状を再現した統合テスト) を追加した。
- 修正なし (`fetchSavingsPlans` の返り値からデデュープ呼び出しだけを一時的に外す) の状態で
  `TestFetchSavingsPlansDedupesOperationVariants` を実行し、`returned 2 rates, want 1` で
  意図通り FAIL することを確認した。
- `mise run check` (backend/frontend の fmt + lint + test) が全て通過することを確認した。
- 実際に backend プロセスを再ビルド・再起動し、`curl ...&refresh=true` で RDS の単価を
  再取得して実データで検証した。修正前は 100 件だった重複 `rate_id` グループが 18 件まで
  減少した。

### 検証で判明した別件 (issue 0052 の対象外、新規 issue 0053 として切り出し)

残った 18 件は調査の結果、Operation 違いの完全重複 (今回の修正対象) とは異なり、**同じ
`rate_id`/`label` だが `price_usd` が本当に異なる** (すべて Oracle、ちょうど 2 倍の価格差)
ペアだった。`dedupeSavingsPlanRates` は「表示・計算に使う値が完全一致する場合のみ」統合する
設計のため、これらは (意図通り) 統合されず別行として残っている。生データを確認したところ、
価格が異なる 2 件は `Operation` が異なるだけでなく、一方は `productDescription: "Oracle"`
Rate 1.6448、もう一方も `productDescription: "Oracle"` Rate 0.832 で、Properties に他の
区別可能な属性 (ライセンスモデル等) が含まれていなかった。RDS for Oracle の
BYOL (Bring Your Own License) / License Included の価格差 (BYOL は概ね半額) と符合する。
つまり、現在のコードは Oracle の BYOL/LI を区別する情報を一切保持しておらず、これは
「重複除去」ではなく「区別に必要な属性が欠落している」という別種のバグのため、本 issue の
スコープ外として issue 0053 に切り出した。

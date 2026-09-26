# Savings Plans のライセンス依存レートで BYOL 等の区別が rate_id/label に反映されない (RDS Oracle / EC2 Windows)

Created: 2026-07-19
Completed: 2026-07-21
Model: Claude Opus 4.8

## 症状

issue 0052 (Operation 違いの完全重複除去) の実データ検証中に判明した別件。Savings Plans の
一部の instance_type について、同じ `rate_id` (同じ offeringId + usageType +
productDescription) を持つが `price_usd` が有意に異なる複数行が存在する。

issue 0052 で追加した重複除去 (`dedupeSavingsPlanRates`) は「表示・計算に使う値が完全一致する
場合のみ」統合する設計のため、これらの行は (意図通り) 統合されずに両方残る。しかし `rate_id`/
`label`/`attributes` は完全に同一なので、ブラウザ上では「見た目には全く同じ行が価格だけ違って
複数存在する」ように見え、React の key 重複警告も (issue 0052 の対応後も) 引き続き発生する。

RDS (Oracle) と EC2 (Windows) の両方で確認した。

```json
{"rate_id": "8870e805-fb04-4245-820e-231a54e5121b#APN1-InstanceUsage:db.m8i.2xl#Oracle", "label": "db.m8i.2xlarge / Oracle", "price_usd": 1.6448}
{"rate_id": "8870e805-fb04-4245-820e-231a54e5121b#APN1-InstanceUsage:db.m8i.2xl#Oracle", "label": "db.m8i.2xlarge / Oracle", "price_usd": 0.832}
```

```json
{"rate_id": "3873f699-df91-49d4-9d6a-8bb78d5ed11a#APN1-BoxUsage:c3.2xlarge#Windows", "label": "c3.2xlarge / Windows", "price_usd": 0.772}
{"rate_id": "3873f699-df91-49d4-9d6a-8bb78d5ed11a#APN1-BoxUsage:c3.2xlarge#Windows", "label": "c3.2xlarge / Windows", "price_usd": 0.404}
```

## 再現手順

1. `mise run backend:run` でバックエンドを起動する (issue 0052 修正後のバイナリであること)。
2. 実 AWS プロファイルで RDS / EC2 の単価をキャッシュ再取得する。

   ```
   curl "http://127.0.0.1:8089/api/aws/profiles/<profile>/pricing?service=rds&region=ap-northeast-1&refresh=true" -o rds-fresh.json
   curl "http://127.0.0.1:8089/api/aws/profiles/<profile>/pricing?service=ec2&region=ap-northeast-1&refresh=true" -o ec2-fresh.json
   ```

3. `rate_id` の重複と価格差を集計する。

   ```python
   import json
   from collections import Counter
   for path, engine_key in [("rds-fresh.json", "engine"), ("ec2-fresh.json", "os")]:
       with open(path) as fp:
           data = json.load(fp)
       ids = [r["rate_id"] for r in data["rates"]]
       dupe = {k: v for k, v in Counter(ids).items() if v > 1}
       print(path, "duplicate groups:", len(dupe))
       breakdown = Counter()
       for rid in dupe:
           matched = [r for r in data["rates"] if r["rate_id"] == rid]
           breakdown[matched[0]["attributes"].get(engine_key, "?")] += 1
       print(path, "breakdown:", breakdown)
   ```

4. 2026-07-19 時点の実データでの結果:
   - RDS: 重複 18 件、すべて `engine: Oracle`、グループサイズ 2、価格比はいずれもおよそ 2 倍。
   - EC2: 重複 10199 件、すべて `os: Windows`、グループサイズ 2、価格比は 1.7〜2.1 倍程度で
     ばらつきがある (固定比ではない)。

## 原因

Amazon RDS for Oracle には BYOL (Bring Your Own License) と License Included (LI) という
2 つのライセンスモデルがあり、LI は Oracle のライセンス費用を含むため BYOL のおよそ 2 倍の
価格になることが知られている (今回確認した価格比とも符合する)。EC2 の Windows についても
同様に、ライセンス込み/持ち込み等の課金体系の違いにより同一 instance_type で複数の価格が
存在しうる。

`DescribeSavingsPlansOfferingRates` の `Properties` には `productDescription` (RDS)/
`os` 相当の情報のみが含まれ、ライセンスモデルを示す属性は別途存在しない。価格が異なる行同士は
`Operation` フィールドの値が異なる (例: `CreateDBInstance:0020` と `CreateDBInstance:0005`)
が、`Operation` の生の値からライセンスモデルを判定できるマッピングは API レスポンスのどこにも
含まれておらず、`backend/internal/aws/pricing.go` の `instanceSavingsPlanRate` /
`savingsPlanProperties` もこの情報を一切パースしていない。

issue 0052 で追加した `dedupeSavingsPlanRates` は、これらの行を (価格が異なるため正しく)
別行として残す。つまり issue 0052 の対応は「本来区別すべき行を誤って統合してしまう」バグを
防いでいる一方で、根本原因である「ライセンスモデル (または同等の区別軸) を示す属性の欠落」
自体は解決していない。

## 対応方針

以下のいずれか、または組み合わせで対応する。実装時に最も堅牢な方式を選定する。

- Reserved Instances / On-Demand 側の同じ instance_type + engine/os の価格データ (ライセンス
  モデルが `productDescription` に "Windows" と "Windows (Amazon VPC)" のように、あるいは
  RDS で "Oracle" と "Oracle (BYOL)" のように明示されているはずの既存データ) と価格を突き
  合わせ、Savings Plans 側のどちらの行がどちらのライセンスモデルに対応するかを推測する。
  単価データに依存した方式のため、価格改定等で比率が変わった場合に誤判定するリスクがある点に
  留意する。EC2 Windows は価格比が固定でない (1.7〜2.1 倍) ことから、単純な比率判定ではなく
  実際の Reserved/On-Demand レートとの突き合わせが必要になる可能性が高い。
- `Operation` の値と AWS 公式ドキュメント/価格表を突き合わせ、ライセンスモデルとの対応関係を
  確認できるか調査する (対応が確認できれば `attrs` に `license_model` を追加し `rate_id`/
  `label` にも反映する)。
- 上記のいずれも困難な場合、最低限の対応として `rate_id` に `Operation` を追加して一意にし、
  `label` にも `Operation` 由来の連番等 (例 "パターン A" / "パターン B") を付けて区別可能に
  する。ライセンスモデルの実体を示せない点はドキュメント上の既知の制約として明記する。
- RDS/EC2 以外 (ElastiCache) や、Oracle/Windows 以外の商用ライセンスエンジン
  (SQL Server 等) でも同じ問題が起きていないか実データで確認する。

## 検証

- RDS Oracle / EC2 Windows の Savings Plans 単価を再取得し、`rate_id` の重複が解消される
  (または少なくとも `label` で区別できる) ことを確認する。
- 既存の `TestInstanceSavingsPlanRate` / `TestDedupeSavingsPlanRates` が壊れていないことを
  確認する。
- `mise run check` が全て通過することを確認する。

## 根拠

issue 0052 (Operation 違いの完全重複除去) の実データ検証で、修正後も RDS に 18 件の重複が
残ることに気付き調査した。生データを確認したところ、Operation 違いの完全重複 (issue 0052 の
対象、price まで完全一致) とは異なり、この 18 件は price_usd が約 2 倍異なっていた。RDS for
Oracle の BYOL/LI の価格差 (概ね 2 倍) と符合することから、ライセンスモデル違いの可能性が
高いと判断した。念のため EC2 でも同様の検証を行ったところ、Windows で 10199 件という RDS
より遥かに大きい規模の重複が見つかり、同じ構造の問題が複数サービスにまたがって存在することを
確認した。

## 解決方法

一時デバッグテストで実際の AWS API (`GetProducts` と `DescribeSavingsPlansOfferingRates`) を
呼び、`SavingsPlanOfferingRate.Operation` (Properties とは別の SDK フィールド) が On-Demand
側の `Product.Attributes["operation"]`/`["licenseModel"]` と対応することを実データで確認した
(RDS Oracle: `CreateDBInstance:0020`→`License included`、`CreateDBInstance:0005`→
`Bring your own license`。EC2 Windows: `RunInstances:0002`→`No License required`、
`RunInstances:0002:box`→`License Included - Infrastructure`)。追加の API 呼び出し無しで
Savings Plans 側から逆引きできることが分かったため、以下の方針で実装した。

- `PriceRate` に内部専用フィールド `Operation`(`json:"-"`)を追加し、
  `savingsPlanRateFrom`/`instanceSavingsPlanRate` が `SavingsPlanOfferingRate.Operation` を
  保持するようにした。
- `instanceOnDemandRatesFromDocument` が On-Demand/Reserved の生属性を解析する過程で、新設した
  `recordOperationLicenseModel` により `operation → licenseModel` の対応表を副産物として組み立てる
  ようにした (`fetchOnDemandAndReserved` の戻り値に追加)。同一 Operation に矛盾する licenseModel
  が観測された場合は先勝ちで警告ログを出す。
- `getPricing` で On-Demand/Reserved と Savings Plans の並行フェッチが完了した後、新設した
  `applySavingsPlanLicenseModel` が対応表を使って Savings Plans レートの
  `RateID`/`Label`/`Attributes["license_model"]` にライセンスモデルを反映するようにした
  (dedupe 自体は price_usd も含めて完全一致する行のみ統合するため、既存の
  `dedupeSavingsPlanRates` の後段に追加しても正しく機能する)。
- 対応範囲は「Savings Plans のみ」ではなく「On-Demand/Reserved も含めて統一する」方針を採用し、
  `instanceLabel`(RDS/EC2)と `curatedInstanceAttributes`(EC2)にも license_model を追加した。
  表示方針は「license_model が判明していれば常に表示する」(既存の deployment_option/
  storage_type と同じ扱い) を採用した。
- frontend の `PRICING_ATTRIBUTE_FILTERS` に ec2/rds 向けの `license_model` 絞り込みチップを
  追加した。

RDS Oracle (License Included/BYOL 実データ相当) の 2 行が Savings Plans 側でも distinct な
RateID/Label に解決されることを再現テスト (`TestGetPricingResolvesSavingsPlanLicenseModel`) で
確認し、`mise run check` (backend/frontend の fmt・lint・test) が全て通過することを確認した。

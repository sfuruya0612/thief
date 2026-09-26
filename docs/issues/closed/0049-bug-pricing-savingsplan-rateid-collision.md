# Savings Plans のレートで rate_id が衝突し React key 重複・選択状態の誤連動が起きる

Created: 2026-07-19
Completed: 2026-07-19
Model: Claude Sonnet 5

## 症状

Pricing 画面 (`/api/aws/profiles/{profile}/pricing`) の EC2 / RDS の Savings Plans グループで、同じ `instance_type` かつ同じ支払い条件でも `productDescription` (EC2 は OS、RDS/ElastiCache は engine) が異なる複数の単価行が、同一の `rate_id` を持ってしまう。

ブラウザのコンソールには次の React 警告が (該当行の分だけ) 出力される。

```
Warning: Encountered two children with the same key, `%s`. Keys should be unique so that
components maintain their identity across updates. Non-unique keys may cause children to
be duplicated and/or omitted — the behavior is unsupported and could change in a future
version.%s 8870e805-fb04-4245-820e-231a54e5121b#APN1-InstanceUsage:db.m7g.12xl
```

`PricingPanel` の選択状態 (`lib/pricingSelection.ts` の `selection: Record<rate_id, {checked, qty}>`) は `rate_id` をキーにしているため、重複した `rate_id` を持つ行はチェック状態を共有してしまう。ある行をチェックすると、見た目には気づかないまま同じ `rate_id` を持つ他の行 (異なる OS / engine) も同時にチェックされたのと同じ内部状態になり、見積もり (`lib/pricingEstimate.ts`) の計算にも波及する。

## 再現手順

1. `mise run backend:run` / `mise run frontend:run` を起動し、実 AWS プロファイルで RDS または EC2 の単価を取得する (更新ボタンでキャッシュを最新化)。
2. `/tmp/thief/price/{service}/{region}.json` の `data.rates[].rate_id` を集計し、重複を確認する。

```python
import json
from collections import Counter
with open('/tmp/thief/price/rds/ap-northeast-1.json') as fp:
    data = json.load(fp)['data']
ids = [r['rate_id'] for r in data['rates']]
dupe = {k: v for k, v in Counter(ids).items() if v > 1}
print(len(dupe))  # ap-northeast-1 の実データで 202 件 (RDS) / 13882 件 (EC2) の重複を確認済み
```

3. 重複した `rate_id` の1つについて `label` を見ると、`engine`/`os` だけが異なる複数行であることが分かる。

```json
{"rate_id": "8870e805-fb04-4245-820e-231a54e5121b#APN1-InstanceUsage:db.m7g.12xl", "label": "db.m7g.12xl / MariaDB", ...}
{"rate_id": "8870e805-fb04-4245-820e-231a54e5121b#APN1-InstanceUsage:db.m7g.12xl", "label": "db.m7g.12xl / MySQL", ...}
{"rate_id": "8870e805-fb04-4245-820e-231a54e5121b#APN1-InstanceUsage:db.m7g.12xl", "label": "db.m7g.12xl / PostgreSQL", ...}
```

4. ブラウザで Pricing 画面の RDS/EC2 カードを開き DevTools コンソールを見ると、上記の key 重複警告が大量に出力される。

## 原因

`backend/internal/aws/pricing.go` の `instanceSavingsPlanRate` (EC2/RDS/ElastiCache の Savings Plans 共通処理) が `RateID` を次の形で生成している。

```go
return PriceRate{
    RateID:     ptrStr(offering.OfferingId) + "#" + usageType,
    ...
```

`usageType` (例: `APN1-InstanceUsage:db.m7g.12xl`) には instance_type は含まれるが `productDescription` (RDS の engine・EC2 の OS) は含まれない。Savings Plans は同じ `offeringId` + `usageType` に対して MySQL/MariaDB/PostgreSQL (RDS) や Linux/Windows/RHEL (EC2) など複数の `productDescription` の料金を返すため (Savings Plans が特定の OS/engine に縛られず柔軟に適用される契約であることの表れ)、`productDescription` を含まない現在の `RateID` は一意性を保証できない。

`ecsSavingsPlanRate` は ECS 側で同様の生成をしているが、ECS の Fargate には engine/os の別軸を持つ行が少なく (`os`/`architecture` のみ)、実データでは重複が確認されていない (別途確認が必要)。

## 対応方針

`RateID` に `productDescription` (または `attrs` に格納する `os`/`engine` と同じ情報) を含めて一意性を確保する。例:

```go
RateID: ptrStr(offering.OfferingId) + "#" + usageType + "#" + productDescription,
```

既存のキャッシュファイルは古い形式の `rate_id` を含んだままになるため、修正後は利用者側で更新ボタンによる再取得が必要になる (自動移行は行わない、既存の「キャッシュは更新ボタンでのみ再取得」という仕様と一貫させる)。

## 根拠

issue 0045 のフォローアップ (RDS Aurora 除外 + 属性フィルタ機能の追加) の実ブラウザ確認中に、RDS の Engine 属性フィルタの動作検証で意図しない結果が出たことをきっかけに発見した。`/tmp/thief/price/rds/ap-northeast-1.json` (2026-07-19 時点、Aurora 除外後の実データ) を集計し、202 件の `rate_id` 重複を確認した。同様に EC2 (`/tmp/thief/price/ec2/ap-northeast-1.json`) でも 13882 件の重複を確認した。ElastiCache は 0 件 (cacheEngine 違いの SP 行が実データ上存在しないため)。

## 解決方法

`backend/internal/aws/pricing.go` の `instanceSavingsPlanRate` (ec2/rds/elasticache 共通) で、
対応方針通り `RateID` に `productDescription` を追加した。

```go
RateID: ptrStr(offering.OfferingId) + "#" + usageType + "#" + productDescription,
```

`ecsSavingsPlanRate` (ECS Fargate) は対象外とした。ECS は OS/architecture の別軸が
`productDescription` ではなく `usageType` 文字列自体に `"Windows"`/`"ARM"` として埋め込まれる
実装になっており (例: `APN1-Fargate-ARM-vCPU-Hours:perCPU`)、`offeringId + usageType` の組で
既に一意になるため (実データ調査で重複 0 件だったことと整合する)。この根拠をコード上のコメント
として残した。

既存のキャッシュファイルは古い形式の `rate_id` を含んだままになる点は対応方針に記載の通り
既存仕様 (「キャッシュは更新ボタンでのみ再取得」) と一貫させ、自動移行は行っていない。

### 実施した検証

- `backend/internal/aws/pricing_test.go` の `TestInstanceSavingsPlanRate` に、issue で報告した
  実シナリオ (同じ `offeringId` + `usageType` = 同じ `db.m7g.12xlarge`、`productDescription`
  のみ MySQL/MariaDB で異なる) を再現するテストケースを追加し、生成される `RateID` が異なる
  ことを確認した。
- 修正前のコード (`git stash` で `pricing.go` のみ一時的に戻す) に対して同じテストを実行し、
  `RateID collision: MySQL and MariaDB both got "offer-1#APN1-InstanceUsage:db.m7g.12xl"` で
  意図通り FAIL することを確認した (テストがこの不具合を検出できることの裏付け)。
- 修正後は `TestInstanceSavingsPlanRate` (新規ケース含む) と `TestEcsSavingsPlanRate` が
  全て PASS することを確認した。
- `mise run check` (backend/frontend の fmt + lint + test) が全て通過することを確認した。
- 実 AWS プロファイルでの EC2/RDS Savings Plans 再取得によるブラウザ実機確認 (React key
  重複警告が消えること) は、実 AWS 環境での更新ボタン操作が必要なため本セッションでは
  実施していない。ユニットテストで `rate_id` 生成ロジック自体の重複が解消されることを
  実データと同型の入力で検証済みであり、フロントエンド側 (`lib/pricingSelection.ts` の
  `Record<rate_id, ...>`、React の `key={rate_id}`) はバックエンドが返す `rate_id` の一意性を
  前提にしているだけで独自のロジックは持たないため、バックエンド側の修正のみで解消すると
  判断した。

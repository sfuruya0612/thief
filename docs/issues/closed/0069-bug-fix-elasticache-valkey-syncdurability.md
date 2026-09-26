# Elasticache OnDemand の Valkey 行が SyncDurability メーターで重複するのを label と属性で区別する

Created: 2026-07-23
Completed: 2026-07-23
Model: Claude Opus 4.8

## 背景

AWS Pricing の Elasticache OnDemand で、cache.r7g / cache.m6g などのノードで同一 instanceType の行が複数表示されるバグがある。

## 原因

ElastiCache Valkey には、同一 instanceType / cacheEngine に対して 2 つの usagetype を持つ SKU が存在する (ap-northeast-1 の cache.r7g.large の生 GetProducts で確認)。

- `APN1-NodeUsage:cache.r7g.large` (operation: CreateCacheCluster:Valkey) — 通常のノード基本料金。標準 On-Demand 単価 (0.2104 USD/hr = Redis 0.263 x 0.8、AWS 公式の Valkey 20%引きに一致)。
- `APN1-SyncDurability-NodeUsage:cache.r7g.large` (operation: CreateCacheCluster:Valkey) — 同期耐久性オプション (Multi-AZ 同期書き込み) の追加メーター (0.0379 USD/hr)。

`instanceLabel` (pricing.go の elasticache ケース) が Elasticache のラベルを `instanceType / cacheEngine` の 2 属性だけで生成するため、この 2 SKU が同一ラベル `cache.r7g.large / Valkey` に潰れ、テーブルに 2 行として表示される。Redis / Memcached は NodeUsage 1 件のみ (ExtendedSupport 系は除外済み) のため単一行で、Valkey だけが重複していた。r7g / m6g の全サイズで発生する。

## Reopen (2026-07-23)

当初は SyncDurability メーターを ExtendedSupport と同じ層で除外して重複を解消した (label が一意になり重複は消えた)。しかしユーザーから「SyncDurability を除外せず、フィルターとして追加し、各行が SyncDurability かどうかを表示してほしい」との方針変更があった。除外では SyncDurability の単価を一切参照できなくなるため、除外をやめて label と属性で区別する方式に作り直す。

「各行が SyncDurability かどうかを表示」の見せ方は、RDS の storage_type (Standard / IO-Optimized を全行のラベルに付す) と同じく、全 Elasticache 行のラベル末尾に Standard / SyncDurability を明示する方式を選んだ。SyncDurability メーターは Valkey にのみ存在するが、各行の表示を揃えるため Redis / Memcached も含め全行に付す。

## 修正方針

SyncDurability メーターを除外せず、RDS の storage_type と同じ方式で区別する。

- `instanceLabel` の elasticache ケースで、usagetype に SyncDurability を含む行は `SyncDurability`、それ以外は `Standard` を、全行のラベル末尾に付す。これで重複していた 2 行が別ラベルになり、Redis / Memcached を含む全行が Standard か SyncDurability かを明示する。
- `curatedInstanceAttributes` の elasticache ケースで、sync_durability 属性を全行に付与する (SyncDurability を含めば `SyncDurability`、それ以外は `Standard`)。
- frontend の `PRICING_ATTRIBUTE_FILTERS.elasticache` に sync_durability チップ (label "Sync durability") を追加する。Standard / SyncDurability の 2 値が存在するときだけチップが表示される (AttributeFilterBar の値種別 > 1 の条件)。

ExtendedSupport (EOL 延長サポート課金) の除外はそのまま残す (ユーザー指示は SyncDurability のみに言及)。Savings Plans の行は instanceSavingsPlanRate が label / 属性を独自に生成する経路のため、本変更の影響を受けない (sync_durability は付かない)。

## 完了条件

- cache.r7g / cache.m6g など全ノードで Valkey の通常ノード課金と SyncDurability 課金が別ラベルの行として表示される。
- 全 Elasticache 行のラベルが Standard か SyncDurability かを明示し、sync_durability 属性とチップで絞り込める。
- backend の pricing テストに、標準ノード課金 (Standard) と SyncDurability 課金 (SyncDurability) 双方が残り正しくラベル付けされるケースを追加する。
- mise run check が全て通過する。

## 解決方法

backend/internal/aws/pricing.go を変更した。

1. `instanceOnDemandRatesFromDocument` の除外条件から SyncDurability を外し、ExtendedSupport のみの除外に戻した。

```go
if strings.Contains(attrs["usagetype"], "ExtendedSupport") {
	return nil
}
```

2. `instanceLabel` の elasticache ケースで、全行のラベル末尾に Standard / SyncDurability を付すようにした (RDS の storage_type と同じ方針)。

```go
case "elasticache":
	syncDurability := "Standard"
	if strings.Contains(attrs["usagetype"], "SyncDurability") {
		syncDurability = "SyncDurability"
	}
	return joinNonEmpty(" / ", attrs["instanceType"], attrs["cacheEngine"], syncDurability)
```

3. `curatedInstanceAttributes` の elasticache ケースで、sync_durability 属性 (Standard / SyncDurability) を全行に付与するようにした。

```go
if strings.Contains(attrs["usagetype"], "SyncDurability") {
	out["sync_durability"] = "SyncDurability"
} else {
	out["sync_durability"] = "Standard"
}
```

frontend/src/lib/pricingAttributeFilters.ts の elasticache に sync_durability チップ (label "Sync durability") を追加した。

backend/internal/aws/pricing_test.go の期待値を、全 Elasticache 行のラベルに Standard / SyncDurability が付く形に更新した。Redis の NodeUsage は `cache.t3.micro / Redis / Standard`、Valkey の NodeUsage は `cache.r7g.large / Valkey / Standard`、Valkey の SyncDurability メーターは `cache.r7g.large / Valkey / SyncDurability` (0.0379、sync_durability=SyncDurability) になる。frontend/src/lib/pricingAttributeFilters.test.ts の elasticache 軸テストを 3 軸 (instance_family / engine / sync_durability) に更新した。

label が Standard 行と SyncDurability 行で一意になったため、`onDemandHourlyByLabel` (frontend/src/components/pricing/ServiceCard.tsx) の後勝ちも解消し、Reserved Instance の節減率が各 label の正しい On-Demand 単価で算出される。

`mise run check` (backend / frontend の fmt + lint + test) が全て通過することを確認した。

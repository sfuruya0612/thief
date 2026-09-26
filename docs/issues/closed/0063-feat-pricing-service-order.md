# AWS Pricing のサービス表示順を固定する

Created: 2026-07-21
Completed: 2026-07-21
Model: Claude Opus 4.8

## 背景

`docs/issues/TODO.md` に「AWS Pricing のサービスの順番は EC2, EC2 Spot, RDS, Elasticache, Compute Savings Plans, EC2 Instance Savings Plans, Database Savings Plans とする」という要望がある。

要望リストには ECS (Fargate) の記載が無かったが、2026-07-21 にユーザーへ確認し、ECS は EC2 Spot の後ろに残すと決定した。最終的な順序は次の通り。

1. EC2
2. EC2 Spot
3. ECS (Fargate)
4. RDS
5. ElastiCache
6. Compute Savings Plans
7. EC2 Instance Savings Plans
8. Database Savings Plans

## 現状

- サービスのスラッグと表示名は `lib/pricingSelection.ts` の `PRICING_SERVICES` / `PRICING_SERVICE_LABELS` に定義される。現在の順は `ec2, rds, elasticache, ecs, compute-sp, ec2-instance-sp, database-sp, ec2-spot`。
- サービス選択バー (`components/pricing/ServiceSelectorBar.tsx`) は `PRICING_SERVICES` 順で描画される。
- サービスカードの一覧 (`views/PricingPanel.tsx`) は `state.activeServices` の順 (永続化された配列の順) で描画されるため、既存ユーザーでは `PRICING_SERVICES` を並べ替えてもカード順に反映されない。

## 目的

- Pricing のサービス選択バーとカード一覧を、上記で決定した固定順で表示する。
- 既存ユーザー (永続化済み) でも新しい順序で表示されるようにする。

## 完了条件

- `PRICING_SERVICES` を決定した順序 (`ec2, ec2-spot, ecs, rds, elasticache, compute-sp, ec2-instance-sp, database-sp`) に並べ替える。
- `PricingPanel` のカード描画を `state.activeServices` 順ではなく `PRICING_SERVICES` 順 (アクティブなものだけ) に変更する。
- `mise run check` が全て通過する。

## 解決方法

- `lib/pricingSelection.ts` の `PRICING_SERVICES` を決定順 (`ec2, ec2-spot, ecs, rds, elasticache, compute-sp, ec2-instance-sp, database-sp`) に並べ替えた。選択バー (`components/pricing/ServiceSelectorBar.tsx`) はこの配列順で描画するため自動的に新順序になる。
- `views/PricingPanel.tsx` のカード一覧の描画を `state.activeServices.filter(isPricingService)` から `PRICING_SERVICES.filter((service) => activeSet.has(service))` に変更した。これにより永続化済みユーザーでもカードが `PRICING_SERVICES` の固定順で表示される。
- `migratePricingState` の補完順は `PRICING_SCHEMA_MIGRATIONS` 由来で `PRICING_SERVICES` の並びに依存しないため、`lib/pricingSelection.test.ts` のマイグレーション期待値は変更不要だった。

### 検証

- `mise run frontend:lint`: 0 errors、警告 9 件 (既存のみ)。
- `mise run frontend:test`: 57 ファイル / 506 テスト全て pass (`PricingPanel.test.tsx` を含む)。
- `mise run frontend:fmt`: 変更ファイルは整形済み (unchanged)。

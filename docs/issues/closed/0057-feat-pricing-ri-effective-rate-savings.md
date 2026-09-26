# Reserved Instance の単価に前払い、月額、実効時間単価、On-Demand 比の節減率を表示する

Created: 2026-07-21
Completed: 2026-07-21
Model: Claude Opus 4.8

## 背景

Pricing 画面の単価表 (`components/pricing/RateGroupSection.tsx`) は、Reserved Instances (以下 RI) の行について、時間単価 (継続分) と、前払いがある場合の「+ $X 前払い」しか表示しない。
RI は前払いと時間単価の組み合わせで実質コストが決まるため、時間単価だけでは支払オプション (No Upfront / Partial Upfront / All Upfront) をまたいだ比較ができない。
特に All Upfront は継続時間単価が 0 になり、前払い額を時間あたりに按分しないと On-Demand との割安さが読み取れない。

実効的な時間単価と On-Demand 比の節減率は、AWS 自身の RI 料金ページでも示される基本的な比較軸である。
右レールの見積もり (`components/pricing/Estimator.tsx`) はチェックした行について月額継続、前払い一括、実効月額の 3 系統を出すが、単価表の行そのものには実効値も節減率も出ない。
単価表の各 RI 行に、前払い額、月額、実効時間単価、On-Demand 比の節減率を表示する。

## 現状

- `reservedRateFromTerm` (`backend/internal/aws/pricing.go`) は、`price_usd` に継続の時間単価、`upfront_usd` に前払い額を入れる。
- 同一の Price List ドキュメントから On-Demand と RI の行が生成され、両者は同じ `label` を持つ (`instanceOnDemandRatesFromDocument` が On-Demand と Reserved に同じ label を割り当てる)。このため RI 行に対応する On-Demand の時間単価は、同じ表の中で label 一致により引ける。
- `lib/pricingEstimate.ts` は `HOURS_PER_MONTH = 730`、契約月数 (`contractMonthsFromLease`: 1yr は 12、3yr は 36) を持ち、実効月額を「継続月額 + 前払い / 契約月数」で計算する。
- 単価表の RI 行 (`RateGroupSection`) は時間単価と前払いのみを表示し、月額、実効時間単価、節減率は表示しない。

## 目的

単価表の RI 行に、次の 4 値を表示する。

- **前払い料金**：`upfront_usd` (契約時に 1 回発生する一括額)。
- **月額料金 (継続)**：`price_usd × 730` (毎月発生する継続課金。All Upfront では 0 になる)。
- **実効時間単価**：`price_usd + upfront_usd / (730 × 契約月数)` (前払いを契約期間の時間数で按分して時間単価に含めた値)。
- **On-Demand 比の節減率**：`(onDemand_price_usd − 実効時間単価) / onDemand_price_usd` (同一 label の On-Demand 時間単価に対する割引率)。

実効時間単価の分母 `730 × 契約月数` は 1yr で 8760 時間、3yr で 26280 時間となり、`pricingEstimate.ts` の実効月額計算と整合する (実効月額を 730 で割ると実効時間単価に一致する)。

## 対応方針

フロントのみの変更で完結する。
必要なデータ (`price_usd` / `upfront_usd` / `term.lease`、および同一 label の On-Demand の `price_usd`) はいずれもバックエンドが既に返している。

- `HOURS_PER_MONTH` と `contractMonthsFromLease` を再利用する純関数を追加する (`pricingEstimate.ts` への追加、または `lib/` の新規ファイル)。
  - 実効時間単価を返す関数。
  - 継続月額を返す関数。
  - On-Demand 時間単価を受け取り節減率を返す関数。
- `ServiceCard` で、`table.rates` の On-Demand 行から label をキーとした時間単価の対応表を作り、`RateGroupSection` に渡す (または RI 行の派生値をあらかじめ計算して渡す)。
- `RateGroupSection` の RI 行に 4 値を表示する。
- On-Demand の行 (`model === 'on_demand'`) には実効値も節減率も表示しない。
- 同一 label の On-Demand が見つからない場合、節減率は「—」を表示する (欠損を偽の 0% と区別する)。

## 設計上の論点

- **RI 行の主表示をどれにするか**：現状は継続の時間単価を主に出している。支払オプションをまたいで比較できるのは実効時間単価のため、実効時間単価を主表示にし、前払い、月額、節減率を内訳として添える構成を推奨する。行が過密にならないレイアウトを実装時に検討する。
- **label 一致の信頼性**：On-Demand と RI は同一ドキュメント由来で label が一致するため、label をキーにした対応付けは信頼できる。対応が見つからない異常時は節減率を「—」にする防御を入れる。
- **既存の見積もり計算との整合**：`pricingEstimate.ts` の `HOURS_PER_MONTH` と `contractMonthsFromLease` を再利用し、按分の定義が単価表と見積もりで食い違わないようにする。
- **Savings Plans への展開 (対象外)**：本 issue は RI を対象とする。SP のレート照会 API (`DescribeSavingsPlansOfferingRates`) が返す `price_usd` は offering (PlanType / PaymentOption / 期間の組) 単位のレートで、前払い額そのものは返さないため `upfront_usd` は常に 0 になる (`instanceSavingsPlanRate` のコメント参照)。この `price_usd` が支払オプション別の実効時間単価を既に織り込んだ値か (その場合 upfront_usd=0 でも Partial / All Upfront の実効時間単価は `price_usd` で正しい)、継続分のみで前払い按分が別途要る値か (その場合 Partial / All Upfront は過小評価になる) は offering rate の意味論に依存し、実 AWS で裏取りしないと確定できない。SP への展開はこの確認を前提とした将来の拡張とし、本 issue では判断しない。
- **通貨と桁**：金額は既存の `formatUnitPrice` / `formatMoney` を用いる。節減率はパーセント表示 (小数第 1 位程度) とし、負の節減率 (On-Demand より高い異常値) もそのまま表示して隠さない。

## 完了条件

- 単価表の各 RI 行に、前払い料金、月額料金、実効時間単価、On-Demand 比の節減率が表示される。
- All Upfront (継続時間単価 0 / 前払い > 0)、Partial Upfront、No Upfront のいずれでも、実効時間単価と節減率が正しく算出される。
- On-Demand の行には実効値も節減率も表示されない。
- 同一 label の On-Demand が無い RI 行では、節減率が「—」になる。
- `frontend/src/components/pricing/RateGroupSection.test.tsx` (または算出用の純関数のテスト) に、3 種の支払オプションでの実効時間単価と節減率、および On-Demand 欠損時の「—」を検証するテストを追加する。
- `CHANGES.md` の `## develop` に `[ADD]` エントリを追記する (種別順 UPDATE → ADD → CHANGE → FIX を守り、次行に 2 文字インデントで `- @sfuruya0612` を付ける)。
- `mise run check` が全て通過する。

## 検証

- frontend：`mise run frontend:lint` / `mise run frontend:test`。All / Partial / No Upfront の RI と対応する On-Demand を用意し、実効時間単価と節減率の算出、On-Demand 欠損時の「—」を検証する。
- 実ブラウザで、EC2 / RDS / ElastiCache の RI 行に 4 値が表示され、支払オプション違いの行を実効時間単価で比較できることを確認する。

## 解決方法

対応方針どおりフロントのみの変更で実装した。

- `lib/pricingEstimate.ts` に `effectiveHourlyRate`/`monthlyRecurring`/`savingsPercent` を追加した。いずれも既存の `HOURS_PER_MONTH`/`contractMonthsFromLease` を再利用し、単価表と見積もりの按分定義を一致させている。`savingsPercent` は同一 label の On-Demand 時間単価が `undefined` または 0 以下のとき `null` を返す。
- `lib/format.ts` に `formatPercent` を追加した。符号をそのまま表示し (負値も隠さない)、`toFixed(1)` で小数第 1 位まで丸める。
- `components/pricing/ServiceCard.tsx` で `table.rates` 全体 (属性フィルタ適用前) から `model === 'on_demand'` の行を label をキーに集めた `onDemandHourlyByLabel: Map<string, number>` を作り、`RateGroupSection` に渡すようにした。
- `components/pricing/RateGroupSection.tsx` に `onDemandHourlyByLabel` プロパティを追加し、`rate.model === 'reserved'` の行のみ新設の `ReservedRatePrice` コンポーネントで実効時間単価 (主表示) + 内訳 (継続月額、前払い額 (0 のときは非表示)、On-Demand 比節減率) を表示するようにした。On-Demand/Savings Plans/Spot の行は既存表示のまま変更していない。節減率が負 (異常値) の場合は `.pr-rate-savings-negative` で視覚的に区別しつつ、値自体は隠さず表示する。
- テストは `pricingEstimate.test.ts`/`format.test.ts` に新規関数の単体テストを追加し、`RateGroupSection.test.tsx` に All/Partial/No Upfront の 3 パターンと On-Demand 欠損時の「—」、On-Demand 行に実効値・節減率が出ないことを検証するテストを追加した。
- `mise run check` が全て通過することを確認した。

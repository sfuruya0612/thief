# EC2 / RDS / ElastiCache の単価表をインスタンスファミリで絞り込めるようにする

Created: 2026-07-21
Completed: 2026-07-21
Model: Claude Opus 4.8

## 背景

Pricing 画面の単価表は、インスタンスタイプでの絞り込み手段としてテキスト検索 (`components/pricing/ServiceCard.tsx` の `instanceFilter`) しか持たない。
この検索は `RateGroupSection.matchesInstanceFilter` が label と属性値に対して大文字小文字を無視した部分一致を行うだけで、ファミリ単位の選択にはならない。
例えば `m5` と入力すると `m5` だけでなく `m5a` / `m5d` / `m5n` / `m5dn` / `db.m5` 等も一致し、ファミリを 1 つに絞る操作にならない。

一方、OS やエンジンのように値の種類が少ない属性は、`components/pricing/AttributeFilterBar.tsx` のトグルチップで複数選択できる。
インスタンスタイプはこのチップ絞り込みの対象外にされている (`lib/pricingAttributeFilters.ts` のコメント「instance_type は種類が多すぎるためテキスト検索に任せる」)。
しかし利用者が単価を比較する際は、まず世代やファミリ (m6i / c7g / r6g 等) で候補を絞り込みたい場面が多い。
ファミリはインスタンスタイプそのものより種類が桁違いに少なく、チップによる複数選択の一覧提示に向く。

## 現状

- `lib/pricingAttributeFilters.ts` の `PRICING_ATTRIBUTE_FILTERS` は次の属性だけをチップ絞り込みの対象にしている。
  - **ec2**：os、license_model
  - **rds**：engine、deployment_option、storage_type、license_model
  - **elasticache**：engine
  - **ecs**：なし
- `backend/internal/aws/pricing.go` の `curatedInstanceAttributes` は `instance_type` を属性に詰めるが、ファミリを表す属性は持たない。
- Savings Plans の行も `instanceSavingsPlanRate` が `instance_type` を属性に詰めるが、ファミリは持たない。
- インスタンスタイプの表記はサービスで異なる (EC2 は `m5.large`、RDS は `db.r6g.large`、ElastiCache は `cache.t4g.micro`)。

## 目的

EC2 / RDS / ElastiCache の単価表を、インスタンスファミリのチップで複数選択して絞り込めるようにする。

## 対象範囲

- 対象は EC2、RDS、ElastiCache の 3 サービスとする。
- ECS (Fargate) はインスタンスタイプという概念を持たないため対象外とする (`instance_type` 属性が無く、ファミリも定義できない)。
- 絞り込みは 1 サービスのカード内の全モデル (On-Demand / Reserved Instance / Savings Plans) の行に適用する。

## 対応方針

ファミリの導出をバックエンドの正規化に集約し、フロントはチップ絞り込みの定義を 1 行追加する構成を採る。

### backend

- `curatedInstanceAttributes` (ec2 / rds / elasticache の分岐) に `instance_family` を追加する。
- `instanceSavingsPlanRate` の属性構築にも `instance_family` を追加し、Savings Plans の行でも同じキーで絞り込めるようにする。
- ファミリは `instance_type` からサイズを表す末尾のドット区切りトークンを除いた文字列とする。
  - `m5.large` はファミリ `m5`
  - `db.r6g.4xlarge` はファミリ `db.r6g`
  - `cache.t4g.micro` はファミリ `cache.t4g`
- 純関数として切り出し、テーブル駆動テストで各サービスのインスタンスタイプ表記を検証する。Savings Plans 行の `instance_type` は `spInstanceType` (pricing.go) が usageType 由来の省略形や、まれにファミリのみの大文字表記 (例: `R7G`) を返し得るため、これらが On-Demand / RI と同じファミリへ正規化されるか (揃わない場合はどう扱うか) をテストケースに含める。

### frontend

- `PRICING_ATTRIBUTE_FILTERS` の ec2 / rds / elasticache に `{ key: 'instance_family', label: 'Family' }` を追加する。
- `AttributeFilterBar` は値の種類が 1 以下のフィルタを自動で隠し、`matchesAttributeSelection` はキーを持たない行を対象外として扱うため、追加の分岐は不要である。
- テキスト検索 (`instanceFilter`) はそのまま残し、ファミリチップとテキスト検索の両方が AND 条件で効くようにする。

## 設計上の論点

- **導出をバックエンドに置く根拠**：フロントの絞り込みは `attributes[key]` を全行一律に参照するため、フロントで `instance_type` を解析しても On-Demand / RI と Savings Plans で処理は重複しない (フロント導出も 1 箇所で済む)。それでもバックエンドに置くのは、curated attribute はバックエンドが `rate.attributes` に詰め、フロントはフィルタ定義のみを宣言するという既存アーキテクチャ (`lib/pricingAttributeFilters.ts` 冒頭コメント) に揃えるためである。バックエンドの `curatedInstanceAttributes` と `instanceSavingsPlanRate` に置けば、正規化を 1 箇所に集約でき、Savings Plans の行にも同じキーで適用できる。フロント導出との比較は実装時に行い、既存の分担に沿う本方針を第一候補とする。
- **RDS / ElastiCache の接頭辞の扱い**：`db.` / `cache.` を含む形 (`db.r6g`) を属性値として保持すると曖昧さがない。表示ラベルで接頭辞を落として読みやすくする (`storage_type` の `valueLabels` と同じ方式) かどうかは実装時に判断する。
- **ファミリの件数**：ファミリは数十種類になり得る。チップは横方向に折り返して表示するため許容できるが、多すぎる場合はドロップダウンへの変更も候補になる。まずチップで実装し、実データの件数を見て判断する。
- **issue 0055 との関係**：issue 0055 (Savings Plans の独立サービス化) が先に入る場合、分離後の Savings Plans サービスのカードにも `instance_family` のチップ絞り込みを追加する必要がある。`PRICING_ATTRIBUTE_FILTERS` の対象サービスを、分離後のサービス構成に合わせて更新すること。また 0055 の compute-sp カードは EC2 と Fargate の行が混在し、Fargate 行は `instance_family` キーを持たない。`matchesAttributeSelection` はキーを持たない行を対象外 (常に一致) として扱うため、compute-sp カードで Family チップを選んでも Fargate 行は絞り込まれず残る。これはキー無し行の既存の意味論と一貫しており許容する。compute-sp カードで Family を選んでも Fargate 行が残る挙動を検証で確認する。
- **ファイルキャッシュの無効化**：`instance_family` は `curatedInstanceAttributes` が `rate.attributes` に足す新キーだが、`internal/pricecache` はスキーマ版も TTL も持たないため、デプロイ前に作られたキャッシュファイルには `instance_family` が無く、`attributeValueOptions` が空を返して Family チップが出ない (`AttributeFilterBar` が値 1 以下のフィルタを隠すため)。issue 0055 で導入するキャッシュ無効化機構 (スキーマ版付与または起動時クリア) に本 issue の新キーも含める。0055 より先に本 issue を実装する場合は、本 issue 側で同等の無効化を入れる。

## 完了条件

- EC2 / RDS / ElastiCache の単価表にファミリのチップが表示され、複数選択で行を絞り込める。
- ファミリチップとテキスト検索が同時に効く (AND 条件)。
- ECS のカードにはファミリチップが表示されない。
- Savings Plans の行もファミリで絞り込める (対象サービスの場合)。
- `backend/internal/aws/pricing_test.go` にファミリ導出の純関数のテストを追加する。
- `frontend/src/lib/pricingAttributeFilters.test.ts` にファミリキーの選択肢抽出と一致判定のテストを追加する。
- `CHANGES.md` の `## develop` に `[ADD]` エントリを追記する (種別順 UPDATE → ADD → CHANGE → FIX を守り、次行に 2 文字インデントで `- @sfuruya0612` を付ける)。
- `mise run check` が全て通過する。

## 検証

- backend：`mise run backend:test`。各サービスのインスタンスタイプ表記からファミリが正しく導出されることを検証する。
- frontend：`mise run frontend:lint` / `mise run frontend:test`。ファミリチップの表示、複数選択、テキスト検索との併用を検証する。
- 実ブラウザで、EC2 / RDS / ElastiCache の単価表でファミリチップを選択し、On-Demand / RI / SP の行が同時に絞り込まれることを確認する。

## 解決方法

backend の `curatedInstanceAttributes` (ec2/rds/elasticache) と `instanceSavingsPlanRate` に、新設した純関数 `instanceFamily` (インスタンスタイプから末尾のドット区切りサイズトークンを除去する) を使って `instance_family` を追加した。
frontend は `PRICING_ATTRIBUTE_FILTERS` の ec2/rds/elasticache に `{ key: 'instance_family', label: 'Family' }` を追加した (`AttributeFilterBar` と `matchesAttributeSelection` は既存の汎用ロジックのため変更不要)。

`instanceFamily` のテーブル駆動テストを追加し、SP 行の usageType 由来の省略形 (`db.r7g.4xl` 等) でも On-Demand/RI と同じファミリへ正規化されることを確認した。
`TestPriceRatesFromDocument` の既存 Attributes アサーションと `pricingAttributeFilters.test.ts` のキー一覧アサーションを新キーに合わせて更新し、`attributeValueOptions`/`matchesAttributeSelection` の instance_family 経由の挙動テストを追加した。

`mise run check` (fmt/lint/test) が backend/frontend ともに通過することを確認した。

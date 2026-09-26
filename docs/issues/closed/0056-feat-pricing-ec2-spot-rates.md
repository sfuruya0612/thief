# EC2 の Spot 単価を独立サービスとして単価表に追加する

Created: 2026-07-21
Completed: 2026-07-21
Model: Claude Opus 4.8

## 背景

Pricing 画面は On-Demand、Reserved Instances (以下 RI)、Savings Plans (以下 SP) の単価を扱うが、Spot 単価を扱わない。
issue 0045 (料金画面の追加) では Fargate Spot を「動的価格で Price List に単価がないため v1 の対象外」として明示的に除外した。
EC2 Spot も同様に未対応である。
Spot は On-Demand より大幅に安く、コスト比較の実務では欠かせない選択肢のため、EC2 の Spot 単価を単価表に追加する。

当初は EC2 と Fargate の両方を対象にしていたが、Fargate Spot の単価を返す公開 API が確認できていないため、Fargate Spot は pending の issue 0059 へ切り出し、本 issue は EC2 Spot に一本化する。

## 現状

- `types/aws.ts` の `PriceModel` は `on_demand` / `reserved` / `savings_plan` の 3 値で、`spot` を持たない。
- `backend/internal/aws/pricing.go` の `ValidatePricingService` と `GetPricing` は、いずれも `pricingServiceSpecs` マップのメンバーシップで service を検証し、その `spec` (awsServiceCode / spServiceCode / spPlanTypes) から On-Demand / RI / SP のカタログ取得を駆動する。Spot はこの `spec` の形に乗らない。
- pricing / savingsplans のクライアントはグローバルサービスのため us-east-1 に固定し、表示対象リージョンは API のフィルタ値としてのみ使う。一方 `backend/internal/aws/ec2.go` には、対象リージョンで EC2 クライアントを生成するパターン (`NewClient(ctx, profile, region, ...)`) が既にある。
- `internal/pricecache` は TTL を設けず、ファイルが存在すれば常に fresh として扱う。並行リクエストの重複排除 (singleflight) は `pricecache.Fetch` の内部にあり、`Fetch` は Load / Save と疎結合で singleflight だけを使える。
- フロントの `PRICING_SERVICES` は 4 サービス固定で、`PricingPanel` が固定長のフックを無条件に呼ぶ (rules-of-hooks)。`usePricing` (`api/queries.ts`) は `staleTime: Infinity` を固定で持ち、service 別の `staleTime` を受け取らない。`PRICING_SERVICE_LABELS` / `PRICING_SERVICE_ICON_KEY` / `PRICING_ATTRIBUTE_FILTERS` は `Record<PricingService, ...>` でキー網羅が必須である。

## 目的

EC2 の Spot 単価を、独立したサービス `ec2-spot` として指定リージョンの単価表に `spot` モデルで追加する。

## 対象範囲とデータソース

- `ec2:DescribeSpotPriceHistory` (対象リージョンの EC2 クライアント) から取得する。
- このレスポンスは、インスタンスタイプ、ProductDescription (OS 相当)、アベイラビリティゾーン、タイムスタンプごとの Spot 価格の時系列である。
- `StartTime` を現在時刻付近に固定し、直近のスナップショットのみを取得して 1 リクエストあたりの取得量を有界にする (`StartTime` 無指定では直近約 90 日分の時系列を全ページ走査することになり、ライブ取得のレイテンシとコストが過大になる)。
- 表示は、インスタンスタイプと OS の組ごとに、ゾーン横断の代表値 (現在のゾーン最小値) をリージョン単位で 1 行に集約する (ゾーン別の行出しは v1 では行わない。「ゾーン粒度」参照)。
- Fargate Spot は本 issue の対象外とし、pending の issue 0059 で追跡する。

## 設計上の決定

- **独立サービス `ec2-spot` にする**：Spot は動的価格でカタログ価格 (On-Demand / RI / SP) と鮮度の要件が異なる。issue 0055 の SP 独立サービス化と同じく、Spot を独立サービス (別キャッシュキー) として扱い、リソースサービスの安定した表と、鮮度およびキャッシュの方針を分離する。
- **`pricingServiceSpecs` のエントリにはしない (spec 経路から分離する)**：`ValidatePricingService` は現状 `pricingServiceSpecs` のメンバーシップ検査そのものであり、`GetPricing` も同じマップから `spec` を引いて On-Demand / RI / SP のカタログ取得を駆動する。`ec2-spot` は `awsServiceCode` / `spServiceCode` / `spPlanTypes` を持たないため、`pricingServiceSpecs` に通常エントリとして足すと `GetPricing` がカタログ取得へ誤流入して破綻する。よって (1) サービス許可判定を `pricingServiceSpecs` のメンバーシップとは別に `ec2-spot` を許容する形へ分離し、(2) ハンドラで `pricecache.Load` を呼ぶ前に `ec2-spot` を Spot 専用のライブ取得関数へ分岐させる。`ec2-spot` は Load と Save のいずれも通らず `pricecache.Fetch` の singleflight だけを通す。Load も `path` 経由で `ValidateService` を実行し `ec2-spot` は `validServices` 非登録のため弾かれるので、分岐を `GetPricing` の前段に置くと `Load` が先に 500 になる。分岐位置はハンドラの Load 前に限る。
- **ライブ取得とし、ディスクキャッシュに書かない。ただし singleflight は維持する**：Spot は分単位から時間単位で変動するため、`pricecache` の Load / Save (TTL なしのディスク保存) には載せず、リクエストごとにライブ取得する。ただし同一リージョンへの同時リクエストを 1 回に畳むため、`pricecache.Fetch` (singleflight) は通し、その loader 内では Save を呼ばない (`Fetch` は Load / Save と疎結合で singleflight だけを使えるため)。`Fetch` は `path` を経由しないので `ec2-spot` を `pricecache` の `validServices` に足す必要はない (下記の検証の非対称)。`pricecache` に TTL 機構は新設しない (YAGNI)。
- **鮮度の実現範囲を明示する**：フロントは `ec2-spot` の `usePricing` の `staleTime` のみ有限値 (例: 60 秒) にする (他サービスは `Infinity` のまま)。これには `usePricing` に `staleTime` を注入する引数追加 (既定は `Infinity` を維持) が要る。ただし全体設定が `refetchOnWindowFocus: false` で `refetchInterval` も無いため、自動再取得はマウント時 (`refetchOnMount`) と手動更新に限られ、パネルを開いたままにすると連続的には最新化されない。Spot の鮮度はマウント時と手動更新で最新化される点を UI に明示する (古い値を fresh と偽装しない)。短い TTL をキャッシュに持たせる代替もあるが、TTL 機構の新設を避けるためライブ取得 + `Fetch` (singleflight) を採る。
- **サービス検証の非対称**：`ec2-spot` はサービス許可判定 (aws 側) には追加するが、ディスクキャッシュ (`pricecache` の Load / Save) を経由しないため `pricecache` の `validServices` には追加しない。この非対称を実装とテストで明示する。
- **PriceModel への `spot` 追加**：`PriceModel` に `spot` を追加し、group を `Spot` とする (backend の `PriceRate.Model`、フロントの型、`GROUP_ORDER`、`format` を更新する)。
- **リージョン別クライアント**：Spot のために対象リージョンの EC2 クライアント生成を追加する (pricing / savingsplans の us-east-1 固定とは別扱い)。
- **OS 語彙の正規化 (実ドメインに合わせる)**：AWS SDK for Go v2 (`service/ec2`) の `SpotPrice.ProductDescription` は `RIProductDescription` 型で、値は `Linux/UNIX` / `Linux/UNIX (Amazon VPC)` / `Windows` / `Windows (Amazon VPC)` が中心である (VPC-only アカウントでは `(Amazon VPC)` 付きが支配的)。末尾の ` (Amazon VPC)` を除去してから On-Demand の `operatingSystem` 語彙 (`Linux` / `Windows` 等) へ正規化して `os` 属性に詰める。`RIProductDescription` の Go enum は網羅的でなく、`DescribeSpotPriceHistory` の product-description は RHEL や SUSE も返す。product-description フィルタの文書に載る値 (`Linux/UNIX`、`Red Hat Enterprise Linux`、`SUSE Linux`、`Windows` と各 ` (Amazon VPC)` 変種) を最初からマッピング対象に含める。Price List 側は RHEL / SUSE の略記を用いるため、Spot のフルネームから On-Demand 側の語彙へのリネームも要る。最終的な網羅は実 AWS で確認する。正規化しないと `os` チップが `Linux` と `Linux/UNIX (Amazon VPC)` に分裂する。
- **ゾーン粒度 (決定)**：Spot 価格はリージョン内のゾーンで異なるが、既存の単価表がリージョン単位であることと行数を抑えることを優先し、v1 はゾーン横断の代表値 (現在のゾーン最小値) をインスタンスタイプと OS の組ごとに 1 行へ集約する。RateID はゾーンを含めず instance_type と os から構成する (集約後は組ごとに一意になる)。ゾーン別の行出しは将来の拡張とする。
- **見積もりでの扱い**：Spot はコミットメントを伴わないため、月額は時間単価 × 730 (前払いも契約期間もなし) になる。ただし Spot 価格は変動するため、見積もりは参考値である旨を UI に明示する。

## frontend 対応方針

`ec2-spot` を独立サービスにするため、issue 0055 が 7 サービス化で行うのと同水準の登録を行う。

- `PRICING_SERVICES` に `ec2-spot` を追加する。これに伴い `PricingPanel` の `usePricing` / `useRefreshPricing` を固定長のフックとして 1 対追加し、`queries` / `refreshes` レコードと 4 つの useEffect / useMemo の依存配列 (`pruneStaleRates` / `rates` / `ssoExpired` / `lastFetchedAt`) へ配線する。「固定 4 サービス」の rules-of-hooks コメントも更新する。
- `PRICING_SERVICE_LABELS` / `PRICING_SERVICE_ICON_KEY` / `ServiceSelectorBar` に `ec2-spot` を追加する。アイコンは Spot 用のインライン SVG を用いる。
- `PRICING_ATTRIBUTE_FILTERS` に `ec2-spot` のエントリ (os、および issue 0054 の instance_family) を追加する。`Record<PricingService, ...>` のキー網羅が必須のため、追加しないと `tsc` エラーになる。
- `GROUP_ORDER` に `Spot` を追加する。
- `usePricing` に `staleTime` を注入する引数を足し、`ec2-spot` のみ有限値、他は `Infinity` を既定にする。
- issue 0054 の instance_family 絞り込みを `ec2-spot` にも及ぼす (backend の Spot 正規化で instance_family を attributes に詰め、`PRICING_ATTRIBUTE_FILTERS['ec2-spot']` に family チップを持たせる)。
- 既存ユーザの永続化された `activeServices` には `ec2-spot` が無いため、issue 0055 が導入する一度きりの補完移行 (`PRICING_SERVICES` にあって `activeServices` に無いメンバーを初回のみ既定 active へ補完する) の対象に `ec2-spot` を含める。0056 を 0055 より先に実装する場合は 0056 側で同等の一度きり移行を入れる (毎ロード補完にはしない。ユーザーが OFF にした状態を保持するため)。

## 完了条件

- `ec2-spot` サービスが独立したカードとして追加され、指定リージョンの EC2 Spot 単価を `spot` モデルで一覧できる。
- `ec2-spot` は `pricingServiceSpecs` に足されず、ハンドラで `pricecache.Load` の前に分岐して Load / Save を通らず、`GetPricing` のカタログ取得経路 (On-Demand / RI / SP) も通らない。
- Spot の RateID がゾーンを含めず instance_type と os から一意に構成される (ゾーン横断の最小値へ集約した後)。
- Spot はディスクキャッシュ (`pricecache` の Save) に書かれず、`pricecache.Fetch` の singleflight は通す (同一リージョンへの同時リクエストが 1 回に畳まれる)。
- ProductDescription が ` (Amazon VPC)` 除去と `operatingSystem` 語彙への正規化を経て `os` 属性に入り、`os` チップが On-Demand と揃う。
- frontend の登録一式 (`PRICING_SERVICES` / `PricingPanel` の固定長フックと 4 依存配列 / `PRICING_SERVICE_LABELS` / `PRICING_SERVICE_ICON_KEY` / `ServiceSelectorBar` / `PRICING_ATTRIBUTE_FILTERS` (os と instance_family) / `GROUP_ORDER` / `usePricing` の `staleTime` 引数) が揃い、`tsc` が通る。
- Spot の鮮度がマウント時と手動更新で最新化される旨が UI に明示される (古い値を fresh と偽装しない)。
- 既存ユーザの永続化状態でも `ec2-spot` カードが初回ロード時に一度だけ既定表示される (以後ユーザーが OFF にした状態は保持される。issue 0055 の一度きり補完移行が `ec2-spot` を含む)。
- Fargate Spot は本 issue の対象外であり、pending の issue 0059 で追跡する。
- `backend/internal/aws/pricing_test.go` に、`DescribeSpotPriceHistory` の手書きフェイクを使った Spot の取得と正規化のテストを追加する (OS 語彙正規化は ` (Amazon VPC)` 付き変種に加え、`Red Hat Enterprise Linux` / `SUSE Linux` とその ` (Amazon VPC)` 変種が On-Demand 側の語彙 (`RHEL` / `SUSE`) へ正規化されるケースを含める。リネーム表の誤りは手書きフェイクだけでは著者の選んだ値で通ってしまうため、実 AWS 検証でも Spot の `os` が On-Demand の `operatingSystem` 語彙と一致することを確認する)。
- `backend/internal/api/handlers_pricing_test.go` (ハンドラ層) に、`ec2-spot` が `pricecache.Load` の前で分岐して Load / Save を通らず、`GetPricing` のカタログ取得経路 (On-Demand / RI / SP) も通らないことを検証するテストを追加する。この分岐と Save 非経由はハンドラ層の責務であり、aws 層の `pricing_test.go` は構造上 Load / Save を呼ばないため当該検証は aws 層では空虚になる。aws 層は Spot の取得と OS 正規化に、ハンドラ層は Load 前分岐と Save 非経由とカタログ経路非通過に検証を割り当てる。
- `CHANGES.md` の `## develop` に `[ADD]` エントリを追記する (種別順 UPDATE → ADD → CHANGE → FIX を守り、次行に 2 文字インデントで `- @sfuruya0612` を付ける)。
- `mise run check` が全て通過する。

## 検証

- backend：`mise run backend:test`。`DescribeSpotPriceHistory` のモックレスポンスから、インスタンスタイプと OS の組ごとにゾーン横断の代表値 (最小) が採られること、OS 語彙が On-Demand と揃うこと (` (Amazon VPC)` 変種を含む)、`ec2-spot` がカタログ取得経路を通らないこと、Save が呼ばれない (ディスクに書かれない) が `Fetch` の singleflight は通ることを検証する。
- frontend：`mise run frontend:lint` / `mise run frontend:test`。`spot` モデルの表示、`GROUP_ORDER` の並び順、`ec2-spot` の `staleTime` が有限であること、`PRICING_ATTRIBUTE_FILTERS['ec2-spot']` のチップを検証する。`tsc` が通ること。
- 実 AWS でリージョンを指定し、`DescribeSpotPriceHistory` のゾーン別変動と最新値採用の挙動、`StartTime` 境界での取得量を確認する。Spot がファイルキャッシュのディレクトリに書き込まれないことを確認する。

## 解決方法

設計上の決定に記載した方針をそのまま実装した。

### backend

- `EC2SpotService = "ec2-spot"` を追加し、`ValidatePricingService` と `GetPricing` の両方で `resourceServiceSpecs`/`savingsPlanServiceSpecs` のメンバーシップ判定より前段で分岐させた。`GetPricing` は `ec2-spot` の場合、対象リージョンの EC2 クライアント (`newEC2Client`、pricing/savingsplans の us-east-1 固定とは別扱い) を生成して `getEC2SpotPricing` を呼ぶ。
- `getEC2SpotPricing`/`fetchEC2SpotRates` は `ec2SpotAPI` インターフェース (テストでフェイクに差し替え可能) 経由で `DescribeSpotPriceHistory` を呼び、`StartTime = now - ec2SpotLookbackWindow (1時間)` で直近のスナップショットのみを取得する。取得した行はインスタンスタイプと正規化後の os の組をキーに、ゾーン横断の最小値へ集約する (RateID はゾーンを含まない)。
- `spotOSFromProductDescription` で `RIProductDescription` の生文字列から ` (Amazon VPC)` を除去し、`Linux/UNIX`→`Linux`、`Red Hat Enterprise Linux`→`RHEL`、`SUSE Linux`→`SUSE`、`Windows`→`Windows` へ正規化した。RHEL/SUSE 変種は SDK の enum 定数に無いため、生文字列の switch で扱う。
- ハンドラ (`handlers_pricing.go`) は `isLiveOnly := service == EC2SpotService` を Load 呼び出しの前段で判定し、`isLiveOnly` のときは `pricecache.Load` を呼ばず (`ec2-spot` は `pricecache.validServices` に未登録のため Load を呼ぶと 400 になってしまう)、`pricecache.Fetch` のローダー内でも `Save` を呼ばないようにした。`Fetch` 自体 (singleflight) は経由させ、同時リクエストの重複排除は維持する。

### frontend

- `PRICING_SERVICES` に `ec2-spot` を追加し (8 サービス)、`PricingPanel` の固定長フックパターンを 7→8 に拡張した。`usePricing` に `staleTime` 引数 (既定 `Infinity`) を追加し、`ec2-spot` の呼び出しのみ `EC2_SPOT_STALE_TIME = 60_000` を渡す。
- `PRICING_SERVICE_LABELS`/`PRICING_SERVICE_ICON_KEY` (アイコンは Icons.tsx に spot 専用の稲妻アイコンを追加)、`PRICING_ATTRIBUTE_FILTERS['ec2-spot']` (instance_family/os)、`GROUP_ORDER` への `Spot` 追加を行った。
- `ServiceCard` にライブ取得である旨のバッジ (「ライブ取得 (自動更新なし)」) を `ec2-spot` カードのみに表示し、`Estimator` にも Spot 価格が変動する旨の注記を追加した。
- 永続化状態の移行は `PRICING_SCHEMA_VERSION` を 2 に上げ、`PRICING_SCHEMA_MIGRATIONS[2] = ['ec2-spot']` を追加した。issue 0055 で 版 1 まで移行済みのユーザーも、次回ロード時に版 2 への移行で `ec2-spot` が一度だけ既定表示される。

### テスト

- backend: `TestSpotOSFromProductDescription`（RHEL/SUSE を含む全 8 値 + 未知値）、`TestFetchEC2SpotRates`（ゾーン最小値集約、ページネーション、不正行のスキップ、StartTime の窓）、`TestGetEC2SpotPricing` を追加した。ハンドラ層には `TestHandlePricingEC2SpotBypassesDiskCache` を追加し、Load 前分岐が機能していることを検証した。`pricecache` 側には `ec2-spot` が `validServices` に含まれないことを検証するケースを追加した。
- frontend: `pricingAttributeFilters.test.ts`/`pricingSelection.test.ts`（複数リリースにまたがる移行の一度きり実行を含む）/`PricingPanel.test.tsx` を更新・追加した。
- `mise run check` が全て通過することを確認した。

### 未検証事項 (issue 0060 へ切り出し)

`ec2SpotLookbackWindow` (1時間) の妥当性と `spotOSFromProductDescription` の RHEL/SUSE 変種の網羅性は、この開発環境に実 AWS 認証情報が無く実データで確認できなかった。これらは issue 0060 として切り出し、実 AWS 環境での確認を別途行う。

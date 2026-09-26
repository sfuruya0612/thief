# AWS Pricing の単価確認と月額見積もり画面を追加する

Created: 2026-07-18
Model: Claude Opus 4.8

## 背景

thief には AWS のリソース一覧と Cost Explorer による実績コストの閲覧はあるが、購入前の単価を確認したり、使用量を仮定して月額を試算したりする画面がない。

デザインプロジェクト (`Query Editor.dc.html` の Turn 10 から 14、実装メモ「AWS 料金画面」節) に、AWS Price List の単価表と月額見積もりを 1 画面にまとめる料金リファレンスの設計がある。
採用案は 13a/13b (複数サービス選択でサービスカードが中央にスタックし、右レールで見積もりを集計する) と、14 (料金データを初回選択時に非同期取得してローカルにキャッシュする) である。

料金は On-Demand だけでなく、Reserved Instances (以下 RI) と Savings Plans (以下 SP) の割引後単価も確認したい。
AWS は 2025 年 12 月に Database Savings Plans を追加した。
Database SP の対象は Aurora / DynamoDB / RDS / ElastiCache 等と広いが、本 issue では実装済みの RDS と ElastiCache を扱う。

インスタンスやノードの時間単価はリージョンごとに異なるため、リージョンを指定しない単価には意味がない。
このためリージョンを表示条件とし、サービスとリージョンの単位で `/tmp/thief/price/{service}/{region}.json` にローカルキャッシュを持つ。
Price List API のレスポンスは巨大で、リージョンや属性で絞っても 1 サービス数百 KB になり得るため、初回取得後はキャッシュから即時表示する。

## 目的

- AWS の主要サービスについて、指定リージョンの単価 (On-Demand / RI / SP) を一覧できるようにする。
- 単価表から項目を選び、使用量を入力して月額を試算できるようにする。
- 料金データをローカルにキャッシュし、初回取得後は即時表示、明示的な更新でのみ再取得する。

## 対象範囲

対象サービスは、実装済みの AWS サービスのうち次の 4 つとする。

- **EC2** (インスタンス)
- **RDS** (インスタンス)
- **ElastiCache** (ノード)
- **ECS** (Fargate タスク)

各サービスで対応する料金モデルは AWS の提供状況に従う。

| サービス | On-Demand | Reserved Instances | Savings Plans |
| --- | --- | --- | --- |
| EC2 | 対応 (Hrs) | 対応 (Standard/Convertible × 1yr/3yr × No/Partial/All Upfront) | Compute SP + EC2 Instance SP |
| RDS | 対応 (Hrs) | 対応 (1yr/3yr × No/Partial/All Upfront、OfferingClass なし) | Database SP (1yr / No Upfront のみ) |
| ElastiCache | 対応 (Hrs) | 対応 (1yr/3yr × 支払オプション、OfferingClass なし) | Database SP (1yr / No Upfront のみ) |
| ECS (Fargate) | 対応 (vCPU-Hours + GB-Hours) | 非対応 | Compute SP |

補足事項は次の通り。

- ECS (Fargate) は RI を持たない。UI では RI セクションを非活性にし、非対応である旨を注記する。
- OfferingClass (Standard / Convertible) は EC2 RI 固有の属性で、RDS と ElastiCache の RI には現れない。正規化型の `offering_class` はこの 2 サービスで null になる。
- Database SP は 1 年契約かつ No Upfront のみで、3 年契約と前払いオプションを持たない。正規化型の `upfront_usd` は常に 0、`term.lease` は 1yr、`term.payment` は No Upfront になる。
- Database SP は新世代のインスタンスファミリ (Gen7 以降) のみ対象で、旧世代 (M5 / R5 / T3 / T4g 等) は対象外である。旧世代を選ぶと SP 行が現れないため、UI に「旧世代は Savings Plans 対象外 (RI で確認)」と注記する。
- Fargate Spot は動的価格で Price List に単価がないため、v1 の対象外とする。

### GB 課金項目のスコープ (v1 は対象外)

EBS や RDS のストレージ ($/GB-月)、データ転送 ($/GB) のような GB 課金項目は、v1 の対象外とする。
これらは Price List ではインスタンスとは別の productFamily (`Storage` / `Data Transfer` 等) に入り、単位も `GB-Mo` / `GB` とインスタンス時間単価と異なる。
データ転送は送信元と送信先の 2 軸選択と階段課金を伴い、本画面の「インスタンスタイプで絞ってレートを選ぶ」モデルに乗らない。
v1 はインスタンスとノードと Fargate タスクの時間単価に限定し、GB 課金項目は将来の拡張とする。
正規化型の `unit` は Fargate メモリの `GB-Hours` を含むが、これはストレージの `GB-Mo` とは別物である点に注意する (混同すると月額換算がずれる)。

この対象範囲はユーザ確認済みである (時間単価のみ、ストレージとデータ転送は対象外)。

## データ取得

料金データは 2 系統の AWS API から取得し、サーバ側で 1 つの正規化レート表に統合する。

- **On-Demand と RI**: `pricing:GetProducts` (`aws-sdk-go-v2/service/pricing`)。`ServiceCode` に `AmazonEC2` / `AmazonRDS` / `AmazonElastiCache` / `AmazonECS` を指定し、`regionCode` とサービス固有の属性でフィルタする。`GetProductsOutput.PriceList` は price document の生 JSON 文字列の配列 (`[]string`) であり、SDK の型付き構造体ではない。各文字列を `json.Unmarshal` し、`terms.OnDemand` と `terms.Reserved` (RI は `termAttributes` の LeaseContractLength / OfferingClass / PurchaseOption と、`priceDimensions` の前払い Quantity + 時間 Hrs) をネストした map から防御的に取り出す。欠損フィールドや型不一致のエントリはスキップする。
- **SP (Compute / EC2 Instance / Database)**: `savingsplans:DescribeSavingsPlansOfferingRates` (`aws-sdk-go-v2/service/savingsplans`)。`savingsPlanTypes` に `Compute` / `EC2Instance` / `Database`、`serviceCodes` に対象サービスを指定する。レスポンス `searchResults` は型付き構造体で、`rate` / `unit` / `serviceCode` / `usageType` / `savingsPlanOffering` (paymentOption / durationSeconds / planType) と、属性を name/value で持つ `properties` 配列を返す。インスタンスタイプ等の属性は `properties` から取り出す。

両 API とも `NextToken` を最後まで辿り、全ページを取得する。
`GetProducts` は 1 リージョンで数千プロダクトに達するため、ページングを省くと結果が欠ける。

Price List のエンドポイントは us-east-1 / ap-south-1 / eu-central-1 に限られ、savingsplans はグローバル (us-east-1) である。
どちらもクライアントは us-east-1 で生成し、表示対象のリージョンは API のフィルタで絞る。
既存の `internal/aws/cost.go` が `newCostExplorerClient(ctx, profile, "us-east-1")` でリージョン固定のクライアントを作っているため、これに倣う。

認証はアクティブプロファイルの資格情報を用いる。
料金値そのものはアカウントに依存しないが、`GetProducts` と `DescribeSavingsPlansOfferingRates` の呼び出しには AWS 認証が必要なため、プロファイルを認証のためだけに使う。
`pricing` と `savingsplans` の 2 クライアント生成は `errgroup` で並列化してよい (直近の並列化方針と整合)。

### 取得の完全性 (部分結果を完全としてキャッシュしない)

キャッシュが誤りを固定化しないよう、取得のアトミック性を保証する。

- On-Demand / RI の取得 (`GetProducts`) がページング途中で失敗した場合は、その表を破棄してエラーを返し、キャッシュに書き込まない。
- SP の取得 (`DescribeSavingsPlansOfferingRates`) が失敗した場合は、On-Demand / RI のみの表に `partial: true` と `missing_models` メタを付けて縮退する。フロントはこのメタを見て「Savings Plans を取得できていない」と明示する。
- いずれの場合も、不完全な表を「完全」としてキャッシュしないことを不変条件とする。

## キャッシュ

- 保存先は `/tmp/thief/price/{service}/{region}.json`。1 ファイルにサービス×リージョンの正規化レート表と取得時刻 (`fetched_at`) を持つ。プロファイルはパスに含めない (単価はアカウント非依存で、プロファイル間で共有できる)。
- TTL は設けない。キャッシュファイルが存在すれば常にそれを即時表示する (ユーザ判断)。再取得は、キャッシュが不在または破損している場合と、明示的な更新の場合に限る。
- リクエストに `?refresh=true` を付けると強制再取得する (ツールバーの更新ボタンから使う)。`fetched_at` は鮮度表示 (最終更新時刻) にのみ用い、期限判定には使わない。
- ファイル書き込みは、宛先と同一ディレクトリに一時ファイルを作ってから `os.Rename` する atomic write とする (`internal/snippet/snippet.go` の実装に倣うが、当該ロジックは package-private のため `pricecache` に再実装する)。一時ファイルを別ファイルシステムに作ると rename が失敗するため、必ず同一ディレクトリに作る。
- `service` は対象 4 サービス (`ec2` / `rds` / `elasticache` / `ecs`) の固定の許可リストで検証する。`region` は `^[a-z0-9-]+$` 相当の許可リストで検証する。どちらもパスを組み立てる前に検証し、キャッシュヒット時も検証を先に通す。許可リストにより、生成されるファイルはサービス×リージョンで有界 (最大でも数百ファイル、各数十 KB) になり、掃除は不要である。
- キャッシュファイルが壊れている場合 (JSON パース不能、スキーマ不整合、`fetched_at` 欠損) は、`Load` がミス扱いにして再取得を促す。エラーをハンドラに伝播させずクラッシュもしない。壊れたファイルは次の `Save` で atomic に上書きされる。
- ディレクトリは 0700、ファイルは 0600 で作成する。保存先は `127.0.0.1` にバインドしたローカル開発ツールの前提で共有 `/tmp` を用いるが、`THIEF_PRICE_CACHE_DIR` でユーザ固有ディレクトリ (例: XDG 配下) に変更できる (「設計上の論点」参照)。
- 同一 service/region の同時ミスや同時更新による二重フェッチを防ぐため、フェッチ経路を service/region をキーとする `golang.org/x/sync/singleflight` (既存依存) で囲む (実行中の 1 回に畳む)。更新は明示的な操作のため常に再取得するが、singleflight が同時の重複リクエストを 1 回にまとめる。
- フロントエンドは TanStack Query のメモリキャッシュのみを使い、バックエンドのファイルキャッシュを正とする。ブラウザ側の IndexedDB は使わない。

## 正規化レート型 (backend が返す JSON)

サービス間で共通のレート表とする。JSON タグは snake_case。

```
{
  "service": "ec2",
  "region": "ap-northeast-1",
  "fetched_at": "2026-07-18T09:00:00Z",
  "partial": false,
  "missing_models": [],
  "rates": [
    {
      "rate_id": "<安定した一意 ID>",
      "model": "on_demand" | "reserved" | "savings_plan",
      "group": "On-Demand" | "Reserved Instance" | "Compute Savings Plans" | "EC2 Instance Savings Plans" | "Database Savings Plans",
      "label": "m5.large / Linux / Shared",
      "attributes": { "instance_type": "m5.large", "os": "Linux" },
      "term": { "lease": "1yr" | "3yr" | null, "offering_class": "standard" | "convertible" | null, "payment": "No Upfront" | "Partial Upfront" | "All Upfront" | null },
      "unit": "Hrs" | "Quantity" | "vCPU-Hours" | "GB-Hours",
      "price_usd": 0.096,
      "upfront_usd": 0,
      "currency": "USD"
    }
  ]
}
```

`rate_id` は Price List の SKU と offerTermCode、rateCode、または savingsplans の offering 属性から組み立てる安定した文字列とする。
単価改定に追従できるよう努めるが、AWS の rateCode や offering id は改定で変わり得るため、追従は best-effort とする (解決できない選択は読み込み時に破棄する。後述)。
`attributes` は表示ではなく絞り込みに用いる (表示は整形済みの `label` を使い、UI に snake_case を直接渡さない)。

## 対応内容

### backend

- `internal/aws/pricing.go` を新規作成する。消費側インターフェース `pricingAPI` (`GetProducts`) と `savingsPlansAPI` (`DescribeSavingsPlansOfferingRates`) を定義し、公開関数 `GetPricing(ctx, profile, region, service)` が 2 クライアントを生成して非公開の取得関数へ渡す (`internal/aws/athena.go` のインターフェース分離に倣うが、pricing は 2 クライアントを取る点が異なる)。pricing の変換は price document の生 JSON 文字列を入力に取る防御的パーサ、savingsplans の変換は型付き構造体からの変換とし、いずれも純関数に分離する。両 API のページングと、対象サービスの許可リスト検証を含める。取得の完全性 (前述) を守る。
- `internal/pricecache/pricecache.go` を新規作成する。`Load(dir, service, region) (data []byte, fetchedAt time.Time, ok bool, err error)` と `Save(dir, service, region, data, fetchedAt)` で `{dir}/{service}/{region}.json` を読み書きする。atomic write (同一ディレクトリ temp + rename)、許可リスト + 名前検証、壊れたファイルのミス扱い、0700/0600、singleflight を担う。戻り値と `fetched_at` を刻む主体 (fresh 応答時にハンドラが now を刻む) は `internal/gcp/projectstore.go` の `LoadProjectsFromDisk` に倣う。`fetched_at` は鮮度表示にのみ使い、TTL 判定は行わない。既存の `internal/cache/cache.Cache` (インメモリ TTL) には載せない。
- `internal/config/config.go` に `PriceCacheDir` を追加する。既定値は `/tmp/thief/price`、環境変数は `THIEF_PRICE_CACHE_DIR`。`Config` 本体と YAML ミラーの `fileConfig` の両方にフィールドを足し、`Defaults` / `applyFile` / `applyEnv` に `SnippetsDir` と同じ流儀で配線する。
- `internal/api/handlers_pricing.go` を新規作成し、`internal/api/routes.go` に `GET /api/aws/profiles/{profile}/pricing` を追加する。ハンドラは profile を `ValidateProfileName` で検証し、`?service=` を許可リストで、`region` を許可リストで検証する (`profileAndRegion` は region 空時に既定へ暗黙フォールバックするため、その挙動に依存せず明示的に検証する)。ファイルキャッシュを確認し、ヒットしなければ `aws.GetPricing` を呼んで保存してから返す。
- エラーマッピングを分離する。SSO 期限切れは 401 `SSO_TOKEN_EXPIRED` にするが、`pricing:GetProducts` と `savingsplans:DescribeSavingsPlansOfferingRates` の IAM 権限不足 (AccessDenied) は、SDK の `smithy.APIError` のエラーコードで判定してから 403 + 必要な IAM 権限を示すコードで返す (既存の `IsSSOTokenExpired` は "not authorized" 等を部分一致で拾うため、そのままでは権限不足を SSO 期限切れに誤分類する)。スロットリング (`ThrottlingException`) は専用コードで返し、フロントの導線を「時間を置く」に倒す。キャッシュ I/O エラーはクライアントへ汎用メッセージを返し、絶対パス等の詳細は `slog` にサーバ側記録する。
- CLI コマンドは追加しない (Athena / クエリエディタや snippets と同様に Web 専用機能とする)。
- go.mod に `aws-sdk-go-v2/service/pricing` と `aws-sdk-go-v2/service/savingsplans` を追加する (AWS 公式 SDK)。`mise run backend:tidy` で go.sum を更新する。
- テストは、pricing の防御的パーサに生 JSON 文字列を直接食わせるテーブル駆動テスト、savingsplans の手書きフェイクとオーケストレーションのテスト、`t.TempDir()` を使ったファイルキャッシュのラウンドトリップ (壊れたファイル、部分結果、キャッシュヒットとミスを含む) のテスト、ハンドラの `httptest` テストを書く。

### frontend

- `types/aws.ts` に `PriceRateRaw` (backend snake_case) と `PriceRateRow` (UI camelCase)、レート表の Raw/Row を追加する。
- `lib/normalizePricing.ts` を新規作成し、Raw から Row への純変換 `priceTableFromRaw` を実装する。
- `lib/pricingEstimate.ts` を新規作成する。小計関数 `subtotal(qty, rate)` は単一の数値ではなく `{ recurringMonthly, upfrontOnce, effectiveMonthly }` を返す。月額は時間単価に対して 730 時間/月 (365×24/12 の近似) で計算し、RI/SP の `effectiveMonthly` は継続課金に前払いの月割 (`upfront_usd / 契約月数`) を加える。契約月数は `term.lease` から引く (1yr は 12、3yr は 36)。Database SP は前払いを持たないため月割は 0 になる。`estimate(selection, rates)` はサービス別の内訳と、月額継続の合計、前払い一括の合計、実効月額の合計の 3 系統を返す。現テーブルに存在しない `rate_id` の選択は安全にスキップする。
- `api/endpoints.ts` に `getPricing(profile, region, service, refresh)` を、`api/queries.ts` に `usePricing` を追加する。`usePricing` は `useResources` に倣う通常の `useQuery` とし (`useInfiniteQuery` や `runToken` は使わない)、queryKey は `['aws', 'pricing', service, region]` とする (単価はアカウント非依存のため profile は含めない)。`staleTime` は `Infinity` とし、自動再取得はしない (更新は下記の mutation のみ)。更新ボタンは `invalidateQueries` では backend のファイルキャッシュにヒットして強制再取得にならないため、`useRefreshGcpProjects` (`api/queries.ts`) に倣い `useMutation` で `getPricing(..., refresh=true)` を呼び `queryClient.setQueryData` で差し替える。
- `views/PricingPanel.tsx` を新規作成し、`views/AccountView.tsx` に `activeService === 'pricing'` の分岐を追加する (Cost Explorer と同じカスタムパネル分岐)。対象は固定 4 サービスのため、`PricingPanel` で 4 個の `usePricing` を無条件に呼び、`enabled: activeServices.includes(service)` でゲートする (可変長の `activeServices` を map してフックを呼ぶと rules-of-hooks 違反になる)。取得したレート表は親で保持し、`ServiceCard` は Row を props で受ける純表示コンポーネント、`Estimator` は親が集約する。
- 構成は次の通り。
  - `PricingToolbar`: リージョン選択 (グローバル `region` / `onRegionChange` を共有。切替は他パネルにも波及する)、通貨表示 (USD 固定)、更新ボタンとキャッシュ鮮度表示 (「ローカルキャッシュ · 最終更新 MM/DD HH:mm」)、公式料金ページへのリンク。
  - `ServiceSelectorBar`: 対象 4 サービスの複数選択トグル。選択集合が中央スタックの表示順になる。
  - 中央スクロール領域の `ServiceCard[]`: サービスごとにカード。ヘッダーは `position: sticky` で、サービス名、選択項目数/全項目数、サービス小計、折りたたみを表示する。本体は料金モデル (On-Demand / RI / SP) を小見出しで区切る。RI と SP の条件 (期間 / クラス / 支払オプション) は、行チェックの羅列ではなくセレクタで先に絞ってから表に出す (行数の爆発を避ける。後述)。行はインスタンスタイプ (RDS はエンジン/クラス/デプロイ、ElastiCache はノードタイプ) で絞り込める。
  - 右レールの `Estimator`: サービス別グループごとに、チェックした行を明細 (数量入力 → 小計) として並べる。合計は月額継続 / 前払い一括 / 実効月額の 3 系統を分けて表示する (前払い一括と月額継続を 1 つの総合計に混ぜない)。中央スタックと右レールは独立してスクロールする。
- UI 状態は各 `ServiceCard` が個別に持つ (サービスごとに取得するため、あるカードがスケルトン中に別カードが表示済みになり得る)。状態は loading (スケルトン) / ready / refreshing (明示的な更新中。キャッシュ表示のままカードヘッダーに進捗と「更新中」バッジ) / error (キャッシュがあれば表示して再試行、キャッシュがなければエラーと再試行) / empty (該当する単価がない) を列挙する。`partial` メタがあれば「Savings Plans を取得できていない」と明示する。
- 空状態と非対応の表現を定める。プロファイル未選択時は「認証のためプロファイルを選択」を出す。ECS の RI 非対応、旧世代インスタンスの SP 非対応は、無言で省略せず注記する。
- SSO 期限切れは `error instanceof ApiError && error.code === 'SSO_TOKEN_EXPIRED'` で判定し `SSOExpiredBanner` を表示する。セッションタブ badge の追随 (`AccountView` の profiles invalidate) は他のカスタムパネルと同様に対象外とする。
- `lib/serviceMeta.ts` の `SERVICES` に `pricing` エントリ (group `cost`、`color` は `var(--svc-pricing)` を app.css に新設するか `var(--svc-costexplorer)` を流用) を `costexplorer` の直後に追加する。`SERVICE_TO_PATH` はカスタムパネルのため追加しない。
- アイコンは、gitignore 対象の公式アイコン取得を避けるため、`components/icons/Icons.tsx` にインライン SVG を 1 つ足して `Sidebar.tsx` の `AwsIcons[svc] ?? Icons[svc]` フォールバックに乗せる。公式アイコンを使う場合は `AwsIcons.tsx` の `AWS_ICON_FILES` と `frontend/scripts/fetch-aws-icons.mjs` の `ICON_FILENAMES` の両方に追加する (片方だけだと 404 になる)。
- 新規サブコンポーネントは `components/pricing/` に置き、CSS クラスは `pr-` 接頭辞で `app.css` に追加する (既存の `qe-` / `lv-` の機能別接頭辞に倣う)。
- 状態の永続化は `lib/storage.ts` の `PersistedState` に optional で `pricing` を追加する (追加は非破壊のためマイグレーション不要)。選択はリージョンをまたぐと `rate_id` が変わるため、リージョンでキーする。形は `pricing?: { activeServices: string[]; collapsed: Record<string, boolean>; selection: Record<string /*region*/, Record<string /*service*/, Record<string /*rate_id*/, { checked: boolean; qty: number }>>> }` とする。3 段ネストの状態のため `useReducer` + 永続化で実装する。読み込み時に現テーブルへ解決できない `rate_id` は破棄する。
- 数量入力は `type=number` の min/step を設定し、負値と NaN を拒否、空入力は 0 として扱う。行チェックボックスにレート名ラベル、サービストグルに `aria-pressed`、折りたたみに `aria-expanded` を付ける。

## 設計上の論点

- **GB 課金項目のスコープ**: v1 は EBS / RDS のストレージ、データ転送等の GB 課金項目を対象外とし、インスタンス / ノード / Fargate タスクの時間単価に限定する (ユーザ確認済み)。含める場合は unit の追加 (`GB-Mo` / `GB`)、productFamily フィルタ、Estimator の分岐、UI のセクションが増え、データ転送は送信元と送信先の 2 軸選択と階段課金のため別 issue への分離が要る。将来の拡張とする。
- **配置**: AWS サイドバーの Cloud Financial Management に Cost Explorer の隣で追加する (ユーザ判断)。AWS ビューはプロファイルに紐づくため、料金画面もアクティブプロファイルの資格情報で単価を引く。デザインメモは料金画面でセッションタブを非表示にする案だが、認証にプロファイルを使う都合上、セッションタブは表示のままとする。単価はアカウント非依存である点を UI で補足する。
- **RI/SP の対応**: SP は EC2 と ECS では Compute SP と EC2 Instance SP、RDS と ElastiCache では Database SP を対象とする (ユーザ判断で「3 に加えて database SP」)。RDS と ElastiCache は EC2 Instance SP の対象外、ECS は RI の対象外である。UI は各サービスで提供される料金モデルのみを表示する。
- **RI/SP の条件の見せ方**: EC2 の RI と SP は条件の組み合わせが多く (RI で 12 通り、SP を含めると数十行)、行チェックの羅列は実運用に耐えない。料金モデルをセグメントで選び、RI/SP の期間 / クラス / 支払オプションをセレクタで先に絞ってから表に出す方式を採る。行はインスタンスタイプ単位とし、RDS と ElastiCache にも絞り込みを用意する。
- **見積もりの合計**: On-Demand の月額継続と、All Upfront RI の前払いを 1 つの総合計に混ぜると、初月実支出でも定常月額でもない中間値になり誤解を生む。Estimator は月額継続 / 前払い一括 / 実効月額の 3 系統を分けて表示する。730 時間/月は近似であり実月の時間数と異なる旨を UI に注記する。
- **キャッシュの保存先と脅威モデル**: `/tmp/thief/price` は共有 `/tmp` 上の予測可能パスで、マルチユーザホストではシンボリックリンクや TOCTOU の懸念がある。本ツールは `127.0.0.1` バインドの単一ユーザ開発用途を前提とし、0700/0600、同一ディレクトリ temp、許可リスト検証で緩和する。より厳格にするなら `THIEF_PRICE_CACHE_DIR` でユーザ固有ディレクトリ (`config.Dir()` の XDG 配下相当) に変更できる。
- **エラーの分離**: 新規 IAM 権限 (`pricing:GetProducts` / `savingsplans:DescribeSavingsPlansOfferingRates`) の不足を SSO 期限切れと区別する。区別しないと再ログインでは直らない問題を SSO 導線に誘導してしまう。
- **キャッシュの鮮度**: TTL は設けず、キャッシュは更新ボタンでのみ再取得する (ユーザ判断)。`fetched_at` を最終更新時刻として表示し、再取得の要否はユーザが判断する。手動更新のみのため、キャッシュが古いまま使われ得る点は許容する。
- **通貨**: USD 固定とする。JPY 併記は為替レートの注入が必要なため v1 の対象外とする。
- **フロントのキャッシュ**: バックエンドのファイルキャッシュを正とし、ブラウザ側の IndexedDB は使わない。

## 完了条件

- AWS サイドバーの Cloud Financial Management に料金画面が追加され、選択したリージョンの EC2 / RDS / ElastiCache / ECS の単価を On-Demand / RI / SP の別に一覧できる。
- 各サービスで提供される料金モデルのみが表示され、ECS の RI 非対応と旧世代の SP 非対応が UI に注記される。
- 単価表の項目を選び数量を入力すると、サービス別内訳と、月額継続 / 前払い一括 / 実効月額の 3 系統の合計が算出される。
- 料金データは初回取得後 `/tmp/thief/price/{service}/{region}.json` にキャッシュされ、TTL なしで常に即時表示し、更新ボタンでのみ再取得できる (mutation + setQueryData により明示的に再取得する)。
- 取得が部分的に失敗した場合、不完全な表を完全としてキャッシュせず、SP 欠落はフロントに明示される。
- IAM 権限不足は SSO 期限切れと区別され、403 で返る。
- 選択サービス、選択項目、数量はリージョンとサービスごとに保持され、ブラウザをリロードしても復元される。リージョン切替で解決できない選択は破棄される。
- SSO 期限切れ時は `SSOExpiredBanner` が表示される。
- `CHANGES.md` の `## develop` に `[ADD]` エントリを追記する (種別順 UPDATE → ADD → CHANGE → FIX を守り、次行に 2 文字インデントで `- @sfuruya0612` を付ける)。
- `mise run check` が全て通過する。

## 検証

- backend: `mise run backend:build` / `mise run backend:test`。防御的パーサ (生 JSON 文字列入力)、ファイルキャッシュ (壊れたファイル / 部分結果 / キャッシュヒットとミス)、許可リスト検証、エラー分離 (IAM 403 と SSO 401)、ハンドラの単体テスト。
- frontend: `mise run frontend:lint` / `mise run frontend:test`。`pricingEstimate.ts` は All Upfront RI (時間単価 0 / 前払い > 0) と On-Demand + RI 混在で 3 系統の内訳を検証する。`normalizePricing.ts` の変換、`PricingPanel` の状態分岐 (loading / ready / stale / error / empty / partial) をテストする。
- 実ブラウザで、料金画面の描画、サービス選択、料金モデルのセレクタ、単価表示、見積もりの 3 系統合計、更新ボタンでの再取得、キャッシュ鮮度表示、リージョン切替時の選択の扱い、リロード後の状態復元を確認する。実 AWS 認証が必要な取得部分はモックレスポンスで代替してよい。
- `CHANGES.md` にエントリが追記され、変更内容と整合していることを確認する。

## 解決方法

設計通り、backend に `internal/aws/pricing.go` (Price List / Savings Plans 取得と正規化)・`internal/pricecache` (ファイルキャッシュ)・`internal/api/handlers_pricing.go` を、frontend に `types/aws.ts` の Raw/Row 追加・`lib/normalizePricing.ts`・`lib/pricingEstimate.ts`・`lib/pricingSelection.ts`・`components/pricing/*`・`views/PricingPanel.tsx` を実装した。

### 元仕様からの訂正

RDS/ElastiCache の RI について、仕様書 (本 issue 冒頭) は「OfferingClass は EC2 RI 固有の属性で RDS/ElastiCache の RI には現れず、正規化型の `offering_class` はこの 2 サービスで null になる」としていたが、実 AWS データで確認したところ `termAttributes.OfferingClass` は `"Standard"` という値そのものは持ち、null にはならない (値の種類が `standard` の 1 つしかないだけ)。フロントの `RateGroupSection` は条件セレクタの選択肢が 1 つ以下なら自動的に隠す設計にしたため、この訂正は UI 上の見た目には影響しない (RDS/ElastiCache では offeringClass セレクタが出ないのは変わらない) が、null 前提のコードを書いていた場合は分岐を誤る差異だったため記録する。

### 実ブラウザ確認で発見し修正した実装バグ

単体テストは手作りの Price List JSON を入力にしていたため検出できず、実際に AWS Price List / Savings Plans API を呼んで初めて次の 3 件のバグが判明し、修正した。

1. **`handlePricing` の `ValidateProfileName` がスペースを含む正当なプロファイル名を拒否する**: 許可文字集合 `[A-Za-z0-9_-]` が `"CT Audit"` のような実在のプロファイル名を 400 で弾いていた。`handleEC2` 等の既存リソースハンドラは同じ検証を呼んでおらず素通しにしている実態と不整合だったため、`handlePricing` からも `ValidateProfileName` の呼び出しを削除した (profile はキャッシュファイルパスに使わずAWS SDK の認証にのみ使うため、パストラバーサル対策は不要)。
2. **RDS/ElastiCache/EC2 の On-Demand/RI 取得に `productFamily` (EC2 は追加で `tenancy`) の絞り込みが無かった**: `ServiceCode` のみで `GetProducts` を呼んでいたため、CPU Credits・ストレージ・Provisioned IOPS・Dedicated Host・専有インスタンス等の無関係な課金項目まで取得され、ラベルが空や `"Any"`、単位が `Hrs` 以外 (`GB-Mo`/`IOPS-Mo`/`ACU-Months`/`API Calls` 等) の行が大量に混入していた (RDS で 18660 行、EC2 で 107794 行)。サービスごとの `productFamily` (EC2: `Compute Instance`、RDS: `Database Instance`、ElastiCache: `Cache Instance`、ECS: `Compute`) を API フィルタに追加し、パース後にも二重チェックを入れた。EC2 はさらに `tenancy=Shared` を追加し、専有ホスト/専有インスタンスを除外した。
3. **`DescribeSavingsPlansOfferingRates` のページング終端条件の誤り**: 最終ページで `NextToken` が `nil` ではなく空文字列で返るケース (RDS/ElastiCache の Database SP のように該当件数が少なく 1 ページで完結する場合) があり、空文字列を次ページありと誤認してリクエストすると AWS 側の正規表現バリデーションで 400 になり、Savings Plans 取得全体が失敗して `partial: true` に縮退していた。`NextToken == nil || *NextToken == ""` を終端条件にする防御を `fetchSavingsPlans` と (未確認だが同型のため念のため) `fetchOnDemandAndReserved` の両方に入れた。

いずれも `internal/aws/pricing_test.go` に回帰テストを追加済み。

### 実装漏れの追加

仕様書にある `refreshing` (キャッシュ表示のままカードヘッダーに進捗と「更新中」バッジ) が実装から漏れていたため、`ServiceCard` に「更新中…」バッジを追加した。

### 波及して見つかった既存バグ (テスト基盤)

`PricingPanel` のコンポーネントテストを書く過程で、`@testing-library/react` の自動クリーンアップ (`afterEach(cleanup)`) が `vite.config.ts` の `test.globals: false` 環境では登録されておらず (`afterEach` がグローバルスコープに存在しないため)、同一テストファイル内の複数 `render()` が `document.body` に蓄積し得る状態だったことが判明した。`src/setupTests.ts` に明示的な `afterEach(cleanup)` を追加して修正した。これに伴い、`internal/aws/pricing_test.go` の `TestPriceRatesFromDocument` (ec2 ケース) が `doc.Terms.Reserved` の map イテレーション順序に依存してフレーキーに失敗することも判明し (`sortPriceRates` を比較前に適用するよう修正)、`fetchOnDemandAndReserved`/`fetchSavingsPlans` の filters 個数アサーションも `productFamily`/`tenancy` フィルタ追加に合わせて更新した。

この過程で見つかった、本 issue のスコープ外の別バグ (Cloud Logging の gRPC エラー分類漏れ、`app.css` のコメント内 `*/` による CSS 構文エラー、サイドバー `SvcItem` の `queryFn` なし `useQuery` による console.error) は issue 0046/0047/0048 として個別に記録し、本 issue では修正していない。

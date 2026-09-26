# Savings Plans を Compute / EC2 Instance / Database の独立したサービスに分離する

Created: 2026-07-21
Completed: 2026-07-21
Model: Claude Opus 4.8

## 背景

Pricing 画面は現在、Savings Plans (以下 SP) のレートを EC2 / RDS / ElastiCache / ECS の各サービスカードの中に `group` として同居させている。
その結果、同じ SP の種類が複数のカードに分かれて現れる。

- Compute SP は EC2 のカードと ECS のカードの両方に現れる (EC2 のカードには EC2 インスタンスの Compute SP レート、ECS のカードには Fargate の Compute SP レートが入る)。
- Database SP は RDS のカードと ElastiCache のカードの両方に現れる。

SP は本来、特定のリソースサービスに属する割引ではなく、複数サービスにまたがって柔軟に適用されるコミットメントである。
Compute SP は EC2 と Fargate と Lambda に、EC2 Instance SP は特定リージョンの EC2 インスタンスファミリに、Database SP は RDS や ElastiCache 等にまたがって適用される。
このため、SP をリソースサービスのカードに埋め込む現在の構成は、同じ SP 種別が複数のカードに散らばる原因になっている。
SP を種別ごとの独立したサービスとして扱い、リソースサービスのカードから切り離す。

## 現状

- `backend/internal/aws/pricing.go` の `pricingServiceSpecs` は ec2 / rds / elasticache / ecs の 4 サービスを持ち、各サービスの `getPricing` が On-Demand / RI と SP を並行取得して 1 つの表に統合する。
- `spGroup` は SavingsPlanType から `Compute Savings Plans` / `EC2 Instance Savings Plans` / `Database Savings Plans` の 3 つの group 名を返す。
- SP のライセンスモデル (`license_model`) は、同じサービスの On-Demand 取得で組み立てた operation から licenseModel への対応表を使い、`applySavingsPlanLicenseModel` が逆引きして反映する (issue 0053)。
- フロントの `PRICING_SERVICES` は ec2 / rds / elasticache / ecs の 4 つ固定で、`PricingPanel` が 4 個の `usePricing` を無条件に呼ぶ (rules-of-hooks のため)。
- `RateGroupSection` の「SP対象外」バッジは、同じカード内の On-Demand / RI の `instance_type` を、そのカードの SP レートの `instance_type` 集合 (`spInstanceTypes`) と突き合わせて判定する。

## 目的

SP を Compute / EC2 Instance / Database の 3 種別の独立したサービスに分離し、リソースサービスのカードから SP を除去する。

## 対象範囲

分離後のサービス構成を次のとおりとする。

| サービス | 含まれるモデル |
| --- | --- |
| EC2 | On-Demand、Reserved Instance |
| RDS | On-Demand、Reserved Instance |
| ElastiCache | On-Demand、Reserved Instance |
| ECS (Fargate) | On-Demand |
| Compute Savings Plans | Compute SP (EC2 インスタンスと Fargate のレート) |
| EC2 Instance Savings Plans | EC2 Instance SP |
| Database Savings Plans | Database SP (RDS と ElastiCache のレート) |

## 対応方針

### backend

- サービスのモデルを再設計し、SP 種別ごとのサービススラッグ (例: `compute-sp` / `ec2-instance-sp` / `database-sp`) を追加する。
- 各 SP サービスは自身の SP レートのみを取得する。`DescribeSavingsPlansOfferingRates` に、その SP 種別が適用される全リソースの `savingsPlanTypes` と `serviceCodes` を指定する。
  - **compute-sp**：types に Compute、serviceCodes に Ec2 と Fargate を指定する (Lambda 等の範囲拡大は本 issue の対象外とし、設計上の論点に記す)。
  - **ec2-instance-sp**：types に EC2Instance、serviceCodes に Ec2 を指定する。
  - **database-sp**：types に Database、serviceCodes に Rds と Elasticache を指定する。
- `savingsPlanRateFrom` の振り分けを thief サービススラッグ基準から、レスポンス各件の `SavingsPlanOfferingRate.ServiceCode` 基準へ作り替える。compute-sp と database-sp は 1 回の取得で複数 serviceCode の行が混在するため、現状の `service == "ecs"` / `service == "ec2"` / `service == "elasticache"` によるスラッグ分岐 (Fargate 判定、os と engine の属性切替、ElastiCache Serverless の processing-unit 行除外) をそのまま流用すると、compute-sp の Fargate 行が `instanceSavingsPlanRate` に流れて誤変換され、database-sp の ElastiCache Serverless 除外が効かなくなる。行ごとの `ServiceCode` (AmazonEC2 / AmazonRDS / AmazonElastiCache は instance 経路、AmazonECS は Fargate 経路) で正規化関数と属性分岐を選ぶ。行の `ServiceCode` が常に非空でこの 4 値のいずれかであることを実データで確認し、空値や未知値の行は捨てて警告ログを出す (スラッグ前提の暗黙の全行処理に頼らない)。
- リソースサービス (ec2 / rds / elasticache / ecs) は SP の取得をやめ、On-Demand / RI のみを取得する。
- 許可リストは 2 パッケージに独立して 2 系統ある点に注意し、両方を新 7 スラッグに揃える。`internal/aws` の `pricingServiceSpecs` (これを参照する `ValidatePricingService`) と、`internal/pricecache` の `validServices` (これを参照する `ValidateService`。`path` 経由で Load/Save が必ず通る) の双方を更新する。
- ハンドラの `?service=` 検証も新構成に合わせる。

### frontend

- `PRICING_SERVICES` を新構成 (7 サービス) に更新し、`PricingPanel` の `usePricing` / `useRefreshPricing` を固定長で呼ぶ既存パターンを維持したまま数を増やす。あわせて `queries` / `refreshes` の `Record` リテラルと、`eslint-disable react-hooks/exhaustive-deps` を付けた 4 つの依存配列 (`pruneStaleRates` effect、`rates` memo、`ssoExpired` memo、`lastFetchedAt` memo) を新サービス分まで拡張する。exhaustive-deps を無効化しているため、依存配列への追記漏れは lint や tsc で検出されず、新サービスの prune、見積もり、SSO 判定、最終取得時刻が黙って更新されない退行になる (`Record` リテラルのキー欠落は tsc が検出する)。
- `PRICING_SERVICE_LABELS` / `PRICING_SERVICE_ICON_KEY` / `ServiceSelectorBar` に新サービスを追加する。
- `ServiceCard` の `GROUP_ORDER` を簡素化する (リソースカードは SP の group を持たなくなり、SP カードは 1 つの SP group のみを持つ)。
- `PRICING_ATTRIBUTE_FILTERS` に SP サービスのエントリ (instance_type 系のファミリ、os、engine、license_model 等) を追加する。
- `Estimator` はサービス別にグループ化するため、SP サービスがそれぞれ独立した見積もりグループになる。

## 設計上の論点

- **license_model の解決経路が切り離される (最重要)**：`applySavingsPlanLicenseModel` は、同じサービスの On-Demand 取得で作った operation から licenseModel への対応表に依存する (issue 0053)。SP を独立サービスにすると、SP サービスは On-Demand データを取得しないため、この対応表が作れない。取りうる案は次のとおり。
  - (a) SP サービスの取得時に、対象リソースの `GetProducts` (On-Demand) も呼んで operation から licenseModel への対応表だけを組み立てる (On-Demand の行自体は表に出さない)。API 呼び出しは増えるが、結果はファイルキャッシュされるため常時のコストにはならない。
  - (b) ライセンスモデルの区別を諦め、区別できない旨を UI に注記する。
  - (a) を推奨する。ライセンスモデル差が価格差として現れる (issue 0053) 以上、区別を落とすと誤解を生むためである。この対応表構築のコストが本 issue の主要な複雑性になる。
  - licenseModel を持つのは EC2 (Windows 等) と RDS (Oracle 等) のみで、ElastiCache と Fargate は持たない (`recordOperationLicenseModel` は operation と licenseModel の両方が非空のときだけ記録する)。このため対応表の元となる On-Demand 取得は、compute-sp と ec2-instance-sp では EC2 のみ、database-sp では RDS のみで足りる (serviceCode をまたいだ対応表のマージは不要)。
  - 補助の On-Demand 取得は best-effort とする。主データ (`DescribeSavingsPlansOfferingRates`) が成功していれば、補助 On-Demand の失敗で SP サービス全体を失敗させず、ライセンスの区別を落として SP レートは返す。現状の `getPricing` は On-Demand 失敗を致命扱いにするため、この非対称なエラー分離を SP サービスにそのまま流用すると分離前より SP が脆くなる退行になる。縮退の表現も既存の `partial` と `missing_models=["savings_plan"]` を流用しない。この表現は「SP 取得失敗、On-Demand / RI のみ表示」を意味し (`ServiceCard` の固定文言もそれを前提とする)、SP のみを表示する SP カードでは意味が反転する (SP は取得できており、SP カードに On-Demand / RI 行は無い)。「ライセンス区別が未解決」を表す別の縮退表現 (専用の理由コードまたは注記) を backend と frontend に定義し、`ServiceCard` の縮退文言をサービス種別で分岐させる。さらにコールドキャッシュ時は、この補助 On-Demand 取得 (compute-sp と ec2-instance-sp が引く EC2 On-Demand) が、ec2 リソースサービス自身の主取得 (`fetchOnDemandAndReserved`。On-Demand / RI のレート行と opLicense を同時に返す) とも同じ `GetProducts` を重複して呼ぶ。スロットリングで補助取得が失敗すると license 区別が黙って落ちるため、この重複を減らす。ただし ec2 の主取得はレート行を必要とする本取得であって補助取得ではないので、SP 側 2 本の補助取得だけを singleflight で畳んでもコールド時の EC2 On-Demand は 3 本が 2 本になるだけで 1 本にはならない。真に 1 本にするには ec2 の主取得も同一 singleflight に載せ、共有ローダが (レート行, opLicense) を返して ec2 はレート行を、SP 側は opLicense のみを取る形にし、かつ両者の `GetProducts` フィルタ (serviceCode、region、tenancy=Shared、capacitystatus=Used、preInstalledSw=NA) を完全一致させる (一致しないとキー衝突で異なるペイロードを共有する)。この畳み込みの主目的はスロットリング時に license 区別を落とさない堅牢性であり、取得本数削減の性能面の是非は計測 (issue 0058) に委ねる。
- **「SP対象外」バッジは廃止する (決定)**：現状はリソースカード内で On-Demand / RI の instance_type と SP の instance_type を突き合わせてバッジを出している。SP を分離するとこの判定はカードをまたぐ参照になり、対応する SP サービスが非アクティブまたは未取得のとき空集合を「SP 無し」と誤判定する (現状の `!!spInstanceTypes` ガードは undefined のみを弾き空集合は弾かないため、未取得時に全行が誤ってバッジ表示される) など脆くなる。分離後は各 SP カードが適用対象のレートを直接一覧するため、どのインスタンスタイプが SP 対象かはカードの一覧そのものが示す。世代を断定する静的注記は置かない (「新世代のみ対象」は Database SP の特性 (issue 0045) であって Compute SP と EC2 Instance SP には当てはまらず、全 SP カードに一律で出すと Compute SP が旧世代に使えないという誤解を招くため)。
- **永続化された選択の移行**：チェック行の選択は service ごとの rate_id で永続化される。分離により SP の rate_id が ec2 / rds 等のサービスから SP サービスへ移るため、更新前に ec2 等の下で選択されていた SP 行は `pruneStaleRates` で破棄される。過去の SP 選択が失われるが、許容する (リージョン切替時の選択破棄と同じ挙動)。また既存ユーザの永続化された `activeServices` は旧 4 スラッグのみで新サービス (SP 3 種と、issue 0056 の `ec2-spot`) を含まないため、`initialPricingState` (永続化があればそのまま返す) のままだと分離後に新サービスのカードが既定で非表示になる。これを避けるため、`PricingPersistedState` に単調増加のスキーマ版整数を追加する (単発の boolean 移行済みフラグは使わない)。ロード時に `persisted.version` が現行版より小さければ、その版までに既定 active で追加された新メンバーのうち `activeServices` に無いものを補完し、`version` を現行版へ更新する。既定 active のサービスを追加する各リリース (本 issue で SP 3 種、issue 0056 で `ec2-spot`) が版を +1 し、その版で補完すべきメンバーを対応づける。boolean フラグ 1 個では、0055 が先にデプロイされてフラグが立った後に 0056 が `ec2-spot` を追加しても再補完されず、`ec2-spot` が既定非表示のままになる (0056 の完了条件に反する) ため、リリースをまたいで合成できる版番号が要る。同一版内では初回ロードのみ補完し、以後は補完しない。毎ロードで集合差分を埋める実装にすると、ユーザーが意図的に OFF にした新サービスのカードをリロードのたびに復活させてしまい、リソースサービスは OFF を保持できるのに新サービスだけ保持できない非対称な退行になるためである (これは `lib/storage.ts` の view `bigquery` から `gcp` への変換が値ベースで冪等なのと異なり、集合差分の補完は冪等化できないため版番号による制御が要る)。より根本的な代替として、`activeServices` を「有効サービスの許可リスト」ではなく「ユーザーが無効化したサービスの拒否リスト」として持てば、新サービスは常に既定表示となり移行自体が不要になる。この破壊的変更を採るかは実装時に判断する。
- **SP サービスのアイコン**：SP には AWS 公式アイコンの対応がないため、`components/icons/Icons.tsx` のインライン SVG を用いる (公式アイコンの gitignore 方針に従う)。
- **単一 group の SP カードの見出し重複**：SP カードは 1 つの SP group しか持たず、その group 見出し (`spGroup` が返す "EC2 Instance Savings Plans" 等) がカードタイトル (`PRICING_SERVICE_LABELS` の同名) と重複する。SP カードでは group 見出しを抑制するか、重複表示を許容するかを決めておく (`ServiceCard` の group 表示ロジックをサービス種別で分岐させる要否を明記する)。
- **ファイルキャッシュの無効化 (最重要)**：`internal/pricecache` はスキーマ版も TTL も持たず、ファイルが存在すれば常に fresh を返す。本 issue はリソースサービス (ec2 / rds / elasticache / ecs) のキャッシュキー (service / region) を変えずに取得内容を On-Demand / RI のみへ変える [CHANGE] のため、デプロイ前に作られた `ec2/<region>.json` 等 (SP 行を含む) が手動更新まで fresh 配信され続ける。結果、リソースカードが SP を表示したまま新 SP カードとも重複し、完了条件「リソースカードから SP が除去される」に反する。`Load` が版不一致を miss 扱いにして再取得させる無効化機構を入れる。`pricecache` は正規化レート表の中身に依存しない疎結合を意図しているため (`cacheFile` は `{fetched_at, data}` のみ)、版の所有は呼び出し側 (handler または aws パッケージ) に置き、path にバージョン接頭辞を入れて `pricecache` は文字列キーとして扱うだけにする方式を優先する (`cacheFile` にスキーマ版フィールドを足すと疎結合が崩れる)。版をインクリメントする責務は呼び出し側が持つ。または起動時に `PriceCacheDir` をクリアする。issue 0054 が追加する `instance_family` も旧キャッシュには無いため、同じ無効化が要る。
- **他 issue との関係**：issue 0054 (ファミリ絞り込み) は分離後の SP サービスにも適用する。issue 0057 (RI 単価の実効値と節減率) は RI がリソースサービス側に残るため、リソースカード側の変更として扱う。issue 0056 (EC2 Spot) も同じサービスモデルに独立サービス `ec2-spot` を追加するため、実装順序を明示する (本 issue を先行させ、0056 はその独立サービス方式に倣うのを推奨する)。本 issue の対象範囲表は「ECS は On-Demand のみ」「EC2 は On-Demand / RI のみ」を権威的な定義とし、Spot は 0056 が独立サービスとして別途追加する。本 issue はサービスモデルを再構成するため、これらと実装順序を調整する。

## 完了条件

- SP が Compute / EC2 Instance / Database の独立したサービスとして表示され、リソースサービスのカードから SP が除去される。
- 各 SP サービスのカードで、その SP 種別が適用される全リソースのレートを一覧できる。
- ライセンスモデルの区別 (issue 0053) が分離後も維持される (設計上の論点で採った方針に従う)。
- リソースサービスのカードは On-Demand / RI (ECS は On-Demand) のみを表示する。
- 新 SP サービス (compute-sp / ec2-instance-sp / database-sp) のカードでも `instance_family` のチップ絞り込みが効く (issue 0054 の機能を退行させない。`PRICING_ATTRIBUTE_FILTERS` に新 SP サービスのエントリを追加する)。
- 混在 serviceCode のレスポンスが行ごとの `ServiceCode` で正しく振り分けられる (compute-sp で Fargate 行が誤変換されない、database-sp で ElastiCache Serverless の行が混入しない)。
- 旧スキーマ (SP 行を含む、または `instance_family` を持たない) のキャッシュファイルが、デプロイ後に fresh として配信されない (スキーマ版の不一致で miss 扱いになる、または起動時にクリアされる)。
- 既存ユーザの永続化状態で、分離後も新サービス (SP と ec2-spot) のカードが既定で表示される。補完は単調増加のスキーマ版で制御し、各版で追加した新メンバーの補完が版ごとに一度だけ走る (0055 と 0056 が別リリースで順次デプロイされても、各版の補完が確実に一度ずつ走り ec2-spot が既定表示される)。以後ユーザーが OFF にした状態は保持される (毎ロードで復活させない)。
- リソースカードの「SP対象外」バッジが廃止される (どのインスタンスタイプが SP 対象かは各 SP カードのレート一覧が示し、世代を断定する静的注記は置かない)。
- ライセンス逆引きの補助 On-Demand 取得が失敗したときの縮退が、既存の `savings_plan` 欠落表現とは別の「ライセンス未解決」表現で示され、SP カードで意味が反転しない。
- backend のパーサとオーケストレーションのテスト、`internal/pricecache` の許可リスト検証のテスト、フロントの状態分岐のテストを新構成に合わせて更新する。
- `CHANGES.md` の `## develop` に `[CHANGE]` エントリを追記する (サービスモデルと永続化の後方互換を壊す変更のため。種別順 UPDATE → ADD → CHANGE → FIX を守り、次行に 2 文字インデントで `- @sfuruya0612` を付ける)。
- `mise run check` が全て通過する。

## 検証

- backend：`mise run backend:test`。各 SP サービスが対象リソースの SP レートを取得すること、ライセンスモデルの解決が維持されること、許可リスト検証が新構成に追従することを検証する。
- frontend：`mise run frontend:lint` / `mise run frontend:test`。7 サービスの表示、SP サービスの独立表示、リソースカードから SP が消えること、永続化された選択の破棄を検証する。
- 実ブラウザで、SP サービスが独立したカードとして選択して表示され、リソースカードに SP が同居しないことを確認する。ライセンスモデル差のある RDS Oracle 等で、分離後もライセンスモデルの区別が維持されることを確認する。

## 解決方法

設計上の論点に記載した方針 (a) を採用し、次のとおり実装した。

### backend

- `pricingServiceSpecs` を `resourceServiceSpecs` (ec2/rds/elasticache/ecs、On-Demand/RI のみ) と `savingsPlanServiceSpecs` (compute-sp/ec2-instance-sp/database-sp) の 2 マップに分割した。`GetPricing` は service がどちらのマップに属するかで `getResourcePricing` / `getSavingsPlanPricing` に分岐し、必要なクライアント (Price List / Savings Plans) のみを生成する。
- `fetchSavingsPlans` は `DescribeSavingsPlansOfferingRates` の各行が持つ `SavingsPlanOfferingRate.ServiceCode` を `resourceKindFromServiceCode` で ec2/rds/elasticache/ecs (Fargate) に正規化してから行ごとに `savingsPlanRateFrom` へ渡すようにした。thief サービススラッグではなく行の実 ServiceCode で分岐するため、compute-sp の EC2/Fargate 混在や database-sp の RDS/ElastiCache 混在を正しく振り分ける。未知の ServiceCode の行は警告ログを出して破棄する。
- ライセンスモデル解決は `getSavingsPlanPricing` で、SP レート取得 (必須・失敗で全体失敗) の後に `licenseSource` (compute-sp/ec2-instance-sp は ec2、database-sp は rds) の On-Demand を補助的に取得して行う。補助取得が失敗しても SP レート自体は返し、`PriceTable.LicenseUnresolved=true` で縮退を示す (旧 `Partial`/`MissingModels` は廃止し、意味の反転を避けた)。compute-sp と ec2-instance-sp が同時に ec2 の補助取得を要求するケースは `licenseAuxGroup` (singleflight) で 1 回に畳んだ。
- `internal/pricecache` の `validServices` を新 7 スラッグに揃え、キャッシュディレクトリを `pricingCacheSchemaVersion = "v2"` のサブディレクトリに切り替えて (`pricingCacheDir`)、旧スキーマのキャッシュファイルが新デプロイ後も fresh 扱いされないようにした。

### frontend

- `PRICING_SERVICES` を 7 サービスに拡張し、`PricingPanel` は `usePricing`/`useRefreshPricing` を引き続き固定長 (7 個) で無条件に呼ぶ形を維持した。`queries`/`refreshes` の `Record` リテラルと、`pruneStaleRates` effect / `rates` memo / `ssoExpired` memo / `lastFetchedAt` memo の依存配列を新 3 サービス分拡張した。
- 永続化状態の移行は単調増加のスキーマ版 (`PRICING_SCHEMA_VERSION` / `PRICING_SCHEMA_MIGRATIONS` / `migratePricingState`) で実装した。版が現行に達していなければ、その版までに追加された新サービスのうち未選択のものだけを `activeServices` に補完し、版を進める。ユーザーが手動で OFF にした既存サービスは補完対象にならない。
- `ServiceCard` の「SP対象外」バッジと `RateGroupSection` の `spInstanceTypes` 判定は廃止した (SP がカードをまたぐ参照になり、非アクティブ時に誤判定するため)。SP カードは group が 1 種類のみになるため、`PRICING_SERVICE_LABELS` を `spGroup` の返す文字列と一致させ、一致する場合は group 見出しを `hideTitle` で抑制した。
- `license_unresolved` を `PriceTableRaw`/`PriceTableRow` に追加し (`partial`/`missing_models` を置換)、`ServiceCard` の縮退文言をライセンス未解決向けに書き換えた。

### テスト

- backend: `pricing_test.go` を新シグネチャ (`getResourcePricing`/`getSavingsPlanPricing`) に合わせて全面更新し、`TestResourceKindFromServiceCode`・`TestFetchSavingsPlansDispatchesMixedServiceCodes` を新規追加した。`pricecache_test.go`/`handlers_pricing_test.go` にも新スラッグとキャッシュ版隔離の検証を追加した。
- frontend: `pricingSelection.test.ts` にスキーマ移行のテストを追加し、`PricingPanel.test.tsx`/`RateGroupSection.test.tsx`/`normalizePricing.test.ts`/`pricingEstimate.test.ts`/`Estimator.test.tsx` を新シェイプ (7 サービス、`licenseUnresolved`、`hideTitle`) に合わせて更新した。
- `mise run check` が全て通過することを確認した。

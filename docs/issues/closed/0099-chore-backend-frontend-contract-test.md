# backend の JSON タグと frontend の Raw 型の対応を検証する contract テストを整備する

Created: 2026-08-05
Model: Claude Fable 5
Completed: 2026-08-07

## 背景

TODO.md の「backend の JSON タグと frontend の Raw 型の対応を検証する contract テストを整備したい (issue 0092 のスコープ外として送り)」に対応する。

backend は `backend/internal/aws/` 等の Resource 構造体 (24 ファイルに 32 個) の JSON タグで API レスポンスの形状を定める。
frontend は `frontend/src/types/aws.ts` の Raw インターフェース (名前が Raw で終わる interface で、52 個) でそれを受ける。
`frontend/src/types/common.ts` にも Raw インターフェースが 4 個 (プロファイル、呼び出し元アイデンティティ、オブジェクトプレビュー、オブジェクト一覧のエンベロープ) あるが、リソース一覧の契約ではないためこの集計に含めない。
両者の対応を検証する仕組みは現状皆無で、OpenAPI 定義もゴールデンファイルもコード生成も存在しない。
片側だけフィールドを追加、改名した場合、TypeScript の型チェックも Go のテストも通ったまま、実行時に undefined が表示されるまで気付けない。
TODO.md はこの項目を issue 0092 のスコープ外として送られたものと記録している。

## 対応方針

検証の実現手段は次の 4 案のいずれにも決まっていない。
案 4 以外はツールまたは依存の追加を伴う。

1. Go 側で reflect により JSON タグ一覧をエクスポートし、TypeScript 側で Raw 型定義をパースして突き合わせる自作ツール。依存追加は最小だが、TypeScript の型パースの自作は保守負担が大きい。
2. backend のテストでゴールデン JSON を生成し、frontend のテストで `satisfies XxxRaw` により型適合を検証する。実行時の値ベース検証で、optional フィールドの網羅にはフィクスチャの設計が要る。
3. OpenAPI 定義を導入し、両側をスキーマから検証または生成する。最も堅牢だが、`backend/internal/api/routes.go` の 98 のルート登録 (うち GET は 84) に対する定義作成と生成ツールの依存追加が必要で、リポジトリの「依存は最小限」の方針との調整が要る。
4. JSON タグ一覧をゴールデンファイルとしてコミットし、backend のテストが reflect で生成した一覧と文字列比較する。依存追加が無く最も軽いが、frontend 側の Raw 型との突き合わせは人手 (ゴールデンファイル更新時に Raw 型も見直す運用) に残り、片側変更の自動検出という目的を backend 側でしか達成できない。

## pending にした理由

検証手段が 4 案のいずれにも決まっておらず、自動検出の範囲と依存追加のトレードオフという設計判断を伴う。
また、32 の Resource 構造体と 52 の Raw インターフェースの対応表が存在せず、どこまでを検証対象にするか (全型か、変更頻度の高い型だけか) のスコープも未決定である。
グローバル規約により、設計判断が必要で保留中の issue は issues/pending/ に置く。

## 調査結果 (2026-08-05)

対応表の作成にあたり対象を数え直した結果、「背景」と「pending にした理由」に書いた「32 個」は実際より少ない。
`backend/internal/aws/` の JSON タグ付き構造体 (テストファイルを除く) は 66 個ある。
うち 6 個 (`pricing.go` の priceListDocument、priceTermDoc、priceTermAttributesDoc、priceDimensionDoc と、`sso_cache.go` の ssoCacheEntry、`sso_token.go` の ssoTokenFile) は Price List API のレスポンスや SSO キャッシュファイルを読むための非公開型で、API レスポンスに現れないため契約の対象外とする。
残る 60 個が契約の対象である。
また `backend/internal/api/models.go` の `ValueResponse` (54 行) も、Secrets Manager と SSM の値取得エンドポイントのレスポンスを定めるため対象に加える。
`models.go` にはもう 1 つ JSON タグ付きの `SSMValueResponse` (46-49 行) があり、GET `/api/aws/profiles/{profile}/ssm/parameters/{name}` (`handlers_aws.go:220`、`routes.go:41`) が返すが、frontend はこのエンドポイントを呼ばず対応する Raw が無い。
`models.go` の `ProfileInfo` (21 行) と `CallerIdentityInfo` (40 行) は `common.ts` の `ProfileRaw` と `CallerIdentityRaw` に対応するが、「背景」の定めのとおり本 issue の対象外とする (対応関係だけをここに記録する)。

frontend 側の受け手は `aws.ts` の Raw 52 個だけではない。
Athena の 8 構造体は `frontend/src/types/query.ts` の Raw 8 個が受ける。
したがって対応表に載せる backend 側は 62 型 (internal/aws の 60 個と `ValueResponse` と `SSMValueResponse`)、frontend 側は 60 Raw (`aws.ts` の 52 個と `query.ts` の Athena 8 個) で、frontend に対応を持たない backend 型は `SSOAccountResource` と `SSMValueResponse` の 2 つである。
`SSOAccountResource` (`sso.go:13`) は GET `/api/aws/profiles/{profile}/sso` (`handlers_aws.go:196-201`) が返すが、frontend はログイン開始の POST (`endpoints.ts:170`) しか呼ばず、対応する Raw 型が無い。
CLI (`internal/cli/sso.go:101`) はこの型を使う。

調査タスクの完成条件は「32 の Resource 構造体と 52 の Raw インターフェースの全てが少なくとも 1 行に現れること」だが、数え直しにより対象が上記のとおり広がったため、「backend 側 62 型と frontend 側 60 Raw の全てが少なくとも 1 行に現れること」と読み替える。
次の対応表 (62 行) はこれを満たす。
frontend Raw の定義ファイルは、Athena の 8 個が `query.ts`、それ以外が `aws.ts` である。

| backend 型 | 定義ファイル | frontend Raw | 備考 |
| --- | --- | --- | --- |
| APIGatewayResource | apigw.go | APIGWRaw | 名前が一致しない |
| AthenaCatalog | athena.go | AthenaCatalogRaw | query.ts |
| AthenaDatabase | athena.go | AthenaDatabaseRaw | query.ts |
| AthenaWorkgroup | athena.go | AthenaWorkgroupRaw | query.ts |
| AthenaColumn | athena.go | AthenaColumnRaw | query.ts |
| AthenaTable | athena.go | AthenaTableRaw | query.ts |
| AthenaQueryExecution | athena.go | AthenaExecutionRaw | query.ts、名前が一致しない |
| AthenaResultColumn | athena.go | AthenaResultColumnRaw | query.ts |
| AthenaResultPage | athena.go | AthenaResultPageRaw | query.ts |
| CFNStackResource | cfn.go | CFNStackRaw | 名前が一致しない |
| CFNStackDetail | cfn.go | CFNStackDetailRaw | |
| CFNParameter | cfn.go | CFNParameterRaw | |
| CFNOutput | cfn.go | CFNOutputRaw | |
| CFNStackEvent | cfn.go | CFNStackEventRaw | |
| CFNStackResourceSummary | cfn.go | CFNStackResourceRaw | 名前が一致しない |
| CloudFrontResource | cloudfront.go | CloudFrontRaw | |
| CloudFrontBehavior | cloudfront.go | CloudFrontBehaviorRaw | |
| LogGroupInfo | cloudwatchlogs.go | CWLogGroupRaw | 名前が一致しない |
| LogEventInfo | cloudwatchlogs.go | CWLogEventRaw | 名前が一致しない |
| LogEventPage | cloudwatchlogs.go | CWLogEventPageRaw | 名前が一致しない |
| CostResource | cost.go | CostRaw | |
| ForecastResource | cost.go | ForecastRaw | |
| DynamoResource | dynamo.go | DynamoRaw | |
| DynamoKeyAttribute | dynamo.go | DynamoKeyAttributeRaw | |
| DynamoIndexSchema | dynamo.go | DynamoIndexSchemaRaw | |
| DynamoTableSchema | dynamo.go | DynamoTableSchemaRaw | |
| EC2Resource | ec2.go | EC2Raw | |
| ECRRepoResource | ecr.go | ECRRepoRaw | |
| ECRImageResource | ecr.go | ECRImageRaw | |
| ECSResource | ecs.go | ECSRaw | |
| ECSServiceResource | ecs_exec.go | ECSServiceRaw | |
| ECSTaskResource | ecs_exec.go | ECSTaskRaw | |
| ECSTaskContainerDetail | ecs_exec.go | ECSTaskContainerDetailRaw | |
| ECSContainerResource | ecs_exec.go | ECSContainerRaw | |
| ElastiCacheResource | elasticache.go | CacheRaw | 名前が一致しない |
| ElastiCacheParameter | elasticache.go | CacheParameterRaw | 名前が一致しない |
| ELBResource | elb.go | ELBRaw | |
| ELBListenerResource | elb.go | ELBListenerRaw | |
| ELBRuleResource | elb.go | ELBRuleRaw | |
| ELBTargetGroupResource | elb.go | ELBTargetGroupRaw | |
| ELBTargetHealthResource | elb.go | ELBTargetHealthRaw | |
| IAMResource | iam.go | IAMRaw | |
| KinesisResource | kinesis.go | KinesisRaw | |
| LambdaResource | lambda.go | LambdaRaw | |
| NATGatewayResource | natgw.go | NATGWRaw | 名前が一致しない |
| PriceTable | pricing.go | PriceTableRaw | |
| PriceRate | pricing.go | PriceRateRaw | |
| PriceTerm | pricing.go | PriceTermRaw | |
| RDSResource | rds.go | RDSRaw | |
| RDSParameter | rds.go | RDSParameterRaw | |
| RDSClusterParameterGroup | rds.go | RDSClusterParameterGroupRaw | |
| RegionResource | regions.go | RegionRaw | |
| S3Resource | s3.go | S3Raw | |
| S3ObjectResource | s3_object.go | S3ObjectRaw | |
| SecretResource | secretsmanager.go | SecretRaw | 名前が一致しない |
| SQSResource | sqs.go | SQSRaw | |
| SSMParameterResource | ssm.go | SSMParamRaw | 名前が一致しない |
| SSOAccountResource | sso.go | (対応なし) | frontend に消費者が無い。CLI 専用 |
| WAFResource | waf.go | WAFRaw | |
| WAFRule | waf.go | WAFRuleRaw | |
| ValueResponse | models.go (internal/api) | ValueRaw | 名前が一致しない |
| SSMValueResponse | models.go (internal/api) | (対応なし) | frontend に消費者が無い |

テスト基盤の前提も確認した。

- API レスポンスにエンベロープは無い。`serveCached` (`server.go:125-145`) から `writeJSON` (`handlers_aws.go:406-409`) が構造体のスライスをそのままエンコードする。契約は構造体の形状そのものである。
- frontend の `tsconfig.json` は `resolveJsonModule` が有効 (10 行) で、include は `src` 全体 (20 行)。`npm run lint` の `tsc --noEmit` はテストファイルも型検査する。JSON を import して型検査に使う下地は既にある。
- backend にゴールデンテストの先例は無い (grep で "golden" は 0 件)。最も近いのは `waf_test.go:361-387` の `json.Marshal` 結果へのキー存在アサーションである。
- 変更頻度は直近 3 か月で internal/aws が 59 コミット、`aws.ts` が 28 コミットで、構造体と Raw を同時に変えたコミットが複数ある (b24ffe9、656249d、3548b7a 等)。特定の型に変更が偏ってはいないため、対象は全型とする。

## 決定した設計

「対応方針」の 4 案を、依存追加の量、保守負担、検出できる不整合の種類で比較し、案 2 (ゴールデン JSON) を採用する。
「対応方針」は「案 4 以外はツールまたは依存の追加を伴う」としていたが、調査の結果この前提は成り立たない。
案 2 が使う go test、tsc、vitest はいずれも導入済みで、依存の追加を伴わないためである。
以下の比較はこの訂正を前提とする。

- 案 1: 依存追加は無いが、TypeScript の型定義パーサの自作が保守負担として重い。型適合の検査は tsc が既にできることであり、自前実装で二重化することになる。採らない。
- 案 2: 依存追加が無く (go test、tsc、vitest は導入済み)、保守はゴールデンファイルの再生成に集約される。フィールドの欠落 (両方向)、改名、型の不一致を検出できる。採用する。
- 案 3: OpenAPI は生成ツールの依存追加が「依存は最小限」の方針と衝突する。98 ルートのスキーマ整備は、Raw 型対応の検証という本 issue の目的を超える。採らない。
- 案 4: backend 側の変更しか検出できず、frontend の Raw の改名や型変更に気付けない。タグ名の一覧では数値か文字列かの型も検証できない。採らない。

採用した案 2 の設計は次のとおり。
「対応方針」の案 2 は `satisfies` による検証としていたが、`satisfies` を含む単純な型適合の検査は構造的部分型が余分なキーを許すため、backend 側にだけあるフィールド (Raw の追加漏れ) を検出できない。
この点を補い、キー集合の双方向一致を型レベルで検査する形に具体化する。

- backend のテストが、契約対象の各構造体について reflect で全フィールドに非ゼロの決定的な値を埋めたインスタンスを生成し、JSON にした結果をゴールデンファイルとしてコミットされたものと比較する。全フィールドに値を入れるのは、`omitempty` のフィールドや nil のスライスと map の脱落を防ぎ、キー集合を固定するためである。
- ゴールデンファイルは frontend から import できる場所に置き、backend のテストと frontend の型検査が同一ファイルを参照する。
- frontend はゴールデンを import し、型レベルで Raw とゴールデンのキー集合の双方向一致と各フィールドの型の適合を検査する。ネストした構造体はトップレベルのキー検査だけでは内部の不整合を素通しするため、各階層に同じ検査を再帰的に適用する。検査は `tsc --noEmit` (`npm run lint` に含まれる) で落ちる形にする。
- 検出できる不整合は、フィールドの欠落 (backend だけにある、frontend だけにある、の両方向)、改名、型の不一致である。optional の食い違いは、ゴールデンが全フィールドを持つ前提により、Raw 側の必須フィールドの欠落とキー集合の差分として検出する。
- Raw 側が意図的に optional を宣言するフィールド (backend が `omitempty` を付けるフィールド。docs/issues/0109 が追加する 7 つの `*_fetch_failed` フラグが該当する) も検査の対象に含める。フィラーが非ゼロ値を入れるためゴールデンにはキーとして必ず現れ、TypeScript の `keyof` は optional のキーも含むため、キー集合の双方向一致の検査は optional の宣言に影響されずに成立する。型の適合はゴールデンの値の型が Raw のフィールド型から undefined を除いた型に適合するかで検査する。
- 検出できない不整合は null 許容 (`| null`) の過不足である。ゴールデンは全フィールドに非 null の値を持つため、backend が null を返し得るかどうかは検査に現れない。この限界は受け入れる。
- backend が `omitempty` を付けるフィールドを Raw 側が必須 (optional でない) と宣言する食い違いも検出できない。ゴールデンは全フィールドに値を入れて生成するためキーが常に現れ、実行時にキーが欠落して undefined になる危険が検査に現れないためである。ゼロ値で生成した第 2 のゴールデンと突き合わせ、キーが落ちるフィールドに optional の宣言を強制する案も検討したが、ゴールデンと検査の層が倍になる保守負担がこの 1 種の不整合の検出に見合わないため採らない。この限界も受け入れ、`omitempty` と optional の対応付けはフィールドを追加する issue 側 (docs/issues/0109 等) のレビューで担保する。
- Raw 側がリテラルユニオンで backend 側が string のフィールド (対応表の範囲で該当するのは `PriceRateRaw.model` の `PriceModel` ユニオンのみ) は、フィラーの生成値がユニオン外になるため型適合の検査から除外し、除外の一覧を理由付きで検査ファイルに残す。
- スコープは対応表の frontend Raw を持つ 60 対とする。`SSOAccountResource` と `SSMValueResponse` は frontend に消費者が無いため対象外とし、`common.ts` の Raw 4 個と `nonaws.ts` の Raw (BigQuery、Datadog、TiDB) も「背景」の定めのとおり対象外とする。

決定した設計の実装は issue 0110 として起票した。

## 完了条件

- issue 0110 の完了後に確認する。
- docs/issues/0110 が close され、「決定した設計」に定めたゴールデン JSON と型レベル検査が 0110 の完了条件のとおり実装されている。
- 本 issue 自体はコードの変更を伴わない。

## 関連

- docs/issues/closed/0092: 本 issue の起票元。TODO.md がこの項目を 0092 のスコープ外として記録している。
- docs/issues/0110: 決定した設計の実装 issue。
- docs/issues/0109: 契約対象の 4 構造体に `*_fetch_failed` フラグを追加する issue。docs/issues/0110 はその完了後に行う。

## 解決方法

- 本 issue は issue 0110 の完了確認だけを求めるもので、コードの変更は行っていない。
- 完了条件「issue 0110 の完了後に確認する」を確認した。issue 0110 は `docs/issues/closed/0110-chore-contract-test-golden-json.md` に close 済みで、実装は 1 コミットとして記録されている。
- 完了条件「『決定した設計』に定めたゴールデン JSON と型レベル検査が 0110 の完了条件のとおり実装されている」を現状に対して確認した。backend の `backend/internal/contract/` パッケージが対応表の 60 型のレジストリと決定的なフィラーを持ち、`TestGolden` が `frontend/src/types/__contract__/` にコミットされた 60 個のゴールデン JSON と生成結果を比較し、`UPDATE_GOLDEN=1` で再生成できる。frontend の `frontend/src/types/contract.check.ts` が同じ 60 個のゴールデンを import し、Raw 型とのキー集合の双方向一致と各フィールドの型の適合を全階層で検査し、`tsc --noEmit` (`npm run lint` に含まれる) で落ちる形になっている。
- 「決定した設計」の個別事項も 0110 の実装に現れていることを確認した。7 つの `*_fetch_failed` フラグはゴールデンにキーとして現れて検査対象に含まれ、`PriceRateRaw.model` のリテラルユニオンは理由付きの除外一覧として `contract.check.ts` の冒頭に記録され、対象は frontend Raw を持つ 60 対で `SSOAccountResource` と `SSMValueResponse` は含まれていない。
- 「決定した設計」が受け入れるとした omitempty と optional の食い違いの限界に対し、0110 は JSON タグ一覧のゴールデン (`backend/internal/contract/testdata/tags.golden`) と `TestTagsGolden` でタグの変更そのものを検出する形の補完を加えている。方式 (ゴールデン JSON) は保たれており、経緯は docs/issues/closed/0110 の「実装詳細の乖離の記録」にある。
- 完了条件「本 issue 自体はコードの変更を伴わない」のとおり、本 close でリポジトリのコードは変更しておらず、`CHANGES.md` への追記も行わない。
- `mise run check` を 1 回実行し、ベースラインからの新たな失敗が無く通ることを確認した。

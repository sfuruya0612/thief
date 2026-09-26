# WAF の Associated が ALB 以外の関連リソースを数えず CLOUDFRONT スコープでは常に 0 になるのを修正する

Created: 2026-07-27
Completed: 2026-07-27
Model: Claude Fable 5 / Claude Sonnet 5

## 症状

WAF の一覧の Associated 列に 0 が表示される。
Associated 列は一覧 API のレスポンスの `associated_count` フィールドの表示である。
TODO.md の次の項目に対応する。

> WAF の Associated が 0 になってしまっていてどこかおかしい

TODO に記録された観測はこの 0 表示だけで、対象 ACL のスコープ、エラーメッセージ、レスポンス断片は残っていない。
コード調査により、権限が揃っていても 0 になる実装漏れの経路が 2 つ実在することを確認した。

- REGIONAL スコープの Web ACL では、API Gateway や AppSync 等 (ALB 以外) だけが関連付いている場合に 0 になる。
- CLOUDFRONT スコープの Web ACL では、関連付けの取得処理自体が無く常に 0 になる。

## 再現手順

1. ALB 以外のリソース (例: API Gateway REST API のステージ) だけを関連付けた REGIONAL の Web ACL を用意する。関連付けの存在は `aws wafv2 list-resources-for-web-acl --web-acl-arn <ARN> --resource-type API_GATEWAY` で `ResourceArns` が 1 件以上返ることで確認する (このコマンドに scope の引数は無く、対象スコープは ARN から決まる)。
2. CloudFront ディストリビューションを関連付けた CLOUDFRONT の Web ACL を用意する。関連付けの存在は `aws cloudfront list-distributions-by-web-acl-id --web-acl-id <ACL の ARN>` で 1 件以上返ることで確認する。
3. `mise run backend:run` で API サーバを起動し、`GET /api/aws/profiles/{profile}/waf` を呼ぶ。
4. `associated_count` を確認する。期待される観測結果はどちらの Web ACL も 1 以上、実際の観測結果はどちらも 0 になる。

backend のリソースキャッシュは TTL 1 時間 (`backend/internal/api/server.go:18` の `cacheTTL`) のため、修正後の検証では frontend の Refresh ボタン (`POST /api/cache/invalidate`) 等でキャッシュを破棄してから確認する。

## 原因

原因は 2 つある。
1 つ目は `listWAFACLs` (`backend/internal/aws/waf.go:95-171`) にある REGIONAL スコープの取得漏れ。
2 つ目は CLOUDFRONT スコープの関連取得が、`listWAFACLs` にも呼び出し元の `ListWAFResources` (`backend/internal/aws/waf.go:42`) にも実装されていないこと。

1 つ目について。
139 行からの関連リソース取得が `ResourceType` を指定せずに `ListResourcesForWebACL` を呼んでいる。

```go
associatedCount := 0
if scope == waftypes.ScopeRegional {
    resOut, resErr := client.ListResourcesForWebACL(gctx, &wafv2.ListResourcesForWebACLInput{
        WebACLArn: s.ARN,
    })
```

aws-sdk-go-v2 wafv2 v1.74.1 の `api_op_ListResourcesForWebACL.go:54-57` に、`ResourceType` を省略すると `APPLICATION_LOAD_BALANCER` として扱われると明記されている。
`ResourceType` の列挙値は 8 種類 (`service/wafv2/types/enums.go:1368-1375`): APPLICATION_LOAD_BALANCER, API_GATEWAY, APPSYNC, COGNITO_USER_POOL, APP_RUNNER_SERVICE, VERIFIED_ACCESS_INSTANCE, AMPLIFY, AGENTCORE_GATEWAY。
いずれもリージョナルリソースで、CLOUDFRONT はこの列挙に含まれない (CLOUDFRONT を含む `AssociatedResourceType` (`types/enums.go:34-44`) は別の列挙で、`ListResourcesForWebACL` の入力には使われない)。
そのため ALB 以外の 7 種別の関連リソースが数えられない。

2 つ目について。
上記の `if scope == waftypes.ScopeRegional` により、CLOUDFRONT スコープでは関連リソース取得を最初から行わず、`associatedCount` が初期値 0 のまま `newWAFResource` (`backend/internal/aws/waf.go:173-184`) に渡る。
`ListResourcesForWebACL` は CLOUDFRONT スコープに使えない (SDK ドキュメントが CloudFront は `ListDistributionsByWebACLId` を使うと明記) ため、スコープ分岐自体は正しく、CloudFront 側 API を呼ぶ経路がどこにも無いことが問題になる。

なお、取得エラーは `handleIgnoredErr` (`backend/internal/aws/errors.go:84-93`) が slog.Warn に落として 0 件扱いにするため、権限不足でも症状は同じ 0 表示になる。
実装漏れの 2 経路はコードで確定しており、修正対象とする。
2 経路は別々の API への別々の修正だが、片方だけ直しても Associated の 0 表示は残りの経路で再現し続け、TODO の要望を満たせないため、分割せず本 issue で両方を扱う。
TODO の観測がどの原因によるものかの切り分けは、完了条件の再観測で行う。

## 修正方針

- REGIONAL スコープ: `waftypes.ResourceType("").Values()` が返す全種別 (現行 8 種類) についてそれぞれ `ListResourcesForWebACL` を呼び、`ResourceArns` の件数を合計する。対象種別の一覧は関数 `wafRegionalResourceTypes() []waftypes.ResourceType` に、種別ごとの取得結果の合算は純関数 (`sumResourceARNs(arnLists [][]string) int` 等。取得に失敗した種別は nil 要素として渡す) に切り出してテスト可能にする。
  - `Values()` を使うことで、SDK 更新で種別が追加されたとき実装が自動で追随する。完了条件の 8 値固定テストは SDK 更新の検知に使う (テストが落ちたら期待値を更新し、追加種別での動作を確認する)。
  - 却下案: 8 種別のハードコード配列。SDK に種別が追加されたとき同じ過少カウントが再発するため却下。
  - 却下案: 主要な種別 (ALB, API Gateway, AppSync) だけに絞る。絞った種別以外が関連付いたときに同じ過少カウントが再発するため却下。
  - 種別は排他のため、重複排除は不要 (ARN はサービス名とリソース種別をパスに含む形式で、1 つの ARN が複数の `ResourceType` に該当することは無い)。
  - `ListResourcesForWebACLOutput` にページネーションは無い。
  - 種別ごとの呼び出しは既存どおり `handleIgnoredErr` で個別に処理し、1 種別の失敗が他の種別の集計を止めないようにする。`ListResourcesForWebACL` は種別ごとに対象サービス側の IAM 権限を要求する場合があり、権限が無い種別は Warn ログと 0 件扱いに劣化する。
    - Warn ログには属性 `web_acl_arn` (ACL の ARN) と `resource_type` (種別) を含め (`handleIgnoredErr` (`backend/internal/aws/errors.go:84-93`) は可変長の属性引数を取る)、どの ACL のどの種別の取得が失敗したかを判別できるようにする。現行の呼び出し (`waf.go:146`) の属性 `web_acl` (ACL 名) はこの 2 属性で置き換える。
    - 同じ `listWAFACLs` 内のタグ取得失敗の Warn (`waf.go:157`) にも属性 `web_acl_arn` を追加する (現行の `web_acl` (名前) は残す)。完了条件の切り分けは Warn ログを ACL の ARN で突き合わせるため、失敗の系統によって参照キーが名前と ARN に分かれると突き合わせが 2 系統になるのを避ける。
    - ログ属性は出力の指定であり自動テストの対象外とする (完了条件の切り分けで参照できることをもって確認する)。
    - 権限が揃わないプロファイルでは ACL ごとに最大で種別数分の Warn が定常的に出るが、完了条件の切り分けに要る観測可能性を優先して受容する (ログレベルは Warn のまま変えない)。
  - API 呼び出しは ACL あたり現行 3 回 (GetWebACL, ListResourcesForWebACL, ListTagsForResource) から 10 回 (ListResourcesForWebACL が 8 回) に増える。種別ごとの 8 回の呼び出しは ACL ごとの goroutine 内で直列に行う。同時に実行中の呼び出し数の上限は従来と同じ `wafACLConcurrency = 10` (`backend/internal/aws/waf.go:38`) のままで、docs/issues/closed/0081 が wafv2 のレート制限が厳しいことを理由に選んだ同時実行数を増やさないため、`wafACLConcurrency` の値も見直さない (総呼び出し数と所要時間は増えるが、同時実行密度は変わらない)。増加によるスロットリングは Warn ログと過少カウントとして現れる既知の劣化として受容し、リトライやレート制御の追加は計測なしに行わない。
    - 却下案: 種別ごとの 8 回の呼び出しを errgroup で並列化する。直近の faaaf2e はサービスをまたぐ独立した呼び出しの並列化であり、本件はレート制限が厳しい同一 API への多重化のため事情が異なる。並列化すると同時に実行中の呼び出しが最大 80 になり、docs/issues/closed/0081 の判断を崩すため却下。
    - 却下案: 総呼び出し数の増加に合わせて `wafACLConcurrency` を下げる。同時実行密度は増えないため下げる根拠が無く、スロットリングが起きれば Warn ログと過少カウントで観測できるため、観測なしの変更は行わない。
- CLOUDFRONT スコープ: `ListWAFResources` の CLOUDFRONT 側 goroutine (`backend/internal/aws/waf.go:72-79`) 内で、`listWAFACLs` の完了後に次を直列に行う (`listWAFACLs` との並列化は計測根拠が無いため行わない)。
  - `listWAFACLs` が返した ACL が 0 件なら、ディストリビューションの走査を行わずに終える。ACL 件数は同一 goroutine 内で走査より先に判明しているため、構造変更なしに省略できる。この省略は AWS クライアント呼び出しを含む goroutine 内の分岐で純関数に切り出せないため、ページネーション途中失敗の分岐と同じく自動テストの対象外とする。
  - us-east-1 の CloudFront クライアント (既存の `newCloudFrontClient`) を使い、`ListDistributions` でディストリビューション一覧をページネーション込みで一度だけ走査する (API 呼び出し回数はページ数に比例)。`page.DistributionList` が nil のページは読み飛ばす (`backend/internal/aws/cloudfront.go:44-46` の既存処理を踏襲)。
    - 却下案: ACL ごとに `ListDistributionsByWebACLId` を呼ぶ。`cloudfront:ListDistributionsByWebACLId` 権限が新たに要る点が主因で却下 (`ListDistributions` は CloudFront 一覧 (`backend/internal/aws/cloudfront.go:38-50`) で使用済みの権限で、権限の種類を増やさない)。API 呼び出し回数は ACL 数とページ数のどちらが多いかに依存するため、回数削減は却下理由にしない。
  - `DistributionSummary.WebACLId` を集計して ACL ARN ごとの件数マップを作り、`listWAFACLs` が返した ACL 群に適用する。
  - `newCloudFrontClient` の生成に失敗した場合は `handleIgnoredErr` の経路で Warn ログと全 ACL 0 表示に劣化させ、WAF の一覧全体はエラーにしない (`newWAFClient` の失敗が一覧全体をエラーにするのとは異なり、CloudFront 側の不備で WAF ビュー自体を落とさない)。
  - ページネーションの途中で失敗した場合は、部分的に集計した件数マップを適用せず破棄し、クライアント生成失敗と同じく Warn ログと全 ACL 0 表示に劣化させる。部分集計を適用すると、どの ACL が過少カウントかを判別できないまま一部だけ正しい値が出る見かけの正確さを生むため採らない。この分岐は AWS クライアントを直接呼ぶ経路にあり純関数に切り出せないため、自動テストの対象外とする。
  - `WAFResource` (`backend/internal/aws/waf.go:18-28`) に `ARN string` (JSON タグ `-`) を追加し、`newWAFResource` (`backend/internal/aws/waf.go:173-184`) で写す。現状 ARN は `listWAFACLs` のループ変数 `s` (`waftypes.WebACLSummary`) のフィールド `s.ARN` として参照されるだけで `WAFResource` に残らない (docs/issues/closed/0075 も `WAFResource` に ARN が含まれないことを明記している) ため、ARN での突き合わせにはフィールド追加が要る。
    - JSON タグを `-` にするのは、ARN を件数マップとの突き合わせにだけ使い、frontend も CLI も読まないため。backend のリソースキャッシュ (`backend/internal/cache/cache.go`) は Go の値をそのまま保持し JSON を経由しないため、`-` でも突き合わせに支障は無い。frontend の Raw 型と Row にも追加せず、API レスポンスの形は変わらない。
    - 却下案: JSON タグ `arn` でレスポンスに公開する。バグ修正で API の公開形を広げる必要が無く、公開すると互換性の維持対象が増えるため却下。
    - 却下案: `WebACLId` に `arnLastSegment` (`backend/internal/aws/waf.go:318-325`) を適用し、ACL の ID で突き合わせる。WAFv2 の ID も WAF Classic の GUID も UUID 形式のため、Classic の値が WAFv2 の ID と衝突しない保証が確率の議論になり却下。
    - 却下案: `listWAFACLs` の戻り値に ARN の並行リストを追加する。リソースの属性を並行リストで持ち回るより `WAFResource` のフィールドにする方が単純なため却下。
  - `newWAFResource` は位置引数のまま、`arn string` を `name` の直後に追加する (7 引数から 8 引数)。既存の `TestNewWAFResource` (`backend/internal/aws/waf_test.go:13`) は id / name / description に互いに異なる値を与えて引数順の取り違えを検出する方式で (`waf_test.go:66` のコメントに明記)、期待値も `WAFResource` 全体を書いて `reflect.DeepEqual` で比較しているため、全ケースで `arn` にも他と異なる値を与えて期待値に `ARN` を追加する。docs/issues/0086 が `TestNewWAFRule` をフィールド単位比較に変えるのと異なり `reflect.DeepEqual` 比較を維持するのは、追加が短い文字列 1 フィールドで期待値リテラルの更新で足り、全フィールドを見る比較の網羅性を保てるため。
    - 却下案: `waftypes.WebACLSummary` を受け取るシグネチャに変える。`ruleCount` (GetWebACL 由来) と `tags` (ListTagsForResource 由来) は summary に含まれないため引数は減らず、テストで summary の構築が増えるだけのため却下。
    - 却下案: 引数をまとめるオプション構造体を導入する。引数 8 個の関数 1 つのために導入する規模ではないため却下。
  - 集計は純関数 `cloudFrontACLCounts(summaries []cftypes.DistributionSummary) map[string]int` に、適用は純関数 `applyCloudFrontCounts(acls []WAFResource, counts map[string]int)` (ACL の `ARN` フィールドでマップを引いて `AssociatedCount` を埋める) に切り出してテスト可能にする。新規の関数と集計処理は `backend/internal/aws/waf.go` と `waf_test.go` に置く (`waf.go` に cloudfront パッケージの import が新たに増える)。`backend/internal/aws/cloudfront.go` は変更しない。
  - `listWAFACLs` のシグネチャは変えない。
    - 却下案: `listWAFACLs` に件数マップの引数を追加する。REGIONAL 側で nil を渡す非対称なシグネチャになり、goroutine 内での取得後適用で足りるため却下。
  - WAFv2 の Web ACL が関連付いたディストリビューションでは `WebACLId` に ACL の ARN が入り、WAF Classic の関連付けでは GUID が入る。この区別の出典は `DistributionConfig.WebACLId` のドキュメント (aws-sdk-go-v2 cloudfront v1.61.1 `types/types.go:2355-2372`) で、実装が読む `DistributionSummary.WebACLId` (`types/types.go:2655`) のドキュメントには区別の記載が無い。両者は同じ Web ACL の関連付けを表す値のため `DistributionConfig` の記述が適用されるとみなし、みなしの妥当性は完了条件の実データ確認で裏付ける。WAF Classic の GUID は WAFv2 の ACL の ARN と一致しないだけで誤カウントは生じない。
  - 既知の限界: マルチテナントディストリビューションの関連付けは数えない。ディストリビューションテナントは Web ACL をテナント単位で上書きまたは無効化でき (`WebAclCustomization`、aws-sdk-go-v2 cloudfront v1.61.1 `types/types.go:7065`)、テナント自体は `ListDistributions` に現れない (本体は `ConnectionMode` が `tenant-only` として現れる: `types/types.go:2675-2678`)。そのためテナントにだけ関連付いた Web ACL は本修正後も 0 のままになり、テナント側で保護を無効化していても本体の `WebACLId` を 1 と数える。テナントまで数えるには `ListDistributionTenantsByCustomization` の追加呼び出しとその新権限が要るため採らず、この過少と過大の両方向を既知の限界として受容する。完了条件の切り分けで 0 のままの ACL を調べる際は、テナントへの関連付けの可能性を確認対象に含める。
  - 既知の限界: 継続的デプロイのステージングディストリビューションも数える。ステージングは `ListDistributions` の一覧に本体と別のエントリとして現れ (`DistributionSummary.Staging`、aws-sdk-go-v2 cloudfront v1.61.1 `types/types.go:2633-2638`)、プライマリの複製として作られるため、プライマリと同じ Web ACL を指す `WebACLId` を持ち得る。その場合は関連付けの実体 1 つが 2 件と数えられる。`Staging` での除外は継続的デプロイの利用中に限る過大カウントのための追加分岐になるため入れず、既知の限界として受容する。切り分けで件数が想定より大きい場合は、ステージングディストリビューションの有無を確認対象に含める。
  - この方針により、WAF ビューの Associated 表示が `cloudfront:ListDistributions` 権限を使うようになる。この権限は CloudFront 一覧ビューが既に使っており本アプリとして新種の要求ではないが、WAF ビューだけを使うプロファイルには実質的な追加要件になるため CHANGES.md に明記する。権限が無い場合は既存の `handleIgnoredErr` 経路で Warn ログと 0 表示に劣化する (一覧全体は落とさない)。
- テストは純関数に対して行う。wafv2 クライアントのインターフェース化やモック導入は行わない (docs/issues/closed/0067 と docs/issues/closed/0075 で確立した方針)。この方針の帰結として、`listWAFACLs` と `ListWAFResources` から純関数 (`wafRegionalResourceTypes`, `sumResourceARNs`, `cloudFrontACLCounts`, `applyCloudFrontCounts`) への配線は自動テストの対象外になる。配線の正しさは完了条件の実環境確認 (再現手順の 2 構成で 1 以上が表示されること) で担保する。
- 取得エラーを Warn ログと 0 件扱いに落とす既存の `handleIgnoredErr` の方針は変えない (取得失敗時の 0 と真の 0 を frontend で区別する設計は、完了条件に記載のとおり本 issue では扱わない)。
- frontend と CLI の変更は不要 (`associated_count` の値が正しくなるだけで形は変わらない)。

## 完了条件

- `cloudFrontACLCounts` のテーブル駆動テストが `backend/internal/aws/waf_test.go` にあり、次のケースを含む: 複数ディストリビューションが同じ ACL ARN を指す場合の合算、`WebACLId` が nil と空文字のディストリビューションの除外、ディストリビューションが 0 件の場合の空マップ。
- `applyCloudFrontCounts` のテーブル駆動テストがあり、次のケースを含む: ARN が一致する ACL に件数が入る、マップに無い ARN の ACL は 0 のまま、マップのキーが WAF Classic の GUID のみの場合にどの ACL も 0 のまま。
- `wafRegionalResourceTypes` のテストがあり、既知の 8 値 (APPLICATION_LOAD_BALANCER, API_GATEWAY, APPSYNC, COGNITO_USER_POOL, APP_RUNNER_SERVICE, VERIFIED_ACCESS_INSTANCE, AMPLIFY, AGENTCORE_GATEWAY) と集合として一致することを検証している (順序は比較しない。`Values()` のドキュメント (`service/wafv2/types/enums.go:1378-1381`) が並び順の安定を保証していないため。SDK 更新で種別が増えたときはテストが落ちて追随を確認できる)。
- `sumResourceARNs` のテーブル駆動テストがあり、次のケースを含む: 全種別の件数が合算される、一部の種別が 0 件でも残りが合算される、一部の種別が nil (取得失敗) でも残りが合算される。
- `TestNewWAFResource` (`backend/internal/aws/waf_test.go:13`) の全ケースで `arn` 引数に他の引数と異なる値が与えられ、期待値に `ARN` フィールドが追加されている (引数順の取り違えを検出する既存方式の維持)。
- `WAFResource` の `json.Marshal` 出力にキー `"arn":` と `"ARN":` が現れないこと (JSON タグ `-` の検証) を確認するテストがある (`TestWAFResourceJSONHasDescription` (`backend/internal/aws/waf_test.go:346`) と同じ `strings.Contains` 方式。検査文字列はコロンまで含めたキーの形にし、他フィールドの ARN 値に含まれる `arn:` の文字列への誤反応を避ける)。
- TODO の観測元の環境で、0 と表示されていた Web ACL のスコープと ID を再観測して issue に記録する。再観測できない場合 (対象の Web ACL が削除済み等) はその理由を記録し、以降の実環境確認は現存する Web ACL で行う。
- 実環境の確認はスコープごと (REGIONAL / CLOUDFRONT) に行い、該当スコープの Web ACL が環境に無い場合はその旨を記録して省略する。
- 修正後に、再現手順の 2 構成 (ALB 以外のリソースだけを関連付けた REGIONAL の Web ACL と、CloudFront ディストリビューションを関連付けた CLOUDFRONT の Web ACL) のそれぞれで `associated_count` が 1 以上で表示されることを確認する。ALB だけが関連付いた ACL は修正前でも 1 以上を返すため、確認対象にしない。該当構成の Web ACL が環境に無い場合は前項に従いその旨を記録して省略する。1 以上にならない場合は次のとおり切り分ける。
  - backend の Warn ログに該当する取得失敗があれば (REGIONAL は属性 `web_acl_arn` と `resource_type` で該当 ACL を特定する。CLOUDFRONT はクライアント生成と `ListDistributions` の失敗が全 ACL の 0 表示に波及する)、原因の種別 (権限不足、スロットリング等) を記録して本 issue を close する。
  - Warn ログが無く、関連リソースも実在しない場合は、観測が真の 0 だったことを記録して close する。
  - Warn ログが無く、関連リソースが実在するのに 0 のままの場合は、原因を特定するまで本 issue を close しない。判明した事実 (ACL の構成、ログ、関連リソースの一覧、マルチテナントディストリビューションへの関連付けの有無) を本 issue に追記して調査を続け、特定でき次第、上の 2 分岐または修正の追加に帰着させる。
- CLOUDFRONT スコープの確認では、関連付いたディストリビューションの `DistributionSummary.WebACLId` に WAFv2 の ACL の ARN が入っていることを、`aws cloudfront list-distributions` の出力で実データ 1 件について確認する (このフィールドのドキュメントには WAFv2 と WAF Classic の値の区別が書かれていないため)。確認できる構成が環境に無い場合は、`DistributionConfig.WebACLId` のドキュメント (`types/types.go:2355-2372`) を根拠とすることを記録して省略する。
- 取得失敗時の 0 と真の 0 の区別は本 issue では扱わず、「取得失敗の 0 と真の 0 の区別」を別 issue として起票する。取得失敗が Warn ログと 0 表示に劣化して UI から区別できないことは、切り分けの観測結果に依存しない実装の性質のため、起票を Warn ログの観測に条件付けない。
- `CHANGES.md` の `## develop` の `[FIX]` 群に、種別順 (UPDATE → ADD → CHANGE → FIX) を保つ位置へ挿入する形で、WAF ビューの表示に `cloudfront:ListDistributions` 権限が新たに要ることと、権限不足時は Warn ログと 0 表示に劣化することを含めた `[FIX]` エントリを記載し、担当者行を付ける。
- `mise run check` が通過する。

## 未確定論点

- TODO の観測がどの原因 (実装漏れ 2 経路、権限不足、真の 0) によるものかは、記録からは特定できない。切り分けは完了条件の再観測で行う。

## 関連

- docs/issues/0086 (ルール詳細の JSON 表示): 同じ `backend/internal/aws/waf.go` と `waf_test.go` を触る。番号順 (docs/issues/0086 → 本 issue) に実装すれば衝突しない。
- docs/issues/0087 (WAF の列順変更): 本 issue は `associatedCount` の値を、docs/issues/0087 は列の位置を直す。触るファイルが異なるため衝突しない。
- docs/issues/0089, docs/issues/0090 (CloudFront の列変更と Behaviors タブ): `backend/internal/aws/cloudfront.go` を変更するのはそれらで、本 issue は既存の `newCloudFrontClient` と `ListDistributions` を呼ぶだけのため衝突しない。
- docs/issues/0086, docs/issues/0087, docs/issues/0089, docs/issues/0090: `CHANGES.md` の `## develop` は 5 issue 全てが変更する。番号順に直列で実装し、各 issue のエントリを種別順 (UPDATE → ADD → CHANGE → FIX) を保つ位置へ挿入すれば衝突しない。
- docs/issues/pending/0083 (GetSession の設定キャッシュ調査): 本 issue の実装で WAF ビューの表示時に AWS 設定のロード (CloudFront クライアント生成) が 1 回増える。0083 は `ListWAFResources` のクライアント生成を調査対象に挙げているため、0083 の調査が行われる際にはこの追加分も自然に対象へ入る。その事実の記録としてここに書く (0083 自体は pending のまま変更しない)。

## 解決方法

修正方針どおり `backend/internal/aws/waf.go` を変更した。

- `WAFResource` に `ARN string` (JSON タグ `-`) を追加し、`newWAFResource` の引数に `arn string` を `name` の直後に追加した (7 引数 → 8 引数)。
- REGIONAL スコープの関連取得を、`wafRegionalResourceTypes() []waftypes.ResourceType` (`waftypes.ResourceType("").Values()` を経由) が返す全 8 種別それぞれについて `ListResourcesForWebACL` を呼ぶように変更し、`sumResourceARNs(arnLists [][]string) int` で合算するようにした。種別ごとの取得失敗は `handleIgnoredErr` で個別に Warn ログ (属性 `web_acl_arn` / `resource_type`) に落とし、他種別の集計は継続する。タグ取得失敗の Warn ログにも `web_acl_arn` を追加した。
- CLOUDFRONT スコープについて、`ListWAFResources` の CLOUDFRONT 側 goroutine に `applyCloudFrontAssociatedCounts` を追加した。`listWAFACLs` が返す ACL が 0 件なら省略し、それ以外では us-east-1 の CloudFront クライアントで `ListDistributions` をページネーション込みで走査し、`cloudFrontACLCounts(summaries []cftypes.DistributionSummary) map[string]int` で ACL ARN ごとの件数マップを作り、`applyCloudFrontCounts(acls []WAFResource, counts map[string]int)` で適用する。クライアント生成失敗、ページネーション途中の失敗はいずれも Warn ログと全 ACL 0 表示への劣化とし、部分集計は適用しない。

テストは `backend/internal/aws/waf_test.go` に完了条件どおり追加した。

- `TestWAFRegionalResourceTypes`: 既知の 8 値との集合一致 (順序非依存)。
- `TestSumResourceARNs`: 全種別合算、一部 0 件、一部 nil (取得失敗) の 3 ケース。
- `TestCloudFrontACLCounts`: 複数ディストリビューションの合算、`WebACLId` が nil / 空文字の除外、summaries 0 件時の空マップの 3 ケース。
- `TestApplyCloudFrontCounts`: ARN 一致、未一致、GUID キーのみのマップの 3 ケース。
- `TestNewWAFResource`: 全ケースに他フィールドと異なる `arn` を追加し、期待値に `ARN` を追加 (引数取り違え検出方式は維持)。
- `TestWAFResourceJSONHasNoARNKey`: `json.Marshal` 出力に `"arn":` / `"ARN":` が含まれないことを確認。

`mise run check` は全ステージ通過を確認した (frontend: 69 ファイル / 595 テスト全 pass、lint は既存の警告のみで新規のエラー・警告は無し。backend: `go vet` / `staticcheck` / `govulncheck` (0 vulnerabilities) / `golangci-lint` 通過、`go test -race -cover ./...` は全パッケージ ok、`internal/aws` は本 issue のテストを含めて pass)。

CHANGES.md の `## develop` の `[FIX]` 群の先頭 (種別順 UPDATE → ADD → CHANGE → FIX を保つ位置) にエントリを追加し、`cloudfront:ListDistributions` 権限の新規要求と、権限不足時に Warn ログと 0 表示に劣化する挙動を明記した。

### 実環境確認について

この作業環境には AWS 認証情報や実際の AWS アカウントへのアクセスが無く、`aws wafv2` / `aws cloudfront` コマンドを実行できない。そのため、完了条件のうち以下の実環境確認は実施できなかった。

- TODO の観測元の Web ACL の再観測 (スコープ・ID の特定)。
- REGIONAL (ALB 以外の関連付け) / CLOUDFRONT の 2 構成を用意した `associated_count` が 1 以上になることの実データ確認。
- `DistributionSummary.WebACLId` に WAFv2 の ACL ARN が入ることの実データ確認。

修正方針の「未確定論点」に記載したとおり、これらの実データ確認が無い状態では、`DistributionSummary.WebACLId` に関する「みなし」(修正方針の項目、`DistributionConfig.WebACLId` のドキュメント記述を根拠に `DistributionSummary` 側にも同じ区別が適用されるとみなす) は issue が許容するフォールバック (`types/types.go:2355-2372` を根拠とする記録) のまま未検証で残る。TODO の観測がどの原因 (実装漏れ 2 経路のいずれか、権限不足、真の 0) によるものかの切り分けも、実環境アクセスが無いため特定できていない。

コード調査で確定した実装漏れ 2 経路 (REGIONAL の `ResourceType` 省略による ALB 限定化、CLOUDFRONT の関連取得コード自体の欠如) は、コードそのものから明確に確認できる不具合であり、その修正はテストで担保できる範囲において完了している。実環境での再現・再検証はユーザー環境で `mise run backend:run` 後に上記再現手順を実施して確認する必要がある。

取得失敗時の 0 と真の 0 の区別は docs/issues/0091 として別途起票した (完了条件どおり)。

# CloudFront の Drawer に Behaviors タブを追加してビヘイビアの一覧を表示する

Created: 2026-07-27
Completed: 2026-07-27
Model: Claude Sonnet 5

## 背景

TODO.md の次の項目に対応する。

> CloudFront のビヘイビアの一覧を Drawer に足してリストでみれるようにしたい

CloudFront の Drawer のタブは `DRAWER_TABS` (`frontend/src/components/Drawer/Drawer.tsx:31` の定義内、cloudfront のエントリは 41 行) で `['Overview', 'Tags']` と定義されており、キャッシュビヘイビアを表示する場所が無い。
backend の `CloudFrontResource` (`backend/internal/aws/cloudfront.go:13-22`) にもビヘイビアのフィールドが無い。
SDK の `DistributionSummary` (aws-sdk-go-v2 cloudfront v1.61.1 `types/types.go:2557, 2574`) は `CacheBehaviors` (追加ビヘイビア) と `DefaultCacheBehavior` (既定ビヘイビア) を含んでおり、既存の `ListDistributions` (`backend/internal/aws/cloudfront.go:38-50`) のレスポンスから追加の API 呼び出しなしで取得できる。

## 目的

CloudFront の Drawer に Behaviors タブが増え、既定ビヘイビアを含むビヘイビアの一覧を評価順で参照できる。

## 設計判断

- backend は `CloudFrontResource` に `behaviors` (JSON タグ `behaviors` の構造体スライス) を追加し、一覧レスポンスに埋め込む。専用エンドポイントは作らない。
  - 却下案: WAF の Rules (docs/issues/closed/0075) と同じ遅延取得エンドポイント。docs/issues/closed/0075 が遅延取得を採った理由は、一覧を開くだけで全 Web ACL のルールを運ぶことになるレスポンス肥大であり、判断軸はデータ量にある。CloudFront のビヘイビアは次項の概算のとおり 1 ディストリビューションあたり数 KB にとどまるため、同じ軸で埋め込みを選べる。遅延取得にしてもキャッシュ済みの同じレスポンスを切り出すだけで AWS API 呼び出しは減らず、エンドポイントとフックの実装が増えるだけのため却下。
  - サイズについて (いずれも概算): ビヘイビア 1 件はフィールド 6 つで JSON にしておよそ 150 から 250 バイト。件数はディストリビューションあたり既定 1 + 追加 25 件とすると (25 はサービスクォータ「Cache behaviors per distribution」の既定値としてこの概算の前提に置く仮の値。引き上げ可能で、値が数倍でも受容の結論は変わらない)、1 ディストリビューションあたり数 KB にとどまる。一覧レスポンス全体では、100 ディストリビューションが平均 5 件のビヘイビアを持つ場合でおよそ 100 KB、全件が既定値いっぱいまで持つ場合でおよそ 650 KB の増分になり、ローカルホスト間の転送として受容する。
  - backend のリソースキャッシュはプロセス内メモリ (`backend/internal/cache/cache.go`) で永続化されないため、フィールド追加にデータ移行の考慮は不要。
  - `behaviors` は既定 + 追加を `append` で組み立てるため通常 1 件以上になる。`DefaultCacheBehavior` が nil で追加も無い異常系では nil のまま返し、JSON では null にする (`origins` および docs/issues/0089 の `aliases` と同じ扱い)。frontend は `raw.behaviors ?? []` で吸収する。
- ビヘイビア要素の型 `CloudFrontBehavior` は次のフィールドを持つ: `path_pattern` (string)、`target_origin_id` (string)、`viewer_protocol_policy` (string)、`allowed_methods` ([]string)、`compress` (bool)、`is_default` (bool)。
  - 追加ビヘイビアを `CacheBehaviors.Items` の順で先に並べ、既定ビヘイビアを末尾に置く。`CacheBehavior.PathPattern` のドキュメント (aws-sdk-go-v2 cloudfront v1.61.1 `types/types.go:380-398`) は、リクエストのパスがディストリビューションにビヘイビアが列挙された順で比較され、どのパスパターンにも一致しない場合に既定ビヘイビアが使われると記しており、末尾がこの評価順に一致する。
    - 却下案: 既定を先頭に置く。評価順と逆になるため却下。
  - WAF のルール一覧が backend でソートする (`sortWAFRules`、`backend/internal/aws/waf.go:327-333`) のと扱いが異なるのは、前提が異なるためである。WAF はレスポンスの並び順が保証されず明示の `Priority` フィールドを持つためソートするが、CloudFront は一覧の並び順そのものが評価順としてドキュメントに記され、順序フィールドも無いため、配列順を保持する。
  - 評価順は番号列で表す。`DataTable` のソート (`frontend/src/components/DataTable.tsx:85-92` の `toggleSort`) は昇順と降順を往復するだけで未ソート状態に戻せないため、行の並びだけで評価順を表すと、一度列をソートした後に評価順の表示へ戻せなくなる。番号は backend に持たせず、`cloudfrontFromRaw` で配列の添字から 1 始まりの `order` (number) として導出する (既定行は末尾の番号になる)。
    - 却下案: 番号列を設けず行の並びだけで表す。上記のソートの一方向性により却下。
    - 却下案: backend が `order` フィールドを返す。番号は `behaviors` 配列の添字から一意に導出できる派生値で、backend に持たせると配列順と番号の二重表現になり不整合の余地が生まれる。Raw は事実 (配列順) のみを持ち、表示用の派生値は frontend で導く (表示文字列 `Default (*)` を frontend で導くのと同じ基準) ため却下。
  - 既定ビヘイビアは `path_pattern` を空文字、`is_default` を true とする。SDK の `DefaultCacheBehavior` には `PathPattern` フィールドが無く、既定のパスパターンは `*` 固定で変更できないと `CacheBehavior.PathPattern` のドキュメントに記されているため、空文字で固定する。表示文字列 `Default (*)` を backend で焼き込まず、frontend で `is_default` から導く (Raw は事実のみを持つという既存の Raw/Row 分離に従う)。
  - SDK の型に合わせた変換を行う: `AllowedMethods` は `*AllowedMethods`、その `Items` は `[]Method` (string 基底の enum 型)、`ViewerProtocolPolicy` も enum 型、`Compress` は `*bool`。enum は string へ明示的にキャストし、`AllowedMethods` が nil または `Items` が nil なら空スライス、`Compress` が nil なら false にする。
  - 却下案: `cache_policy_id` を含める。値が UUID で、名前解決には `GetCachePolicy` の追加呼び出しと権限が要るため却下。
  - 変換は純関数 `cloudfrontBehaviorsFromSummary` に切り出し、`DefaultCacheBehavior` が nil のケースと `CacheBehaviors` が nil のケースを吸収する (どちらも欠けた場合は nil を返す)。両フィールドは SDK 上 required member のため実レスポンスでは非 nil で、nil の吸収は防御になる。追加ビヘイビアが 0 件の実際の形は `CacheBehaviors` が非 nil で `Items` が空になる。
- frontend は `CloudFrontRaw` に `behaviors: CloudFrontBehaviorRaw[] | null`、`CloudFrontRow` に `behaviors: CloudFrontBehaviorRow[]` を追加し、`cloudfrontFromRaw` (`frontend/src/lib/normalize.ts:562-573`) で `raw.behaviors ?? []` から写す。Raw の snake_case を Row の camelCase (`pathPattern` 等) に変換する。
  - `CloudFrontBehaviorRow` は `DataTable` の行型制約 (`id: string`、行キーは `DataTable.tsx:176` の `key={r.id}`) に合わせて `id` を持つ。`id` には `order` の文字列表現を使う。`order` は配列の添字由来のため構成によらず一意で、行キーが衝突しない。
    - 却下案: 追加行は `pathPattern` を、既定行は `Default (*)` を `id` にする (docs/issues/closed/0075 が WAFRuleRow の `id` にルール名を充てた前例)。PathPattern の一意性は SDK ドキュメント (aws-sdk-go-v2 cloudfront v1.61.1 `types/types.go:380-398` の `CacheBehavior.PathPattern`) に記載が無く、重複した場合に React の行キーが衝突するため却下。
- Drawer は `DRAWER_TABS` の cloudfront を `['Overview', 'Behaviors', 'Tags']` にし、新規コンポーネント `DrawerCloudFrontBehaviors.tsx` を追加する。
  - データは一覧キャッシュからの再引き (`useResources('cloudfront', ...)` の結果を id で `find`) で得る。この再引きは `DrawerSecretEdit` 以来の既存パターン (docs/issues/closed/0066 も踏襲) で、`DrawerWAFRules.tsx:23-24` と `DrawerCacheParameters.tsx` が実装の参照先になる。
    - 却下案: `Drawer` の props でビヘイビアを渡す (`AccountView` が選択行から取り出して `overviewRows` のように受け渡す)。`Drawer` と `ServicePanel` は 15 サービス共通の汎用部品で、props はサービス横断の形に限られている。サービス固有のデータを props に増やすと、サブタブを持つサービスが増えるたびに汎用部品のインターフェースが太る。再引きは同じ TanStack Query キャッシュを読むだけで追加の取得も発生せず、既存サブタブと実装形も揃うため却下。
    - ガードは `DrawerCacheParameters.tsx` と同じ 2 段 (該当行が無くかつ一覧取得中なら `DrawerLoading`、一覧取得済みで該当行が無ければ空表示) にする。キャッシュ未取得と 0 件の区別 (未取得の間は `DrawerLoading` を表示する) は docs/issues/closed/0085 で確立しており、一覧未取得時に空表示を誤って出す不具合 (2a008c7 で ElastiCache の Parameters タブについて修正されたもの) を作り込まない。
    - 再取得で該当 id の行が消えた場合は空表示 (`No behaviors.`) になる。
  - 一覧クエリがエラーのときの表示は一覧ビュー側の責務とする既存方針 (docs/issues/closed/0082, docs/issues/closed/0085) に従い、本 issue では扱わない。構造上もこの分担で穴が無い: `AccountView.tsx:125-126` が一覧の `data ?? []` から選択行を引いて `Drawer` に渡し、`Drawer.tsx:210` は `resource && ...` の形でしか中身を描画しないため、一覧が未取得またはエラーの間は Behaviors タブ自体が描画されない。したがって 2 段ガードの 1 段目 (`DrawerLoading`) は実アプリでは通常到達しない防御で、完了条件のテストはコンポーネント単体の契約として検証する。また、28d042a と 2a008c7 の修正が `DrawerError` の追加まで含んだのはサブタブが自前クエリを持つ場合であり、Behaviors タブは自前クエリを持たないため `DrawerError` を置く余地が無い。
  - 一覧テーブルは `DataTable` を使い、列定義は `cloudfrontBehaviorColumns` として `frontend/src/components/tables/columns.tsx` に置く。列は `order` (ヘッダ `Precedence`)、`pathPattern` (ヘッダ `Path pattern`。既定行は `Default (*)` 表示)、`targetOriginId` (ヘッダ `Target origin`)、`viewerProtocolPolicy` (ヘッダ `Viewer protocol`)、`allowedMethods` (ヘッダ `Allowed methods`。`', '` のカンマ結合、空スライスはダッシュ)、`compress` (ヘッダ `Compress`) の 6 列。列幅は実装時に確定してよく、完了条件は列の集合と順序、合計 100% で判定する (docs/issues/0089 と同じ立て方)。
    - `compress` セルは true で ✓、false でダッシュを表示する (`cacheParameterColumns` と `rdsParameterColumns` の `isModifiable` 列 (`columns.tsx:223-226`, `273-276`) と同じ、リポジトリの bool 列の既存方式)。`filterValue` は与えず、列フィルタは `filterText` の既定動作 (`row[key]` の文字列化、`frontend/src/components/DataTable.tsx:33-39`) による true / false の一致に委ねる。セルの表示 (✓ とダッシュ) と絞り込みの文字列 (true / false) が一致しない点は、既存の bool 列と共通の既知差として受容する。
      - 却下案: セルに true / false の文字列を表示する。表示と絞り込みは一致するが、リポジトリの bool 列の表示方式と分かれるため却下。
    - `pathPattern` 列には `filterValue` を与え、表示と同じ文字列 (既定行は `Default (*)`、追加行はパスパターン) で絞り込めるようにする。既定行の `pathPattern` は空文字のため、既定動作のままでは列フィルタに `Default` と入力したとき既定行が消え、表示と食い違う。ソートは `row[key]` に基づくため `pathPattern` 昇順で空文字の既定行が先頭に来るが、評価順の表示は `order` 列のソートで戻せるため受容する。
    - `allowedMethods` 列は表示のカンマ結合 (`', '`) と既定の絞り込みの文字列化 (スペース無しのカンマ結合) が異なるが、既存の `origins` 列および docs/issues/0089 の `aliases` 列と共通の既知差として受容し、本 issue では扱わない。
  - 行選択は無効とし、`onSelect={() => {}}` と `selectedId={null}` を渡す (`DrawerCacheParameters.tsx:57-58` と同じ。docs/issues/0086 が `DrawerWAFRules.tsx` の行選択を有効にするため、そちらは前例に使わない。`DataTable` の props は両方必須 (`DataTable.tsx:11-12`) のため省略できない)。
  - `behaviors` が 0 件 (既定ビヘイビアが取れない異常系) の場合は `<p className="muted">No behaviors.</p>` を表示し、`DataTable` の定型文 (`No resources match current filters`) を出さない。空表示は各タブがその場に書く既存の慣行 (`DrawerCacheParameters.tsx:47` の `No parameter group.`) に従う。AGENTS.md のディレクトリ図に載っている `DrawerEmpty` は実在しない (`frontend/src` に該当ファイルもシンボルも無い)。
    - 却下案: 空表示の共通コンポーネントを新設する。既存タブの空表示の置き換えを伴う整理になり、本 issue のスコープを超えるため却下。
  - タブ名 `Behaviors` は英語ハードコードとし i18n に載せない (docs/issues/closed/0066 の方針)。
- ビヘイビアが既定 1 件のみの場合もテーブルに 1 行表示する (空表示にしない)。
- 追加の AWS API 呼び出しと権限は不要。
- CLI の変更は不要 (CLI の CloudFront 表 (`backend/internal/aws/torow.go:66-68`) にビヘイビア列は要望されていない)。

## 完了条件

- ビヘイビア変換の純関数 `cloudfrontBehaviorsFromSummary` のテーブル駆動テストが `backend/internal/aws/cloudfront_test.go` にあり、次のケースを含む: 既定 + 追加複数件の順序 (追加が Items 順で先、既定が末尾)、`CacheBehaviors` が nil で既定のみ 1 件、`CacheBehaviors` が非 nil で `Items` が空 (required member を持つ実レスポンスで追加ビヘイビアが 0 件の形) でも既定のみ 1 件、`DefaultCacheBehavior` が nil で追加のみが並ぶ、どちらも nil のとき nil を返す (空スライスと区別する)、`AllowedMethods` が nil で空スライス、`Compress` が nil で false、enum 値が string に写る、`is_default` フラグ。
- `CloudFrontResource` の `json.Marshal` 出力に `behaviors` キーが現れることと、0 件 (nil) のとき値が `null` になることと、値のある要素に `path_pattern`, `target_origin_id`, `viewer_protocol_policy`, `allowed_methods`, `compress`, `is_default` の 6 キーが現れることを検証するテストがある。
- `cloudfrontFromSummary` が `behaviors` を `cloudfrontBehaviorsFromSummary` の出力どおりに埋めることのフィールド単位アサーションが `backend/internal/aws/cloudfront_test.go` に追加されている。既存の `TestCloudfrontFromSummary` (`cloudfront_test.go:10`) のテーブルは state 正規化専用の構造 (`{name, status, want string}`) のため変更せず、テスト関数を新設する (docs/issues/0089 の `aliases` の検証と同じ方式)。
- `cloudfrontFromRaw` のテスト (`frontend/src/lib/normalize.test.ts`。describe は docs/issues/0089 で新設した `cloudfrontFromRaw` のものに追加する (docs/issues/0089 の完了後に着手するため、未新設の分岐は生じない)) に、`behaviors` が null のとき空配列になるケースと、snake_case から camelCase に写るケースと、`order` が配列の添字から 1 始まりで導出されるケースと、`id` が `order` の文字列表現になるケースがある。
- `Drawer.test.tsx` の `describe('Drawer のタブ構成')` に、cloudfront のタブが `['Overview', 'Behaviors', 'Tags']` であるケースが追加されている。
- `Drawer.test.tsx` に、cloudfront のリソースを選択して Behaviors タブを選ぶとビヘイビアの行の内容 (与えたデータの `path_pattern` の値) が表示されるケースが追加されている (`Drawer.test.tsx:131` の RDS パラメータタブの describe と同じ、タブを選んで中身のテキストを確認する方式)。タブ定義だけ増えて分岐が未実装のまま通過することを防ぐ配線の検証のため、ビヘイビアを持つデータを与えて行の表示まで確認する。空表示の `No behaviors.` はデータが配線されていなくても表示され得るため、確認対象にしない。
- `DrawerCloudFrontBehaviors` のテストがあり、次のケースを含む: 既定行が `Default (*)` と表示され末尾に置かれる、既定のみのデータで 1 行が表示される (空表示にならない)、該当行が無くかつ一覧取得中のとき `Loading…` が表示される (`DrawerCacheParameters.test.tsx:69` と同じ文字列アサーション)、一覧取得済みで該当 id の行が無いとき `No behaviors.` が表示され `Loading…` にならない、`behaviors` が空配列のとき `No behaviors.` が表示され `No resources match current filters` が表示されない。
- `cloudfrontBehaviorColumns` の `key` の並びが `order`, `pathPattern`, `targetOriginId`, `viewerProtocolPolicy`, `allowedMethods`, `compress` の 6 列でこの順に完全一致することと、対応する `header` が `Precedence`, `Path pattern`, `Target origin`, `Viewer protocol`, `Allowed methods`, `Compress` であることと、`width` の合計 100% を `frontend/src/components/tables/columns.test.tsx` で検証している (個々の幅の値は検証しない)。
- `allowedMethods` セルが複数値を `', '` (カンマ + 半角スペース) の結合で表示し、空スライスでダッシュになることと、`compress` セルが true で ✓、false でダッシュになることを `columns.test.tsx` で検証している (`columns.test.tsx:30-44` の既存方式に合わせる)。
- `compress` 列の絞り込み (`filterText` の既定動作による true / false の文字列一致) は `DataTable` 既定の挙動のため、専用のテストは書かない。`pathPattern` 列の `filterValue` は本 issue で書くコードのため、既定行で `Default (*)`、追加行でパスパターンを返すことを `columns.test.tsx` で検証する。
- ビヘイビアの詳細 (キャッシュポリシー名、関数関連付け等) の表示と、一覧クエリがエラーのときの Drawer 側の表示は本 issue では扱わない。
- `CHANGES.md` の `## develop` の `[ADD]` 群に、種別順 (UPDATE → ADD → CHANGE → FIX) を保つ位置へ挿入する形で `[ADD]` エントリと担当者行が記載されている。
- `mise run check` が通過する。

## 関連

- docs/issues/0089 (CloudFront の列変更): 同じ `backend/internal/aws/cloudfront.go`、`frontend/src/types/aws.ts` の CloudFront の型定義、`frontend/src/lib/normalize.ts`、`frontend/src/components/tables/columns.tsx` を触るため同時進行はできない。docs/issues/0089 の完了後に着手する。
- docs/issues/0087 (WAF の列順変更): 同じ `frontend/src/components/tables/columns.tsx` と `columns.test.tsx` を触る。対象の配列が異なるため、番号順に実装すれば衝突しない。
- docs/issues/0086 (WAF ルール詳細): 同じ `frontend/src/types/aws.ts`、`frontend/src/lib/normalize.ts`、`normalize.test.ts` を触る。編集箇所は型と関数の単位で独立しており、番号順に実装すれば衝突しない。
- docs/issues/0088 (WAF の Associated 修正): `backend/internal/aws/cloudfront.go` を変更せず既存関数を呼ぶだけのため、本 issue と衝突しない。
- docs/issues/0086, docs/issues/0087, docs/issues/0088, docs/issues/0089: `CHANGES.md` の `## develop` は 5 issue 全てが変更する。番号順に直列で実装し、各 issue のエントリを種別順 (UPDATE → ADD → CHANGE → FIX) を保つ位置へ挿入すれば衝突しない。
- docs/issues/closed/0075 (WAF の Rules タブ): Drawer サブタブの前例。遅延取得を採らなかった理由の対比として参照。
- docs/issues/closed/0066, docs/issues/closed/0085 (一覧キャッシュ再引きと DrawerLoading): 実装テンプレート。

## 解決方法

設計判断のとおりに実装した。

- backend: `backend/internal/aws/cloudfront.go` に `CloudFrontBehavior` (`AllowedMethods []string` 含む 6 フィールド) と純関数 `cloudfrontBehaviorsFromSummary` を追加し、`CloudFrontResource` に `Behaviors []CloudFrontBehavior` (JSON タグ `behaviors`) を追加した。追加ビヘイビアを `CacheBehaviors.Items` の順で先に並べ、既定ビヘイビアを末尾に置く。`DefaultCacheBehavior`・`CacheBehaviors` の双方が nil のときのみ nil を返し、それ以外は必ず 1 件以上を含む。`AllowedMethods` は nil または `Items` が nil のとき空スライス (nil ではない) にし、`Compress` は nil のとき false にする。
- backend テスト: `cloudfront_test.go` に `TestCloudfrontBehaviorsFromSummary` (順序、`CacheBehaviors` nil、`CacheBehaviors.Items` 空、`DefaultCacheBehavior` nil、両方 nil の 5 ケース)、`TestCloudfrontFromSummaryBehaviors` (`cloudfrontFromSummary(d).Behaviors` と `cloudfrontBehaviorsFromSummary(d)` のフィールド単位比較。`CloudFrontBehavior` はスライスを含み `!=` で比較できないため個別フィールド比較にした)、`TestCloudFrontResourceJSONBehaviors` (`json.Marshal` で 0 件時 `"behaviors":null`、値ありで 6 キー全て出ることを検証) を追加した。既存の `TestCloudfrontFromSummary` は変更していない。
- frontend の型: `frontend/src/types/aws.ts` に `CloudFrontBehaviorRaw` (snake_case) と `CloudFrontBehaviorRow` (camelCase、加えて `id`/`order` を持つ) を追加し、`CloudFrontRaw.behaviors: CloudFrontBehaviorRaw[] | null`、`CloudFrontRow.behaviors: CloudFrontBehaviorRow[]` を追加した。
- normalize: `frontend/src/lib/normalize.ts` の `cloudfrontFromRaw` で `raw.behaviors ?? []` を `map` し、配列の添字から 1 始まりの `order` と、その文字列表現の `id` を導出しつつ snake_case から camelCase へ変換した。`normalize.test.ts` に null→空配列、snake_case→camelCase 変換、order/id 導出の 3 ケースを追加した。
- 列定義: `frontend/src/components/tables/columns.tsx` に `cloudfrontBehaviorColumns` (`order`, `pathPattern`, `targetOriginId`, `viewerProtocolPolicy`, `allowedMethods`, `compress` の 6 列、幅合計 100%) を追加した。`pathPattern` は既定行で `Default (*)` を表示・フィルタするための `filterValue` を持つ。`compress` は true で緑の ✓、false でダッシュ (既存の `isModifiable` 列と同じ方式)。`allowedMethods` はカンマ + 半角スペース結合、空はダッシュ。`columns.test.tsx` に列幅合計・allowedMethods 結合とダッシュ・compress の ✓ とダッシュ・pathPattern の filterValue (既定/追加) の 6 テストを追加した。
- Drawer: 新規コンポーネント `frontend/src/components/Drawer/DrawerCloudFrontBehaviors.tsx` を追加した。`useResources('cloudfront', ...)` で一覧キャッシュを再引きし id で `find` する (専用エンドポイントは追加していない)。該当行が無くかつ一覧取得中は `DrawerLoading`、一覧取得済みで該当行の `behaviors` が 0 件なら `<p className="muted">No behaviors.</p>`、それ以外は `DataTable` で表示する 2 段ガードにした。行選択は無効 (`onSelect={() => {}}`, `selectedId={null}`)。`Drawer.tsx` の `DRAWER_TABS.cloudfront` を `['Overview', 'Behaviors', 'Tags']` にし、`Behaviors` タブの描画を配線した。
- Drawer テスト: `DrawerCloudFrontBehaviors.test.tsx` を新規作成し、ローディング表示・一覧取得済みで該当無し・`behaviors` 空配列・既定のみ 1 件表示・既定行が `Default (*)` で末尾に並ぶことの 5 ケースを検証した。`Drawer.test.tsx` に、cloudfront のタブ構成が `['Overview', 'Behaviors', 'Tags']` になることと、Behaviors タブをクリックするとビヘイビアの `path_pattern` が表示されることの 2 ケースを追加した。
- `mise run check` (backend の fmt/vet/staticcheck/govulncheck/golangci-lint/test -race、frontend の prettier/eslint/tsc/vitest) が全て通過することを確認した (frontend: 70 ファイル 621 テスト、backend: 全パッケージ pass、govulncheck は既知の間接依存 1 件のみで直接影響コードは 0 件)。
- `CHANGES.md` の `## develop` の `[ADD]` 群冒頭 (最初の既存 `[ADD]` エントリの直前、種別順 UPDATE→ADD→CHANGE→FIX を保つ位置) にエントリを追加した。

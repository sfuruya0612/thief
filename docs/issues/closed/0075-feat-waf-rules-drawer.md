# WAF の Web ACL のルール一覧を Drawer の Rules タブで参照できるようにする

Created: 2026-07-25
Completed: 2026-07-26
Model: Claude Fable 5

## 背景

TODO.md の次の項目に対応する。

> WAF のルールを Drawer でみれるようにしたい

backend の `listWAFACLs` (`backend/internal/aws/waf.go`) は Web ACL ごとに `GetWebACL` を呼んでいるが、使っているのはルール件数 (`len(acl.WebACL.Rules)`) だけで、ルールの中身 (名前、優先度、アクション) は捨てている。
レスポンスの `WAFResource` が持つのは `id`, `name`, `state`, `scope`, `rule_count`, `associated_count`, `tags`, `cost_monthly` であり、ARN は含まれず、ルール一覧を返すエンドポイントも存在しない。

frontend の WAF の Drawer タブは `frontend/src/components/Drawer/Drawer.tsx` の `DRAWER_TABS` で `waf: ['Overview', 'Tags']` と定義されており、ルールを表示するタブがない。
利用者はルール件数 (`wafColumns` の `Rules` 列と `wafOverviewRows` の `Rules` 行) までしか確認できず、どのルールがどのアクションで動いているかを見るには AWS コンソールを開く必要がある。

なお、WAF の一覧に Description を追加する要望は docs/issues/0074 で扱う。

## 目的

WAF の一覧で Web ACL を選択すると、Drawer の `Rules` タブでその Web ACL のルール一覧 (名前、優先度、アクション、種別) が優先度順に参照できる。

## 設計判断

- backend に `GET /api/aws/profiles/{profile}/waf/rules?region=<region>&scope=<scope>&id=<id>&name=<name>` を新設する。
  - `GetWebACL` (aws-sdk-go-v2 wafv2 v1.74.1) は name + id + scope の組、または ARN で Web ACL を引ける (入力の必須フィールドはない)。一覧が既に持つ id / name / scope の 3 つをクエリパラメータで受け取る方式を採る。3 つ組はキャッシュキー (`cacheKey("waf-rules", profile, region, scope, id)`) ともそのまま対応する。
  - 却下案 1: ARN 方式 (`?arn=`)。`WAFResource` と `WAFRow` に ARN が無く、フィールド追加が必要になる。ARN からスコープ (クライアントのリージョン選択に必要) を導出するパースも要り、3 つ組方式より変更が増えるため却下。
  - 却下案 2: 一覧レスポンス (`WAFResource`) にルール一覧を埋め込む。一覧を開くだけで全 Web ACL のルールを運ぶことになりレスポンスが肥大するため、Drawer で選択したときだけ取得する遅延取得 (RDS Parameters、CFN Resources と同じ方針) を採る。
- `scope` / `id` / `name` のいずれかが欠けたリクエスト、および `scope` が `REGIONAL` / `CLOUDFRONT` 以外のリクエストには `writeBadRequest` で 400 を返す。`region` は既存ハンドラと同じく `profileAndRegion` がサーバ既定リージョンへフォールバックするため 400 の対象にしない。
  - 必須パラメータ検証は既存ハンドラと同じ直書きとし、docs/issues/0078 の `requireQueryParam` 抽出で他と一緒に置き換えられる。`scope` の値検証は presence 検証ではないためヘルパーの対象外とし、エラーメッセージは `<name> query parameter is required` とは別の文言にする (docs/issues/0078 の grep 完了条件を壊さないため)。
- ハンドラは `serveCached` + 上記キャッシュキーの既存パターンに載せる。`name` は id が scope 内で一意なためキーに含めない。CLOUDFRONT の Web ACL は閲覧リージョンごとに別キーでキャッシュされるが、値は同一で実害がないため許容する。
- スコープからクライアント用リージョンを決めるロジックは純関数 `wafClientRegion(scope, region) string` に切り出す (CLOUDFRONT は us-east-1、`ListWAFResources` と同じ扱い)。wafv2 クライアントのインターフェース化は行わない (docs/issues/closed/0067 の方針) ため、この純関数がテストの対象になる。
- AWS 層に `WAFRule` 構造体、`ListWAFRules(ctx, profile, region, scope, name, id)`、SDK の `Rule` から変換する純関数 `newWAFRule`、`priority` 昇順に整列する純関数 `sortWAFRules` を追加する (`GetWebACL` のレスポンス配列の順序は SDK に文書化されていないため明示的に整列する。`Priority` は Web ACL 内で一意であることが文書化されており、整列は決定的になる)。`WAFRule` のフィールドは次の 4 つとする。
  - `name`: ルール名。
  - `priority`: 優先度。
  - `action`: `Rule.Action` (`RuleAction` 構造体) の非 nil フィールド名 (Allow / Block / Count / Captcha / Challenge / Monetize)。ルールグループ参照で `OverrideAction` が設定されている場合は `Override: None` / `Override: Count` の形式で返す。非 nil 判定は手書き分岐のため、SDK 更新で増えたフィールドやどちらも nil の場合は空文字に落ちる。
  - `statement`: ルールの種別。SDK の `Statement` は union interface ではなく 16 個のポインタフィールドを持つ構造体のため、非 nil のフィールドを判定して種別名にマップする。`ManagedRuleGroupStatement` は `VendorName/Name` 形式、`RuleGroupReferenceStatement` は名前フィールドを持たないため ARN の末尾セグメントを切り出して表示する。その他はフィールド名から導出した短い種別名 (例: `ByteMatchStatement` は `ByteMatch`)。`AndStatement` / `OrStatement` / `NotStatement` は入れ子を展開せず `AND` / `OR` / `NOT` とだけ表示する。どのフィールドも非 nil でない場合は `Unknown` を返す (SDK 更新で種別が増えた場合の既定値)。
  - 却下案: `Statement` の中身を丸ごと JSON で返す。入れ子の巨大な構造体になりテーブル表示に向かないため、種別名への要約にとどめる。マッチ条件の詳細表示は本 issue では扱わない。
- エラーは既存の `writeAWSError` の経路に従う。AccessDenied の誤マップは docs/issues/0073 で先に修正される。
- frontend は次の構成とする。
  - `Drawer` に渡る `resource` は `BaseRow` で `scope` を持たないため、`DrawerRDSParameters` と同様に `useResources('waf', ...)` の一覧キャッシュから該当行を引き直して `scope` を得る。呼び出しは `Drawer` の `region` prop をそのまま渡す (一覧と同一 queryKey になり追加リクエストが出ない)。引き直しは `id` で行う (REGIONAL と CLOUDFRONT に同名の Web ACL が存在しうるため `name` では引かない)。id と scope の対応は Web ACL の生存中不変のため、キャッシュが stale でも誤らない。一覧キャッシュに行がまだ無い間は query を `enabled: false` にし、ローディング表示を出す (空表示ともエラー表示とも区別する)。
  - `types/aws.ts` に `WAFRuleRaw` / `WAFRuleRow`、`lib/normalize.ts` に `wafRuleFromRaw`、`api/endpoints.ts` に `getWAFRules`、`api/queries.ts` に `useWAFRules` (queryKey `['aws', 'waf-rules', profile, region, scope, id]`、`staleTime: 60_000`) を追加する。
  - エラー表示コンポーネント `DrawerError` (`<DrawerError error={...} />`) を共有モジュール `components/Drawer/drawerError.tsx` に新設し、`DrawerValueEditor.tsx` のローカル関数 `errorText` (エラーをメッセージ文字列へ変換) をこれで置き換える。`DrawerError` は `ApiError` ならステータスとコードとメッセージを、それ以外は文字列化した内容を表示する。この置き換えで `DrawerValueEditor` の `error` / `saveError` の表示は、現行のメッセージのみからステータスとコードを含む形に変わる (Secret / SSM の編集タブに及ぶ意図的な表示変更であり、`DrawerValueEditor.test.tsx` の期待値を更新する)。ラベルは英語ハードコードとし i18n に載せない。根拠は `DrawerError` が表示する本文が AWS 由来の英語メッセージであることに置く (docs/issues/closed/0066 のタブ名方針は参照専用タブに限る宣言で、適用先の `DrawerValueEditor` は `useTranslation('drawerAws')` で i18n 済みの Edit タブのため援用しない)。なお AGENTS.md frontend 節の「i18n ライブラリは未導入」という記述は実装 (react-i18next 導入済み) と食い違っており、本 issue で更新する (完了条件参照)。SSO 期限切れの専用表示はサブタブでは行わない (一覧側の `SSOExpiredBanner` が既に担う)。サブタブのエラー表示の共有部品はここが導入元になり、docs/issues/0076 と docs/issues/0082 が同じ部品を使う。
    - 却下案 1: 既存の `ErrorBanner` (`components/ErrorBanner.tsx`) の再利用。表示する情報 (ステータス、コード、メッセージ) は同等だが、`.error-banner` はビュー全体幅のアイコン付きバナーで Drawer のセクション内には過大であり、`error instanceof Error` でない値に `null` を返すため取得失敗が無表示に戻る経路が残る。このため部品は Drawer 専用に分け、表示する情報は `ErrorBanner` と揃える。重複と見なさない判断は docs/issues/0078 の台帳にも記載する。
    - 却下案 2: `errorText` を文字列関数のまま共有化する。表示のレイアウトと色の指定が各サブタブに分散し、docs/issues/0082 の 10 箇所への適用で同じ JSX が複製されるため、コンポーネントとして共有する。
  - サブタブのエラー表示の規則を本 issue で確定する: query の `data` が無いときはエラー表示のみを出し、`data` があるときは既存表示の上部にエラーを出して表示中のデータを消さない。docs/issues/0076 と docs/issues/0082 はこの規則に従う。
  - `components/Drawer/DrawerWAFRules.tsx` を新設する (`DrawerCFNResources.tsx` と同じ hook + `DrawerLoading` + `DataTable` 構成)。取得エラー時は `DrawerError` で、0 件の空表示と区別できるエラーメッセージをタブ内に表示する (`DrawerValueEditor` と `DrawerObjectBrowser` が `error` を表示する既存前例に倣う)。列定義は `components/tables/columns.tsx` に `wafRuleColumns` (name / priority / action / statement の 4 列) として追加する。
  - `Drawer.tsx` の `DRAWER_TABS` を `waf: ['Overview', 'Rules', 'Tags']` に変更し、`dbody` に分岐を追加する。タブ名は英語ハードコードとし i18n に載せない (docs/issues/closed/0066 で確立した方針)。新設コンポーネントは既存サブタブと同様 `Drawer.tsx` から直接 import し、`Drawer/index.ts` には載せない。
- CLI には WAF コマンドが存在しないため、CLI 側の追加は本 issue では扱わない。
- 追加の権限は不要。`wafv2:GetWebACL` は一覧取得で既に使用している。

## 完了条件

- `GET /api/aws/profiles/{profile}/waf/rules` が、Web ACL のルール一覧を `priority` 昇順で JSON (`name`, `priority`, `action`, `statement`) として返す。ルート登録とハンドラ配線をレビューで確認し、実 AWS で叩いた結果を本 issue に記録する。実 AWS を使えない場合はその旨と代替根拠 (テストとレビュー) を記録する (ローカル環境の floci シードは wafv2 を含まない)。
- `scope` / `id` / `name` のいずれかが欠けたリクエスト、および `scope` が `REGIONAL` / `CLOUDFRONT` 以外のリクエストに 400 を返すことを、ルータ (`http.ServeMux`) 経由でリクエストを流すハンドラのテストで検証している。`region` 未指定はサーバ既定リージョンで処理される (既存ハンドラと同じ)。
- `wafClientRegion` のテストがあり、CLOUDFRONT スコープで us-east-1 を返すことを検証している。
- `newWAFRule` と `sortWAFRules` にテーブル駆動テストがある。`Action` 設定ルール、`OverrideAction` 設定ルール (マネージド / 非マネージドのルールグループ参照)、どの `Statement` フィールドも非 nil でないルール (Unknown になる) のケースを含む。`newWAFRule` の `Statement` 判定は 16 フィールドすべてに分岐があり、テストは 16 フィールドそれぞれを非 nil にしたケースで期待する種別名を固定している (種別名の取り違えをテストで検出するため。ケースは機械的に列挙できる)。
- `wafRuleFromRaw` にユニットテストがある。
- `DrawerWAFRules.test.tsx` があり、4 列の表示、ルール 0 件の空表示、取得エラー時のエラーメッセージ表示、一覧キャッシュに行が無い間のローディング表示の 4 ケースを検証している。
- `components/Drawer/drawerError.tsx` が `DrawerError` を export し、`DrawerValueEditor` と `DrawerWAFRules` の両方が使う。使用箇所は grep (`<DrawerError`) で確認できる。
- `DrawerValueEditor.test.tsx` が更新され、`error` と `saveError` の表示にステータスとコードとメッセージが含まれることを検証している。
- ルールのマッチ条件 (`Statement` の中身) の詳細表示、CLI へのコマンド追加、AccessDenied の誤マップ修正 (docs/issues/0073)、他サブタブへのエラー表示の展開 (docs/issues/0082) は本 issue では扱わない。
- AGENTS.md frontend 節の「多言語対応: 現状は日本語 UI 文字列のハードコードのみ。i18n ライブラリは未導入」という記述が、実装 (react-i18next 導入済み、`src/i18n/locales/ja/` に 14 ネームスペース) に合わせて更新されている。
- `CHANGES.md` の `## develop` に `[ADD]` エントリと担当者行が記載されている。
- `mise run check` が通過する。

## 関連

- docs/issues/0073 (AccessDenied の誤マップ修正): 本 issue の新設経路で顕在化しやすい問題の引受先。番号順で先に修正される。
- docs/issues/0074 (WAF 一覧への Description 追加): 本 issue と同じ `backend/internal/aws/waf.go` と `frontend/src/components/tables/columns.tsx` を触る。番号順 (docs/issues/0074 → 本 issue) に実装すれば衝突しない。
- docs/issues/0076 (RDS の Parameters タブ分割): 本 issue が導入するサブタブのエラー表示部品 (`DrawerError`) と表示規則を使う。
- docs/issues/0078 (重複の共通化): 本 issue が追加する必須パラメータ検証の直書き 3 箇所も、`requireQueryParam` への置き換え対象になる。
- docs/issues/0080 (frontend のキャッシュ設定): 本 issue が追加する `useWAFRules` が候補 2 (staleTime の既定化) の整理対象に含まれる。
- docs/issues/0081 (backend の並列化): `listWAFACLs` の構造を書き換える。番号順で本 issue の後になる。
- docs/issues/0082 (サブタブのエラー表示の横断修正): 既存サブタブへの展開の引受先。

## 解決方法

### backend

- `backend/internal/aws/waf.go` に `WAFRule` (`name` / `priority` / `action` / `statement`)、`ListWAFRules(ctx, profile, region, scope, name, id)`、純関数 `wafClientRegion` (CLOUDFRONT は us-east-1)、`newWAFRule`、`wafRuleAction` (Action の非 nil フィールド名、OverrideAction は `Override: None` / `Override: Count`、どちらも nil は空文字)、`wafRuleStatement` (16 フィールドの非 nil 判定。ManagedRuleGroup は `VendorName/Name`、RuleGroupReference は ARN 末尾セグメント、And/Or/Not は展開せず `AND`/`OR`/`NOT`、該当なしは `Unknown`)、`sortWAFRules` (priority 昇順) を追加した。
- `backend/internal/api/handlers_aws.go` に `handleWAFRules` を追加した。`scope` / `id` / `name` の欠落は `writeBadRequest` で 400 (`<name> query parameter is required`)、`scope` の値検証は別文言 (`scope must be REGIONAL or CLOUDFRONT`) とし docs/issues/0078 の grep 条件を壊さない。`serveCached` + `cacheKey("waf-rules", profile, region, scope, id)` に載せた (`name` は id が scope 内で一意なためキーに含めない)。
- `backend/internal/api/routes.go` に `GET /api/aws/profiles/{profile}/waf/rules` を登録した。
- テスト: `waf_test.go` に `TestWAFClientRegion`、`TestNewWAFRule` (Action 6 種、Override 2 種、Statement 16 フィールド全分岐、空 Action / nil Statement)、`TestSortWAFRules`。`handlers_aws_test.go` に `TestHandleWAFRulesValidation` (ServeMux 経由で 4 ケースの 400 とメッセージを検証)。

### frontend

- `types/aws.ts` に `WAFRuleRaw` / `WAFRuleRow` (DataTable の行キーとして `id` にルール名を使う)、`lib/normalize.ts` に `wafRuleFromRaw`、`api/endpoints.ts` に `getWAFRules`、`api/queries.ts` に `useWAFRules` (queryKey `['aws', 'waf-rules', profile, region, scope, id]`、`staleTime: 60_000`、scope 確定まで `enabled: false`) を追加した。
- `components/Drawer/drawerError.tsx` に共有部品 `DrawerError` を新設し (ApiError はステータス・コード・メッセージ、それ以外は文字列化。英語ハードコード)、`DrawerValueEditor.tsx` のローカル `errorText` を置き換えた。サブタブのエラー表示規則 (data 無しはエラーのみ、data 有りは既存表示の上部) を部品のコメントに明記した。
- `components/Drawer/DrawerWAFRules.tsx` を新設した。`useResources('waf', ...)` の一覧キャッシュから `id` で該当行を引き直して `scope` を得て、行が無い間は `DrawerLoading` を表示する。列定義は `columns.tsx` の `wafRuleColumns` (name / priority / action / statement)。`Drawer.tsx` の `DRAWER_TABS` を `waf: ['Overview', 'Rules', 'Tags']` に変更し dbody に分岐を追加した。
- テスト: `DrawerWAFRules.test.tsx` (4 列表示、0 件の空表示、取得エラー表示、キャッシュ未着時のローディングと取得未発火の 4 ケース)、`normalize.test.ts` に `wafRuleFromRaw`、`DrawerValueEditor.test.tsx` に ApiError の取得/保存エラー表示の 2 ケースを追加した。

### ドキュメント

- AGENTS.md frontend 節の多言語対応の記述を実装 (react-i18next 導入済み、14 ネームスペース) に合わせて更新した。
- CHANGES.md の `## develop` に `[ADD]` エントリを記載した。

### 実 AWS での確認について

実 AWS 環境は本作業で利用できなかった (ローカルの floci シードも wafv2 を含まない)。代替根拠として次を記録する。

- ルート登録 → `handleWAFRules` → `ListWAFRules` の配線をレビューで確認した (`routes.go` の登録、ハンドラの `serveCached` 呼び出し、AWS 層の `GetWebACL` 呼び出しと引数の対応)。
- ルータ経由のハンドラ検証テスト、`wafClientRegion` / `newWAFRule` (Statement 16 フィールド全分岐) / `sortWAFRules` / `wafRuleFromRaw` の純関数テスト、`DrawerWAFRules` のコンポーネントテストがすべて通過している (`mise run check` 通過)。

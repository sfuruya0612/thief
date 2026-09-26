# Drawer のサブタブが取得エラーを空表示にして 0 件と区別できないのを修正する

Created: 2026-07-25
Completed: 2026-07-26
Model: Claude Fable 5

## 症状

Drawer のサブタブ (CFN の Events / Resources / Tags、ECR の Images、ECS の Services / Tasks、ELB の Listeners / Targets、DynamoDB の Items、ElastiCache の Parameters) で、データ取得がエラーになっても空のテーブルが表示され、「対象が 0 件」と区別できない。
docs/issues/0076 のレビューで見つかった問題を切り出したもの (TODO.md 由来ではない)。
docs/issues/closed/0071 が設計判断として記載した「取得失敗はエラーを表示する」という方針は、RDS 側でも未実装のまま (docs/issues/0076 が引き受ける) であり、他のサブタブにも適用されていない。

## 再現手順

1. SSO トークンが期限切れ、または対象 API の権限が無い状態にする。
2. thief で CloudFormation の Stack を選択し、Drawer の `Resources` タブを開く。
3. 期待: 取得に失敗したことが分かるエラー表示。実際: 空のテーブルが表示され、リソースが 0 件の Stack と見分けが付かない。

## 原因

対象のサブタブコンポーネントが、query の `error` を捨てて `data` と `isLoading` だけを使っている。
`DrawerCFNResources` の冒頭を例に挙げる (以降の描画処理は省略)。

```tsx
export function DrawerCFNResources({ profile, region, stack }: DrawerCFNResourcesProps) {
  const { data, isLoading } = useCFNStackResources(profile, region, stack);
```

`error` を参照する分岐が無く、失敗時は rows が空のままテーブルが描画される。

`error` を表示しているのは `DrawerObjectBrowser` / `DrawerObjectPreview` / `DrawerValueEditor` (とそのアダプタの `DrawerSecretEdit` / `DrawerSSMEdit`) のみ。
捨てているのは `DrawerCacheParameters` / `DrawerCFNEvents` / `DrawerCFNResources` / `DrawerCFNTags` / `DrawerDynamoItems` / `DrawerECRImages` / `DrawerECSServices` / `DrawerECSTasks` / `DrawerELBListeners` / `DrawerELBTargets` の 10 件 (`DrawerRDSParameters` も同様だが docs/issues/0076 のタブ分割で回復されるため対象外)。
`DrawerCFNOverviewExtra` (Overview タブの補助行) と `DrawerTerminal` (接続対象セレクタの選択肢) も query のエラーを使っていないが、一覧テーブルの空表示とは表示の設計が別になり、本 issue の症状 (0 件と区別できない) に当たらないため対象外とする。
対応する場合は別 issue を起票する。

## 修正方針

- docs/issues/0075 で導入される `DrawerError` 部品 (`components/Drawer/drawerError.tsx`) を上記 10 件に適用し、docs/issues/0075 で確定するエラー表示規則 (query の `data` が無いときはエラー表示のみ、`data` があるときは既存表示の上部にエラーを出して表示中のデータを消さない) に従って、取得エラー時に空表示と区別できるエラーメッセージを表示する。
- 変更はエラー表示の分岐の追加のみで、正常系の表示は変えない。既存パターンの機械適用であり 10 件を 1 issue で扱う。
- 複数の query を持つタブ (`DrawerELBListeners` のリスナーとルール、`DrawerELBTargets` のターゲットグループとヘルス、`DrawerDynamoItems` のスキーマと検索結果) では、query ごとにエラーを表示する。`DrawerDynamoItems` の検索フォームと入力値は、上記の表示規則により失われない。
- `DrawerCacheParameters` が持つ 2 本の query のうち 1 本目は `useResources` の一覧キャッシュ参照 (`data` のみ使用) であり、一覧キャッシュのエラーは一覧ビュー側 (`SSOExpiredBanner` / `ErrorBanner`) が表示するため対象外とする。本 issue の対象はパラメータ取得の query 1 本になる。
- 追加の API 呼び出し、権限、backend の変更は不要。

## 完了条件

- 上記 10 件のサブタブすべてが `DrawerError` 部品を使い、取得エラー時にエラーメッセージを表示する。対象 10 ファイルのそれぞれに `<DrawerError` があることを grep で確認できる (docs/issues/0075 / docs/issues/0076 の使用箇所が別にあるため、件数の合計ではなくファイル単位で判定する)。
- 代表 2 件のテストがある: `DrawerCFNEvents.test.tsx` (既存) に取得エラーのケースを追加し、`DrawerELBListeners.test.tsx` を新設して複数 query (リスナーとルール) それぞれの取得エラーが表示され 0 件の空表示と区別できることを検証する。
- 正常系 (データ表示、0 件) の表示が変わらないことを、既存テストがある `DrawerCFNEvents` / `DrawerECSTasks` の 2 件はテストの通過で確認する。残る 8 件には既存テストが無いため、変更がエラー分岐の追加のみであることのコードレビューで確認し、結果を本 issue に記録する。
- エラー表示の文言とラベルは docs/issues/0075 の `DrawerError` の仕様 (英語ハードコード、i18n に載せない) に従い、本 issue で新しい文言を定義しない。
- ローディング中や未取得と 0 件の区別 (`enabled` ゲートの整理)、一覧キャッシュ参照 (`useResources`) のエラー表示、`DrawerCFNOverviewExtra` / `DrawerTerminal` / `DrawerRDSParameters` の対応は本 issue では扱わない。
- ElastiCache の Parameters タブ (`DrawerCacheParameters`) に残る未取得と 0 件の区別の問題 (docs/issues/0076 が RDS 側で直す症状と同型) も本 issue では扱わず、次バッチで bug issue として起票する。
- `CHANGES.md` の `## develop` に `[FIX]` エントリと担当者行が記載されている。
- `mise run check` が通過する。

## 解決方法

対象 10 ファイルすべてに `DrawerError` (`components/Drawer/drawerError.tsx`) を適用した。表示規則は docs/issues/0075 で確定したとおり「query の `data` が無いときはエラー表示のみ、`data` があるときは既存表示の上部にエラーを出して表示中のデータを消さない」で統一し、非ローディング分岐を `<>{error != null && <DrawerError error={error} />}{data !== undefined && (既存表示)}</>` の形に置き換えた。新しい文言は定義していない。

ファイル別の適用内容:

- `DrawerCFNEvents` / `DrawerCFNResources` / `DrawerECRImages`: 単一 query。見出しの件数は `data !== undefined` のときのみ表示する (未取得時に `(0)` と誤読させない。docs/issues/0075 / 0076 のパターンと同じ)。
- `DrawerCFNTags`: `data?.tags` 参照を `data !== undefined` ゲート + `data.tags` に変更し、エラー分岐を追加。
- `DrawerECSServices` / `DrawerECSTasks`: FacetBar + DataTable (+ Tasks は詳細ペイン) を `data !== undefined` 分岐の内側に移し、エラー分岐を追加。正常系の構造は変えていない。
- `DrawerELBListeners` / `DrawerELBTargets`: 親 query (Listeners / Target groups) と子 query (Rules / Targets) の両方に query ごとのエラー表示を追加。子の選択状態は親の `data` 分岐内に残るため、子のエラー時も親テーブルは表示されたまま。
- `DrawerDynamoItems`: スキーマ query と Items query に個別のエラー表示を追加。検索フォームはスキーマの `data` 分岐内にあるため、Items のエラー時もフォームと入力値は失われない。スキーマ未取得時はエラーのみ表示する。
- `DrawerCacheParameters`: パラメータ取得 query のみに適用。`useResources` の一覧キャッシュ参照は方針どおり対象外 (コメントで明記)。

完了条件の検証:

- 10 ファイルすべてに `<DrawerError` があることをファイル単位の grep で確認した (Events/Resources/Tags/Images/Services/Tasks/CacheParameters = 各 1 箇所、ELBListeners/ELBTargets = 各 2 箇所、DynamoItems = 3 箇所)。
- テスト: `DrawerCFNEvents.test.tsx` に取得エラーのケース (DrawerError の文言が表示され、テーブルと件数が出ない = 0 件表示と区別できる) を追加。`DrawerELBListeners.test.tsx` を新設し、(1) Listeners query のエラーで DrawerError のみ表示されること、(2) Listeners 成功後にリスナーを選択して Rules query がエラーになった場合、Listeners テーブルを保ったまま Rules 側に DrawerError が表示され件数もテーブルも出ないことを検証。
- 正常系: 既存テストのある `DrawerCFNEvents` / `DrawerECSTasks` はテスト通過で確認。残る 8 件は変更がエラー分岐の追加 (+ 既存表示の `data !== undefined` ゲートへの移動) のみであることをコードレビューで確認した。なお見出しの件数表示が「常に表示」から「data 取得済みのときのみ表示」に変わるが、これは docs/issues/0075 / 0076 で確定済みパターンの一部であり、data 取得済みの正常系 (0 件含む) の表示は変わらない。
- `mise run check` 通過 (frontend 68 ファイル 582 テスト、backend テスト、両 lint すべて通過)。
- ElastiCache Parameters タブの未取得と 0 件の区別は方針どおり本 issue では扱わず、次バッチで bug issue として起票する。

## 関連

- docs/issues/0075 (WAF Rules タブ): エラー表示部品 `DrawerError` と表示規則の導入元。番号順で先に実装される。
- docs/issues/0076 (RDS の Parameters タブ分割): `DrawerRDSParameters` のエラー表示はそちらで回復されるため本 issue の対象外。
- docs/issues/closed/0071: サブタブのエラー表示という設計判断の出典。

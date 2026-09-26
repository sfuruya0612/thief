# WAF の一覧の列順を Web ACL, Description, State, Scope, Region, Rules, Associated に変更する

Created: 2026-07-27
Completed: 2026-07-27
Model: Claude Fable 5

## 背景

TODO.md の次の項目に対応する。

> WAF の一覧のカラムは Web ACL, Description, State, Scope, Region, Rules, Associated の順にして欲しい

`wafColumns` (`frontend/src/components/tables/columns.tsx:1261-1309`) の現在の列は、先頭から `name` (ヘッダ `Web ACL`、20%)、`state` (8%)、`scope` (13%)、`ruleCount` (ヘッダ `Rules`、10%)、`associatedCount` (ヘッダ `Associated`、12%)、`description` (17%)、`region` (20%) の順で並んでいる。
この構成は docs/issues/closed/0074 が Description 列を追加したときのもので、Description が末尾近く (region の手前) に置かれており、要望の順と一致しない。
列の集合は要望と同じ 7 列で、並び順だけが異なる。

## 目的

WAF の一覧の列が Web ACL, Description, State, Scope, Region, Rules, Associated の順で表示される。

## 設計判断

- `wafColumns` の配列要素を並べ替えるのみとし、列の追加と削除は行わない。各要素の列定義オブジェクトは `key`, `header`, `width`, `align` 等のフィールドを変えずに丸ごと動かす。
- 並べ替えは列の `key` を変えない。列幅リサイズ (`frontend/src/hooks/useColumnResize.ts:18` の `colWidths`) と列フィルタ (`frontend/src/components/DataTable.tsx:60` の `colFilters`) はどちらも `key` を添字とするコンポーネント内 state (永続化なし) のため影響を受けない。FacetBar は列定義と独立した Env, state, region, Team の 4 軸 (`frontend/src/views/AccountView.tsx:128-139`) で絞り込むため影響を受けない。
- 各列の `width` は列に付けたまま動かす。docs/issues/closed/0074 で合計 100% になるよう配分済みであり、並べ替えでは合計が変わらない。
  - 却下案: 並び順に合わせて幅を再配分する。現行の幅で表示が崩れているという観測が無く、変更の根拠が無いため却下。
  - 幅の完了条件の立て方は docs/issues/0089 と異なる: docs/issues/0089 は新設する列の幅を実装時に確定させるため個々の値を検証しないが、本 issue は「既存の幅を動かさない」こと自体が設計判断のため、幅と列の対応の保持を検証する。
- Drawer の Overview (`wafOverviewRows`、`frontend/src/components/Drawer/overviewRows.tsx:245-254`) の行順は変更しない。TODO の要望は一覧の列順であり、Overview は Resource ID を先頭に置く別系統の表示のため対象外とする。
- backend と CLI に変更は無い (表示順は frontend の列定義だけで決まる)。
- 追加の AWS API 呼び出しと権限は不要。

## 完了条件

- `wafColumns` の `key` の並びが `name`, `description`, `state`, `scope`, `region`, `ruleCount`, `associatedCount` の 7 列でこの順に完全一致することと、対応する `header` が `Web ACL`, `Description`, `State`, `Scope`, `Region`, `Rules`, `Associated` であることを `frontend/src/components/tables/columns.test.tsx` で検証している。
- 各列の `width` が並べ替え前と同じ対応 (`name` 20%, `description` 17%, `state` 8%, `scope` 13%, `region` 20%, `ruleCount` 10%, `associatedCount` 12%) を保っていることを検証している。合計 100% を検証する既存テスト (docs/issues/closed/0074 で追加) も通過する。`ruleCount` と `associatedCount` の `align: 'right'` が保持されていることも検証している。
- セル描画の新規テストは追加しない。並べ替えは列定義オブジェクトを丸ごと動かすため、セル描画と `key` の対応は変わらず、既存の description セルのテスト (`columns.test.tsx:30-44`) の通過をもって確認する。
- `columns.test.tsx` 冒頭のコメント (`columns.test.tsx:1-2`) は wafColumns と docs/issues/closed/0074 に限定した説明のため、本 issue でファイル全体の説明 (列定義のテスト) に改める。docs/issues/0089 と docs/issues/0090 も同じファイルにテストを足すため、最初に触る本 issue で直す。
- Overview の行順の変更は本 issue では扱わない。
- `CHANGES.md` の `## develop` の `[UPDATE]` 群に、種別順 (UPDATE → ADD → CHANGE → FIX) を保つ位置へ挿入する形で `[UPDATE]` エントリと担当者行が記載されている。
- `mise run check` が通過する。

## 関連

- docs/issues/closed/0074 (Description 列の追加): 現在の列構成と幅の導入元。
- docs/issues/0086 (WAF ルール詳細): 同じ WAF ビューの要望だが、docs/issues/0086 は Drawer のサブタブと backend を、本 issue は一覧の列定義を触るためファイルが重ならず衝突しない。
- docs/issues/0088 (Associated 件数の修正): 本 issue は `associatedCount` 列の位置を、docs/issues/0088 は値を直す。触るファイルが異なるため衝突しない。
- docs/issues/0089 (CloudFront の列変更): 同じ `frontend/src/components/tables/columns.tsx` と `columns.test.tsx` を触る。対象の配列が異なるため、番号順に実装すれば衝突しない。
- docs/issues/0090 (CloudFront の Behaviors タブ): 同じ `frontend/src/components/tables/columns.tsx` と `columns.test.tsx` を触る。対象の配列が異なるため、番号順に実装すれば衝突しない。
- docs/issues/0086, docs/issues/0088, docs/issues/0089, docs/issues/0090: `CHANGES.md` の `## develop` は 5 issue 全てが変更する。番号順に直列で実装し、各 issue のエントリを種別順 (UPDATE → ADD → CHANGE → FIX) を保つ位置へ挿入すれば衝突しない。

## 解決方法

- `frontend/src/components/tables/columns.tsx` の `wafColumns` の要素の並びを `name`, `description`, `state`, `scope`, `region`, `ruleCount`, `associatedCount` の順に変更した。各列定義オブジェクト (`key`/`header`/`width`/`align`/`cell`) はそのまま動かしただけで、値は変更していない。
- `frontend/src/components/tables/columns.test.tsx` の冒頭コメントをファイル全体の説明に改め、列の並び (`key`/`header`)、各列の幅の対応、`ruleCount`/`associatedCount` の `align: 'right'` を検証するテストを追加した。
- `CHANGES.md` の `## develop` の `[UPDATE]` 群の先頭に `[UPDATE]` エントリを追加した。
- `mise run check` が backend / frontend ともに成功することを確認した。

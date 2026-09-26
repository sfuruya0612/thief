# columns.tsx の全サービスの列順序を検証するテストを整備する

Created: 2026-08-05
Model: Claude Fable 5
Completed: 2026-08-07

## 背景

TODO.md の「columns.tsx の各サービスの列順序を検証するテストを整備したい (issue 0093 のスコープ外として送り)」に対応する。

`frontend/src/components/tables/columns.tsx` は AWS サービスの列定義 33 個をエクスポートするが、`columns.test.tsx` がカバーするのは 5 個だけである。
残りの列定義は、列の追加や並べ替えの際に順序が意図通りかを検証する手段が無い。
実例として docs/issues/closed/0087 (WAF の列順の変更) は「列順が要望と違う」ことが issue になっており、順序はこのリポジトリで実際に壊れてきた性質である。
docs/issues/closed/0089 (CloudFront の列の削減と Alternate domains の表示内容の是正) も、列定義の変更要望が繰り返されてきた実例である。

既存テストの形式は `columns.test.tsx` にある。
51-70 行が `key` の配列と `header` の配列をそれぞれ `.toEqual` で検証し、30-33 行の別ブロックが `width` の合計が 100% になることを確認する。

## 対応方針

- `columns.test.tsx` の既存形式を踏襲し、未カバーの列定義エクスポート全てにテストを追加する。
- 各テストは `key` の配列、`header` の配列、`width` 合計の 3 点を検証する。個別の `width` 値は検証しない。docs/issues/closed/0089 で「幅の個別値は調整の自由度として残す」方針が採られている。
- 全 33 エクスポートの `width` 合計を実測した結果、100% にならないのは `rdsParameterColumns` (96%)、`cacheParameterColumns` (96%)、`s3ObjectColumns` (90%) の 3 つである。この 3 つは現状の合計値を期待値としてテストし、幅そのものは変更しない。100% に直す案は、表示 (列幅の配分) を変える変更がテスト整備という本 issue の範囲を超え、0089 の「幅は調整の自由度として残す」方針とも整合しないため採らない。
- セルのレンダリング関数 (`render`) の出力検証はスコープ外とする。レンダリングは各サービスの表示仕様に踏み込むため、列順序の退行検知という本 issue の目的を超える。
- `gcpColumns.tsx` (5 エクスポート) と `nonAwsColumns.tsx` (2 エクスポート) はスコープ外とする。TODO は columns.tsx を名指ししており、非 AWS 側は列順変更の要望実績も無い。必要になった時点で個別に起票する。

## 完了条件

- `columns.tsx` の全エクスポートについて、`key` の配列と `header` の配列を `.toEqual` で検証するテストが存在する。
- `width` 合計の検証が全エクスポートに存在する。期待値は `rdsParameterColumns` と `cacheParameterColumns` が 96%、`s3ObjectColumns` が 90%、残りの全定義が 100% である。
- `columns.tsx` の列定義 (key / header / width) には変更を加えない。
- `gcpColumns.tsx` と `nonAwsColumns.tsx` には変更を加えない。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0093: 本 issue の起票元。
- docs/issues/closed/0087: 列順の変更要望の実績。順序検証テストの必要性の根拠。
- docs/issues/closed/0089: 列の削減と表示内容の是正の実績。幅の個別値を調整の自由度として残す方針の出典。

## 解決方法

- `columns.test.tsx` に、全列定義の列順序と列幅の合計を一覧で検証するテーブル駆動のブロック (`describe.each`) を追加した。
  対象は、既存の個別 describe が順序と幅の合計を検証済みの `wafColumns` / `cloudfrontColumns` / `kinesisColumns` を除く 30 エクスポートで、各エクスポートについて `key` の配列と `header` の配列を `.toEqual` で検証するテストと、列幅の合計を検証するテストを追加した。
- 列幅の合計の期待値は `rdsParameterColumns` と `cacheParameterColumns` が 96%、`s3ObjectColumns` が 90%、残りが 100% で、完了条件の実測値と一致した。個別の `width` 値は docs/issues/closed/0089 の方針に従い検証していない。
- `cacheColumns` と `cloudfrontBehaviorColumns` の個別 describe にあった列幅の合計テストは一覧網羅ブロックへ移し、個別 describe には表示分岐の検証だけを残した。
- `columns.tsx`、`gcpColumns.tsx`、`nonAwsColumns.tsx` には変更を加えていない。
- `mise run check` が通ることを確認した。

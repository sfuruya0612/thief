# Savings Plans のレート取得に渡す絞り込みパラメータを検証するテストを追加する

Created: 2026-08-07
Model: Claude Fable 5
Completed: 2026-08-08

## 背景

issue 0102 (ElastiCache のリクエストパラメータ検証) の棚卸しで見つかった。

`backend/internal/aws/pricing.go` の `fetchSavingsPlans` (773-780 行付近) は `DescribeSavingsPlansOfferingRatesInput` に `SavingsPlanTypes` / `ServiceCodes` と、リージョン絞り込みの `Filters` を設定して呼び出す。
SDK の生成コード (savingsplans v1.34.1 の `api_op_DescribeSavingsPlansOfferingRates.go`) を確認すると、これらのフィールドに required の注記は無く、全て省略可能な絞り込みパラメータである。
つまり削除しても API 呼び出しは成功し、絞り込み範囲だけが静かに変わる。
特にリージョンの `Filters` を削除すると全リージョンのレートが返り、`PriceRate` 構造体はリージョンを保持するフィールドを持たないため、混入がレスポンス上どこにも現れないまま「選択中リージョンの価格」として提示されるおそれがある。

`pricing_test.go` の `fakeSavingsPlansClient` は Input を `NextToken` の nil 判定にしか使っておらず、`SavingsPlanTypes` / `ServiceCodes` / `Filters` を検証するアサーションは存在しない。
なお同ファイルの `GetProductsInput` (Pricing API) 側は `TestFetchOnDemandAndReservedEC2Filters` 等で Filters の件数検証が既にあり、こちらは対象外である。

## 対応方針

- `savingsplansAPI` 相当のインターフェースと `fakeSavingsPlansClient` によるテスト基盤は既にあるため、抽出は不要。fake で受け取った `DescribeSavingsPlansOfferingRatesInput` を記録し、検証を追加するだけでよい。
- `SavingsPlanTypes` / `ServiceCodes` / リージョンの `Filters` が期待どおり設定されることを検証する。ページネーションの既存テストがあるため、複数ページの全呼び出しで維持されることも合わせて検証する。

## 完了条件

- `fetchSavingsPlans` について、`SavingsPlanTypes` / `ServiceCodes` / リージョンの `Filters` の設定を全呼び出しで検証するテストが存在する。
- これらの設定を削除するとテストが失敗する (実装時に一時的に削除して確認する)。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0102: 起票元の棚卸し。検証手法の先例。

## 解決方法

`fetchSavingsPlans` が `DescribeSavingsPlansOfferingRates` へ渡す `SavingsPlanTypes` / `ServiceCodes` / リージョンの `Filters` を検証するテストを追加した。本番コードは変更していない。

### 実装

対応方針のとおり抽出は不要だった。`fetchSavingsPlans` は既に `savingsPlansAPI` インターフェースを引数で受け取る形になっており、テストからフェイクを差し込める。pricing.go には差分が無い。

### テスト (backend/internal/aws/pricing_test.go)

既存の `fakeSavingsPlansClient` に `inputs` フィールドを足し、受け取った Input を呼び出し順に記録するようにした。`savingsPlansAPI` の唯一の呼び出し元である `fetchSavingsPlans` はページごとに Input を新しく確保するため、記録した後に内容が書き換わることはなく、ポインタのまま記録している。この前提は呼び出し元の実装に依存するため、Input を作り直さずフィールドだけ書き換えて再送する呼び出し元が増えた場合は値でコピーする必要がある旨をコメントに明記した。既存の利用箇所 11 件はいずれも `describe` を名前付きで指定しているため、フィールドの追加による影響は無い。

`TestFetchSavingsPlansSendsPlanTypesServiceCodesAndRegionFilter` を追加した。テーブル駆動の 2 ケースで、それぞれ 2 ページ構成のレスポンスを返す。テスト名は検証対象 3 種を列挙する形にした。`SendsFilters` だと、同ファイルの `TestFetchOnDemandAndReservedEC2Filters` が実際に `GetProductsInput.Filters` 1 フィールドだけを検証しているため、`Filters` フィールドのみを見るテストと読める。

期待する Input 全体を `cmp.Diff` で比較する。0117 から 0118 で採った方式に揃えたもので、絞り込み 3 種に加えて `NextToken` の引き継ぎと、`UsageTypes` や `Operations` のような期待していないフィールドが設定されないことも同時に固定できる。`DescribeSavingsPlansOfferingRatesInput` と `SavingsPlanOfferingRateFilterElement` はいずれも unexported フィールドを持つため `cmpopts.IgnoreUnexported` を渡している。

期待値は `savingsPlanServiceSpecs` を参照せず、`savingsPlanServiceSpec` をテスト内で組み立てて渡す。実装が内部に持つ定数を経由すると、定数を固定値に置き換える変更を検出できない。2 ケースで `planTypes` / `serviceCodes` / `region` の値をすべて変え、`planTypes` と `serviceCodes` はどちらも 2 件ずつ持たせてある。リージョンには既定値になりやすい `ap-northeast-1` を使っていない。この設計により、引数を無視して固定値を送る実装も、先頭 1 件だけを送る実装も、両ケースで落ちる。当初はケース 1 を `planTypes = [Compute]` / `region = "ap-northeast-1"` としていたが、これは最もありがちなハードコード値と偶然一致し、そのケース単体では固定値化を検出できないとレビューで指摘され、値をずらした。期待値の文字列 (`"Compute"` / `"EC2Instance"` / `"Database"` / `"AmazonEC2"` / `"AmazonECS"` / `"AmazonRDS"` / `"AmazonElastiCache"` / `"region"`) は SDK の enum の実値を確認して書き下している。

フェイクは次ページの有無を、受け取った `NextToken` ではなく呼び出し回数で決める。`NextToken` の引き継ぎを壊した実装でも 2 回で必ず終端するため、無限ループにならずに検証できる。引き継ぎ自体は `wantInputs[1]` の `NextToken` の比較で固定している。

### 検出力の確認

完了条件が指定する 3 種の絞り込みの削除に加え、値の固定化、フィルタ属性の差し替え、要素数の切り詰め、`NextToken` の引き継ぎ、想定外フィールドの混入を 1 つずつ壊し、そのたびにテストが失敗することを実測した (全 2 サブテスト中の失敗数)。

| 壊した箇所 | 失敗したサブテスト数 |
| --- | --- |
| `SavingsPlanTypes` の設定行を削除 | 2 |
| `ServiceCodes` の設定行を削除 | 2 |
| リージョンの `Filters` を丸ごと削除 | 2 |
| `SavingsPlanTypes` を `SavingsPlanTypeCompute` 固定に変更 | 2 |
| `ServiceCodes` を `SavingsPlanRateServiceCodeEc2` 固定に変更 | 2 |
| `Filters` のリージョンを `"ap-northeast-1"` 固定に変更 | 2 |
| `Filters` の `Name` を region 以外の属性に変更 | 2 |
| `SavingsPlanTypes` を先頭 1 件に切り詰め | 2 |
| `ServiceCodes` を先頭 1 件に切り詰め | 2 |
| `next = out.NextToken` を `next = nil` に変更 (pricing.go 803 行) | 2 |
| `SavingsPlanOfferingIds` を新たに設定する (pricing.go 779 行) | 2 |

測定中、`next = out.NextToken` という文字列が pricing.go 内に 3 箇所あり、当初は `fetchSavingsPlans` 以外の関数を壊して失敗 0 件と誤って記録した。行番号を指定して測り直している。以降は、行番号を指定しない場合にパターンが 1 箇所しかないことを検査してから置換する測定スクリプトを使っている。ビルド失敗を失敗 0 件と数えないよう、測定はビルド結果を判定してから失敗数を数えている。確認後、実装は元に戻し、`git diff` で復元を確認した。

### 今回のテストで検証していない範囲

`savingsPlanServiceSpecs` の allowlist そのもの (compute-sp / ec2-instance-sp / database-sp に割り当てた `planTypes` と `serviceCodes` の値) は固定していない。本 issue の完了条件は `fetchSavingsPlans` が spec の値を Input に載せることの検証であり、allowlist の内容は別の関心事である。allowlist を参照して期待値を作ると、値を変えたときにテストも一緒に動いて検出できなくなる。この allowlist の値がリポジトリ内のどのテストからも固定されていない点はレビューで指摘されたが、完了条件の対象外のため本 issue では扱わない。

`DescribeSavingsPlansOfferingRates` が失敗した場合のエラー経路は既存の `TestFetchSavingsPlansError` が担当する。最終ページの `NextToken` が空文字列のときの打ち切りは既存の `TestFetchSavingsPlansPaginationStopsOnEmptyStringNextToken` が担当する。

`mise run check` の通過を確認済み。CHANGES.md の `## develop` の `### misc` に記載済み。

# Cost Explorer の Group by が Linked account のとき表とグラフの行ラベルを Account Name (Account ID) にする

Created: 2026-09-09
Model: Claude Fable 5.1
Completed: 2026-09-09

## 背景

`docs/issues/TODO.md` の次の項目に対応する。

> CostExplorer で Group by に Linked account を選択したとき、表に Account ID だけでなく Account Name (Account ID) の形でアカウント名も出したい

backend (フィールドの追加と名前の取得) と frontend (ラベルの整形と表示) を分割せず 1 つの issue で扱う。backend が `account_name` を返しても frontend が表示しなければ要望は満たされず、frontend だけ先に実装しても backend が名前を返すまでラベルは変わらない。片方だけでは検証も close もできない。

現状、Cost Explorer 画面 (`frontend/src/views/CostExplorerPanel.tsx`) で Group by に `Linked account` を選ぶと、クロス表 (`CostCrossTable`) の Group 列とグラフ (`CostChart`) の系列名には 12 桁のアカウント ID だけが表示される。アカウント名はどの層にも存在しない。

- backend: `backend/internal/aws/cost.go` の `getCost` (290 行目から) は `GetCostAndUsage` の `Groups[].Keys[0]` をそのまま `CostResource.Service` に入れる (328 行目)。`GroupByDimension` が `LINKED_ACCOUNT` のとき `Keys[0]` はアカウント ID であり、アカウント名を返す API は呼んでいない。`CostResource` (18 から 24 行目) のフィールドは `TimePeriod` / `Service` / `UnblendedAmount` / `NetAmortizedAmount` / `Unit` の 5 つで、名前を載せる場所が無い。
- backend: アカウント名の取得手段は既にある。`GetDimensionValues` は `LINKED_ACCOUNT` の応答で `DimensionValuesWithAttributes.Attributes["description"]` にアカウント名を返す (SDK の `api_op_GetDimensionValues.go` の godoc: "LINKED_ACCOUNT - The description in the attribute map that includes the full name of the member account. The value field contains the Amazon Web Services ID of the member account.")。issue 0162 で追加した `costDimensionValues` (151 行目から) がこの API のページングを実装し、`costAccountNameAttribute = "description"` (86 行目) もキーワード照合のために定義済みである。ただし `costMatchDimensionValues` (179 行目から) は一致した ID だけを返し、名前は照合の後に捨てている。`GetDimensionValues` が呼ばれるのはキーワードが空でないときだけで (`costFilter`、259 行目から)、Group by が `LINKED_ACCOUNT` であることを理由に呼ぶ経路は無い。
- API: `backend/internal/api/handlers_cost.go` の `handleCost` は `[]CostResource` をそのまま JSON にして返す。`CostResource` は `backend/internal/contract/contract.go` の 49 行目で契約テストに登録されており、JSON 形状は `frontend/src/types/__contract__/CostResource.json` と `backend/internal/contract/testdata/tags.golden` のゴールデンで固定されている。フィールドを増やすとゴールデンの再生成が要る。
- frontend: `frontend/src/api/queries.ts` の `useCost` (200 行目から) は `CostRaw.service` を `CostRow.service` に写し、行 `id` を `${time_period}/${service}` で合成する (217 行目)。`frontend/src/lib/costAggregate.ts` の `aggregateCost` は `groupKeyOf: (r) => r.service` (23 行目) で行を束ね、その値が `CostCrossTable` の `r.group` (`frontend/src/components/tables/CostCrossTable.tsx` 78 行目) と `CostChart` の `s.name` (`frontend/src/components/charts/CostChart.tsx` 55 行目) にそのまま表示される。`CostExplorerPanel` の `groupBy` state は `useCost` の引数と select の値にだけ使われ、集計や表には渡っていない。
- frontend: 「名前 (コード)」の併記は `frontend/src/components/Sidebar.tsx` 65 行目 (`{r.name === r.code ? r.code : \`${r.name} (${r.code})\`}`) に先例がある。名前とコードが同じときはコードだけを出す。

## 目的

Group by に `Linked account` を選んだとき、クロス表の Group 列とグラフの系列名が `Account Name (Account ID)` の形になり、ID だけでは判別しにくいアカウントを名前で見分けられるようにする。他の Group by (Service / Usage type / Region) の表示は変えない。

## 設計判断

### アカウント名は backend が `GetDimensionValues` で取得し、`CostResource` に専用フィールドで載せる

`getCost` は、`costGroupByDimension(opts.GroupByDimension)` が `LINKED_ACCOUNT` で、かつ `GetCostAndUsage` の結果に 1 つ以上の `Groups` があるときだけ、`costDimensionValues(ctx, client, LINKED_ACCOUNT, start, end)` を 1 回呼び、`Value` (アカウント ID) から `Attributes["description"]` (アカウント名) への対応表を作る。各 `CostResource` には `Keys[0]` (ID) に対応する名前を新設フィールド `AccountName string \`json:"account_name"\`` に入れる。対応表に無い ID、`Attributes` が nil の値、`description` が空文字の値は `AccountName` を空文字にする。Group by が `LINKED_ACCOUNT` 以外のときは `GetDimensionValues` を呼ばず、`AccountName` は常に空文字にする。

対応表の作り方は次のとおりとする。

- `Value` が空文字の次元値は対応表に入れない。`costMatchDimensionValues` が `Value` の空文字を除外している (181 から 184 行目) のと同じ扱いにする。入れると、`Keys` が空で `Service` が空文字になる `CostResource` (`TestGetCostResults` の `Keys` 無しのケースと同型) に空文字キーの名前が付く。
- 同じ `Value` が複数回 (複数ページにまたがる場合を含む) 現れたら、後に処理した値の `description` で上書きする (map への単純代入)。Cost Explorer の応答で同じ ID が重複する想定は無いが、実装の書き方で挙動が変わらないようテストで固定する。テストは複数ページにまたがるケースだけを置く。同一ページ内の重複も同じ map への代入で処理されるため、個別のケースは置かない。

`backend/internal/api/handlers_cost.go` の `costCacheKey` と `frontend/src/api/queries.ts` の `useCost` の `queryKey` は変更しない。どちらも `CostQueryOptions` 由来の値だけで構成されており、応答にフィールドが増えてもキャッシュの条件は変わらない。

- `Service` フィールドの中身 (ID) と `ResourceID()` / `ResourceName()` は変えない。`Service` を `Account Name (Account ID)` に置き換える案は却下する。`ResourceID()` と frontend の行 `id` (`${time_period}/${service}`) の値が変わり、同じアカウントを指す行の識別子が名前の有無や変更で揺れるため。表示用の文字列を識別子に混ぜない。
- 汎用名 (`group_name` など) ではなく `account_name` にする。名前を返す API があるのは `LINKED_ACCOUNT` だけで、他の 3 次元に名前の供給元は無い (Region と Usage type と Service はキーがそのまま表示名)。使い道の無い一般化はしない。
- `GetDimensionValues` の失敗は `getCost` のエラーとして返す。名前を空にして ID だけで続行する案は却下する。「名前が登録されていない」と「取得に失敗した」を画面から区別できず、エラーが表示上は正常な結果に見えるため。issue 0162 の `costResolveKeyword` が次元の取得失敗を部分結果で続行せずエラーにしているのと同じ判断である。
- `GetDimensionValues` は `GetCostAndUsage` の後に呼び、`Groups` が空なら呼ばない。名前を付ける対象が無いときに課金される呼び出しを増やさないため。errgroup で `GetCostAndUsage` と並列に呼ぶ案は、`Groups` の有無で呼び出しを省けなくなり、並列化で縮む時間もリクエスト 1 往復分に限られるため却下する。
- キーワードが空でなく Group by が `LINKED_ACCOUNT` のリクエストでは、`costResolveKeyword` と本 issue の取得で `GetDimensionValues(LINKED_ACCOUNT)` を 1 リクエスト中に 2 回呼ぶことになる。`costResolveKeyword` の取得結果を再利用して 1 回に減らす案は却下する。`costFilter` / `costResolveKeyword` / `costMatchDimensionValues` の 3 つの戻り値に一致 ID 以外の情報を足し、絞り込みの解決と表示名の付与を結合することになるため。この組み合わせの追加コストは `GetDimensionValues` 1 回 (0.01 USD) で、`handleCost` の `serveCached` により同じ条件の再取得は `cacheTTL` の間はキャッシュから返る。
- 対応表の構築は `costMatchDimensionValues` とは別の関数にする。照合 (キーワードに一致した ID を返す) と対応表の構築 (ID から名前を引けるようにする) は入力も出力も異なる。

### 追加の API 呼び出しと権限

- 追加で呼ぶ API は `ce:GetDimensionValues` のみで、issue 0162 のキーワード絞り込みが既に使っている。新しい IAM 権限は不要。
- `GetDimensionValues` はリクエストごとに 0.01 USD が課金される (AWS Cost Explorer の料金ページ https://aws.amazon.com/aws-cost-management/aws-cost-explorer/pricing/ の "Each request using your primary billing view, which contains cost management data associated with your account, will incur a cost of $0.01."、2026-09-09 確認)。Group by が `LINKED_ACCOUNT` で結果が空でないリクエストごとに 1 回増える。
- AWS Organizations API は使わない。Cost Explorer の `description` 属性で名前が取れるため、権限の追加と別サービスへの依存を増やさない。

### frontend は `accountName` を Row に写し、集計の群キーで `Account Name (Account ID)` を組み立てる

- `frontend/src/types/aws.ts` の `CostRaw` に `account_name: string`、`CostRow` に `accountName: string` を追加し、`useCost` で写す。行 `id` は変えない。
- `frontend/src/lib/costAggregate.ts` に、`CostRow` から表示ラベルを返す純関数 `costGroupLabel(r: CostRow): string` を追加する。`accountName` が空文字か `service` と同じ値なら `service` を、それ以外なら `` `${accountName} (${service})` `` を返す。`aggregateCost` の `groupKeyOf` を `costGroupLabel` に差し替える。
- ラベルを `groupKeyOf` で作ると、`aggregateCostRows` の系列の順位付け、`Other` へのまとめ、`crossTableRows` の群キー、`CostChart` の系列名がすべて同じ文字列を使い、`CostCrossTable` / `CostChart` / `aggregateCostRows` / `CostExplorerPanel` の変更が要らない。`CostCrossTable` と `CostChart` に `groupBy` を渡して表示時に整形する案は、`Other` の判定に使う群キーと表示名が別の値になり、整形を 2 つの部品に重複して持つことになるため却下する。
- 同じ名前のアカウントが 2 つあっても、ラベルに ID が含まれるため別の群として集計される。
- `accountName` が空文字なら ID だけを表示するため、backend が名前を返さない次元 (Service / Usage type / Region) と、名前が登録されていないアカウントの表示は現状と同じになる。frontend は `groupBy` の値で分岐しない。
- クロス表の Group 列見出し ("Group") は変えない。

### 契約テストと変更履歴

- `CostResource` にフィールドを追加するため、`UPDATE_GOLDEN=1 go test ./internal/contract/` で `frontend/src/types/__contract__/CostResource.json` と `backend/internal/contract/testdata/tags.golden` を再生成する。
- `CHANGES.md` の `## develop` に `[ADD]` として記載する。既存フィールドの意味と値は変えず、フィールドを 1 つ足す後方互換の追加である。

## 完了条件

- `backend/internal/aws/cost.go` の `CostResource` に `AccountName string \`json:"account_name"\`` が追加されている。`ResourceID()` と `ResourceName()` の戻り値は変更前と同じである。
- `getCost` は、`costGroupByDimension(opts.GroupByDimension)` が `LINKED_ACCOUNT` で `GetCostAndUsage` の結果に `Groups` が 1 つ以上あるとき、`GetDimensionValues` を `Dimension = LINKED_ACCOUNT`、`TimePeriod = GetCostAndUsage と同じ start / end` で呼び、各 `CostResource.AccountName` に `Keys[0]` と同じ `Value` を持つ次元値の `Attributes["description"]` を入れる。
- `getCost` は次の場合に `GetDimensionValues` を呼ばない。`backend/internal/aws/cost_test.go` の `fakeCostExplorer` の呼び出し記録で確認する。
  - `GroupByDimension` が `LINKED_ACCOUNT` 以外 (空文字を含む)
  - `GroupByDimension` が `LINKED_ACCOUNT` で `GetCostAndUsage` の `ResultsByTime` に `Groups` が 1 つも無い
- `backend/internal/aws/cost_test.go` に、`GroupByDimension = LINKED_ACCOUNT` の `getCost` について次のケースがテーブル駆動で存在し、通る。
  - ID に対応する名前がある: `AccountName` に `description` の値が入る
  - `GetDimensionValues` の結果に無い ID: `AccountName` が空文字
  - `Attributes` が nil の値: panic せず `AccountName` が空文字
  - `description` が空文字の値: `AccountName` が空文字
  - `NextPageToken` で 2 ページに分かれた結果: 2 ページ目の値の名前も入る
  - `Value` が空文字の次元値を含む結果: 対応表に入らず、`Keys` が空で `Service` が空文字の `CostResource` の `AccountName` は空文字のまま
  - 同じ `Value` が 1 ページ目と 2 ページ目に異なる `description` で現れる結果: 2 ページ目の `description` が `AccountName` に入る
  - `GetDimensionValues` がエラーを返す: `getCost` がそのエラーをラップして返し、`CostResource` を返さない
  - キーワードが空でなく `GroupByDimension = LINKED_ACCOUNT`: `GetDimensionValues` が `LINKED_ACCOUNT` について 2 回呼ばれ (キーワード解決と名前取得)、`AccountName` が入る
- `backend/internal/aws/cost_test.go` の既存テスト (`TestGetCostResults` など、`GroupByDimension` が `LINKED_ACCOUNT` 以外のケース) は変更なしで通り、`AccountName` が空文字である。
- `frontend/src/types/__contract__/CostResource.json` と `backend/internal/contract/testdata/tags.golden` が `account_name` を含む内容に再生成され、`go test ./internal/contract/` が通る。
- `frontend/src/types/aws.ts` の `CostRaw` に `account_name: string`、`CostRow` に `accountName: string` があり (どちらも必須フィールド)、`frontend/src/api/queries.ts` の `useCost` が `account_name` を `accountName` に写す。`CostRow.id` の合成式は変更前と同じである。
- `CostRaw` / `CostRow` を組み立てる既存のテストヘルパー `frontend/src/lib/costAggregate.test.ts` の `row()` と `frontend/src/views/CostExplorerPanel.test.tsx` の `raw()` が `accountName` / `account_name` を持つオブジェクトを返し (既定値は空文字、ケースごとに指定できる引数を追加する)、既存のケースが型エラーなく通る。`frontend/src/components/tables/CostCrossTable.test.tsx` は `CostCrossTableRow` を直接組み立てており `CostRow` を経由しないため変更しない。
- `frontend/src/lib/costAggregate.ts` に `costGroupLabel(r: CostRow): string` がエクスポートされ、`aggregateCost` の `groupKeyOf` がこれを使う。`frontend/src/lib/costAggregate.test.ts` に次のケースが存在し、通る。
  - `accountName` が空文字: `service` の値を返す
  - `accountName` が `service` と同じ値: `service` の値を返す
  - `accountName` が空でなく `service` と異なる: `${accountName} (${service})` を返す
  - `aggregateCost` の結果の `crossTableRows[].group` と `series[].name` が、`accountName` を持つ行では `Account Name (Account ID)` の形になり、`accountName` が空文字の行では `service` のままである
  - 同じ `accountName` で `service` (ID) が異なる 2 行が別の群として集計される
- `frontend/src/views/CostExplorerPanel.test.tsx` に、`useCost` が `accountName` 付きの行を返すとき、クロス表に `Account Name (Account ID)` 形式のセルが描画されるケースが存在し、通る。
- 次は変更前と同じである。
  - `frontend/src/components/tables/CostCrossTable.tsx` (Group 列見出しを含む)、`frontend/src/components/charts/CostChart.tsx`、`frontend/src/lib/costAggregateCore.ts`
  - `frontend/src/views/CostExplorerPanel.tsx` の `aggregateCost` 呼び出し
  - `frontend/src/api/queries.ts` の `useCost` の `queryKey` の構成要素
  - `backend/internal/api/handlers_cost.go` の `costCacheKey` の構成要素
  - `backend/internal/aws/cost.go` の `costResolveKeyword`、`costFilter`、`costMatchDimensionValues` のシグネチャと戻り値 (キーワード解決の結果を名前取得に再利用しない)
- `CHANGES.md` の `## develop` に `[ADD]` のエントリがある。
- 「## スコープ外」に挙げた対象を変更していない。
- `mise run check` が通る。

## スコープ外

次は本 issue で変更しない。

- CLI の `thief cost account` (`backend/internal/cli/cost.go` の `showCostByAccount`。`GetCostByAccount` / `CostDetail.GroupKey` を使う別経路で、アカウント名を持たない) の出力
- CLI の `thief cost ls` (`GetCost` を `GroupByDimension` 未指定で呼ぶ。`AccountName` は空文字になり出力は変わらない)
- `MonthlyCostPanel.tsx` (Datadog / TiDB 専用)
- Group by が Service / Usage type / Region のときの表示

## 関連

- docs/issues/closed/0162 (Cost Explorer のキーワード絞り込み): `costDimensionValues` と `costAccountNameAttribute` を本 issue で再利用する。0162 は名前を照合にだけ使い、応答には載せていない。
- docs/issues/closed/0094 (Cost Explorer のアカウントとサービスの絞り込み): `AccountFilter` (アカウント ID による絞り込み) を追加した issue。Group by の `LINKED_ACCOUNT` は 0094 の時点で既に選択肢にあり (同 issue の背景が `GROUP_BY_OPTIONS` に `LINKED_ACCOUNT` があることを前提にしている)、表示はアカウント ID のままだった。

## 解決方法

### backend

- `backend/internal/aws/cost.go`
  - `CostResource` に `AccountName string \`json:"account_name"\`` を追加した。`ResourceID()` と `ResourceName()` は変更していない。
  - `costAccountNames(ctx, client, start, end)` を新設した。`costDimensionValues` で `LINKED_ACCOUNT` の値一覧を全ページ取得し、`Value` (アカウント ID) から `Attributes["description"]` (アカウント名) への対応表 (`map[string]string`) を返す。`Value` が空文字の値は入れない。同じ `Value` は後に処理した値で上書きする。`Attributes` が nil の値は nil マップの索引で空文字になる。取得に失敗したら `get linked account names: %w` でラップして返す。
  - `costHasGroups(results)` を新設した。`ResultsByTime` のいずれかに `Groups` が 1 つ以上あれば真を返す。
  - `getCost` は `GetCostAndUsage` の後、`costGroupByDimension(opts.GroupByDimension)` が `LINKED_ACCOUNT` かつ `costHasGroups` が真のときだけ `costAccountNames` を 1 回呼び、各 `CostResource.AccountName` に `Keys[0]` をキーとした対応表の値を入れる。それ以外では対応表が nil のままで `AccountName` は空文字になる。`costResolveKeyword`、`costFilter`、`costMatchDimensionValues` は変更していない。
- `backend/internal/aws/cost_test.go`
  - `linkedAccountOutput(period, keys...)` を追加した (1 期間に指定した `Keys` の `Groups` を並べた `GetCostAndUsage` の応答。nil の要素は `Keys` 無しの Group)。
  - `TestGetCostLinkedAccountNames` をテーブル駆動で追加した (12 ケース)。各ケースで `fakeCostExplorer.dimensionInputs(LINKED_ACCOUNT)` の件数と `TimePeriod` を検証する。
- `backend/internal/contract/testdata/tags.golden` と `frontend/src/types/__contract__/CostResource.json` を `UPDATE_GOLDEN=1 go test ./internal/contract/` で再生成した (`account_name` が追加された)。

### frontend

- `frontend/src/types/aws.ts`: `CostRaw` に `account_name: string`、`CostRow` に `accountName: string` を追加した (どちらも必須)。
- `frontend/src/api/queries.ts`: `useCost` の `queryFn` で `account_name` を `accountName` に写す 1 行を追加した。`id` の合成式と `queryKey` は変更していない。
- `frontend/src/lib/costAggregate.ts`: `costGroupLabel(r: CostRow): string` を追加してエクスポートし、`aggregateCost` の `groupKeyOf` を `costGroupLabel` に差し替えた。`accountName` が空文字か `service` と同じなら `service` を、それ以外は `${accountName} (${service})` を返す。
- `frontend/src/lib/costAggregate.test.ts`: `row()` に第 5 引数 `accountName = ''` を追加した。`costGroupLabel` の 3 ケースと、`aggregateCost` の Linked account ラベルの 3 ケースを追加した。
- `frontend/src/views/CostExplorerPanel.test.tsx`: `raw()` に第 5 引数 `accountName = ''` を追加した。`accountName` 付きの行でクロス表の Group 列が `Account Name (Account ID)` になるケースを追加した。

### 完了条件の検証

- `AccountName` の追加と `ResourceID()` / `ResourceName()` の不変: `cost.go` の差分で確認。`ResourceID()` / `ResourceName()` の行は変更していない。
- `LINKED_ACCOUNT` かつ `Groups` ありのときの `GetDimensionValues` 呼び出し (Dimension、TimePeriod) と `AccountName` の値: `TestGetCostLinkedAccountNames` の「ID に対応する名前がある場合は AccountName に description が入る」ほか。全ケースで `TimePeriod` の start / end を検証する。
- 呼ばない条件: 同テストの「GroupByDimension が空文字 (SERVICE) なら GetDimensionValues を呼ばず AccountName は空文字」「GroupByDimension が USAGE_TYPE なら GetDimensionValues を呼ばず AccountName は空文字」「LINKED_ACCOUNT でも Groups が 1 つも無ければ GetDimensionValues を呼ばない」(いずれも呼び出し回数 0 を検証)。
- 完了条件のテーブル駆動テストの 9 ケース: 同テストの「ID に対応する名前がある」「GetDimensionValues の結果に無い ID」「Attributes が nil」「description が空文字」「NextPageToken で 2 ページ」「Value が空文字の次元値」「同じ Value が 1 ページ目と 2 ページ目に異なる description」「GetDimensionValues がエラー」「キーワードが空でない場合は LINKED_ACCOUNT が 2 回呼ばれ AccountName が入る」の各ケース。
- 既存テストの不変: `cost_test.go` の既存テストは変更していない。`go test -race ./internal/aws` が通る。
- 契約ゴールデン: 再生成後に `go test ./internal/contract/` が通る。`frontend/src/types/contract.check.ts` の型検査 (`npm run lint` の `tsc --noEmit`) が通る。
- `CostRaw.account_name` / `CostRow.accountName` / `useCost` の写し / `id` の不変: `aws.ts` と `queries.ts` の差分で確認。
- テストヘルパー `row()` / `raw()`: 既定値空文字の第 5 引数を追加し、既存ケースは変更なしで通る。`CostCrossTable.test.tsx` は変更していない。
- `costGroupLabel` のエクスポートと `groupKeyOf` での使用: `costAggregate.ts` の差分で確認。3 ケースは `costAggregate.test.ts` の `describe('costGroupLabel')`。`crossTableRows[].group` / `series[].name` の形と、同じ `accountName` で `service` が異なる 2 行の分離は `describe('aggregateCost (Linked account のラベル)')` の 3 ケース。
- クロス表の描画: `CostExplorerPanel.test.tsx` の「useCost が accountName 付きの行を返すとクロス表の Group 列が "Account Name (Account ID)" になる」。Group 列見出しが `Group` のままであることも同ケースで検証する。
- 変更前と同じであることの確認: `git status --porcelain` の変更ファイルに `CostCrossTable.tsx`、`CostChart.tsx`、`costAggregateCore.ts`、`CostExplorerPanel.tsx`、`handlers_cost.go` は含まれない。`queries.ts` の差分は `accountName` の写し 1 行のみで `queryKey` は変更なし。`cost.go` の `costResolveKeyword` / `costFilter` / `costMatchDimensionValues` は差分なし。
- `CHANGES.md` の `[ADD]` エントリ: 追記済み。
- 「## スコープ外」の対象 (`backend/internal/cli/cost.go`、`MonthlyCostPanel.tsx`) は変更していない。`CostResource.ToRow()` (`torow.go`) も変更しておらず、`thief cost ls` の出力は変わらない。
- `mise run check`: 通過 (backend 全パッケージ ok、frontend 75 ファイル 794 テスト pass。ベースライン 787 から新規 7 件が増え、失敗 0 件)。

方針セクションからの乖離は無い。

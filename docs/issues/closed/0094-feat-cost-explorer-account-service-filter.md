# CostExplorer にアカウントとサービスの絞り込みを追加する

Created: 2026-07-28
Completed: 2026-07-29
Model: Claude Fable 5

## 背景

`docs/issues/TODO.md` の「CostExplorer の画面で AWS アカウント、AWS サービスでの絞り込み機能を追加したい」に対応する。

frontend `frontend/src/views/CostExplorerPanel.tsx` は他の AWS サービスと異なり `AccountView.tsx` の汎用 `ServicePanel` を使わず、独自実装になっている。ファイル冒頭のコメント (1-2 行) には「Cost Explorer 専用パネル。ServicePanel (汎用 15 サービス共通) とは異なり、リソース一覧ではなくコストの chart + クロス集計テーブルを表示するため専用実装とする。」とある。
現在の絞り込みは `serviceFilter` という state (68 行) のみで、`rows` の算出 (83-87 行) は次の通りブラウザ側の部分一致フィルタである。

```tsx
const rows = useMemo(() => {
    if (!serviceFilter.trim()) return allRows;
    const needle = serviceFilter.trim().toLowerCase();
    return allRows.filter((r) => r.service.toLowerCase().includes(needle));
}, [allRows, serviceFilter]);
```

この `serviceFilter` には構造的な問題がある。`allRows` の各行の `service` フィールドは、`backend/internal/aws/cost.go` の `CostResource.Service` (18 行) のコメント (15 行)「GroupKey は GroupByDimension で指定した次元の値 (デフォルトはサービス名) を保持する」の通り、GroupBy で選択した次元の値をそのまま保持する。`GROUP_BY_OPTIONS` (`CostExplorerPanel.tsx` 24-29 行) には SERVICE 以外に USAGE_TYPE / LINKED_ACCOUNT / REGION があり、ユーザーが GroupBy を SERVICE 以外に切り替えると `service` フィールドの実体は使用タイプ名やリンクアカウント ID やリージョン名になる。つまり `serviceFilter` は名前に反して「サービス名の絞り込み」ではなく「現在選択中の GroupBy 次元の値に対する部分一致絞り込み」であり、AWS サービス名とアカウントを独立に絞り込むという要望を満たせない。

なお `cost.go` の `GetCost` (157 行) は `service = group.Keys[0]` として `group.Keys[0]` (AWS Cost Explorer API がグルーピング次元の値として返す文字列そのもの) を加工せずに `CostResource.Service` へ格納している。GroupBy が LINKED_ACCOUNT のとき、この値は AWS が付与する生のアカウント ID であり、名前解決や整形は行われない。したがって本 issue で追加するアカウント絞り込みの入力値も、この生のアカウント ID 形式と一致させる必要がある。

backend 側は AWS サービスでの絞り込みに対応済みである。`backend/internal/aws/cost.go` の `CostQueryOptions` (51-59 行) は `ServiceFilter` フィールドを持ち、`GetCost` (113-182 行) は `ServiceFilter` が空でない場合に次のように `cetypes.Expression` を組み立てて `GetCostAndUsage` の `Filter` に渡す。

```go
if opts.ServiceFilter != "" {
    input.Filter = &cetypes.Expression{
        Dimensions: &cetypes.DimensionValues{
            Key:          cetypes.DimensionService,
            Values:       []string{opts.ServiceFilter},
            MatchOptions: []cetypes.MatchOption{cetypes.MatchOptionEquals},
        },
    }
}
```

`backend/internal/api/handlers_cost.go` の `handleCost` (10-28 行) も既に `q.Get("service")` を `ServiceFilter` に設定している (17 行) が、frontend の `api/endpoints.ts` の `CostQueryOptions` インターフェース (112-119 行) にはコメント (111 行)「サービス名でのフィルタはブラウザ側 (取得済みデータへのフィルタ) で行うため API には持たない」とあり、`service` パラメータを送っていない。したがって backend の `ServiceFilter` は現状呼び出されていない。

一方 AWS アカウントでの絞り込みは backend にも実装がない。`CostQueryOptions` (51-59 行) には `ServiceFilter` に相当する `AccountFilter` のようなフィールドがなく、絞り込みの仕組みも frontend にもない。
`github.com/aws/aws-sdk-go-v2/service/costexplorer v1.63.3` の `types/enums.go` (345 行) には `DimensionLinkedAccount Dimension = "LINKED_ACCOUNT"` が `DimensionService` (351 行) と同じ `Dimension` 型の enum 値として既に定義されており、AWS アカウント (リンクアカウント) での絞り込みは `GetCostAndUsage` の `Filter` で対応可能である。

`GetCost` (113-182 行) は現状、SDK クライアントを関数内部で生成している。`internal/aws/athena.go` は `athenaAPI` インターフェース (102 行) を定義し、`GetQueryResults` 等のメソッド (110 行) を介してクライアントを外部から注入できる構造になっており、`internal/aws/athena_test.go` の `fakeAthena` (16-17 行以降) がこのインターフェースを実装した手書きフェイクとしてテーブル駆動テストに使われている。`GetCost` にはこの構造がなく、フィルタ組み立てロジックを実際の AWS 呼び出しなしに単体テストする手段が現状存在しない。

`frontend/src/views/CostExplorerPanel.test.tsx` には現行のブラウザ側フィルタ挙動を固定する既存テスト「サービス名フィルタは API を再呼び出しせずブラウザ側でクロス表を絞り込む」(48-63 行付近、`getByPlaceholderText('filter by service name (client-side)…')` で入力し API 再呼び出しが起きないことを検証) があり、`serviceFilter` を廃止する本 issue の設計判断はこのテストと矛盾する。したがってこのテストの更新または置き換えを完了条件に含める必要がある。

## 目的

CostExplorer の画面で、GroupBy の選択次元に関係なく、AWS アカウントと AWS サービスをそれぞれ独立に絞り込めるようにする。

## 設計判断

- 現行の `serviceFilter` (`CostExplorerPanel.tsx` 68, 83-87 行) は廃止する。GroupBy 次元に連動してしまう構造的な欠陥があり、改修ではなく置き換えが必要と判断する。
- backend `CostQueryOptions` に `AccountFilter` を追加する。既存の `ServiceFilter` と同じ形式のフィールドとする。`ServiceFilter`/`AccountFilter` はそれぞれ単一値のみを受け付ける (OR や複数値指定には対応しない)。複数条件の絞り込みは現時点の要望に含まれず、対応するとフィルタ UI と `cetypes.Expression` の組み立てが複雑化するため。
- `GetCost` でのフィルタ組み立ては、`ServiceFilter`/`AccountFilter` のどちらか一方のみが指定された場合は、既存の `ServiceFilter` 実装と同様に `Dimensions` を直接持つ単一の `cetypes.Expression` を `Filter` に設定する。両方が指定された場合は、`cetypes.Expression{ And: []cetypes.Expression{ {Dimensions: <ServiceFilter 用の DimensionValues>}, {Dimensions: <AccountFilter 用の DimensionValues>} } }` のように、`And` の要素それぞれに `Dimensions` を持たせた 2 要素の配列を組み立てる。トップレベルの `Expression` に `Dimensions` と `And` を同時に設定しない (`cetypes.Expression` は 1 つのフィルタ機構のみを持つ想定であるため)。
- `AccountFilter` に渡す値は、GroupBy=LINKED_ACCOUNT で返る `CostResource.Service` の値 (`cost.go:157` の `service = group.Keys[0]`、AWS Cost Explorer API が返す生のアカウント ID をそのまま使っており名前解決や整形を行わない) と同じ形式、すなわち加工されていないアカウント ID 文字列であることを前提とする。
- GroupBy=LINKED_ACCOUNT を選択した状態で `AccountFilter` も設定する組み合わせを許可する。GroupBy (結果のグルーピング次元) と Filter (絞り込み条件) は独立した API 概念であり、同時に指定してもエラーにはならない。この組み合わせのテストケースを完了条件に含める。
- `MatchOptions` は既存の `ServiceFilter` と同じ `MatchOptionEquals` (完全一致) を使う。`MatchOptionContains` (部分一致) は採らない。サービス名やアカウント ID は候補が有限かつ既知であり、完全一致で十分なこと、また部分一致は意図しない対象を含めてしまう可能性があるため。
- `GetCost` を単体テスト可能にするため、Athena の `athenaAPI` インターフェース (`athena.go:102`) と `fakeAthena` (`athena_test.go:16` 以降の手書きフェイク) と同じパターンを採用する。Athena のパターンは公開関数 `ListAthenaCatalogs(ctx, profile, region)` が内部で実クライアントを生成して非公開関数 `listAthenaCatalogs(ctx, client athenaAPI)` に処理を委譲する形 (`athena.go:116-122`) であり、公開シグネチャ自体は変更しない。同様に `GetCost(ctx, profile, region, opts)` の公開シグネチャは変更せず、新規の非公開関数 `getCost(ctx, client costExplorerAPI, opts)` を追加してフィルタ組み立てを含む処理をそこに移し、`GetCost` は実クライアントを生成して `getCost` を呼ぶだけにする。`GetCost` は `internal/api/handlers_cost.go:26` と `internal/cli/cost.go:71` の 2 箇所から現行シグネチャのまま呼ばれており、公開シグネチャを変更するとこの 2 箇所がコンパイルエラーになるため、シグネチャ変更は行わない。`costExplorerAPI` インターフェース (`GetCostAndUsage` メソッドを持つ) を定義し、テストでは `getCost` に手書きの `fakeCostExplorer` を注入する。モックライブラリの新規導入は行わない (既存の Athena と同じ手書きフェイクで足りるため)。
- frontend は backend の `ServiceFilter`/`AccountFilter` を呼び出す新しい絞り込み UI に置き換える。GroupBy の選択次元とは独立した state (`serviceNameFilter`/`accountFilter` 等) を持たせ、`api/endpoints.ts` の `CostQueryOptions` に `service`/`account` パラメータを追加して backend に送る。ブラウザ側のフィルタ処理 (現行の `useMemo` によるフィルタ) は廃止し、絞り込みは backend の `GetCostAndUsage` の `Filter` に一本化する。
- `GetCostAndUsage` は呼び出しごとに課金が発生する有償 API である。現行の `serviceFilter` がブラウザ側フィルタで API を再呼び出ししない設計になっているのも、`CostExplorerPanel.tsx` のコメント (71-72 行、「API 呼び出しは期間/Granularity/GroupBy のみに依存させる。サービス名フィルタは取得済みデータに対してブラウザ側で絞り込むだけにし、都度 API を呼び出さない」) の通りこの課金特性を踏まえたものである。テキスト入力を 1 文字入力するたびに `useCost` の `queryKey` を変化させて API を再呼び出しする実装にはしない。`service`/`account` の入力欄はテキスト入力を保持する state と、実際に `useCost` へ渡す確定値の state を分離し、Enter キー押下または入力欄からのフォーカスアウトで確定値を更新する方式とする。これによりアカウント ID などの長い文字列を入力する間の逐次呼び出しを避ける。リポジトリ内に既存の debounce 実装は無く (`frontend/src/` を検索して確認)、新規に debounce ユーティリティや追加ライブラリを導入する必要はない。
- 候補値の入力方法は自由記述のテキスト入力とする。動的セレクト (候補一覧を取得するセレクトボックス) は採らない。動的セレクトは候補一覧を取得する追加の処理 (`ListAccounts` 相当の呼び出し、または `GetCostAndUsage` の LINKED_ACCOUNT グルーピング結果からの抽出) が必要になり、現時点でその追加処理を正当化する要望がないため (YAGNI)。アカウント ID は上記の通り生の ID 文字列を要求するため、テキスト入力でもユーザーは AWS コンソール等で確認した ID をそのまま貼り付ければ動作する。
- 組織 (AWS Organizations) 配下の連結決済アカウントにおける visibility (自アカウント以外のリンクアカウントの絞り込み可否) は本 issue の対象外とする。既存の `ServiceFilter` 実装が呼び出し元の権限をそのまま利用する構造を変更しないため、`AccountFilter` も同様に呼び出し元が権限を持つ範囲でのみ機能する。新規の API 呼び出しは発生しないため、追加の IAM 権限は不要である。
- backend (フィルタ組み立てとテスト) と frontend (UI 置き換えと確定値/入力値分離) を分割せず 1 issue で扱う。両者は「アカウント/サービスの絞り込み」という単一のユーザー価値に対応し、frontend の UI 置き換えは backend の `AccountFilter` 追加なしには意味を持たない (絞り込み対象の一方である `AccountFilter` が backend に存在しないため)。確定値/入力値分離は frontend 側の実装詳細であり、backend の完了条件やテスト範囲を変えるものではないため、分割の判断には影響しない。
- `ServiceFilter`/`AccountFilter` の入力欄を確定済みの状態から空文字に変更して Enter キー押下または入力欄からのフォーカスアウトで確定した場合は、絞り込み解除として扱う。backend の既存 `ServiceFilter != ""` (`cost.go:133`) は空文字を絞り込みなしとして扱う仕様であり、`AccountFilter` も同じ判定方式にするため、frontend は空文字を確定値のまま `service`/`account` パラメータとして送る (パラメータ自体を省略する方式は採らない)。
- `GetCostAndUsage` のページネーション (`NextPageToken`) は本 issue の対象外とする。`cost.go` の現行実装は `GetCostAndUsage` を 1 回呼ぶのみで `NextPageToken` を一切扱っておらず、これは本 issue が導入する変更ではなく既存の挙動である。また `Filter` の追加は結果セットを絞り込む方向にのみ働き、既存より多くのページを返す方向には働かないため、本 issue によってページネーション未対応の影響が新たに生じるわけではない。ページネーション自体への対応は本 issue のスコープ外の既存課題であり、対応する場合は別 issue とする。

## 完了条件

- backend: `CostQueryOptions` (`internal/aws/cost.go`) に `AccountFilter string` を追加する。
- backend: `costExplorerAPI` インターフェース (`GetCostAndUsage` メソッドを持つ) を、Athena の `athenaAPI` (`athena.go:102`) と同じ形式で定義する。
- backend: 新規の非公開関数 `getCost(ctx, client costExplorerAPI, opts)` を追加し、フィルタ組み立てを含む現行 `GetCost` の処理をここに移す。公開関数 `GetCost(ctx, profile, region, opts)` は既存のシグネチャを変更せず、内部で実クライアントを生成して `getCost` を呼ぶだけにする (Athena の `ListAthenaCatalogs`/`listAthenaCatalogs` と同じ公開/非公開分離、`athena.go:116-122`)。`internal/api/handlers_cost.go:26` と `internal/cli/cost.go:71` の既存呼び出しがコンパイルエラーにならないことを確認する。
- backend: `getCost` で `ServiceFilter` と `AccountFilter` のいずれか一方のみが指定された場合は単一の `Dimensions` を持つ `cetypes.Expression` を、両方が指定された場合は `And` に 2 要素 (`ServiceFilter` 用と `AccountFilter` 用、それぞれ `Dimensions` を持つ `Expression`) を格納した `cetypes.Expression` を `Filter` に設定する。トップレベルの `Expression` に `Dimensions` と `And` を同時に設定しない。
- backend: `internal/aws/cost_test.go` に手書きの `fakeCostExplorer` (`athena_test.go` の `fakeAthena` と同じ形の手書きフェイク) を追加し、`getCost` に対して `ServiceFilter` のみ、`AccountFilter` のみ、両方指定、両方未指定、GroupBy=LINKED_ACCOUNT と `AccountFilter` を同時指定、GroupBy=SERVICE と `ServiceFilter` を同時指定、の 6 パターンをテーブル駆動テストで追加する。最後の 2 パターンは GroupBy (結果のグルーピング次元) と Filter (絞り込み条件) が独立した API 概念であることを、`AccountFilter`/`ServiceFilter` の双方について対称に検証するためのものである。各ケースで `fakeCostExplorer` が受け取った `GetCostAndUsageInput.Filter` を、設計判断で示した `cetypes.Expression` の形 (単一 `Dimensions` または `And` に 2 要素) と `cmp.Diff` 等で一致比較する。呼び出しがエラーなく成功することのみを確認するテストでは足りない。
- backend: `handlers_cost.go` の `handleCost` で `q.Get("account")` を `AccountFilter` に設定する。
- backend: `handlers_cost.go` の `cacheKey(...)` 呼び出し (24 行) に `AccountFilter`/`account` パラメータを含め、絞り込み条件が異なるリクエスト間でキャッシュが衝突しないようにする。
- frontend: `api/endpoints.ts` の `CostQueryOptions` に `service`/`account` パラメータを追加し、リクエストに含める。
- frontend: `api/queries.ts` の `useCost` の `queryKey` 配列 (192-204 行付近) に確定値の `opts?.service`/`opts?.account` を追加する。追加しない場合、絞り込み条件を変更しても TanStack Query が既存のキャッシュ結果を再利用し、`useCost` への再取得が行われない (backend の `cacheKey` 側は本 issue で `AccountFilter`/`account` を含めるよう完了条件に定めており、frontend の `queryKey` もこれと対称に保つ必要がある)。
- frontend: `CostExplorerPanel.tsx` の GroupBy 次元に連動する現行の `serviceFilter` (68, 83-87 行) を廃止し、GroupBy とは独立したアカウント絞り込みとサービス絞り込みの、いずれも自由記述のテキスト入力による UI 要素に置き換える。入力欄の表示用 state と `useCost` に渡す確定値の state を分離し、Enter キー押下またはフォーカスアウトで確定値を更新することで、1 文字入力するたびに API を再呼び出ししないようにする。
- frontend: `CostExplorerPanel.test.tsx` の既存テスト「サービス名フィルタは API を再呼び出しせずブラウザ側でクロス表を絞り込む」(48-63 行付近) を、次の 3 点を検証する内容に更新する: (a) service/account 入力欄に文字を入力しただけ (Enter/blur 前) では `useCost` へのクエリが再実行されないこと、(b) Enter キー押下で確定値が更新され `useCost` へのクエリが実行されること、(c) 入力欄からのフォーカスアウトでも同様に確定されクエリが実行されること。
- `mise run check` が全て通過する。

## スコープ外

- AWS Organizations の連結決済アカウントにおける他アカウントの visibility 拡張は本 issue では扱わない。`AccountFilter` は既存の `ServiceFilter` と同様、呼び出し元の権限で参照可能な範囲でのみ機能する。

## 解決方法

### backend

- `internal/aws/cost.go` の `CostQueryOptions` に `AccountFilter string` を追加した。
- `costExplorerAPI` インターフェース (`GetCostAndUsage` の 1 メソッド) を定義し、非公開関数 `getCost(ctx, client costExplorerAPI, opts)` へフィルタ組み立てを含む処理を移した。公開関数 `GetCost(ctx, profile, region, opts)` はシグネチャを変更せず、実クライアントを生成して `getCost` を呼ぶだけにした。Athena の `athenaAPI` / `ListAthenaCatalogs` / `listAthenaCatalogs` と同じ公開・非公開分離である。`internal/api/handlers_cost.go` と `internal/cli/cost.go` の既存呼び出しは無変更でコンパイルできる。
- フィルタ組み立ては、一方のみ指定なら `Dimensions` を直接持つ単一の `cetypes.Expression`、両方指定なら `And` に `Dimensions` を持つ 2 要素を格納した `cetypes.Expression` を `Filter` に設定する。トップレベルに `Dimensions` と `And` を同時に設定しない。`MatchOptions` は既存の `ServiceFilter` と同じ `MatchOptionEquals` である。
- `handlers_cost.go` の `handleCost` で `q.Get("account")` を `AccountFilter` に設定した。
- キャッシュキーの組み立ては `costCacheKey(profile, region, opts)` として関数へ切り出し、`IncludeToday` / `Granularity` / `GroupByDimension` / `ServiceFilter` / `AccountFilter` / `StartDate` / `EndDate` / `Months` の全条件をキーに含めた。切り出しの前は `Granularity` と `GroupByDimension` しか含んでおらず、日付レンジや `Months` を変えてもキャッシュが衝突する状態だったため、`AccountFilter` の追加だけでなく応答内容を左右する条件をすべて含める形に直した。

### frontend

- `api/endpoints.ts` の `CostQueryOptions` に `service` / `account` を追加し、`getCost` のクエリパラメータとして送るようにした。
- `api/queries.ts` の `useCost` の `queryKey` に `opts?.service` / `opts?.account` を追加した。backend の `costCacheKey` と対称に保つためである。
- `CostExplorerPanel.tsx` の GroupBy 次元に連動していた `serviceFilter` (ブラウザ側の部分一致フィルタ) を廃止し、`CostFilterInput` コンポーネントによるサービス名とアカウント ID の 2 つの独立した入力欄に置き換えた。入力中の値 (`serviceInput` / `accountInput`) と API に渡す確定値 (`serviceApplied` / `accountApplied`) を分離し、Enter の押下または入力欄からのフォーカス離脱でのみ確定させる。確定時は `value.trim()` を確定値とし、入力欄の表示値も同じ値に揃える (表示だけ空白付きで残ると実際に絞り込みへ使われる値と画面表示が食い違うため)。
- `MonthlyCostPanel.tsx` のコメントを更新した。同パネルが扱う Datadog / TiDB のコスト API は期間しか受け取らず絞り込み条件を渡す口がないため、ブラウザ側フィルタを維持する理由をコメントに明記した (「AWS Cost Explorer と同じ方式」という記述が本 issue の変更で事実に反するようになったため)。

### テスト

- `internal/aws/cost_test.go`: 手書きの `fakeCostExplorer` を追加し、`TestGetCostFilter` で `ServiceFilter` のみ / `AccountFilter` のみ / 両方 / 両方未指定 / GroupBy=LINKED_ACCOUNT と `AccountFilter` / GroupBy=SERVICE と `ServiceFilter` の 6 パターンを、`GetCostAndUsageInput.Filter` の `cmp.Diff` による構造比較でテーブル駆動検証する。あわせて `TestGetCostTimePeriod` / `TestGetCostResults` / `TestGetCostError` を追加した。
- `internal/api/handlers_cost_test.go` (新規): `TestCostCacheKeyDistinctPerField` で条件を 1 つだけ変えたときにキーが必ず変わること、`TestCostCacheKeyEscapesFilterSeparator` で絞り込み条件に `:` を含む場合にキーが衝突しないこと、`TestHandleCostUsesFilterAwareCacheKey` で HTTP ハンドラ経由でも絞り込み条件がキーに反映されることを検証する。
- `frontend/src/views/CostExplorerPanel.test.tsx`: 既存のブラウザ側フィルタのテストを置き換え、(a) 入力だけでは再呼び出ししない (b) Enter で確定し表示値も trim される (c) フォーカス離脱でも確定する (d) Enter 直後のフォーカス離脱で再呼び出しされない (e) 空文字確定で絞り込みが解除される (f) Group by の変更が確定済みフィルタを巻き込まない、の 6 点を追加した。
- `frontend/src/api/endpoints.test.ts` (新規): 空文字の `service` / `account` がクエリパラメータとして省略されずに送られること、未指定なら付与されないこと、値が URL エンコードされることを、`fetch` に渡された URL のレベルで検証する。`client.ts` の `buildUrl` が `undefined` のみを省く実装であることが「空文字は絞り込み解除として送る」という完了条件の実現部であり、これを固定するテストがリポジトリ内に存在しなかったため追加した。
- テストの検出力はミューテーションで実測した。`buildUrl` の判定を `v === undefined || v === ''` に劣化させると `endpoints.test.ts` の空文字ケースのみが落ち、Group by の変更で `serviceApplied` をリセットするよう劣化させると `CostExplorerPanel.test.tsx` の該当ケースのみが落ちる。

### 作業中に発見した別バグ

`cacheKey` (`internal/api/server.go`) が要素をエスケープせず `:` で連結しており、`:` を含む自由入力で異なる条件のキーが衝突する不具合を発見した。本 issue の完了条件に含まれないため `docs/issues/0096-bug-cache-key-collision-unescaped-parts.md` として別途登録し、そちらで修正した (`url.QueryEscape` によるエスケープ後に `strings.Join` する実装への変更、および `server_test.go` へのテスト 3 件の追加)。

### 完了条件の読み替え

完了条件の「`mise run check` が全て通過する」は、そのままでは満たせない。`mise run backend:lint` の `govulncheck` が既存の依存関係の脆弱性 `GO-2026-6061` (`google.golang.org/grpc@v1.82.0`、Fixed in v1.82.1) で必ず失敗し、これは本 issue の変更前から存在するベースラインの失敗である (対応は `docs/issues/0095-bug-grpc-vulnerable-dependency.md`)。また `staticcheck` はこの作業環境では `internal error in importing` で実行できない。

そのため本 issue では「ベースラインからの新たな失敗が無いこと」を通過条件として読み替え、以下を確認した。

- `go test -race ./...` は全パッケージ成功 (macOS のリンカ警告のみ)。
- `gofmt -l backend` は出力なし。
- `npx prettier --check src` は `All matched files use Prettier code style!`。
- `npx tsc --noEmit` はエラーなし。
- `npx eslint .` は 0 errors / 10 warnings (警告はすべて本 issue の変更前から存在する既存箇所)。
- `npx vitest run` は 71 ファイル / 638 件すべて成功。

### 対応しなかった事項

- `ServiceFilter` / `AccountFilter` の形式バリデーション (アカウント ID が 12 桁数字であるか等) は行っていない。設計判断の通り値の妥当性は Cost Explorer API の判定に委ね、frontend は自由記述のテキスト入力とする方針を維持した。
- Cost Explorer 系ハンドラの SSO トークン期限切れが 401 `SSO_TOKEN_EXPIRED` にマップされず 500 になる問題、および profile/region 切り替え時に絞り込み state が初期化されない問題は、いずれも本 issue の変更前から存在する既存挙動であり完了条件に含まれないため `docs/issues/TODO.md` へ送った。

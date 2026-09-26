# Cost Explorer の絞り込みを Service / Usage type / Linked account を横断する 1 つのキーワード入力にする

Created: 2026-09-09
Model: Claude Fable 5.1
Completed: 2026-09-09

## 背景

`docs/issues/TODO.md` の次の 2 行に対応する (原文どおりに引用。2 行目の半角カンマの後の半角スペース 2 個も原文のまま)。

- 「CostExplorer のフィルターがその時選択している Group にしか効いてなさそう。Usage type を表示している時に AWS サービス名でヒットしない」
- 「フィルターはサービス名、アカウント名でわけず取得している Service,  Usage type,  LinkedAccount 全部に効くようにしたい」

2 行は同じ絞り込み UI と同じ backend の Filter 組み立てを対象にしており、片方だけ実装しても他方の判断が変わるため 1 つの issue で扱う。backend と frontend も分割しない。クエリパラメータ名を `service` / `account` から `keyword` に変えるため、片方だけ実装すると frontend の絞り込みが backend に届かない期間ができる。

### 現状の実装

frontend `frontend/src/views/CostExplorerPanel.tsx` は、`serviceInput` / `serviceApplied` (113-114 行) と `accountInput` / `accountApplied` (115-116 行) の 2 組の state を持ち、`CostFilterInput` (61-98 行) を 2 つ並べて「filter by service name…」と「filter by account ID…」の入力欄を表示する (167-182 行)。確定値は `useCost` (118-125 行) の `service` / `account` として backend に渡す。ブラウザ側で取得済みの行を再度絞り込む処理は無く、`rows` は `data ?? []` (128 行) である。`GROUP_BY_OPTIONS` (25-30 行) は `SERVICE` / `USAGE_TYPE` / `LINKED_ACCOUNT` / `REGION` の 4 択で、`groupBy` state (104 行) はフィルタの state と独立している。

`frontend/src/api/endpoints.ts` の `CostQueryOptions` (116-125 行) と `getCost` (127-143 行) は `service` / `account` をクエリパラメータとしてそのまま送る。`frontend/src/api/queries.ts` の `useCost` (200-228 行) は `queryKey` に `opts?.service` / `opts?.account` を含める。

backend `backend/internal/api/handlers_cost.go` の `handleCost` は `q.Get("service")` / `q.Get("account")` を `CostQueryOptions.ServiceFilter` / `AccountFilter` (17-18 行) に設定し、`costCacheKey` (33-47 行) は両方をキャッシュキーに含める。`backend/internal/aws/cost.go` の `costFilter` (140-158 行) は `ServiceFilter` があれば `SERVICE`、`AccountFilter` があれば `LINKED_ACCOUNT` の `Dimensions` 式を `costDimensionFilter` (125-133 行) で作り、両方あれば `And` にまとめる。`costDimensionFilter` は次のとおり `MatchOptionEquals` (完全一致) の単一値である。

```go
func costDimensionFilter(key cetypes.Dimension, value string) cetypes.Expression {
	return cetypes.Expression{
		Dimensions: &cetypes.DimensionValues{
			Key:          key,
			Values:       []string{value},
			MatchOptions: []cetypes.MatchOption{cetypes.MatchOptionEquals},
		},
	}
}
```

`getCost` (173-228 行) は `GroupBy` (183-185 行) と `Filter` (186 行) を独立に設定し、`costFilter` は `GroupByDimension` を参照しない。`costExplorerAPI` インターフェース (70-72 行) は `GetCostAndUsage` の 1 メソッドだけを持つ。`CostQueryOptions` の godoc (45-56 行) は `ServiceFilter` / `AccountFilter` の説明を含む。

この構造は `docs/issues/closed/0094-feat-cost-explorer-account-service-filter.md` (2026-07-29 完了) で導入された。0094 以前はブラウザ側で `allRows.filter((r) => r.service.toLowerCase().includes(needle))` としており、`r.service` が GroupBy で選んだ次元の値そのものだったため、絞り込みが選択中の Group の値にしか効かなかった。

### TODO 1 行目 (Group にしか効かない) の再現性

AWS Cost Explorer 画面の経路 (`CostExplorerPanel.tsx` から `handleCost`、`getCost` まで) には「選択中の Group にしか効かない」構造は存在しない。根拠は次の 3 点。

- `frontend/src/views/CostExplorerPanel.test.tsx` の「Group by の変更は確定済みのフィルタを巻き込まない」(155-182 行) が、`groupBy` を `USAGE_TYPE` に変えても `getCost` に `service` / `account` が維持されて渡ることを固定している。
- `backend/internal/aws/cost_test.go` の「GroupBy=USAGE_TYPE と ServiceFilter は同時に指定できる」(260-269 行) が、`GroupBy[0].Key` が `USAGE_TYPE` のまま `Filter` に `SERVICE` の `EQUALS` 式が入ることを固定している。
- `costFilter` は `GroupByDimension` を読まない。

一方、`frontend/src/components/MonthlyCostPanel.tsx` の `filteredRows` (58-62 行) は `groupValueOf(r, groupBy).toLowerCase().includes(needle)` として、選択中の `groupBy` の値にだけ効くブラウザ側フィルタを今も持つ。このパネルは Datadog / TiDB の Cost タブ (`views/nonaws/DatadogView.tsx`、`TiDBView.tsx`) から使われ、AWS Cost Explorer 画面では使われない。TODO の文言は「CostExplorer」を指すため、本 issue では `MonthlyCostPanel.tsx` を扱わない。Datadog / TiDB のコスト API は期間しか受け取らず絞り込み条件を渡す口が無い (0094 の「解決方法」に記載) ため、同じ方式は使えない。

TODO の観察が 0094 以前の挙動を指しているのか、現行コードで別経路により再現するのかは、コードの読みだけでは確定できない。現行コードで「Usage type を表示している時に AWS サービス名でヒットしない」が起きる経路として特定できるのは、完全一致 (`EQUALS`、`CASE_SENSITIVE`) のため「EC2」のような略称や大文字小文字の異なる入力が `Amazon Elastic Compute Cloud - Compute` に一致しないことだけであり、これは Group の選択に依存しない。この経路は本 issue の部分一致化で解消する。

### 全次元に効かせるための制約

AWS Cost Explorer API の仕様上、次の制約がある (SDK `github.com/aws/aws-sdk-go-v2/service/costexplorer v1.63.3` のコメントで確認)。

- `api_op_GetCostAndUsage.go` の `GetCostAndUsageInput.Filter` のコメント (90 行) は「Valid values for MatchOptions for Dimensions are EQUALS and CASE_SENSITIVE」である。`types/types.go` の `DimensionValues.MatchOptions` のコメント (1033-1040 行) も「MatchOptions is only applicable for actions related to cost category and Anomaly Subscriptions」「The default values for MatchOptions are EQUALS and CASE_SENSITIVE」である。`GetCostAndUsage` の `Filter` では `CONTAINS` 等の部分一致を使えず、部分一致は API に任せられない。
- `GetCostAndUsage` の応答 `Groups[].Keys` には `GroupBy` で指定した次元の値だけが入る。`GroupBy` は `api_op_GetCostAndUsage.go` の `GroupBy` フィールドのコメント (98-99 行) どおり最大 2 つで、Service / Usage type / Linked account の 3 次元を同時に持てない。取得済みの行に対するブラウザ側の部分一致では、選択中の Group 以外の次元に効かせられない。
- `GetDimensionValues` (`api_op_GetDimensionValues.go`) は、`Dimension` (40 行) と `TimePeriod` (48 行) を必須とし、`Context` (190 行) の既定値 `COST_AND_USAGE` で `SERVICE` / `USAGE_TYPE` / `LINKED_ACCOUNT` の値一覧を返す。`LINKED_ACCOUNT` は「The description in the attribute map that includes the full name of the member account. The value field contains the Amazon Web Services ID of the member account」(102-104 行) であり、アカウント名は `DimensionValuesWithAttributes.Attributes["description"]`、アカウント ID は `Value` に入る。`SearchString` (282-283 行) のコメントは「The value that you want to search the filter values for」のみで、部分一致か、大文字小文字を区別するか、`Attributes` も検索対象かは書かれていない。応答はページングされ (`NextPageToken`、280 行)、`MaxResults` は `SortBy` 指定時のみ有効で上限 1000 (269-275 行)、`SortBy` 指定時は `NextPageToken` と `SearchString` が使えない (305-306 行)。
- `Expression` (`types/types.go` 1308-1331 行) は `And` / `Or` / `Not` / `Dimensions` のうち 1 つだけをルートに持てる。`Or` の中に複数の `Dimensions` を並べられる。
- `types/types.go` の `DimensionValues.Values` のコメント (1042-1044 行) に個数の上限は書かれていない。

リポジトリ内に `GetDimensionValues` の呼び出しは無い (`grep -rn GetDimensionValues backend/` が 0 件)。CLI `backend/internal/cli/cost.go` は `GetCost` を `IncludeToday` だけで呼び (85 行)、`service` / `account` の絞り込みを使っていない。

## 目的

Cost Explorer 画面の絞り込みを 1 つのキーワード入力にし、GroupBy の選択に関係なく、サービス名、使用タイプ名、リンクアカウントの ID と名前のいずれかにキーワードが部分一致するコストだけを表示できるようにする。

## 設計判断

### キーワードから値の集合を解決し、EQUALS の Or で絞り込む

- backend で `GetDimensionValues` を `SERVICE` / `USAGE_TYPE` / `LINKED_ACCOUNT` の 3 次元について呼び、返った値一覧に対して Go 側で大文字小文字を区別しない部分一致 (`strings.Contains(strings.ToLower(v), strings.ToLower(keyword))`) を行い、一致した値の集合を作る。`LINKED_ACCOUNT` は `Value` (アカウント ID) と `Attributes["description"]` (アカウント名) の両方を照合し、一致したら `Value` を集合に入れる。`Attributes` が `nil` または `description` キーが無い場合は `Value` だけを照合する。
- 一致した値の集合から `GetCostAndUsage` の `Filter` を組み立てる。次元ごとに `Dimensions{Key, Values: <一致した値>, MatchOptions: [EQUALS]}` を作り、一致が 1 次元だけならその `Dimensions` をルートに、2 次元以上なら `Or` にまとめる。一致が無い次元は式に含めない。
- 3 次元とも一致が無い場合は `GetCostAndUsage` を呼ばず空のスライスを返す。`Values` が空の `Dimensions` を送らないためと、結果が空になることが確定している呼び出しで課金を発生させないため。
- `GetDimensionValues` の `TimePeriod` は `costDateRange` が返す開始日と終了日をそのまま使う。期間外の値を集合に含めても結果に影響しないが、期間を揃えることで値一覧の量を抑える。
- `GetDimensionValues` は `NextPageToken` が空になるまで全ページを取得する。`SortBy` は指定しない (`SortBy` 指定時は `NextPageToken` と `SearchString` が使えない)。
- 3 次元の取得は `golang.org/x/sync/errgroup` で並列に行い、結果は次元ごとに index を専有するスライスへ書き込む (`backend/internal/gcp/cloudrun.go` 88-121 行の `jobsByLocation` と同じパターン。ただし次元数は 3 で固定のため `SetLimit` は使わない)。いずれかの次元の `GetDimensionValues` がエラーを返したら `errgroup.Group.Wait` が返す最初のエラーを `fmt.Errorf("get dimension values: %w", err)` で包んで `getCost` のエラーとして返し、`GetCostAndUsage` は呼ばない。部分的に取得できた次元だけで絞り込む (部分成功) ことはしない。一致しなかった次元が「一致無し」なのか「取得失敗」なのかを結果から区別できなくなるため。
- `fakeCostExplorer` (`cost_test.go` 141-153 行) に `GetDimensionValues` を追加する。3 つの goroutine から同時に呼ばれるため、受け取った入力の記録は `sync.Mutex` で保護した `map[cetypes.Dimension][]*costexplorer.GetDimensionValuesInput` に呼び出し順で追記する (同じ次元への 2 ページ目の呼び出しで 1 ページ目の記録が上書きされないようにするため)。応答は次元と `NextPageToken` の組をキーにした map から返し、ページングをフェイクで再現できるようにする。 次元ごとに返すエラーを `map[cetypes.Dimension]error` で注入できるようにし、特定の次元だけがエラーになるケースを再現できるようにする。既存の `err` フィールドは `GetCostAndUsage` 用のまま残す。`mise run backend:test` は `-race` 付きで、記録の競合はそこで検出される。
- 一致した値の個数に上限は設けない。`DimensionValues.Values` の上限が SDK に書かれておらず、上限を憶測で決めると本来一致する値を落とす。AWS 側に上限があり超えた場合は、`GetCostAndUsage` が返すエラーを既存の `fmt.Errorf("get cost and usage: %w", err)` (`getCost` 191 行) と `writeAWSError` の経路でそのまま frontend の `ErrorBanner` に表示する。

採らなかった案。

- `GetDimensionValues` の `SearchString` に依存する案。SDK のコメントに一致規則 (部分一致か、大文字小文字の扱い、`Attributes` を検索するか) が書かれておらず、挙動を単体テストで固定できない。Go 側で照合すれば規則をコードとテストで確定でき、アカウント名 (`Attributes["description"]`) にも確実に効く。値一覧の取得量を減らす目的で `SearchString` を併用する案は、AWS 側の一致規則が Go 側より狭いと取りこぼしが起きるため採らない。
- `GetDimensionValues` の `Filter` に `MatchOptions` (`CONTAINS` や `STARTS_WITH`) を付けた `Dimensions` を渡して値一覧を絞る案。`DimensionValues.MatchOptions` のコメントは cost category と Anomaly Subscriptions 専用と明記している。`Expression` のコメント (`types/types.go` 1302-1306 行) にある `STARTS_WITH` の例は `LINKED_ACCOUNT_NAME` を使う。しかし同ファイルの `DimensionValues.Key` のコメント (1024 行) は `LINKED_ACCOUNT_NAME` を `CostCategoryRule` 専用としている。同じ `STARTS_WITH` の例は `api_op_GetDimensionValues.go` の `Filter` フィールドのコメント (229-233 行) にも複製されている。ここでも `LINKED_ACCOUNT_NAME` は同ファイルの `Dimension` フィールドのコメント (34-35 行、綴りは `LINK_ACCOUNT_NAME`) で `CostCategoryRule` 専用とされており、`GetDimensionValues` では使えない。`GetDimensionValues` で部分一致が効く根拠が無い。
- `GetCostAndUsage` の `Filter` に `MatchOptionContains` を指定する案。`GetCostAndUsageInput.Filter` のコメントが `EQUALS` と `CASE_SENSITIVE` だけを有効値としている。
- 取得済みの行 (`Groups[].Keys`) に対するブラウザ側の部分一致。`Keys` には GroupBy の次元しか無く、0094 で廃止した「選択中の Group にしか効かない」構造に戻る。
- `GroupBy` を 2 次元にして 2 つ目の次元をブラウザ側で照合する案。最大 2 次元のため 3 次元を同時に持てず、行数が次元の組み合わせで増えて `aggregateCost` の入力形状も変わる。
- 現行の `service` / `account` の完全一致入力を残したまま、キーワード入力を追加する案。TODO は「サービス名、アカウント名でわけず」と分けない入力を求めており、3 つの入力欄は要望に反する。CLI は `service` / `account` を使っていないため、削除しても frontend 以外の呼び出し元は無い。
- 一致した値の個数に独自の上限を設けて超過時にエラーを返す案。上限値の根拠が無く、AWS 側の上限より小さく取ると一致する値を落とし、大きく取ると意味が無い。

### API とキャッシュ

- `CostQueryOptions` の `ServiceFilter` / `AccountFilter` を削除し、`Keyword string` を追加する。godoc (45-56 行) の `ServiceFilter` / `AccountFilter` の説明を `Keyword` の説明 (3 次元への部分一致、空白のみは絞り込みなし) に置き換える。`handleCost` は `q.Get("keyword")` を `Keyword` に設定し、`service` / `account` パラメータは受け取らない。`costCacheKey` の `ServiceFilter` / `AccountFilter` を `Keyword` に置き換える。旧形式のキーは生成されなくなるだけで衝突はせず、TTL (1 時間) で消える。
- `GetDimensionValues` の結果は個別にキャッシュしない。キャッシュは既存どおり `costCacheKey` による `GetCostAndUsage` の結果 (キーワードを含む条件の組) だけとする。キーワードを新しく確定するたびに、3 次元分の `GetDimensionValues` と 1 回の `GetCostAndUsage` が呼ばれる。`internal/aws` にはキャッシュ機構が無く、`internal/api` の `resourceCache` (`cache.Cache[any]`、`server.go` 34 行) は HTTP ハンドラ層のものであるため、値一覧のキャッシュを入れるには `internal/aws` に新しい保持機構を足すか、ハンドラで値一覧の取得を組み立てる必要がある。どちらも現時点の要望に無い複雑さであり、呼び出し回数が問題になった時点で `resourceCache` に `cacheKey("cost-dimension-values", profile, start, end, dimension)` で入れる案を検討する。
- `Keyword` は `strings.TrimSpace` した結果が空なら絞り込みなしとして扱う。frontend の `CostFilterInput` は確定時に `value.trim()` している (`CostExplorerPanel.tsx` 74-78 行) ため空白のみの値は届かないが、`CostQueryOptions` を直接組む呼び出し元に対する二重防御として backend でも除く。現行の「空白のみの ServiceFilter は絞り込みありとして扱う」(`cost_test.go` 287 行) は、空白のみのキーワードが `GetDimensionValues` の全値に一致して無意味な `Or` を作るため、逆の扱いにする。

### frontend

- `CostExplorerPanel.tsx` の 2 つの `CostFilterInput` を 1 つにし、state を `keywordInput` / `keywordApplied` にする。placeholder は「filter by service / usage type / account…」、title は「Filter by service name, usage type, account ID or name (press Enter to apply)」とする。現行の placeholder と同じく英語のハードコードとし i18n には載せない。
- `CostQueryOptions` (`endpoints.ts`) の `service` / `account` を `keyword` に置き換え、`getCost` はクエリパラメータ `keyword` を送る。`useCost` の `queryKey` も `opts?.keyword` に置き換える。
- Enter とフォーカス離脱で確定する `CostFilterInput` の挙動と、リージョン切り替えで state を初期値に戻す挙動は維持する。

### 権限

- 新たに `ce:GetDimensionValues` の権限が必要になる。既存の `ce:GetCostAndUsage` と同様に Cost Explorer の読み取り権限であり、権限が無い場合は上記のとおり `getCost` がエラーを返し、`GetCostAndUsage` と同じ経路 (`writeAWSError`) で frontend にエラーが届く。
- `GetDimensionValues` は `GetCostAndUsage` と同じくリクエスト単位で課金される Cost Explorer API である。キーワードを確定するたびに、`GetCostAndUsage` の結果がキャッシュに無ければ 3 次元分 (ページングがあればその分) の `GetDimensionValues` 呼び出しが増える。ページ数はキーワードに依存せず期間内の値の総数で決まる (Go 側で照合するため全ページを取得する)。`SortBy` を指定しない場合のページサイズは SDK コメントに書かれていない。各次元の値の総数もコードからは分からないため、複数ページになるかは実 API で確認するまで確定しない (仮説として、サービスごとに複数の使用タイプを持つ `USAGE_TYPE` は 3 次元の中で値の数が最も多くなる)。複数ページになる場合、期間が長いほどページ数と課金が増える。独自上限を設けないため、短いキーワードでは一致した値の数だけ `Values` が長くなり、`GetCostAndUsage` のリクエストサイズも増える。

## 完了条件

- `backend/internal/aws/cost.go` の `CostQueryOptions` から `ServiceFilter` / `AccountFilter` が削除され、`Keyword` が追加され、godoc が `Keyword` の説明に更新されている。
- `costExplorerAPI` インターフェースに `GetDimensionValues` が追加され、`fakeCostExplorer` がそれを実装し、複数 goroutine からの呼び出しの記録を `sync.Mutex` で保護した次元ごとのスライスに呼び出し順で残している。
- `Keyword` が空 (空白のみを含む) のとき、`GetDimensionValues` を呼ばず `Filter` を `nil` にすることを単体テストで固定している。
- `Keyword` が `SERVICE` の値だけに一致するとき、`Filter` が `Dimensions{Key: SERVICE, Values: <一致した値>, MatchOptions: [EQUALS]}` をルートに持つことを単体テストで固定している。
- `Keyword` が 2 次元に一致するとき、`Filter` が `Or` に次元ごとの `Dimensions` を持ち、一致の無い次元を含まないことを単体テストで固定している。
- `Keyword` が 3 次元すべてに一致するとき、`Or` が 3 要素になることを単体テストで固定している。
- `Keyword` が `LINKED_ACCOUNT` の `Attributes["description"]` (アカウント名) にだけ一致するとき、`Values` に対応する `Value` (アカウント ID) が入ることを単体テストで固定している。
- `LINKED_ACCOUNT` の値に `Attributes` が無い (nil) とき、panic せず `Value` だけを照合することを単体テストで固定している。
- 照合が大文字小文字を区別しないこと (例: キーワード「ec2」が「AmazonEC2」に一致し、「Amazon Elastic Compute Cloud - Compute」に一致しない) を単体テストで固定している。
- `GroupByDimension` が `USAGE_TYPE` で `Keyword` がサービス名に一致するとき、`GroupBy[0].Key` が `USAGE_TYPE` のまま `Filter` に `SERVICE` の `Dimensions` が入ることを単体テストで固定している。
- `GroupByDimension` が `REGION` で `Keyword` がサービス名とアカウント ID の両方に一致するとき、`GroupBy[0].Key` が `REGION` のまま `Filter` が `Or` になることを単体テストで固定している (現行の「GroupBy=REGION と ServiceFilter/AccountFilter の両方は同時に指定できる」の置き換え)。
- 3 次元とも一致が無いとき、`GetCostAndUsage` を呼ばずに空のスライスと `nil` エラーを返すことを、フェイクの `gotInput` が `nil` のままであることで固定している。
- `GetDimensionValues` の応答が複数ページのとき、`NextPageToken` が空になるまで取得し全ページの値を照合対象にすることを単体テストで固定している。
- 3 次元のうち 1 つの `GetDimensionValues` がエラーを返したとき、`getCost` が `errors.Is` でそのエラーに到達できるエラーを返し、`GetCostAndUsage` を呼ばないことを単体テストで固定している。
- `handleCost` が `keyword` クエリパラメータを `Keyword` に渡し、`service` / `account` を参照しないこと、`costCacheKey` が `Keyword` だけが異なるリクエストで異なるキーを返すことを `handlers_cost_test.go` で固定している。
- `frontend/src/views/CostExplorerPanel.tsx` のフィルタ入力欄が 1 つになり、確定値が `getCost` の `keyword` として渡ることを `CostExplorerPanel.test.tsx` で固定している。既存の「サービス名フィルタ」「アカウント ID フィルタ」を前提にしたテストは `keyword` を前提にしたものに置き換え、「Group by の変更は確定済みのフィルタを巻き込まない」と「リージョン切り替えで state が初期値に戻る」の 2 つは `keyword` 版として残す。
- `endpoints.test.ts` で `keyword` が空文字でも省略されずに送られること、未指定なら付与されないことを固定している。
- `CHANGES.md` の `## develop` に `[CHANGE]` として、`service` / `account` クエリパラメータの廃止と `keyword` への置き換えが記載されている。
- `REGION` はキーワードの照合対象に含めない (TODO の要望は Service / Usage type / LinkedAccount の 3 次元)。CLI (`backend/internal/cli/cost.go`) の絞り込み追加、`MonthlyCostPanel.tsx` (Datadog / TiDB) の絞り込み変更、一致した値の個数に対する独自上限の追加は扱わない。
- `mise run check` が通過する。

## 未確定論点

- `DimensionValues.Values` に入れられる値の個数の上限が SDK のコメントに無い。短いキーワード (例: 「a」) は多くの値に一致しうる (一致する割合は実測していない)。本 issue の実装は上限の有無に依存しない (上限超過時は AWS のエラーをそのまま表示する)。実装時に実 API で短いキーワードを試し、上限エラーが観測されたらエラーメッセージと一致件数を「解決方法」に記録し、キーワードの絞り込みを促す表示や独自上限の追加は別 issue として起票する。

## 関連

- `docs/issues/closed/0094-feat-cost-explorer-account-service-filter.md`: 現行の `ServiceFilter` / `AccountFilter` と `CostFilterInput` を導入した issue。本 issue はその 2 入力を 1 つのキーワード入力に置き換える。
- `docs/issues/closed/0096-bug-cache-key-collision-unescaped-parts.md`: `cacheKey` のエスケープ。`Keyword` を含むキーもこのエスケープの対象になる。

## 解決方法

### backend

`backend/internal/aws/cost.go`

- `CostQueryOptions` から `ServiceFilter` / `AccountFilter` を削除し、`Keyword` を追加した。godoc を `Keyword` の意味 (空文字と空白のみは絞り込みなし、3 次元に対する大文字小文字を区別しない部分一致、LINKED_ACCOUNT はアカウント ID とアカウント名の両方が照合対象) に更新した。
- `costExplorerAPI` インターフェースに `GetDimensionValues` を追加した。
- `costKeywordDimensions` (SERVICE / USAGE_TYPE / LINKED_ACCOUNT) と `costAccountNameAttribute` (`description`) を追加した。
- `costDimensionValues` は `NextPageToken` が空になるまで `GetDimensionValues` を呼び、全ページの次元値を返す。`SortBy` は指定しない (指定すると `NextPageToken` によるページングが使えない)。
- `costMatchDimensionValues` は `strings.ToLower` と `strings.Contains` で部分一致を判定する。LINKED_ACCOUNT では `Attributes["description"]` (アカウント名) も照合し、一致した場合も `Values` にはアカウント ID (`Value`) を入れる。`Attributes` が nil でも nil マップの索引で false が返るため panic しない。
- `costResolveKeyword` は `errgroup` で 3 次元の取得を並列に行う。各 goroutine は結果スライスの自分の添字だけに書き込むため排他は不要。1 つでも失敗した場合は `get dimension values: %w` でラップして返し、部分的な結果で絞り込まない。
- `costKeywordFilter` は一致した次元の数で分岐する。0 なら nil、1 なら `Dimensions` をルートに持つ `Expression`、2 以上なら `Or` に次元ごとの `Dimensions` を並べる (`Expression` のルートには And / Or / Not / Dimensions のいずれか 1 つしか置けないため)。
- `costFilter` は `strings.TrimSpace` 後が空なら絞り込みなし (`nil, false, nil`)、どの次元にも一致しなければスキップ (`nil, true, nil`) を返す。
- `getCost` はスキップのとき `GetCostAndUsage` を呼ばずに nil スライスを返す。結果が空と確定しているため、リクエストごとに課金される API を呼ばない。frontend は `apiGetList` が JSON の null を `[]` に正規化するため、既存の空応答と同じ扱いになる。

`backend/internal/api/handlers_cost.go`

- `handleCost` が `q.Get("service")` / `q.Get("account")` の代わりに `q.Get("keyword")` を `Keyword` に渡すようにした。
- `costCacheKey` の構成要素を `ServiceFilter` / `AccountFilter` から `Keyword` に置き換えた。

### frontend

- `frontend/src/views/CostExplorerPanel.tsx`: `serviceInput` / `serviceApplied` / `accountInput` / `accountApplied` の 4 つの state を `keywordInput` / `keywordApplied` の 2 つに置き換え、`CostFilterInput` を 1 つにした。placeholder は `filter by service / usage type / account…`、title は `Filter by service name, usage type, account ID or name (press Enter to apply)`。Enter とフォーカス離脱で確定する挙動は変えていない。
- `frontend/src/api/endpoints.ts`: `CostQueryOptions` の `service` / `account` を `keyword` に置き換え、`getCost` が `keyword` クエリパラメータを送るようにした。
- `frontend/src/api/queries.ts`: `useCost` の `queryKey` の `opts?.service` / `opts?.account` を `opts?.keyword` に置き換えた。

### テスト

- `backend/internal/aws/cost_test.go`: `fakeCostExplorer` に `GetDimensionValues` を実装した。並列呼び出しの記録は `sync.Mutex` で保護した次元ごとのスライス (`gotDimInputs`) に呼び出し順で残す。応答は `dimensionPage{dim, token}` をキーにしたマップで次元ごと・ページごとに差し替えられる。`TestGetCostFilter` を `Keyword` 前提のテーブルに置き換え (レビュー指摘により「LINKED_ACCOUNT 以外の次元では Attributes を照合しない」ケースを追加)、`TestGetCostKeywordNoMatch` / `TestGetCostKeywordPaging` / `TestGetCostDimensionValuesError` を追加した。
- `backend/internal/aws/costexplorer_test.go`: `recordingCostExplorer` に、呼ばれたらエラーを返す `GetDimensionValues` を追加した (この 4 経路は次元値を解決しないため)。
- `backend/internal/api/handlers_cost_test.go`: キャッシュキーのテーブルとエスケープのテストを `Keyword` 前提に置き換え、`TestHandleCostIgnoresLegacyFilterParams` を追加して `service` / `account` を読まないことを固定した。
- `frontend/src/views/CostExplorerPanel.test.tsx` と `frontend/src/api/endpoints.test.ts` を `keyword` 前提に置き換えた。

### 未確定論点の扱い

`DimensionValues.Values` に入れられる値の個数の上限は確認していない。Cost Explorer の API はリクエストごとに課金されるため、実 API を呼ばない方針をユーザーが選択した。論点は未解決のまま残す。本実装は上限の有無に依存せず、上限を超えた場合は AWS が返すエラーがそのまま表示される。

### 検証

`mise run check` が通過する (backend の `go vet` / `staticcheck` / `govulncheck` / `go test -race`、frontend の eslint / `tsc --noEmit` / vitest 75 ファイル 787 テスト)。実装前のベースラインにも失敗は無かったため、読み替えは行っていない。

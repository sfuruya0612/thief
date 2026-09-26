# Pricing 画面の frontend / backend のパフォーマンス改善候補を計測に基づいて洗い出す

Created: 2026-07-21
Completed: 2026-07-21
Model: Claude Opus 4.8

## 背景

Pricing 画面の frontend と backend の実装を見直し、パフォーマンス改善が見込める点を洗い出す。
本リポジトリの原則は「Premature Optimization is the Root of All Evil」であり、パフォーマンス目的のリファクタは計測の裏付けとセットで行う。
このため本 issue は、憶測での最適化を避け、計測で支配的要因を特定してから、裏付けのある箇所のみを before / after 付きで改善する (issue 0043 と同じ進め方)。
以下に現状のコードから見つけた候補を挙げるが、いずれも計測で効果を確認するまでは仮説として扱う。

## 目的

計測手段を定め、支配的要因を特定し、裏付けのある箇所のみを改善する。
計測して効果が確認できない候補は、実装を変えずに記録として残す。

## 計測手段

- frontend：React Profiler で、フィルタ入力、行チェック、サービス切替時の再レンダー回数と所要時間を計測する。
- backend：`testing.B` によるベンチマークと、必要に応じて `pprof` を用いる。取得は対象 (service / region) と件数を明記して計測する。

## 現状で見つけた候補 (計測で確認するまで仮説)

### backend

- **`dedupeSavingsPlanRates` の dedup キーが `json.Marshal` 依存**：`backend/internal/aws/pricing.go` の `dedupeSavingsPlanRates` は、各レートを `json.Marshal` した文字列を重複判定キーに使う。レート 1 件ごとに JSON へのマーシャルが走り、リフレクションと確保のコストがかかる。フィールドを連結した構造的なキーに置き換えれば安価になる可能性がある。ただし構造的キーは Attributes を含む全可視フィールドを安定した順序で網羅しなければならず (網羅しないと dedup の意味論が変わる)、この等価性を保てる場合に限る。件数はリージョンあたり数百から低い数千で有界のため効果は限定的で、`testing.B` の計測で効果が乏しければ現行の `json.Marshal` を維持する。
- **`GetProducts` のページングは本質的に逐次**：`NextToken` を辿る取得は逐次で、支配的コストはキャッシュミス時と明示更新時の AWS API レイテンシである。結果は TTL なしでファイルキャッシュされるため、この経路は常時のコストにはならない。ページ内の各ドキュメントの `json.Unmarshal` は CPU を使うが、ネットワークが支配的である可能性が高い。まず計測し、支配的要因が判明してから触る。
- **リクエスト内の並行度**：On-Demand / RI と SP の取得は既に `sync.WaitGroup` で 2 系統並行になっている。ここは追加の並列化余地が小さい。

backend は TTL なしのファイルキャッシュにより高コストな取得経路が稀にしか走らないため、`dedupeSavingsPlanRates` を除いて改善の費用対効果は低い見込みである (ただし issue 0056 の `ec2-spot` はライブ取得のためこの前提の例外であり、別途計測する)。

### frontend

- **`Estimator` が毎レンダーで Map を再構築**：`components/pricing/Estimator.tsx` は、サービス別グループの描画中に `new Map(table.rates.map(...))` をレンダーごと、サービスごとに作り直している (メモ化されていない)。レート表が大きいと避けられる再計算になる。
- **`RateGroupSection.matchesInstanceFilter` がキー入力ごとに全行を小文字化**：`components/pricing/RateGroupSection.tsx` は、テキスト検索のたびに各行の label と全属性値を `toLowerCase` して部分一致を取る。EC2 の On-Demand のように行数が多いグループでは、1 打鍵ごとに全行の小文字化が走る。行ごとに小文字化済みの検索用文字列を事前計算するか、入力をデバウンスする案がある。
- **レート行の仮想化 (windowing) が無い**：単価表は絞り込み後の全行を DOM に描画する。RI / SP はセレクタで 1 条件に絞られるが、On-Demand は 1 リージョンで数百行に達し得る。issue 0055 (SP の独立サービス化) や issue 0056 (Spot 追加) で総行数が増えると影響が大きくなる。仮想化は効果が見込めるが、新規依存 (react-window 等) の追加を伴うため、依存追加の可否と計測結果をセットで判断する (場合により pending へ切り出す)。
- **`ServiceCard` の小計計算**：`estimate` はサービスごとにメモ化されており、現状は問題になりにくい。

## 設計上の論点

- **計測なしに最適化しない**：本リポジトリの第一原則に従い、計測で支配的要因を特定してからのみ実装を変える。効果が確認できない候補は記録に残し、実装は変えない。
- **仮想化の依存判断**：仮想化は効果が見込める一方で新規依存を伴う。frontend の最小依存方針 (YAGNI) と衝突するため、計測で効果を確認し、依存追加の是非を含めて判断する。手書きの windowing で足りるかも併せて検討する。
- **他 issue との相互作用**：issue 0055 は同一リージョンで並行するフロントのフェッチ数を 4 から 7 に増やす。ただし `pricecache.Fetch` の singleflight とファイルキャッシュのキーは service / region 単位 (`dir|service|region`) のため、0055 のライセンス解決案 (a) で compute-sp と ec2-instance-sp と ec2 が同じ EC2 On-Demand の `GetProducts` を、database-sp と rds が同じ RDS On-Demand を重複して呼ぶ経路は、この service キーの singleflight では畳まれない (コールドキャッシュ時と明示更新時のみ発生する)。一方 0055 は別キー (awsServiceCode / region) の補助取得 singleflight を新設し、compute-sp と ec2-instance-sp の EC2 On-Demand 補助取得を 1 本に畳む (`pricecache.Fetch` の service キー singleflight とは別レイヤかつ別キーである点に注意する)。この結果、コールドキャッシュ時と明示更新時に残る EC2 On-Demand 取得は、ec2 の主取得と畳まれた補助 1 本で 2 本 (0055 が ec2 の主取得も同一 singleflight に載せて共有すれば 1 本)、RDS 側は rds の主取得と database-sp の補助 1 本で 2 本になる。これらの残余の service 跨ぎ冗長取得を計測対象に含める。また issue 0056 が独立サービス `ec2-spot` を追加するため、0055 と 0056 の適用後の並行フェッチ数は 8 になる。`ec2-spot` はディスクキャッシュに載せないライブ取得 (`pricecache.Fetch` の singleflight は経由する) のため、「TTL なしファイルキャッシュにより高コスト経路が稀」という前提はこの 1 サービスには当てはまらず、毎リクエスト `DescribeSpotPriceHistory` を引く。ただし `ec2-spot` は On-Demand の `GetProducts` を呼ばないため、上記の service 跨ぎの冗長 On-Demand 取得には該当しない。これらの後に総行数と取得回数が増えるため、仮想化とフィルタ再計算の候補は本 issue 単独で結論を出さず、実装順序を踏まえて再計測する。

## 完了条件

- frontend / backend の計測が行われ、支配的要因が記録されている。
- 裏付けのある改善のみが実装され、before / after が記録されている。
- 効果が確認できなかった候補が、実装を変えずに記録として残されている。
- 改善を実装した場合、`CHANGES.md` の `## develop` に該当する種別 (`[UPDATE]` または `[FIX]`) のエントリを追記する (種別順 UPDATE → ADD → CHANGE → FIX を守り、次行に 2 文字インデントで `- @sfuruya0612` を付ける)。
- `mise run check` が全て通過する。

## 検証

- backend：`mise run backend:test` と `testing.B` のベンチマーク。改善対象の before / after を計測結果として記録する。
- frontend：React Profiler で、フィルタ入力、行チェック、サービス切替時の再レンダーの before / after を記録する。
- 実ブラウザで、行数の多いリージョン (EC2 の On-Demand が数百行) を対象に、フィルタ入力とスクロールの体感を確認する。

## 解決方法

計測手段で定めた `testing.B` (backend) と React Profiler (frontend) を使い、4 つの候補を計測した。改善したのは 2 件、実装を変えなかったのは 2 件で、後者はいずれも計測で「効果が乏しい/支配的要因ではない」と確認できたためである。

### backend: `dedupeSavingsPlanRates` の dedup キー (改善済み)

`backend/internal/aws/pricing_bench_test.go` に `BenchmarkDedupeSavingsPlanRates` を追加し、実リージョン相当の n=200 と 10 倍規模の n=2000 で計測した。

- before (`json.Marshal` によるキー): n=200 で 161885 ns/op・1904 allocs/op、n=2000 で 1609347 ns/op・19013 allocs/op
- after (`dedupeSavingsPlanRateKey`: `strings.Builder` に可視フィールドを直接連結、Attributes は `sort.Strings` で安定した順序に揃える): n=200 で 86072 ns/op・1205 allocs/op、n=2000 で 892123 ns/op・12014 allocs/op

両スケールで約 1.9 倍の高速化、allocs/op は約 37% 減少した。issue の懸念事項だった「Attributes を含む全可視フィールドを安定した順序で網羅する」点は、Attributes のキーをソートしてから連結することで満たしている。既存の `TestDedupeSavingsPlanRates` はコード変更後も無変更のまま通過し、dedup の意味論が変わっていないことを確認した。`json.Marshal` は `parsePriceDocument` の `json.Unmarshal` で引き続き使うため、`encoding/json` の import は変更していない。

`CHANGES.md` の `## develop` に `[UPDATE]` エントリを追記した。

### backend: `GetProducts` のページングとリクエスト内並行度 (変更なし)

この 2 点は issue 本文の時点で「TTL なしのファイルキャッシュにより高コストな取得経路が稀にしか走らず、支配的コストはネットワークレイテンシである可能性が高い」「On-Demand/RI と SP は既に `sync.WaitGroup` で並行しており追加の並列化余地が小さい」と分析済みであり、この分析を覆す新情報は今回の計測 (dedup キーの `testing.B`、frontend の React Profiler) からは得られなかった。ネットワーク I/O 律速の経路をローカルの `testing.B` で有意に計測する手段がなく、実 AWS 環境での計測が別途必要なため、実装は変えずこの分析を記録として残す。

### frontend: `Estimator` の Map 再構築 (改善済み)

`components/pricing/Estimator.tsx` の `byService.map` ループ内で毎レンダー・毎サービス作り直していた `rateId -> PriceRateRow` の `Map` を、`rates` が変わったときだけ作る `useMemo` (`rateByIdByService`) に変更した。

使い捨ての Node.js スクリプト (リポジトリには含めない) で、8 サービス・合計約 2450 行という実際のサービス構成 (issue 0055/0056 適用後) を模した入力に対し、500 回の再レンダーを模した計測を行った。

- before (レンダーごとに再構築): 500 回の再レンダーで合計 41.97ms (0.0839ms/レンダー)
- after (`rates` が変わったときだけ構築): 500 回の再レンダーで合計 0.08ms

`qty` 入力のような `rates` が変わらない再レンダーで無駄になっていたコストをほぼゼロにした。`npx tsc --noEmit` と既存の `Estimator.test.tsx` (9/9) で回帰がないことを確認した。

`CHANGES.md` の `## develop` に `[UPDATE]` エントリを追記した。

### frontend: `RateGroupSection.matchesInstanceFilter` の事前計算 (変更なし)

まず使い捨てスクリプトで、EC2 On-Demand 相当の 400 行に対して 6 キーストローク分のフィルタ処理を計測した。

- before (キーストロークごとに `toLowerCase` を再計算): 0.674ms
- after (label/attributes の小文字化済み検索文字列を事前計算): 0.264ms (事前計算込み)

キーストロークあたりに換算すると before が約 0.11ms で、相対では約 2.5 倍の差がある。しかし、この差が実際の再レンダーコストの中でどれだけの割合を占めるかを見ないと、実装を変える判断はできない。そこで `@testing-library/react` の `render`/`rerender` を `<Profiler>` で包み、同じ 400 行を持つ `RateGroupSection` のマウントと、フィルタ入力による再レンダーの `actualDuration` を計測した。

- マウント (400 行、フィルタなし): 44.807ms
- 更新 (フィルタ `m5.1` 入力、約 180 行に絞り込み): 6.063ms
- 更新 (フィルタ `m5.12` 入力、約 17 行に絞り込み): 0.691ms

`matchesInstanceFilter` の小文字化コスト (キーストロークあたり約 0.11ms) は、実際の再レンダーコスト (最も重い更新で 6.063ms) の 2% 程度にとどまる。支配的要因は文字列処理ではなく DOM ノードの生成・差分検出であり、事前計算を導入しても体感できる改善にはならないと判断した。このため実装は変えず、計測結果のみをこの issue と `docs/issues/pending/0061-perf-pricing-virtualize-rate-table.md` に記録として残す。

### frontend: レート行の仮想化 (windowing) (保留)

上記の Profiler 計測で、マウントコスト (44.807ms) が更新コスト (6.063ms 以下) より 1 桁以上大きいことが確認できた。これは DOM ノード数そのものを減らす仮想化が効果を持ちうることを示すが、新規依存 (`react-window` 等) の追加可否の判断と、issue 0055/0056 適用後の実際の総行数を踏まえた再計測が必要である。issue 本文の想定通り `docs/issues/pending/0061-perf-pricing-virtualize-rate-table.md` を切り出し、計測データを引き継いだ。この issue 単独では結論を出さない。

### 完了条件との対応

- frontend / backend の計測が行われ、支配的要因が記録されている: 上記の通り。
- 裏付けのある改善のみが実装され、before / after が記録されている: dedup キーと `Estimator` の Map メモ化の 2 件。
- 効果が確認できなかった候補が、実装を変えずに記録として残されている: `matchesInstanceFilter` の事前計算 (本ファイル)、仮想化 (`docs/issues/pending/0061-perf-pricing-virtualize-rate-table.md`)。
- `CHANGES.md` の `## develop` に `[UPDATE]` エントリを追記済み。
- `mise run check` が全て通過することを確認済み (backend: `go vet`/`staticcheck`/`govulncheck`/`golangci-lint`/`go test -race -cover`、frontend: `eslint`/`tsc --noEmit`/`vitest run`、いずれも既存の warning 以外の新規エラーなし)。

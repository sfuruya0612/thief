# オブジェクト SQL 検索の結果を ECharts でグラフ表示する

Created: 2026-09-24
Model: Claude Fable 5.1
Completed: 2026-09-26

## 背景

TODO.md の次の項目に由来する。

> Frontend で Object Storage の Object を DuckDB Wasm + OPFS で検索、可視化できるようにしたい。
> - 検索可能なデータ容量は 1 GB までとしておき、これは環境変数で調整可能。
> - 対象とすファイルは csv,  parquet, jsonl, json など、AWS, Google Cloud の Managed サービスが出力する拡張子で代表的なものとする

本 issue は「可視化」(クエリ結果のグラフ表示) を扱う。
「検索」(取り込み、SQL の実行、結果の表表示、容量上限) は docs/issues/0196 で扱い、本 issue はその完了後に着手する。

docs/issues/0196 が追加する `frontend/src/components/Drawer/DrawerObjectQuery.tsx` の `DrawerObjectQuery` は、クエリ結果を `components/query/ResultTable.tsx` の `ResultTable` (`columns: string[]`、`rows: string[][]`) で表として表示するだけで、グラフは持たない。

frontend の既存グラフは用途が固定されている。

- `frontend/src/components/charts/TimeseriesChart.tsx` の `TimeseriesChart` は `series: TimeseriesSeries[]` (`lib/timeseries.ts`、`name` と時刻 / 値の `points`) を受け取る時系列専用で、X 軸は時刻、`xRange` で時間窓を固定する。
- `frontend/src/components/charts/CostChart.tsx` の `CostChart` は Cost Explorer の集計結果専用である。
- どちらも「任意の列を X 軸と Y 軸に選ぶ」入力を持たないため、クエリ結果の `string[][]` をそのまま渡せない。
- ダークテーマ用の文字色 `THEME_TEXT_COLOR` (dark `#a8a8b0`、light `#5c5c66`) は `TimeseriesChart.tsx` と `CostChart.tsx` の両方に非公開の `const` として重複定義されており、他のファイルから import できない。

`ResultTable` は数値セルの判定に非公開の `isNumericCell` (正規表現 `/^-?[\d,]+(\.\d+)?$/`、カンマ区切りを許容) と `parseNumericCell` (カンマを除いて `Number()`) を持ち、数値列は「非空のセルが 1 つ以上あり、非空のセル全てが `isNumericCell` を通る列」(`numericCols`) と定めている。どちらも export されていない。

グラフ描画ライブラリ `echarts` と `echarts-for-react` は `frontend/package.json` に既にあり、新規依存は要らない。

## 目的

`DrawerObjectQuery` のクエリ結果を、列を選んで棒グラフまたは折れ線グラフで見られるようにする。

## 設計判断

### 1. 汎用の `QueryResultChart` を新設する

`frontend/src/components/charts/QueryResultChart.tsx` (新規) に、`columns: string[]`、`rows: string[][]`、X 列の index、Y 列の index の配列、グラフ種類を受け取るコンポーネントを置く。既存の `TimeseriesChart` と同じく `echarts-for-react` でラップし、系列の色は `lib/timeseries.ts` の `SERIES_COLORS` を使う。

文字色は `THEME_TEXT_COLOR` を `frontend/src/components/charts/chartTheme.ts` (新規) に移して export し、`TimeseriesChart.tsx` と `CostChart.tsx` の重複定義をこの import に置き換える。`QueryResultChart` も同じ値を `textStyle.color`、`legend.textStyle.color`、`xAxis.axisLabel.color`、`yAxis.axisLabel.color` に使う (`TimeseriesChart` と同じ箇所)。

- 却下: `TimeseriesChart` の拡張。X 軸が時刻前提で、`xRange` や時間窓の補正が文字列のカテゴリ軸と両立しない。
- 却下: `CostChart` の流用。Cost Explorer の集計形状に固定されている。
- 却下: `THEME_TEXT_COLOR` を 3 つ目の重複として `QueryResultChart.tsx` にも定義する。値の変更漏れが起きる。

### 2. 入力は `string[][]` のまま受け取り、数値判定は `ResultTable` と共有する

`DrawerObjectQuery` が持つ結果 (`string[][]`) をそのまま渡し、`lib/queryChart.ts` (新規) の純関数で系列に変換する。

- `isNumericCell` と `parseNumericCell` を `ResultTable.tsx` から `frontend/src/lib/numericCell.ts` (新規) に移して export し、`ResultTable.tsx` はそこから import する。判定基準は変えない (カンマ区切りを許容し、指数表記と前後の空白は非数値)。
- Y 列の候補は、非空のセルが 1 つ以上あり、非空のセル全てが `isNumericCell` を通る列とする (`ResultTable` の `numericCols` と同じ定義)。値は `parseNumericCell` で数値にする。空文字列 (docs/issues/0196 の変換規則で `null` に対応する) は欠損とし、その点を描かない (`parseNumericCell` の戻り値 0 は使わない)。
- X 列は任意の列を選べる。値は文字列のままカテゴリ軸に置く。
- 却下: `Number()` で有限になる値を数値とする独自の判定。`ResultTable` の数値ソートと Y 列候補の判定がずれ、`1,234` は表では数値なのにグラフでは非数値になる。
- 却下: docs/issues/0196 の結果に Arrow の型情報を保持させて渡す。0196 の結果表現が `string[][]` に決まっており、型情報を持たせると 0196 の変換層と `ResultTable` の両方を変えることになる。

### 3. グラフ種類は棒と折れ線の 2 つにする

集計は SQL 側で行う前提で、描画は `bar` と `line` の切り替えだけを持つ。

- 却下: 円グラフ、散布図。TODO の要望に無く、追加すると軸選択の UI が種類ごとに分岐する。必要になれば別 issue で扱う。

### 4. 軸の初期選択と描けない場合の扱い

結果が表示されたとき、X 列は先頭の列、Y 列は先頭の数値列 (設計判断 2 の基準) を初期選択にする。

- 結果の列が 0 個、または数値列が無い結果では、Chart への切り替えを無効化し、`title` に理由 (列が無い、数値列が無い) を出す。
- 列が 1 個で数値列のときは X と Y が同じ列になる。これを許容する (X はその値をカテゴリとして、Y は同じ値を高さとして描く)。
- Y 列の選択を全て外したときは、グラフの代わりに Y 列を選ぶよう促す文を出す。

### 5. 点数の上限

X 列の distinct な値の数が 5,000 を超える場合はグラフを描かず、SQL で集計または `LIMIT` するよう促す文を出す。行数ではなく distinct 数で判定する (同じ値が繰り返す結果は少数のカテゴリに集約されるため)。docs/issues/0196 の結果上限 10,000 行の範囲でも、カテゴリ 10,000 本の棒グラフは判読できないためである。

### 6. 配置と文言

`DrawerObjectQuery` の結果部に Table / Chart の切り替え (`app.css` の `.seg`) を置く。Chart を選んだときに X 列、Y 列 (複数選択)、グラフ種類の選択と `QueryResultChart` を表示する。クエリを再実行したら軸の選択は設計判断 4 の初期選択に戻す (列構成が変わりうるため)。

Table / Chart の切り替えのラベル、軸選択のラベル、Chart 無効の理由、Y 列未選択の案内、カテゴリ数超過の案内は `drawerStorage` ネームスペースの `drawerObjectQuery.chart.*` に置き、`frontend/src/i18n/locales/ja/drawerStorage.json` と `locales/en/drawerStorage.json` の両方に載せる。グラフ種類の値 `bar` / `line` は ECharts の型名として英語のままとする。

### 7. テスト

- `lib/queryChart.ts` の純関数 (数値列の判定、系列への変換、欠損の扱い、distinct 数上限の判定) と、移した `lib/numericCell.ts` を単体テストで固定する。`ResultTable` の既存テストは変更なしで通ること。
- `QueryResultChart` は `echarts-for-react` をモックし、渡された option の `xAxis.data`、`series` の `type` と `data`、`textStyle.color` / `legend.textStyle.color` / `axisLabel.color` がテーマごとに `THEME_TEXT_COLOR` の値になることを検証する。`echarts-for-react` をモックして `option` を捕まえる方法は `TimeseriesChart.test.tsx` にあるが、テーマごとの色を検証するテストは既存に無い。テーマは `hooks/useTweaks.ts` のモジュール共有ストアから読まれるため、テストでは `resetTweaksForTest()` でストアを未初期化に戻したうえで、`localStorage` の `cloudlens:v1` に `tweaks.theme` を `dark` または `light` で書いてから描画する (`useTweaks.test.tsx` がこの手順で永続化からの復元を検証している)。
- `DrawerObjectQuery` のコンポーネントテストに Table / Chart の切り替え、列 0 個と数値列なしでの Chart 無効化、Y 列未選択の表示、再実行での初期選択への復帰を追加する。

## 完了条件

- `DrawerObjectQuery` の結果部に Table / Chart の切り替えがあり、Chart を選ぶと X 列 (単一選択)、Y 列 (複数選択)、グラフ種類 (`bar` / `line`) の選択と `QueryResultChart` が表示される。
- `isNumericCell` と `parseNumericCell` が `lib/numericCell.ts` から export され、`ResultTable.tsx` と `lib/queryChart.ts` の両方がそれを import している。`ResultTable` の既存テストが変更なしで通る。
- Y 列の候補には、非空のセルが 1 つ以上あり、非空のセル全てが `isNumericCell` を通る列だけが並ぶ。
- 結果の列が 0 個、または数値列が無い結果では Chart への切り替えが無効になり、`title` に理由が出る。
- 初期選択は X 列が先頭の列、Y 列が先頭の数値列である。クエリを再実行すると初期選択に戻る。
- Y 列を全て外すと、グラフの代わりに Y 列を選ぶよう促す文が出る。
- 空文字列の値は欠損として点が描かれず、他の点の描画は止まらない。
- X 列の distinct な値の数が 5,000 を超える結果ではグラフを描かず、集計または `LIMIT` を促す表示が出る。
- `THEME_TEXT_COLOR` が `components/charts/chartTheme.ts` から export され、`TimeseriesChart.tsx`、`CostChart.tsx`、`QueryResultChart.tsx` の 3 つがそれを import している。`QueryResultChart` の option の `textStyle.color`、`legend.textStyle.color`、`xAxis.axisLabel.color`、`yAxis.axisLabel.color` が dark で `#a8a8b0`、light で `#5c5c66` になることをテストで検証している。
- 新規の UI 文言が `locales/ja/drawerStorage.json` と `locales/en/drawerStorage.json` の両方にある。
- 設計判断 7 の単体テストとコンポーネントテストが追加されている。
- `frontend/package.json` に新規依存が増えていない。
- `mise run check` が通過する。
- 扱わない範囲: 円グラフと散布図、グラフ画像の保存、軸選択の永続化。

## 関連

- docs/issues/0196: 同じ TODO 項目の「検索」を扱う。本 issue は 0196 の `DrawerObjectQuery` と結果の `string[][]` (変換規則を含む) に依存する。

## 見送りの記録

- 日付: 2026-09-24
- 見送った Step: implement-issues の Step 3 (依存関係の確認)
- 理由: 本 issue は「## 背景」で docs/issues/0196 の完了後に着手すると定めており、`DrawerObjectQuery` と `string[][]` の結果を前提にする。docs/issues/0196 は起票レビューの 3 ラウンド目でも優先度「高」の指摘が出たためユーザー確認待ちとして `issues/pending/` に置かれており、先行条件が決着していない。
- 再開に必要な条件: docs/issues/0196 が `issues/pending/` から `issues/` へ昇格し、実装されて `issues/closed/` に移ること。

- 日付: 2026-09-25
- 見送った Step: implement-issues の Step 3 (見送りの記録と依存関係の確認)
- 理由: 前回 (2026-09-24) の見送り理由が解消していない。docs/issues/0196 は `issues/pending/` にあり、先行条件が決着していない。
- 再開に必要な条件: 前回と同じ (docs/issues/0196 が `issues/pending/` から `issues/` へ昇格し、実装されて `issues/closed/` に移ること)。

- 日付: 2026-09-25
- 見送った Step: implement-issues の Step 4 (並列実装の実装エージェントの起動)
- 理由: 環境要因。opencode のランナー (`issue-implementer`、モデル `cf-fireworks/deepseek-v4p1-flash`) が起動直後に `APIError: Unauthorized: invalid_token` で失敗し、同じプロンプトでの 1 回の再実行も同じ理由で失敗した (`status=error`、`steps=0`、`tool_calls=0`)。同じランナーは同日 22:46 から 23:05 にかけてレビューエージェントを `status=ok` で実行できていたため、その後にゲートウェイのアクセストークンが失効したものと判断した。implement-issues は実装エージェントの失敗を親や Agent ツールで代行しないと定めているため、実装に入らなかった。worktree に変更は無く、破棄したものは無い。
- 再開に必要な条件: opencode が使う AI ゲートウェイのアクセストークンを更新し、ランナーが `status=ok` で応答を返せること。再開時は「## 設計判断」のとおり実装する (方針の再確認は不要)。
- 前回の見送り理由 (先行条件の docs/issues/0196 が未実装) は解消した。docs/issues/0196 は 2026-09-25 に実装して close 済みである (`docs/issues/closed/0196-feat-object-storage-duckdb-query.md`、コミット 46424f2)。

## 解決方法

### 変更したファイルとシンボル

新規。

- `frontend/src/components/charts/chartTheme.ts`: `THEME_TEXT_COLOR` (`Record<'dark' | 'light', string>`、dark `#a8a8b0`、light `#5c5c66`) を export する。`TimeseriesChart.tsx` と `CostChart.tsx` が持っていた同名の非公開 const は削除し、この import に置き換えた。
- `frontend/src/lib/numericCell.ts`: `ResultTable.tsx` の非公開関数だった `isNumericCell` と `parseNumericCell` を移して export する。判定 (`/^-?[\d,]+(\.\d+)?$/`、カンマ区切りを許容し指数表記と前後の空白は非数値) と `parseNumericCell` の戻り値 (カンマを除いたうえで `Number()` が有限値を返す値はその結果、返さない値は 0) は変えていない。`isNumericCell` を通らなくても有限値になる値 (指数表記、前後に空白がある値) があるため、その除外は呼び出し側が `isNumericCell` で行う。
- `frontend/src/lib/queryChart.ts`: クエリ結果 (`columns: string[]`、`rows: string[][]`) をグラフの入力へ変換する純関数群。`QUERY_CHART_MAX_CATEGORIES` (5000)、`isNumericColumn`、`numericColumnIndexes`、`queryChartCapabilities`、`initialChartSelection`、`queryChartCategories`、`queryChartCategoryCount`、`queryChartOverCategoryLimit`、`queryChartSeries` と、型 `QueryResultChartType` / `QueryChartSeries` / `QueryChartSelection` / `QueryChartDisabledReason` / `QueryChartCapabilities` を export する。数値の判定は `lib/numericCell.ts` を `ResultTable` と共有する。
- `frontend/src/components/charts/QueryResultChart.tsx`: `columns`、`rows`、`xIndex`、`yIndexes`、`type` (`bar` / `line`)、`height` (既定 320) を受け取り、`echarts-for-react` の `ReactECharts` をラップする。X は `type: 'category'` の軸に `queryChartCategories` の値を文字列のまま置き、系列は `queryChartSeries` が返す `(number | null)[]` をそのまま渡す。系列の色は `lib/timeseries.ts` の `SERIES_COLORS`、文字色は `THEME_TEXT_COLOR[tweaks.theme]` を `textStyle.color`、`legend.textStyle.color`、`xAxis.axisLabel.color`、`yAxis.axisLabel.color` に使う。

変更。

- `frontend/src/components/Drawer/DrawerObjectQuery.tsx`: `ObjectQueryPanel` の結果部に Table / Chart の `.seg` 切り替え (`resultView`) を置き、Chart のときに X 列の `<select>`、Y 列の checkbox 群 (`toggleChartY`)、グラフ種類の `.seg` (`CHART_TYPE_OPTIONS`) と `QueryResultChart` を表示する。`chartCapabilities` (= `queryChartCapabilities` の結果、`useMemo` で結果ごとに計算) が `disabledReason` を返す結果では Chart ボタンを `disabled` にし、理由を `title` に出す。`runQuery` の `setResult` の直後に `initialChartSelection` を適用し、クエリの実行ごとに軸の選択を初期選択へ戻す。同じ場所で新しい結果の `queryChartCapabilities` も確かめ、グラフを表示できない結果になったときは `resultView` を表へ戻す (無効な Chart が選択されたまま表もグラフも出ない状態にしないため)。
- `frontend/src/components/query/ResultTable.tsx`: `isNumericCell` と `parseNumericCell` の定義を削除し、`lib/numericCell.ts` から import する。振る舞いは変えていない。
- `frontend/src/components/charts/TimeseriesChart.tsx`、`frontend/src/components/charts/CostChart.tsx`: `THEME_TEXT_COLOR` の重複定義を削除し、`chartTheme.ts` から import する。
- `frontend/src/i18n/locales/ja/drawerStorage.json`、`frontend/src/i18n/locales/en/drawerStorage.json`: `drawerObjectQuery.chart.*` に 10 キー (`table`、`chart`、`xColumn`、`yColumn`、`typeBar`、`typeLine`、`noColumns`、`noNumericColumns`、`selectY`、`tooManyCategories`) を追加する。

### 完了条件の検証

- 結果部の Table / Chart の切り替えと、X 列 (単一選択)、Y 列 (複数選択)、グラフ種類 (`bar` / `line`) の選択と `QueryResultChart` の表示: `DrawerObjectQuery.test.tsx` の「Chart に切り替えると初期選択 (X は先頭の列、Y は先頭の数値列) でグラフを表示する」「グラフから表へ戻せる」「Y 列を追加で選ぶと系列が増える」「グラフの種類を折れ線に切り替えられる」で検証した。切り替えは双方向を、Y 列は 2 つ目の数値列を選んで系列が 2 本になることを確かめている。
- `isNumericCell` と `parseNumericCell` が `lib/numericCell.ts` から export され、`ResultTable.tsx` と `lib/queryChart.ts` の両方が import していること: `ResultTable.tsx` と `queryChart.ts` の import 行。`ResultTable.test.tsx` は変更していない (`git status --porcelain` に出ない) まま 1234 テストの中で通過している。
- Y 列の候補が数値列だけであること: `queryChart.test.ts` の「空セルを許容し、全ての非空セルが数値なら数値列とする」「数値でないセルが混ざる列は数値列としない」「欠損だけの列は数値列としない」「数値列の index だけを列の順に返す」「カンマ区切りを数値として扱う」「行が無い結果では数値列が無い」で検証した。
- 列が 0 個または数値列が無い結果で Chart が無効になり `title` に理由が出ること: `queryChart.test.ts` の「列が無い結果は理由を noColumns にする」「数値列が無い結果は理由を noNumericColumns にする」「数値列があれば無効の理由を返さず、候補列を返す」と、`DrawerObjectQuery.test.tsx` の「列が無い結果では Chart を無効にし理由を title に出す」「数値列が無い結果では Chart を無効にし理由を title に出す」で検証した。
- 初期選択が X 列は先頭の列、Y 列は先頭の数値列であり、再実行で初期選択に戻ること: `queryChart.test.ts` の「X は先頭の列、Y は先頭の数値列を選ぶ」「数値列が無いときは Y を選ばない」「先頭の列が数値列のときは X と Y が同じ列になる」と、`DrawerObjectQuery.test.tsx` の「Chart に切り替えると初期選択 (X は先頭の列、Y は先頭の数値列) でグラフを表示する」「再実行すると軸の選択が初期選択に戻る」で検証した。
- Y 列を全て外すと Y 列を選ぶよう促す文が出ること: `DrawerObjectQuery.test.tsx` の「Y 列を全て外すとグラフの代わりに Y 列の選択を促す」で検証した。
- 空文字列が欠損として描かれず他の点の描画が止まらないこと: `queryChart.test.ts` の「Y 列を系列にし、欠損 (空文字列) は null にする」「Y 列の順序を系列の順序として保つ」と、`QueryResultChart.test.tsx` の「X 列の値を文字列のままカテゴリ軸に置き、Y 列を系列にする」(欠損を含む行の系列が `[1, null, 1234]` になり、他の値が数値のまま残る) で検証した。
- X 列の distinct な値が 5,000 を超える結果でグラフを描かず集計または `LIMIT` を促すこと: `queryChart.test.ts` の「distinct な値の数を返す (重複は 1 つに数える)」「上限ちょうどは超えていない扱い、上限を 1 つ超えると超えている扱いにする」と、`DrawerObjectQuery.test.tsx` の「X 列の distinct な値が上限を超える結果ではグラフを描かず集計を促す」で検証した。
- `THEME_TEXT_COLOR` が `components/charts/chartTheme.ts` から export され 3 つのチャートが import していること、および `QueryResultChart` の option の 4 か所がテーマごとに `#a8a8b0` / `#5c5c66` になること: `TimeseriesChart.tsx`、`CostChart.tsx`、`QueryResultChart.tsx` の import 行と、`QueryResultChart.test.tsx` の「THEME_TEXT_COLOR は dark で #a8a8b0、light で #5c5c66 を指す」「テーマ dark では軸と凡例の文字色を #a8a8b0 にする」「テーマ light では軸と凡例の文字色を #5c5c66 にする」で検証した。
- 新規の UI 文言が ja と en の両方にあること: 両ファイルの `drawerObjectQuery.chart` に同じ 10 キーを追加した。
- グラフを表示できない結果で切り替えが無効になること (再実行で到達する場合を含む): `DrawerObjectQuery.test.tsx` の「グラフ表示のまま数値列が無い結果を再実行すると表へ戻る」「グラフ表示のまま列が無い結果を再実行すると表へ戻る」で、無効になる 2 つの理由のどちらでも表が再び描かれ Chart が理由付きで無効になることを検証した。
- 設計判断 7 の単体テストとコンポーネントテストの追加: `lib/numericCell.test.ts` (新規 7 テスト)、`lib/queryChart.test.ts` (新規 19 テスト)、`components/charts/QueryResultChart.test.tsx` (新規 6 テスト)、`components/Drawer/DrawerObjectQuery.test.tsx` (18 から 29 へ 11 テスト追加) の計 43 テストを追加した。
- `frontend/package.json` に新規依存が増えていないこと: 同ファイルは変更していない (`git status --porcelain` に出ない)。既存の `echarts` と `echarts-for-react` だけを使う。
- `mise run check` の通過: 統合後の作業ツリーで実行して終了コード 0。frontend は 113 ファイル / 1234 テスト通過 (ベースラインの 110 ファイル / 1191 テストから 3 ファイル / 43 テストの増加)、eslint は 0 errors / 9 warnings (warnings は全て本 issue の変更前から存在する)、backend は 18 パッケージすべて ok (`-race`)。ベースラインからの新たな失敗は無い。
- 扱わない範囲 (円グラフと散布図、グラフ画像の保存、軸選択の永続化): いずれも実装していない。`QueryResultChartType` は `bar` と `line` の 2 値、画像保存の導線は無く、`lib/storage.ts` の `PersistedState` は変更していない。

### 方針からの乖離

方針の方式を変える乖離は無い。「## 設計判断」の範囲で決めた実装の詳細は次のとおり。

- `lib/queryChart.ts` には設計判断 2 と 5 が挙げる変換と判定に加えて、`isNumericColumn`、`queryChartCapabilities`、`queryChartOverCategoryLimit` も export した。判定をコンポーネントに散らさず純関数として単体テストで固定するためである。
- Y 列の複数選択は checkbox の並びで実装した。既存に複数選択の共通コンポーネントが無く、設計判断 6 は「複数選択」とだけ定めているためである。
- Chart に切り替えられない理由は無効化した切り替えボタンの `title` に、Y 列未選択とカテゴリ数超過の案内はグラフ領域 (`.empty-hint`、グラフと同じ高さ 320px) に出す。切り替えや選択のたびにレイアウトが動かないようにするためである。
- X 列の distinct 数は空文字列も 1 つの値として数える。X 列の値を行のままカテゴリに使うためで、`queryChart.ts` のコメントに明記した。
- グラフ種類 (`chartType`) はクエリの再実行で初期値に戻さない。設計判断 6 が再実行で戻すと定めているのは軸の選択だけだからである。

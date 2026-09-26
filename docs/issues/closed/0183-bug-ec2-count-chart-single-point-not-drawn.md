# EC2 の Running インスタンス数の折れ線グラフが記録 1 点のときに何も描かれない

Created: 2026-09-18
Model: Claude Fable 5.1
Completed: 2026-09-18

## 症状

EC2 の一覧画面 (`Running instances` のグラフ) で、時系列 API は値を返しているのにグラフ領域に線も点も描かれず、軸と凡例だけが出る。

```
curl -s "http://127.0.0.1:8089/api/aws/profiles/example-common/ec2/timeseries?region=ap-northeast-1&range=1d"
{"range":"1d","period_seconds":60,"series":[{"name":"Running","points":[{"t":1789702172104,"v":2}]}]}
```

上の応答のように系列は 1 本あり点も 1 つあるため、`TimeseriesChart` の「系列が無いときは No data を出す」分岐には入らず、ECharts に描画が委ねられる。
その結果、`No data` でもエラーでもなく、空の座標軸だけが表示される。

## 再現手順

1. `mise run backend:run` で API サーバを起動し、`mise run frontend:run` でフロントを起動する。
2. EC2 の一覧を開く。一覧の取得 (キャッシュ MISS) で `EC2CountRecorder` に 1 点だけ記録される。
3. 時系列 API を呼ぶと上記のように `points` が 1 要素の応答が返る。
4. 画面の `Running instances` のグラフは、軸と凡例 `Running` は出るが線も点も無い。

ECharts 単体でも再現する。`frontend/` で ECharts 6.1.0 の SVG SSR を使い、`type: 'line'`、`showSymbol: false`、データ 1 点で描画すると、系列の `<path>` の `d` 属性は `M315 65` (始点のみで線分が無い) になり、symbol の要素も出力されない。
同じ条件で `showSymbol: true` にすると点の marker (`<path ... transform="matrix(3,0,0,3,315,65)">`) が出力され、データ 2 点にすると線分 `M90 90L540 65` が出力される。

## 原因

### 主因: 1 点だけの折れ線は線分も symbol も持たない

`frontend/src/components/charts/TimeseriesChart.tsx` は全系列に `showSymbol: false` を指定している。

```ts
series: shown.map((s) => ({
  name: s.name,
  type: 'line',
  showSymbol: false,
  emphasis: { focus: 'series' },
  data: s.points.map((p) => [p.t, p.v]),
})),
```

折れ線は隣接する 2 点を結んで初めて線分になる。点が 1 つしか無い系列は線分を持たず、`showSymbol: false` で点の marker も消しているため、描画されるものが何も残らない。
この性質は EC2 に限らず、欠測 (`v: null`) に前後を挟まれた孤立した 1 点にも当てはまる。ECS の `LiveTaskCount` は RUNNING タスクが 0 のサービスでデータ点が送られないため、孤立点が生じうる。

### EC2 の系列が 1 点になりやすい理由

`backend/internal/aws/ec2_metrics.go` の `EC2CountRecorder` は、`backend/internal/api/handlers_aws.go` の `handleEC2` が `serveCached` のクロージャの中で AWS から一覧を実際に取得したとき (キャッシュ MISS と Refresh) にだけ記録する。定期ポーリングは行わず、記録はプロセス内メモリにしか無い。
キャッシュ TTL は `backend/internal/api/server.go` の `cacheTTL = time.Hour` である。
そのため API サーバを起動して EC2 の一覧を開いた直後は、profile と region の組ごとに記録が 1 点しか無い。Refresh を押すか TTL が切れるまで 2 点目が増えない。
これは `docs/issues/closed/0177` の決着内容 (一覧の取得時だけ記録する、再起動で消えることを許容する) どおりの動作であり、記録側の不具合ではない。
記録が 1 点になる状況が設計上ありふれているにもかかわらず、描画側が 1 点を描けないことが不具合である。

ECS の系列は CloudWatch の `GetMetricData` の結果を `timeseriesGrid` で等間隔に並べるため、期間全体の点数 (1 日で 1440 点) を常に持つ。1 点だけになることは無く、この症状が EC2 でだけ目立つ。

### 副因: Refresh 直後の時系列が直前の記録を含まない

`frontend/src/lib/refreshView.ts` の `createViewRefresher` は backend のキャッシュを破棄したあと、queryKey の前方一致 `['aws']` で TanStack Query を無効化する。
EC2 の一覧 (`['aws', 'ec2', profile, region]`) と時系列 (`['aws', 'ec2', profile, region, 'timeseries', range]`) の両方が同時に再取得される。
時系列の再取得は `handleEC2Timeseries` がメモリから即座に応答するため、`DescribeInstances` を待つ一覧の再取得より先に完了する。一覧の取得が完了して `Record` が呼ばれるのは時系列の応答の後になるため、Refresh 直後のグラフには今回の記録が含まれない。
`staleTime` (60 秒) が切れた後の再取得やウィンドウのフォーカス復帰で追いつくが、Refresh を押した直後に点が増えないため、利用者からは Refresh でグラフが更新されないように見える。

これはコードの読み取りによる推論であり、実測はしていない。

## 修正方針

### 主因の修正 (frontend)

`TimeseriesChart` で、線分を持てない点には symbol を描く。具体的には系列ごとに各点を見て、前後のどちらにも値のある点 (`v !== null`) が無い孤立点にだけ marker を付ける。
点が 1 つしか無い系列はその点が孤立点になるため、この規則で自然に描かれる。欠測に挟まれた点も同じ規則で描かれる。

実現は ECharts の series の `showSymbol: true` と系列既定の `symbol: 'none'` を組み合わせ、孤立点だけデータ項目を `{ value: [t, v], symbol: 'circle' }` の形にしてデータ項目単位で symbol を上書きする案を第一候補とする。
データ項目単位の `symbol` 上書きが ECharts 6.1.0 で有効であることを、実装時に `frontend/` の SVG SSR で確認する (`showSymbol: false` のままではデータ項目の `symbol` に関係なく描かれないため、この組み合わせが必須である)。
確認できない場合は、孤立点を持つ系列だけ `showSymbol: true` にする案に切り替える。系列全体の点が増えるが、線分が無い点を消してしまうより優先する。

`ResourceCountChart` 側の `No data` や読み込み中の表示は変えない。系列があるのに何も描かれない状態を無くすのが目的であり、1 点のときに別の表示へ逃がすのではなく、1 点をそのまま描く。

### 副因の修正 (frontend)

EC2 の一覧の取得が成功したときに、その profile と region の時系列クエリを無効化して再取得させる。
`ServicePanel` (`frontend/src/views/AccountView.tsx`) が持つ一覧の `useResources` の結果を使い、一覧の `dataUpdatedAt` が進んだときに `queryClient.invalidateQueries({ queryKey: ['aws', service, profile, region, 'timeseries'] })` を呼ぶ。
一覧の取得は `Record` を伴う唯一の経路であるため、一覧の更新のたびに時系列を取り直せば Refresh 直後の 1 点遅れが無くなる。
`refreshView.ts` の無効化の順序を変える案 (一覧の再取得完了を待ってから時系列を無効化する) は、`createViewRefresher` が個々のクエリを知らない汎用の処理列であり、EC2 だけの都合を持ち込むと他の view に影響するため採らない。

### 採らなかった案

- **全系列で常に `showSymbol: true` にする**。1 日の ECS の系列は 1440 点あり、点ごとの marker が線を覆って読めなくなる。`showSymbol: false` にした理由 (線を読ませる) を捨てることになる。
- **backend で 1 点のときに同じ値の点を複製して 2 点にする**。観測していない時刻の値を捏造することになり、`EC2CountRecorder.Series` が「グリッドへ並べ直さないのは、観測していない時刻を欠測として捏造しないため」と明記している方針に反する。
- **`ResourceCountChart` で点が 2 未満のとき「記録が足りない」旨のヒントを出す**。1 点でも値は観測できており、描けるものを描かない理由が無い。欠測に挟まれた孤立点の問題も解決しない。

## 調査結果 (2026-09-18、実装時の方式確定)

「## 修正方針」が実装時の確認に委ねた「データ項目単位の `symbol` 上書きが ECharts 6.1.0 で有効か」を、`frontend/` の ECharts 6.1.0 の SVG SSR で確認した。
系列に `showSymbol: true` と `symbol: 'none'` を与え、孤立点のデータ項目だけを `{ value: [t, v], symbol: 'circle' }` の形にしたときの出力は次のとおりである。

| 系列 | marker の数 | 線分の数 |
| --- | --- | --- |
| 1 点だけ (その点を上書き) | 1 | 0 |
| 5 点で中央の 1 点が欠測に挟まれ孤立 (その点だけ上書き) | 1 | 0 |
| 3 点すべて連続 (上書きなし) | 0 | 1 |

上書きした孤立点にだけ marker が出力され、連続する区間の点には出力されない。
第一候補である案 (データ項目単位の `symbol` 上書き) が成立するため、これを採る。切り替え先として用意していた「孤立点を持つ系列だけ `showSymbol: true` にする案」は採らない。

補足 (2026-09-18、close レビューの指摘による): 上表の 5 点の行は、上書きが指定したデータ項目にだけ効くことを確かめるために中央の 1 点だけを上書きした実験であり、marker の数 1 はその条件での値である。この点列 `[1, null, 5, null, 1]` では先頭と末尾の点も「前後のどちらにも値のある点が無い」孤立点に当たるため、実装した判定 (`isolatedPointFlags`) を通すと 3 点すべてが上書きされ、marker は 3 つ出力される (「## 解決方法」の表)。

## 完了条件

- `TimeseriesChart` に、点が 1 つだけの系列を渡すと ECharts の option でその点に symbol が付く (`showSymbol: false` のままにならない) ことを検証するテストが `frontend/src/components/charts/TimeseriesChart.test.tsx` に追加されている。
- 同テストに、欠測 (`null`) に前後を挟まれた孤立点に symbol が付き、値の連続する区間の点には symbol が付かないことを検証するケースが追加されている。
- 孤立点の判定は `frontend/src/lib/timeseries.ts` の純関数として置き、`frontend/src/lib/timeseries.test.ts` に系列の先頭・末尾・中間の孤立点と、全点連続、全点欠測のケースがある。
- 採った実現方式 (データ項目単位の `symbol` 上書き、または孤立点を持つ系列だけ `showSymbol: true`) で、ECharts 6.1.0 の SVG SSR により 1 点の系列が実際に描かれる (symbol の要素が出力される) ことを実装時に確認し、確認結果を本 issue の「## 解決方法」に記録する。
- EC2 の一覧の取得が成功したとき、同じ profile と region の時系列クエリが無効化されて再取得されることを検証するテストが `frontend/src/views/AccountView.test.tsx` (無い場合は `ResourceCountChart.test.tsx`) にある。
- ECS の一覧画面の `Tasks per cluster` のグラフは、値の連続する区間の描画が変わらない (symbol が線を覆わない)。
- backend (`EC2CountRecorder`、`handleEC2Timeseries`、`handleEC2`) は変更しない。
- `mise run check` が通る。

## 関連

- `docs/issues/closed/0177-feat-ec2-ecs-running-count-timeseries.md`: 本グラフを追加した issue。EC2 の記録が一覧の取得時だけである設計の根拠。
- `docs/issues/closed/0079`: Refresh 時に backend キャッシュ破棄と TanStack Query の無効化の順序を定めた issue。`refreshView.ts` の処理列の出所。

## 解決方法

### 変更内容

- `frontend/src/lib/timeseries.ts` に純関数 `isolatedPointFlags(points)` を追加した。各点について、値を持ち (`v !== null`)、かつ直前と直後のどちらにも値を持つ点が無いときに `true` を返す。欠測の点は常に `false` である。
- `frontend/src/components/charts/TimeseriesChart.tsx` の series を `showSymbol: true` と `symbol: 'none'` の組に変え、`isolatedPointFlags` が `true` を返した点だけデータ項目を `{ value: [t, v], symbol: 'circle' }` の形にして symbol を circle に上書きする。それ以外の点は従来どおり `[t, v]` のまま渡す。「## 修正方針」の第一候補である、データ項目単位の `symbol` 上書きの案をそのまま採った。
- `frontend/src/views/AccountView.tsx` の `ServicePanel` で、一覧の `useResources` から `dataUpdatedAt` を受け取り、その値が 0 以外に変わるたびに `queryClient.invalidateQueries({ queryKey: ['aws', service, profile, region, 'timeseries'] })` を呼ぶ `useEffect` を追加した。queryKey に range を含めないため、全 range の時系列クエリが無効化される。
- テストを追加した。`frontend/src/components/charts/TimeseriesChart.test.tsx` に 3 ケース (1 点だけの系列、欠測に挟まれた孤立点、値の連続する系列)、`frontend/src/lib/timeseries.test.ts` に `isolatedPointFlags` の 8 ケース (1 点、先頭、末尾、中間、全点連続、片側だけ連続、全点欠測、空)、`frontend/src/views/AccountView.test.tsx` に時系列の無効化の 4 ケース (取得成功、未取得、`dataUpdatedAt` の前進、失敗した再取得)。

backend は変更していない。

### 完了条件の検証

- 1 点だけの系列に symbol が付くテスト: `TimeseriesChart.test.tsx` の「点が 1 つだけの系列はその点に symbol を付ける (線分を持てず何も描かれないのを防ぐ)」が、`showSymbol` が `true` であることと、そのデータ項目が `symbol: 'circle'` を持つことを検証する。
- 孤立点にだけ symbol が付くテスト: 同ファイルの「欠測に挟まれた孤立点にだけ symbol を付け、値の連続する区間の点には付けない」が点列 `[1, 2, null, 9, null]` で、4 番目だけが上書きされ 1 番目と 2 番目が `[t, v]` のままであることを検証する。
- 純関数と先頭・末尾・中間・全点連続・全点欠測のケース: `timeseries.test.ts` の `describe('isolatedPointFlags')` に上記 8 ケースがある。
- ECharts 6.1.0 の SVG SSR による確認: `frontend/` で `echarts.init(null, null, { renderer: 'svg', ssr: true })` を使い、実装と同じ option (series の `showSymbol: true`、`symbol: 'none'`、`isolatedPointFlags` による上書き) で描画し、symbol の `<path>` (`transform="matrix(...)"` を持つ要素) と線分 (`d` 属性に `L` を含む `<path>`) を数えた。結果は次のとおりである。

  | 系列 | marker の数 | 線分の数 |
  | --- | --- | --- |
  | 1 点だけ | 1 | 0 |
  | 5 点 `[1, null, 5, null, 1]` (先頭・中央・末尾が孤立) | 3 | 0 |
  | 3 点すべて連続 | 0 | 1 |

  修正前の option (`showSymbol: false`) では 1 点の系列の出力が `<path d="M315 65">` だけで、symbol の要素は無かった。修正後は symbol が 1 つ出力され、1 点の系列が実際に描かれる。
- EC2 の一覧の取得成功で時系列が無効化されるテスト: `AccountView.test.tsx` の describe 「AccountView の一覧取得に追随した時系列の再取得」の 4 ケース (「一覧の取得が成功したら同じ profile と region の時系列クエリを無効化する」「一覧が未取得 (dataUpdatedAt が 0) の間は時系列を無効化しない」「一覧が取り直されて dataUpdatedAt が進むたびに時系列を無効化する」「一覧の取り直しが失敗して dataUpdatedAt が進まなければ時系列を無効化し直さない」)。
- ECS の `Tasks per cluster` の描画が変わらないこと: 上表の「3 点すべて連続」で marker が 0 であることと、`TimeseriesChart.test.tsx` の「値が連続する系列には symbol を付けない (marker が線を覆わない)」で検証した。値の連続する区間のデータ項目は `[t, v]` のままで symbol を持たない。
- backend を変更しないこと: `git status` の変更ファイルは `frontend/src/` 配下の 6 ファイルだけである。
- `mise run check`: 終了コード 0。frontend は 96 ファイル 1012 テストが通過 (ベースライン 997 から 15 増)、eslint は 9 警告 0 エラー (ベースラインと同数)、`tsc --noEmit` 通過。backend の fmt、lint、test も通過。

### 再現確認

修正前の症状は ECharts 6.1.0 の SVG SSR で再現した (1 点の系列で `<path d="M315 65">` だけが出力され、symbol の要素が無い)。修正後の同じ入力では symbol が 1 つ出力される。
「## 再現手順」の 1 から 4 (API サーバと実 AWS アカウントでの再現) は、このコンテナに AWS の認証情報が無いため実行していない。option を検証するコンポーネントテストと SVG SSR の確認で代えた。

### 方針からの乖離

いずれも方式を保ったままの実装詳細の乖離である。

- `TimeseriesChart.test.tsx` の既存の欠測のケースは点列 `[値, 欠測, 値]` だったが、修正後はこの中央の欠測に挟まれた両端の点が孤立点になり、`[t, v]` のままであることを検証していた既存のアサーションが成立しなくなる。ケースの意図 (欠測が `null` のまま渡ること) は変えず、点列を `[値, 値, 欠測]` に変えた。
- 時系列の無効化は EC2 に限らず、`ServicePanel` を使う全サービスで行う。「## 修正方針」は `ServicePanel` に置くと定めており、`ServicePanel` はサービスを引数で受けるため、EC2 だけに絞る分岐を置く方が不自然である。時系列クエリを持たないサービスでは一致するクエリが無く、無効化は何もしない。
- `AccountView.test.tsx` の `renderView` は QueryClient を省略可能な引数として受け取るように変え、同じ要素を初回描画と再描画で使うための `viewElement` ヘルパーを追加した。

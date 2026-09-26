# EC2 の Running インスタンス数のグラフの X 軸が期間の切り替え (1 day / 7 days / 1 month) に追随しない

Created: 2026-09-18
Model: Claude Fable 5.1
Completed: 2026-09-18

## 症状

EC2 の一覧画面の `Running instances` のグラフで、期間ボタンを 1 day、7 days、1 month と切り替えても X 軸の範囲が変わらず、記録された点の時刻に固定されたままになる。
同じ `ResourceCountChart` を使う ECS の `Tasks per cluster` では期間に応じて X 軸の範囲が変わる。

時系列 API 自体は期間ごとに再取得されており (`useResourceTimeseries` の queryKey は期間を含む)、応答の `range` も切り替わる。描画側だけが期間を反映しない。

## 再現手順

1. `mise run backend:run` で API サーバを起動し、`mise run frontend:run` でフロントを起動する。
2. EC2 の一覧を開く。一覧の取得で `EC2CountRecorder` に点が記録され、`Running instances` のグラフに点が描かれる (issue 0183 の修正後は 1 点でも描かれる)。
3. 期間ボタンを 1 day、7 days、1 month と切り替える。
4. 期待: X 軸が選んだ期間の幅 (1 日、7 日、30 日) になり、点は軸の中でその時刻の位置に描かれる。
5. 実際: どの期間でも X 軸の目盛りが記録された点の時刻の前後だけを示し、軸の範囲が変わらない。
6. 同じ操作を ECS の一覧で行うと、X 軸は期間に応じて変わる。

ECharts 単体でも再現する。`type: 'time'` の xAxis に `min` と `max` を与えず、データ点が 1 つの折れ線を描くと、軸の範囲はその点の時刻を中心に自動で決まり、点列が同じなら軸の範囲も同じになる。

## 原因

`frontend/src/components/charts/TimeseriesChart.tsx` の xAxis は `type: 'time'` だけを指定し、`min` と `max` を持たない。

```ts
xAxis: {
  type: 'time',
  axisLine: { lineStyle: { color: textColor } },
  axisLabel: { color: textColor },
},
```

ECharts は `min` と `max` が無い time 軸の範囲を、全系列のデータ点の最小時刻と最大時刻から自動で決める。つまり軸の範囲は「点の範囲」であり、「期間」ではない。

ECS では backend の `ecsTaskCountSeries` が `TimeseriesRange.Window` で決めた時間窓を `timeseriesGrid` で等間隔に埋め、欠測を `null` の点として返す。点列が期間全体を覆うため、点の範囲と期間が一致し、症状が出ない。

EC2 では `backend/internal/aws/ec2_metrics.go` の `EC2CountRecorder.Series` が期間内 (`now - Duration` 以降) に記録された点だけを返す。記録は一覧の取得時にだけ増えるため点は少なく、その点は 1 day、7 days、1 month のいずれの期間にも含まれる。どの期間でも同じ点列が返り、軸の範囲も同じになる。

応答 `TimeseriesResponse` (`backend/internal/aws/metrics.go`) は `range`、`period_seconds`、`series` だけを持ち、時間窓の開始と終了を含まない。backend は `TimeseriesRange.Window(now)` で窓を計算しているが、frontend には渡していない。frontend は期間の識別子 (`'1d'` など) は持つが、それをどの時刻範囲として描くかを知らない。

issue 0183 は 1 点の系列に marker が付かない問題を直したもので、点が見えるようになった結果、軸が期間に追随しないことが目立つようになった。

## 修正方針

### backend: 応答に時間窓を含める

`TimeseriesResponse` に時間窓の開始と終了をエポックミリ秒で追加する (フィールド名は `start` と `end`、型は `int64`。`MetricPoint.T` と同じ単位にする)。

- ECS (`handleECSTimeseries`) は `TimeseriesRange.Window(now)` の `start` と `end` をそのまま入れる。`ecsTaskCountSeries` がグリッドを組むのに使う窓と一致させるため、窓の計算を 1 回にして両方に渡す (現状は `ecsTaskCountSeries` の内部で計算しており、handler は窓を知らない。`ecsTaskCountSeries` の戻り値に窓を含めるか、handler で `Window` を計算して渡すかは実装時に決めるが、同じ `now` から同じ窓が得られることを保証する)。
- EC2 (`handleEC2Timeseries`) は `EC2CountRecorder.Series` が使っている `now - Duration` から `now` までを入れる。`Window` の切り下げた `end` を使うと、切り下げ後に記録された直近の点が窓の外に出る (ECS はグリッドの外の点を持たないため問題にならないが、EC2 は任意の時刻に記録する)。`Series` の絞り込みと応答の窓が同じ `now` と同じ境界を使うように、`Series` が窓を返すか、handler が窓を計算して `Series` に渡す形にする。

`TimeseriesResponse` は `backend/internal/contract/contract.go` の `Registry` に登録されているため、`backend/` で `UPDATE_GOLDEN=1 go test ./internal/contract/` を実行してゴールデン JSON (`frontend/src/types/__contract__/TimeseriesResponse.json`) を再生成する。

### frontend: 窓を X 軸の範囲にする

- `frontend/src/types/aws.ts` の `TimeseriesResponseRaw` に `start` と `end` を追加し、`TimeseriesResponseRow` にも対応するフィールドを持たせる。`frontend/src/lib/normalize.ts` の `timeseriesResponseFromRaw` で写す。
- `TimeseriesChart` に X 軸の範囲を指定する省略可能な props を追加する (`xRange?: { start: number; end: number }` のような形)。渡されたときだけ xAxis の `min` と `max` に入れ、渡されなければ従来どおり自動にする。`TimeseriesChart` は Datadog Metrics と Datadog Dashboards の時系列グラフ (`frontend/src/views/nonaws/DatadogMetricsView.tsx`、`DatadogDashboardView.tsx`) からも使われており、これらは窓を持たないため既定の挙動を変えない。
- `ResourceCountChart` は応答の `start` と `end` を `TimeseriesChart` に渡す。

### 採らなかった案

- **frontend だけで `range` から `Date.now() - duration` を計算して軸の範囲にする**。backend の変更が要らないが、期間からミリ秒への変換表が frontend にも生え、ECS の窓 (backend が粒度で切り下げる) と frontend の計算がずれる。窓を決めているのは backend なので、backend が返すのが筋である。
- **EC2 の点列も ECS と同様に等間隔のグリッドに並べて欠測を `null` で埋める**。`EC2CountRecorder.Series` が「グリッドへ並べ直さないのは、観測していない時刻を欠測として捏造しないため」と明記している方針に反する。1 日で 1440 点の `null` を返す割に、得られるのは軸の範囲だけである。
- **xAxis の `min` / `max` に `'dataMin'` / `'dataMax'` や `boundaryGap` を使う**。点の範囲から軸を決める方式のままであり、期間を反映しない。

## 調査結果 (2026-09-18、実装時の方針確定)

本 issue は todo-to-issue の起票レビューを経ずに登録したため、implement-issues の Step 3 の定めにより「## 修正方針」を未確定として扱い、実装前にユーザーへ提示した。「## 修正方針」の内容 (backend が応答に時間窓を含め、frontend が省略可能な props で xAxis の範囲に渡す。EC2 の窓は切り下げない) をそのまま採ることで確定した。frontend だけで窓を計算する案への切り替えは採らなかった。

## 完了条件

- `TimeseriesResponse` (backend) が時間窓の開始と終了をエポックミリ秒で返し、ゴールデン JSON `frontend/src/types/__contract__/TimeseriesResponse.json` が再生成されている。`frontend/src/types/contract.check.ts` の型検査が通る。
- `handleEC2Timeseries` の応答の窓が `EC2CountRecorder.Series` の絞り込みに使う境界と一致し、`handleECSTimeseries` の応答の窓が `ecsTaskCountSeries` がグリッドに使う窓と一致することを検証するテストが backend にある。
- `TimeseriesChart` に X 軸の範囲を渡すと ECharts の option の xAxis に `min` と `max` が入り、渡さないと入らないことを検証するテストが `frontend/src/components/charts/TimeseriesChart.test.tsx` にある。
- `ResourceCountChart` が応答の窓を `TimeseriesChart` に渡すことを検証するテストが `frontend/src/components/charts/ResourceCountChart.test.tsx` にある。
- Datadog Metrics と Datadog Dashboards の時系列グラフの xAxis の option は変わらない (既存テストが通る)。
- `mise run check` が通る。

## 関連

- `docs/issues/closed/0177-feat-ec2-ecs-running-count-timeseries.md`: 本グラフと `TimeseriesResponse` を追加した issue。EC2 の記録が一覧の取得時だけである設計の根拠。
- `docs/issues/closed/0183-bug-ec2-count-chart-single-point-not-drawn.md`: 1 点の系列に marker を付けた issue。点が見えるようになったことで本 issue の症状が目立つようになった。
- issue 0184 (「系列過多で Other に集約するとき全系列が欠測の時刻が落ち、離れた時刻の点が線で繋がる」): 同じ `TimeseriesChart` の別の不具合。本 issue とは独立している。

## 解決方法

### 変更内容

backend では時間窓を 1 つの値として持ち回る型を作り、handler で 1 回だけ計算した窓を、点の絞り込み (EC2) またはグリッド (ECS) と応答の両方に渡すようにした。

- `backend/internal/aws/metrics.go`: 窓をエポックミリ秒で表す `TimeseriesWindow` (`Start` / `End`) と `NewTimeseriesWindow(start, end time.Time)` を追加した。`TimeseriesRange.RecordedWindow(now)` を追加し、終端を粒度で切り下げずに `now` のまま、開始を `now - Duration` とする窓を返す。`TimeseriesResponse` に `Start` / `End` (`json:"start"` / `json:"end"`、`int64`) を追加した。`timeseriesGrid` は `(start, end time.Time)` の代わりに `TimeseriesWindow` を受け取る。
- `backend/internal/aws/ec2_metrics.go`: `EC2CountRecorder.Series` の引数を `(rng TimeseriesRange, now time.Time)` から `(w TimeseriesWindow)` に変え、`w.Start` 以上 `w.End` 以下の点を返す。終端も見るのは、窓を確定した後に別リクエストの一覧取得が `Record` を呼ぶと終端より後の点が生じ、応答の窓 (X 軸の範囲) の外に点が混ざるためである。
- `backend/internal/aws/ecs_metrics.go`: `ListECSTaskCountSeries` と `ecsTaskCountSeries` の末尾引数を `now time.Time` から `w TimeseriesWindow` に変え、グリッドは渡された窓から組む。
- `backend/internal/api/handlers_timeseries.go`: `handleEC2Timeseries` は `rng.RecordedWindow(time.Now())` を、`handleECSTimeseries` は `NewTimeseriesWindow(rng.Window(time.Now()))` を 1 回計算し、`Series` / `ecsTaskCountSeries` と応答の `Start` / `End` に同じ値を渡す。
- `backend/internal/api/server.go`: `Server.ecsTaskCountSeries` の関数型の末尾引数を `TimeseriesWindow` に変えた。
- ゴールデン: `UPDATE_GOLDEN=1 go test ./internal/contract/` で `frontend/src/types/__contract__/TimeseriesResponse.json` と `backend/internal/contract/testdata/tags.golden` を再生成した。

frontend では窓を Raw / Row 型に写し、省略可能な props で X 軸の範囲に渡す。

- `frontend/src/types/aws.ts`: `TimeseriesResponseRaw` と `TimeseriesResponseRow` に `start` / `end` を追加した。
- `frontend/src/lib/normalize.ts`: `timeseriesResponseFromRaw` で `start` / `end` を写す。
- `frontend/src/components/charts/TimeseriesChart.tsx`: `XRange` 型 (`start` / `end`) と省略可能な props `xRange` を追加し、渡されたときだけ xAxis に `min` / `max` を入れる。渡されなければ従来どおり ECharts が点の範囲から軸を決める。
- `frontend/src/components/charts/ResourceCountChart.tsx`: 応答の窓が正の幅を持つとき (`data.end > data.start`) だけ `{ start: data.start, end: data.end }` を `xRange` として渡す。`start` / `end` を返さない古い backend の応答は正規化で両方が 0 になり、そのまま渡すと軸が 0 に潰れて全系列が消えるためである。

### 完了条件の検証

- `TimeseriesResponse` が時間窓を返し、ゴールデンが再生成されている: `TimeseriesResponse.json` は `"start": 3, "end": 4` を含む形に再生成した。`contract.check.ts` の型検査 (`mise run frontend:lint` に含む `tsc --noEmit`) は 0 エラーで通過した。
- EC2 の窓が `Series` の絞り込み境界と一致し、ECS の窓がグリッドの窓と一致するテスト: `backend/internal/api/handlers_timeseries_test.go` に次の 2 つを追加した。
  - `TestHandleEC2TimeseriesWindow` (1d / 7d / 30d): 窓の幅が期間と等しいこと、返した点がすべて窓の中にあること、応答の窓で `EC2CountRecorder.Series` を再実行すると同じ点列になることを検証する。
  - `TestHandleECSTimeseriesWindow` (1d / 7d / 30d): `ecsTaskCountSeries` へ渡された窓と応答の `start` / `end` が一致すること、幅が期間と等しいこと、終端が粒度で切り下げられていることを検証する。
  - 渡した窓をグリッドがそのまま覆うことは `backend/internal/aws/ecs_metrics_test.go` の `TestEcsTaskCountSeriesPoints` (末尾の点の時刻の検証を追加) が、切り下げない窓の性質は `backend/internal/aws/metrics_test.go` の `TestTimeseriesRangeRecordedWindow` が検証する。
  - `Series` が窓の両端を含み、終端より後の点を返さないことは `backend/internal/aws/ec2_metrics_test.go` の `TestEC2CountRecorderSeries` のサブテスト「window includes both ends and drops points after the end」が検証する。
- `TimeseriesChart` の xAxis のテスト: `frontend/src/components/charts/TimeseriesChart.test.tsx` に「X 軸の範囲を渡すと xAxis の min と max に入る」と「X 軸の範囲を渡さないと min と max を入れない」を追加した。
- `ResourceCountChart` が窓を渡すテスト: `frontend/src/components/charts/ResourceCountChart.test.tsx` に「応答の時間窓を X 軸の範囲としてグラフへ渡す」、「応答が無いときは X 軸の範囲を渡さない」、「窓が正の幅を持たない応答では X 軸の範囲を渡さない (start / end を返さない古い backend)」を追加した。
- Datadog Metrics と Datadog Dashboards の xAxis は不変: `xRange` を渡さない呼び出しでは `min` / `max` を入れないため、両 View の既存テストは無変更で通過した。
- `mise run check` の通過: 統合後の作業ツリーで通過した。frontend は 96 ファイル / 1017 件 (ベースライン 1012 件から +5。いずれも今回追加したテストで、うち 1 件はレビュー指摘の反映で追加した)、eslint は 0 エラー 9 警告 (ベースラインと同数)、backend は `go test -race` で全パッケージ ok。

### 再現確認

再現手順 1 から 6 は実 AWS アカウントを要し、本環境では実行できない。
代わりに、ECharts 6.1.0 の SVG SSR による描画と、`example/home` を `HOME` にして起動した API サーバの応答で確認した (いずれも 2026-09-18)。

修正前の描画では、`min` / `max` の無い `type: 'time'` の xAxis に 1 点だけ (`t = 1789702172104`) の折れ線を描くと、1 day と 7 days のどちらで描いても軸ラベルの列が同一だった。

```
before (no min/max) 1d: ["0","0.5","1","1.5","2","18:00","06:00","12:00","18:00","06:00","12:00","18","19"]
before (no min/max) 7d: identical = true
```

修正後の描画では、同じ点列で xAxis に窓 (`now - Duration` から `now`) を `min` / `max` として渡すと、3 期間で軸ラベルの列がすべて異なる。

```
after (min/max) 1d : ["0","0.5","1","1.5","2","16:00","20:00","04:00","08:00","12:00","18"]
after (min/max) 7d : ["0","0.5","1","1.5","2","12","13","14","15","16","17","18"]
after (min/max) 30d: ["0","0.5","1","1.5","2","21","25","29","2","5","9","13","17","Sep"]
after all differ = true
```

修正後の API 応答では、EC2 の時系列エンドポイントを 3 期間で呼ぶと、窓の幅が期間そのものになり、終端は切り下げの無い現在時刻になる (記録が無いため `points` は空)。

```
1d:  {"range":"1d","period_seconds":60,"start":1789625945438,"end":1789712345438,"series":[{"name":"Running","points":[]}]}
7d:  {"range":"7d","period_seconds":300,"start":1789107545442,"end":1789712345442,"series":[{"name":"Running","points":[]}]}
30d: {"range":"30d","period_seconds":3600,"start":1787120345445,"end":1789712345445,"series":[{"name":"Running","points":[]}]}
```

| 期間 | `end - start` (ms) |
| --- | --- |
| 1d | 86,400,000 |
| 7d | 604,800,000 |
| 30d | 2,592,000,000 |

### 方針からの乖離

「## 修正方針」からの乖離は無い (backend が応答に窓を含め、frontend が省略可能な props で xAxis に渡す。EC2 の窓は切り下げない)。
方針が実装時に決めるとしていた窓の受け渡し方は、「handler が窓を計算して渡す」形を採った。
これに伴い `EC2CountRecorder.Series` と `ListECSTaskCountSeries` / `ecsTaskCountSeries` / `Server.ecsTaskCountSeries` の引数を窓に変えた。
いずれも `internal/` 配下の変更で、呼び出し元は両 handler とそれぞれのテストだけである。

### レビュー指摘の反映

多観点レビュー (完了条件の充足 / テストと堅牢性 / 規約と整合) で、テストと堅牢性の観点から 2 件 (いずれも優先度 中) の指摘を受けて反映した。

- `EC2CountRecorder.Series` は当初、窓の開始だけで絞り「記録は `Record` を呼んだ時刻に付くため終端より後の点は無い」としていた。この主張は、窓を確定した後に別リクエストの一覧取得が `Record` を呼ぶ競合で成立しない。終端でも絞るように変え、上記のサブテストを追加した。
- `ResourceCountChart` は当初、応答があれば無条件に `xRange` を渡していた。`start` / `end` を返さない古い backend の応答では両方が 0 になり、xAxis の `min` / `max` が 0 で全系列が消える。窓が正の幅を持つときだけ渡すように変え、上記のテストを追加した。

変更点に限った追加レビューでは、テストと堅牢性の観点から 1 件 (優先度 中) の指摘を受けた。終端の判定を置くと、時計の後退で直近の点が終端より後になったときにその点が落ちる、という指摘である。X 軸の `max` は窓の終端なので、終端より後の点は応答に入れても描かれず、落としても表示は変わらない。時計が追い付いた次の応答から戻る一時的な欠落である。終端を最新の点までずらす案は窓の幅が期間と一致しなくなる (`TestHandleEC2TimeseriesWindow` が検証する性質を崩す) ため採らず、この判断を `Series` のコメントに追記した。

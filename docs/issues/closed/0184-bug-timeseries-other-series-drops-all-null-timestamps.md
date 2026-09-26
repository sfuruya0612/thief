# 系列過多で Other に集約するとき全系列が欠測の時刻が落ち、離れた時刻の点が線で繋がる

Created: 2026-09-18
Model: Claude Fable 5.1
Completed: 2026-09-18

## 症状

`TimeseriesChart` は系列数が `maxSeries` (既定 `DEFAULT_MAX_SERIES`) を超えると、`frontend/src/lib/timeseries.ts` の `collapseSeries` で大きい順に `maxSeries - 1` 本を残し、残りを `Other` 系列へ足し合わせる。
この足し合わせを行う `sumPoints` は値を持つ点だけを時刻ごとの合計に入れるため、集約対象の全系列が欠測 (`v: null`) の時刻は `Other` の点列から時刻ごと消える。
その結果、個別に残した系列は欠測の時刻を `null` として持ち線が途切れるのに対し、`Other` だけは欠測の前後の点が配列上で隣接し、ECharts が 1 本の線分で結ぶ。観測していない区間に値があるように見える。

issue 0183 で `TimeseriesChart` に入れた孤立点の判定 (`isolatedPointFlags`) は点列の隣接関係だけを見るため、`Other` に含まれる孤立点は判定されず marker も付かない。判定が前提にしている「欠測が `null` の点として並んでいる」が `Other` では成り立たない。

影響する画面は、`collapseSeries` を経由する時系列グラフのうち系列数が上限を超えるもので、ECS の一覧の `Tasks per cluster` (クラスタ数が `DEFAULT_MAX_SERIES` を超えるとき)、Datadog Metrics と Datadog Dashboards の時系列グラフである。

## 再現手順

1. `frontend/` で次のスクリプトを `repro.mts` として保存し、`node --experimental-strip-types repro.mts` で実行する (`collapseSeries` は他モジュールを import しないため TypeScript を直接実行できる)。

   ```ts
   import { collapseSeries } from './src/lib/timeseries.ts';
   const T = 1_700_000_000_000;
   const grid = (vals: (number | null)[]) => vals.map((v, i) => ({ t: T + i * 60_000, v }));
   // 4 系列を maxSeries = 2 で潰す。上位 1 本を残し、残り 3 本を Other に足す。
   const series = [
     { name: 'big', points: grid([100, 100, 100, 100, 100]) },
     { name: 'a', points: grid([1, null, null, null, 1]) },
     { name: 'b', points: grid([1, null, null, null, 1]) },
     { name: 'c', points: grid([null, null, null, null, null]) },
   ];
   console.log(JSON.stringify(collapseSeries(series, 2)[1].points));
   ```

2. 期待する出力: `Other` の点列は入力と同じ 5 時刻を持ち、中央 3 時刻は欠測のまま残る。

   ```
   [{"t":1700000000000,"v":2},{"t":1700000060000,"v":null},{"t":1700000120000,"v":null},{"t":1700000180000,"v":null},{"t":1700000240000,"v":2}]
   ```

3. 実際の出力 (2026-09-18 に確認): 中央 3 時刻が消え、2 点だけになる。

   ```
   [{"t":1700000000000,"v":2},{"t":1700000240000,"v":2}]
   ```

4. この点列を `TimeseriesChart` に渡すと、`null` の点が無いため ECharts の折れ線は `t0` と `t4` を 1 本の線分で結ぶ (`connectNulls` の既定は `false` だが、繋がないための `null` そのものが無い)。個別に残した系列 `big` は同じ時刻に値を持つため対比で確認できるが、`a` と `b` を個別に描いた場合は中央で途切れる。

## 原因

`frontend/src/lib/timeseries.ts` の `sumPoints` は、`if (p.v === null) continue;` で欠測の点を読み飛ばし、時刻を `totals` の Map に登録しない。集約対象のすべての系列がある時刻で欠測だと、その時刻は出力に現れない。
関数のコメントは「欠測を 0 とみなして足し込まない」ことだけを意図として書いており、欠測の時刻を点列に残すかどうかは決めていない。`collapseSeries` を新設した `docs/issues/closed/0169` も、Other 集約の原則 (dataviz の 4 原則) を定めただけで、欠測の時刻の扱いには触れていない。

ECS の `LiveTaskCount` は RUNNING タスクが 0 のサービスでデータ点が送られず、`timeseriesGrid` が欠測を `null` で埋めるため、集約対象の全クラスタが同じ時刻で欠測になる状況は起きうる。

## 完了条件

- `collapseSeries` が返す `Other` の点列が、集約対象のいずれかの系列に存在する全時刻を含み、全系列が欠測の時刻を `v: null` の点として残す。一部の系列だけが値を持つ時刻は、その値の和になる (現在の挙動を維持する)。
- `frontend/src/lib/timeseries.test.ts` に、上記再現手順の入力で `Other` が 5 時刻を持ち中央 3 時刻が `null` になるケースと、一部の系列だけが値を持つ時刻が和になるケースがある。
- `frontend/src/components/charts/TimeseriesChart.test.tsx` に、`maxSeries` を超える系列を渡したときの `Other` で、欠測に挟まれた点が孤立点として symbol 付きのデータ項目になるケースがある (issue 0183 の孤立点の判定が `Other` にも効くことの確認)。
- `mise run check` が通る。

## 関連

- `docs/issues/closed/0169-feat-datadog-dashboards.md`: `collapseSeries` と `sumPoints` を新設した issue。
- issue 0183 (「EC2 の Running インスタンス数の折れ線グラフが記録 1 点のときに何も描かれない」): 孤立点の判定 (`isolatedPointFlags`) を入れた issue。本 issue は同 issue の close レビュー (テストと堅牢性の観点) で見つかった。

## 解決方法

`frontend/src/lib/timeseries.ts` の `sumPoints` を修正し、`collapseSeries` が返す `Other` の点列が、集約対象のいずれかの系列に存在する全時刻を含むようにした。全系列が欠測の時刻は `v: null` の点として残し、一部の系列だけが値を持つ時刻は従来どおりその値の和にする。

- 原因: `sumPoints` が `if (p.v === null) continue;` で欠測の点を読み飛ばし、その時刻を出力の Map に登録していなかった。集約対象の全系列がある時刻で欠測だとその時刻ごと消え、前後の値を持つ点が配列上で隣接して ECharts が 1 本の線分で結んでいた。
- 修正: 集計を `Map<number, number | null>` で行い、値を持つ点がまだ現れていない時刻だけ `null` として登録する。値を持つ点は従来どおり合算するため、欠測を 0 とみなして足し込まない挙動は変えていない。
- 再現確認: issue の再現手順のスクリプトで、修正前は `Other` が `[t0=2, t4=2]` の 2 点になり中央 3 時刻が落ちることを確認した。修正後は入力と同じ 5 時刻を持ち、中央 3 時刻が `null` になる。

完了条件の検証:

- `Other` が集約対象の全時刻を含み、全系列が欠測の時刻を `null` の点として残す: `frontend/src/lib/timeseries.test.ts` の「集約対象の全系列が欠測の時刻を null として残す (前後の点が線で繋がらない)」で検証する。
- 一部の系列だけが値を持つ時刻が和になる (現在の挙動の維持): 同ファイルの「一部の系列だけが値を持つ時刻は、その値の和になる」「Other は集約した系列を時刻ごとに足し合わせる」「欠測 (null) は 0 として足し込まない」で検証する。
- `Other` でも孤立点の判定が効く: `frontend/src/components/charts/TimeseriesChart.test.tsx` の「Other に集約された欠測に挟まれた点にも symbol を付ける (集約後も孤立点を判定する)」で、`Other` の両端の点が symbol 付きのデータ項目になり、中央 3 時刻が `null` のままになることを検証する。
- `mise run check` の通過: 通過を確認した。

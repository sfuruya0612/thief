# EC2 インスタンス数と ECS タスク数の時系列グラフの実現方式を調査する

Created: 2026-09-16
Model: Claude Opus 5
Completed: 2026-09-17

## 背景

`docs/issues/TODO.md` の次の項目に対応する。

> - [ ] EC2 Instance / ECS Tasks の台数の増減をグラフで表示したい
>     - EC2 は Running なインスタンス数、ECS は各クラスター、サービスごとに Running のタスク数を描画するようにしたい

### 現状の API は時点のスナップショットしか返さない

`backend/internal/aws/ec2.go` の `ListEC2Resources` は `ec2.DescribeInstances` を呼び、その時点のインスタンスを `EC2Resource` の配列で返す。
`backend/internal/aws/ecs_exec.go` の `ListECSServices` は `ecs.DescribeServices` の応答を `ECSServiceResource` に変換し、`ListECSTasks` は `ecs.ListTasks` (`DesiredStatus` 未指定のため ECS の既定で RUNNING のみ) と `ecs.DescribeTasks` を呼ぶ。
いずれも呼び出し時点の現在値だけを返し、過去の値を保持しない。

`backend/internal/api/handlers_aws.go` の `handleECS` / `handleECSServices` / `handleECSTasks` は `serveCached` 経由で応答をキャッシュするが、`backend/internal/cache/cache.go` の `Cache[V]` は 1 キーにつき `Entry[V]` (`Value` / `CachedAt` / `Expiry`) を 1 件だけ保持する TTL 付きインメモリキャッシュであり、履歴を並べて返す機能を持たない。
TTL は `backend/internal/api/server.go` の `cacheTTL = time.Hour` である。
リポジトリ内に永続化層 (`database/sql` を用いる実装) は存在しない。

したがって、台数の増減を描くための時系列データが backend にもフロントにも存在しない。これが要望が満たされていない理由である。

### CloudWatch Metrics はリポジトリで一度も使われていない

`GetMetricData`、`GetMetricStatistics`、`cloudwatch.NewFromConfig`、`aws-sdk-go-v2/service/cloudwatch` のいずれもリポジトリ内に存在しない (不在を確認済み)。
存在するのは CloudWatch Logs (`backend/internal/aws/cloudwatchlogs.go`) だけで、これはログ取得専用であり、メトリクス API とは別のサービスクライアントである。

現状のコードから確認できる EC2 / ECS 関連の API 呼び出しは `DescribeInstances`、`ListClusters` / `DescribeClusters`、`ListServices` / `DescribeServices`、`ListTasks` / `DescribeTasks` に限られる。
IAM ポリシーの定義ファイルはリポジトリ内に存在しないため、利用者のロールに `cloudwatch:GetMetricData` が含まれるかはコードからは判断できない。

## 目的

台数の増減を描くための時系列データをどこから取るかを決め、EC2 と ECS それぞれで実現できる範囲を確定させる。
方式が決まれば、完了条件を持つ実装 issue として書き直せる状態になる。

ファイル名のカテゴリは、調査 issue に将来の変更のカテゴリを付ける前例 (docs/issues/pending/0059 は feat、docs/issues/pending/0083 は perf) に従い feat とする。

## pending にした理由

本 issue は実装方針を 1 つ選ばなければ着手できない。選択肢が 2 つあり、どちらも未解決の前提を抱えている。

**案 A: CloudWatch Metrics から取得する**
`aws-sdk-go-v2/service/cloudwatch` を新規に導入し、`GetMetricData` で時系列を取得する。
backend にポーリングも永続化も要らず、画面を開く前の過去の推移も見られる。
一方で次が未解決である。

- ECS のタスク数のメトリクスとして候補に挙がる `ECS/ContainerInsights` 名前空間の `RunningTaskCount` は、クラスターごとに Container Insights を有効化していないと発行されない (この名前空間と指標名はコード上に根拠が無い外部知識であり、本 issue の時点では未確認)。有効化は利用者のアカウント側の設定であり、追加の課金を伴う。無効なクラスターで何を表示するかを決める必要がある。
- Running な EC2 インスタンス数に相当する標準メトリクスが `AWS/EC2` 名前空間にあるかは未確認である。`AWS/EC2` のメトリクスはインスタンス単位の CPU 使用率などであり、アカウント全体の台数を直接表す指標は確認できていない。Auto Scaling グループ配下であれば `AWS/AutoScaling` の指標が使える可能性があるが、Auto Scaling を使わないインスタンスは対象外になる。
- 追加で必要になる IAM 権限 (`cloudwatch:GetMetricData` 等) を利用者に要求してよいか、権限が無い場合に画面をどう見せるかが未定である。

**案 B: backend が現在値を定期的に記録して時系列にする**
既存の ECS / EC2 の API 呼び出しの結果を一定間隔で記録し、蓄積した値を時系列として返す。
新しい AWS API も追加の IAM 権限も要らない。
一方で次が未解決である。

- 蓄積先が無い。`cache.Cache[V]` は 1 件しか持たないため使えず、永続化層をリポジトリに新設することになる。プロセスを再起動すると履歴が消える設計を許容するかも未定である。
- thief は利用者の手元で起動する API サーバであり、常時起動を前提にしていない。起動していない間の推移は記録できず、「台数の増減」の用途を満たすかが不明である。

どちらを採るかは、新規依存の追加と永続化の有無という設計判断であり、`AGENTS.md` の「不明点はコードを書き始める前に質問する」が挙げる論点 (新規依存ライブラリの追加、永続化スキーマの変更) に該当する。
したがって本 issue は `issues/pending/` に置き、方式が決まるまで実装しない。

## 昇格の決着内容 (2026-09-17)

- 方式: ECS は案 A (CloudWatch Metrics) を採り、EC2 は既存の `DescribeInstances` の結果を backend が集計してプロセス内リングバッファに記録する (案 B の蓄積先のうち最小のもの)。
- EC2 が案 A を採らない理由: AWS/EC2 名前空間の標準メトリクスはインスタンス単位 (`CPUUtilization`、`NetworkIn`/`NetworkOut`、`DiskReadOps` 等) のみで、アカウントやリージョン単位で Running インスタンス数を集計した標準メトリクスは存在しない (「## pending にした理由」時点では未確認としていたが、昇格にあたり確認した)。
  出典: https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/viewing_metrics_with_cloudwatch.html
  `AWS/AutoScaling` の `GroupInServiceInstances` 相当の指標は Auto Scaling グループ配下のインスタンスにしか付かず、Auto Scaling を使わないインスタンスを含むアカウント全体の台数を表せないため、ECS と同様の CloudWatch 集計に寄せることができない。したがって EC2 は案 B (backend 側の記録) を採る。
- ECS の指標: AWS/ECS 名前空間の `LiveTaskCount` (ディメンション `ClusterName`, `ServiceName`) を使う。Container Insights は要求しない。
  `LiveTaskCount` は ACTIVATING / RUNNING / DEACTIVATING の合計で、状態別の内訳を持たない。状態別の `RunningTaskCount` は ECS/ContainerInsights 名前空間にしか無く、クラスターごとの有効化と課金を伴うため採らない。
  出典: https://docs.aws.amazon.com/AmazonECS/latest/developerguide/available-metrics.html
- ECS の粒度: クラスターごとの合計タスク数を出す。`LiveTaskCount` に `ClusterName` 単独のディメンションは無いため、クラスター内の全サービスの値を合算して 1 系列にする。サービスごとの系列は本 issue では扱わない。
- EC2 の記録: 一覧 API が AWS から実際に取得したとき (キャッシュ MISS と Refresh) だけ Running インスタンス数を記録する。定期ポーリングは行わない。
  蓄積先は profile と region ごとのプロセス内リングバッファとし、プロセス再起動で消えることを許容する。
- 期間: 直近 1 日、7 日、1 か月の 3 パターンを利用者が切り替える。CloudWatch の保持期間 (1 分粒度は 15 日、5 分粒度は 63 日) に合わせ、粒度は 1 日 = 60 秒、7 日 = 300 秒、1 か月 = 3600 秒とする。
- 取得タイミング: 一覧を開いたときだけ取得する。一覧の取得とは別の非同期リクエストで時系列を取り、届いた時点でグラフを描く。一覧の表示を時系列の取得で待たせない。
- 欠損の扱い: `LiveTaskCount` は RUNNING タスクが 0 のサービスではデータ点が送られない。グラフは欠損を 0 に潰さず線を切る (`MetricPoint.V` を null のまま返す既存の方針と同じ)。
- 権限と欠損時の表示: `cloudwatch:GetMetricData` が無い場合は既存の ACCESS_DENIED (403) の経路で理由を表示し、グラフを出さない。他の画面には影響させない。
- 新規依存: `github.com/aws/aws-sdk-go-v2/service/cloudwatch` を追加する (クラウドプロバイダ公式 SDK のため依存方針の 4 番目に該当)。
- 調査タスクにあった検証用アカウントでの `GetMetricData` の事前確認は行わず、実装時のテストと利用者の実環境確認に委ねる。

## 完了条件

- backend に `github.com/aws/aws-sdk-go-v2/service/cloudwatch` が追加され、`cloudwatch.GetMetricData` で AWS/ECS の `LiveTaskCount` を取得する関数が `backend/internal/aws/` にある。
- ECS のクラスター時系列エンドポイントが、期間 (`1d` / `7d` / `30d`) を受け取り、全クラスターについてクラスター内の全サービスの `LiveTaskCount` を合算した系列を返す。粒度は期間ごとに 60 秒 / 300 秒 / 3600 秒である。応答の点は `MetricPoint` (`t`, `v`) と同型で、欠損は `v: null` のまま返す。
- EC2 の Running インスタンス数が、`handleEC2` の AWS からの取得成功時に profile と region ごとのプロセス内リングバッファへ記録され、時系列エンドポイントが期間 (`1d` / `7d` / `30d`) で切り出して返す。キャッシュ HIT では記録されない。
- リングバッファの上限点数はファイル内定数で、上限到達時は最古の点を捨てる。複数 goroutine からの同時アクセスで競合しない (`go test -race` で確認)。
- `cloudwatch:GetMetricData` の権限が無いとき、ECS の時系列エンドポイントは 403 ACCESS_DENIED を返し、フロントは理由を表示してグラフ領域を出さない。EC2 の一覧と ECS のクラスター一覧は影響を受けない。
- フロントで EC2 の一覧 (`ServicePanel`) に Running 台数の折れ線グラフ、ECS のクラスター一覧 (`ServicePanel`) にクラスターごとの合計タスク数の折れ線グラフが `TimeseriesChart` で表示される。期間は 1 日 / 7 日 / 1 か月を切り替えられる。欠損は線を切る。
- 時系列の取得は一覧の取得とは別の `useQuery` で行い、一覧の表示は時系列の取得完了を待たない。時系列の取得中はグラフ領域に読み込み中の表示が出る。
- TanStack Query のキーは `['aws', <service>, profile, region, ...]` の形に揃え、期間をキーに含める。
- backend の取得関数とハンドラ、リングバッファ、フロントの正規化関数とグラフ表示のそれぞれにテストがある。
- `mise run check` の通過。

## 備考

- グローバル規約により、pending の issue は修正せずそのまま残す (close しない)。調査タスクの実施と結果の追記は修正に当たらない。
- 方式が決まった後の実装で流用できる既存実装を、調査の時点で確認した範囲で記録する。いずれも方式の選択を先取りするものではない。
    - backend で時系列を返す前例は `backend/internal/aws/costexplorer.go` の `CostDetail` と `backend/internal/datadog/metrics.go` の `MetricPoint` (`T int64` と `V *float64`) / `MetricSeries` / `MetricQueryResult` である。後者は timestamp と値の配列という素直な形で、新しい時系列エンドポイントの型に最も近い。
    - 時間窓をキャッシュキーに含めるハンドラの前例は `backend/internal/api/handlers_datadog_metrics.go` の `handleDatadogMetricsQuery` と `backend/internal/api/handlers_cost.go` の `handleCost` である。
    - フロントで実際に使われているグラフ部品は `frontend/src/components/charts/CostChart.tsx` (`echarts-for-react` の `ReactECharts` をラップした積み上げ棒グラフ、呼び出し元は `frontend/src/views/CostExplorerPanel.tsx`) と `frontend/src/components/charts/TimeseriesChart.tsx` (`frontend/src/views/nonaws/DatadogMetricsView.tsx` が使う折れ線グラフ) である。台数の推移には後者が近い。
    - TanStack Query のキーは `frontend/src/api/queries.ts` の `useResources` が `['aws', service, profile, region]` の形を取る。新しいエンドポイントのキーもこの形に揃える。

## 関連

- TODO.md の同じ行から派生する他の issue は無い。本 issue が親項目と子項目の両方 (EC2 の Running インスタンス数、ECS のクラスターとサービスごとの Running タスク数) を対象とする。EC2 と ECS で採れる方式が異なる可能性があるが、時系列データの取得方式という共通の設計判断を先に決める必要があるため、調査の段階では分割しない。
- `docs/issues/closed/0155` と `docs/issues/closed/0156` は ECS のタスクとコンテナインスタンスの一覧 API を追加した issue で、`ECSTaskResource` と `ECSServiceResource` の形を知る参考になる。
- `docs/issues/pending/0059`、`docs/issues/pending/0083`、`docs/issues/pending/0175`: pending の調査 issue の前例。

## 解決方法

「## 昇格の決着内容 (2026-09-17)」の方式どおりに実装した。

- backend に `github.com/aws/aws-sdk-go-v2/service/cloudwatch` を追加し、`backend/internal/aws/metrics.go` に `MetricPoint` (`t`, `v` で欠損は `v: null`)、`TimeseriesSeries`、`TimeseriesResponse` と、`GetMetricData` を呼ぶ共通ヘルパー `getMetricDataValues` を置いた。期間 (`1d`/`7d`/`30d`) ごとの時間窓と粒度 (60 秒/300 秒/3600 秒) は `TimeseriesRange.Window`/`PeriodSeconds` が決める。
- ECS は `backend/internal/aws/ecs_metrics.go` の `ListECSTaskCountSeries` が、クラスタ内の全サービスの `AWS/ECS` `LiveTaskCount` をメトリクス演算 `SUM(SEARCH(...))` (統計は `Average`) で合算し、クラスタごとに 1 系列を返す。クラスタ名は `^[A-Za-z0-9_-]{1,255}$` に一致するものだけを式に埋め込み、SEARCH 式の構文を壊しうる名前を除く。`backend/internal/api/handlers_timeseries.go` の `handleECSTimeseries` が `serveCached` 経由で応答をキャッシュし (キーに期間を含む)、`cloudwatch:GetMetricData` の権限が無い場合は既存の `writeAWSError` 経由で 403 ACCESS_DENIED を返す。一覧系のエンドポイントとは独立しており、権限が無くても一覧表示には影響しない。
- EC2 は `backend/internal/aws/ec2_metrics.go` の `EC2CountRecorder` が、profile と region の組ごとにプロセス内の固定長リングバッファ (`sync.Mutex` で保護、上限 4096 点) へ Running インスタンス数を記録する。記録は `handleEC2` (`backend/internal/api/handlers_aws.go`) の `serveCached` のクロージャ内、つまり AWS から実際に取得できたとき (キャッシュ MISS と Refresh) だけ行い、キャッシュ HIT では記録しない。`handleEC2Timeseries` は AWS へ問い合わせず、記録済みの点を期間で切り出して返すだけである。
- フロントは `frontend/src/api/queries.ts` に `useResourceTimeseries` (queryKey `['aws', service, profile, region, 'timeseries', range]`) を追加し、一覧の `useResources` とは別のクエリで取得する (一覧の表示は時系列の取得完了を待たない)。`frontend/src/components/charts/ResourceCountChart.tsx` が期間切り替えの UI と読み込み中/エラー表示を持ち、既存の `TimeseriesChart` で折れ線を描く。`frontend/src/views/AccountView.tsx` の `ServicePanel` に `countChartTitle` prop を追加し、EC2 (`Running instances`) と ECS (`Tasks per cluster`) の一覧画面にだけグラフを表示する。
- `backend/internal/aws/metrics.go`・`ec2_metrics.go`・`ecs_metrics.go`、`backend/internal/api/handlers_timeseries.go`、`frontend/src/lib/normalize.ts`、`frontend/src/components/charts/ResourceCountChart.tsx` それぞれにテストを追加した。`EC2CountRecorder` は `go test -race` でリングバッファへの並行アクセスに競合が無いことを確認した。
- 実装の完了後に行った Step 7 の多観点レビューで、`backend/internal/contract/contract.go` の `Registry` に本 issue が追加した `MetricPoint`/`TimeseriesSeries`/`TimeseriesResponse` の 3 型が未登録であり、`AGENTS.md` が定める golden JSON の型契約からこの 3 型が漏れている指摘 (優先度 高) を受けた。`Registry` への 3 型の追加、`UPDATE_GOLDEN=1 go test ./internal/contract/` によるゴールデン JSON の再生成、`frontend/src/types/contract.check.ts` への `Expect<Contract<...>>` エントリの追加で解消し、`npx tsc --noEmit` の通過を確認した。追加レビューでこの修正自体を検証し、指摘は無かった (`contract.check.ts` の型数コメントの 1 件のずれのみ追加で見つかり、同じ追加レビューの過程で修正した)。
- 併せて、「## 昇格の決着内容」に EC2 が案 A (CloudWatch Metrics) を採らない理由 (AWS/EC2 名前空間に Running インスタンス数の account/region 集計メトリクスが存在しないこと、出典付き) を追記した (レビューの中優先度の指摘への対応。pending からの昇格で許容される決着内容への追記であり、既存の記述の書き換えではない)。
- `mise run check` の通過を確認した (frontend 993/993 テスト、backend 全パッケージ ok、fmt/lint に新規の指摘なし)。

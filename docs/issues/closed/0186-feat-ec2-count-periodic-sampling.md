# EC2 の Running インスタンス数を一覧の取得とは独立に定期的に記録し、推移を描けるようにする

Created: 2026-09-18
Model: Claude Fable 5.1
Completed: 2026-09-18

## 背景

EC2 の一覧画面の `Running instances` のグラフは、期間を 1 day / 7 days / 1 month のどれにしても点が 1 つしか描かれないことが多い。
グラフ側の不具合 (issue 0183 の 1 点が描かれない問題、issue 0185 の X 軸が期間に追随しない問題) は解消済みであり、残っているのは記録される点そのものの少なさである。

### 現在の記録の契機

EC2 の台数の記録は `backend/internal/api/handlers_aws.go` の `handleEC2` にある。
`serveCached` に渡すクロージャの中で `s.ec2Counts.Record(profile, region, awsinternal.CountRunningEC2(resources), time.Now())` を呼ぶ。
このクロージャはキャッシュ MISS のときだけ実行される。
一覧のキャッシュ TTL は `backend/internal/api/server.go` の `cacheTTL = time.Hour` であるため、EC2 の画面を開いたまま操作しなくても、1 時間に 1 点しか記録されない。
点が増える契機は次の 2 つに限られる。

- TopBar の Refresh を押す (backend のキャッシュを破棄して AWS から取り直す)。
- キャッシュが切れた後に EC2 の一覧が再取得される。

記録先は `backend/internal/aws/ec2_metrics.go` の `EC2CountRecorder` (profile と region の組ごとのプロセス内リングバッファ、上限 `ec2CountRingSize = 4096` 点) で、プロセスを再起動すると消える。
したがって `thief server` を起動して EC2 の画面を開いた直後は、どの期間を選んでも点は 1 つである。

### 記録の契機を一覧の取得に限った issue 0177 の決着

issue 0177 (`docs/issues/closed/0177-feat-ec2-ecs-running-count-timeseries.md`) の「## 昇格の決着内容 (2026-09-17)」は、EC2 について次を決めている。

> EC2 の記録: 一覧 API が AWS から実際に取得したとき (キャッシュ MISS と Refresh) だけ Running インスタンス数を記録する。定期ポーリングは行わない。

AWS/EC2 名前空間にはアカウントやリージョン単位の Running インスタンス数を表す標準メトリクスがないため、CloudWatch から過去の推移を取る方式は採れない (同 issue で出典付きで確認済み)。
ECS が `LiveTaskCount` で過去の推移を描けるのに対し、EC2 は backend が自分で記録した点しか描けない。
その記録の契機を一覧の取得に限ったことが、点が 1 つしかない直接の原因である。
「台数の増減をグラフで表示したい」という元の要望 (`docs/issues/TODO.md` から派生した 0177 の背景) は、この契機のままでは満たせない。

## 目的

`thief server` が動いている間、EC2 の画面を開いているかどうかにかかわらず、Running インスタンス数を一定間隔で記録し続けるようにする。
これにより、サーバを起動したまま作業していれば 1 日で数百点の推移が描け、期間の切り替えにも意味が出る。
プロセスを再起動すると履歴が消える点は 0177 の決着どおり許容し、本 issue では変えない。

## 設計判断

### 記録の契機

backend に定期サンプリングの goroutine を 1 つ追加する。
一覧の取得時の記録 (`handleEC2` のクロージャ) はそのまま残す。
Refresh を押した直後の値がすぐグラフに入る挙動 (issue 0183 で整えた) を保つためである。
`EC2CountRecorder` は `sync.Mutex` で保護されているため、handler とサンプラーの 2 箇所から `Record` を呼んでも競合しない。

### 対象の profile と region

サンプリングの対象は、起動後に EC2 の一覧が AWS から取得された profile と region の組とする。
`EC2CountRecorder` が保持する組がそのまま対象になる。
対象は追加されるだけで、プロセスが終わるまで外れない。
`EC2CountRecorder` に、保持している組を返す `Keys()` を追加してサンプラーが参照する。

全 profile と全 region を対象にしない理由は次のとおり。

- profile の一覧は `~/.aws/config` の解析 (`backend/internal/aws/profiles.go` の `ListProfiles`) で得られるが、region の一覧は profile ごとに API で取る。全組み合わせを 5 分ごとに `DescribeInstances` で回すと、利用者が見ていないアカウントとリージョンへの呼び出しが大半を占める。
- SSO でログインしていない profile は毎回失敗し、ログを埋める。

利用者が一度開いた組に限れば、呼び出しは見ている範囲に収まり、失敗するのはログインが切れた組だけになる。

### サンプリング間隔

サンプリング間隔は `backend/internal/aws/ec2_metrics.go` の定数 `EC2CountSampleInterval = 5 * time.Minute` とする。
7 日の期間の粒度 (300 秒) と同じ間隔であり、1 日で 288 点、30 日で 8,640 点になる。
`DescribeInstances` は課金対象の API ではない。

### リングバッファの上限

`ec2CountRingSize` は現在 4096 で、5 分間隔だと約 14 日で最古の点を捨て始め、1 month の期間を覆えない。
上限を `Range30Days.Duration() / EC2CountSampleInterval` の 2 倍 (17,280 点) に変え、定期サンプリングの点と Refresh による点の両方が 30 日分収まるようにする。
`MetricPoint` は `T int64` と `V *float64` の 16 バイトで、組ごとの配列は約 270 KB になる。
組の数は利用者が開いた profile と region の数に限られるため、メモリの増加は問題にならない。
現在のコメント (「キャッシュ TTL (1 時間) で 30 日を埋めても 720 点にしかならない」) は前提が変わるため書き直す。

### サンプラーの置き場と終了経路

サンプラーは `backend/internal/api/ec2_sampler.go` に `func (s *Server) runEC2CountSampler(ctx context.Context, interval time.Duration)` として置く。
EC2 の一覧を取る関数 `s.ec2Resources` と記録先 `s.ec2Counts` の両方が `Server` にあり、テストで差し替えられる形になっているためである。
`NewServer(ctx, cfg)` が `go s.runEC2CountSampler(ctx, awsinternal.EC2CountSampleInterval)` で起動する。
`ctx` は `thief server` コマンドの context で、`backend/internal/cli/server.go` のとおりシグナルでキャンセルされる。
ループは `time.NewTicker` で刻み、`ctx.Done()` で `ticker.Stop()` して抜ける。
goroutine を起動する箇所に終了経路を持たせる規約 (`AGENTS.md` の「並行処理」) に沿う。

1 回のサンプリングでは、`Keys()` が返す組を順に回し、組ごとに `context.WithTimeout` (30 秒) を派生させて `s.ec2Resources` を呼び、成功したら `CountRunningEC2` の値を `Record` する。
失敗した組は `slog.Warn("ec2 count sample failed", "profile", profile, "region", region, "err", err)` を出して点を記録しない (取得できなかった時刻を 0 や前回値で補わない)。
失敗しても次の組と次の周期は続ける。
サンプリングの結果は一覧のキャッシュ (`resourceCache`) には書かない。
一覧の鮮度とキャッシュヘッダの意味を変えないためである。

### frontend の扱い

時系列 API の形も `ResourceCountChart` も変えない。
グラフの再取得は既存の経路 (一覧の取得成功時の invalidate、TanStack Query の `staleTime` 60 秒とウィンドウフォーカス時の refetch、期間の切り替え) に任せる。

### 採らなかった案

- **CloudWatch から過去の推移を取る**：背景で述べたとおり、AWS/EC2 名前空間に該当する標準メトリクスがない。
- **全 profile と全 region を対象にする**：「対象の profile と region」で述べたとおり、見ていない範囲への呼び出しとログインが切れた profile の失敗ログが大半になる。
- **サンプリングの結果で一覧のキャッシュも更新する**：一覧の TTL とキャッシュヘッダ (`X-Cache-Status` など) の意味が変わる。グラフの点を増やす目的に対して副作用が大きい。
- **記録をファイルに永続化して再起動をまたぐ**：保存先とフォーマットの設計判断を伴う。定期サンプリングが入れば、サーバを起動している間の推移という価値は永続化なしでも成り立つため、永続化は別 issue に分ける。
- **`useResourceTimeseries` に `refetchInterval` を付けて開いたままのグラフを自動更新する**：このフックは ECS と共用で、ECS では CloudWatch の呼び出しが増える。EC2 だけに限る条件分岐を入れるほどの実需はまだない。
- **間隔を設定項目 (`THIEF_EC2_SAMPLE_INTERVAL` など) にする**：変える実需がない。定数で始め、実需が出てから設定に出す (YAGNI)。

## 完了条件

- `EC2CountRecorder` に、保持している profile と region の組を返す `Keys()` がある。
- `backend/internal/aws/ec2_metrics.go` に `EC2CountSampleInterval` (5 分) があり、`ec2CountRingSize` が `Range30Days.Duration() / EC2CountSampleInterval` の 2 倍以上であることをテストが検証する。
- `backend/internal/api/ec2_sampler.go` の `runEC2CountSampler` が、`Keys()` の各組について `s.ec2Resources` を呼び、成功時に `CountRunningEC2` の値を `Record` し、失敗時は `slog.Warn` を出して記録しない。失敗した組があっても他の組と次の周期の処理は続く。
- `runEC2CountSampler` は `ctx` のキャンセルで終了する。テストは短い間隔と差し替えた `ec2Resources` で複数周期の記録と、キャンセル後に goroutine が抜けることを検証する。
- `NewServer` がサンプラーを起動し、`thief server` の終了 (context のキャンセル) でサンプラーも終了する。
- `handleEC2` の一覧取得時の記録は従来どおり行われる (既存テスト `TestHandleEC2RecordsOnCacheMissOnly` が通る)。
- `EC2CountRecorder` への handler とサンプラーからの同時アクセスに競合がない (`go test -race`)。
- `ec2CountRingSize` と `Record` の godoc コメントが、記録の契機が 2 つ (一覧の取得とサンプリング) になった前提で書き直されている。
- frontend の変更はない。
- `mise run check` の通過。

## 関連

- `docs/issues/closed/0177-feat-ec2-ecs-running-count-timeseries.md`：本グラフと `EC2CountRecorder` を追加した issue。「定期ポーリングは行わない」とした決着を本 issue で変える。
- `docs/issues/closed/0183-bug-ec2-count-chart-single-point-not-drawn.md`：1 点の系列に marker を付け、一覧の取得成功時に時系列クエリを invalidate するようにした issue。
- `docs/issues/closed/0185-bug-ec2-count-chart-x-axis-ignores-range.md`：X 軸を応答の時間窓に合わせた issue。本 issue で点が増えても軸の挙動は変わらない。

## 解決方法

EC2 の Running インスタンス数を、一覧の取得とは独立に一定間隔で記録するサンプラーを追加した。これで `thief server` を起動したまま EC2 の画面を一度開けば、以降は画面を開いていなくても 5 分ごとに点が積まれ、期間の切り替えに意味が出る。

- `backend/internal/aws/ec2_metrics.go`: `EC2CountSampleInterval = 5 * time.Minute` を追加した。`ec2CountRingSize` を `2 * int(30*24*time.Hour/EC2CountSampleInterval)` (17,280 点) に変え、定期サンプリングだけで 30 日分 (8,640 点) を保持できるようにした。`Keys()` と `EC2CountTarget` を追加し、記録を持つ profile と region の組をサンプラーへ渡せるようにした。`ec2CountRingSize` と `Record`、`Series` の godoc を、記録の契機が一覧の取得と定期サンプリングの 2 つになった前提へ書き直した。定数式ではメソッド `Range30Days.Duration()` を呼べないため 30 日は `30*24*time.Hour` で表し、`Range30Days.Duration()` との一致はテストで固定する。
- `backend/internal/api/ec2_sampler.go`: `runEC2CountSampler(ctx, interval)` と `sampleEC2Counts(ctx)` を追加した。`Keys()` の各組について `s.ec2Resources` を 30 秒のタイムアウト付きで呼び、成功時に `CountRunningEC2` の値を `Record` する。失敗した組は `slog.Warn("ec2 count sample failed", ...)` を出して記録せず、他の組と次の周期を続ける。サンプリング結果は一覧の `resourceCache` には書かない。ループは `time.NewTicker` で刻み、`ctx.Done()` で `ticker.Stop()` して終了する。
- `backend/internal/api/server.go`: `NewServer` が `go s.runEC2CountSampler(ctx, awsinternal.EC2CountSampleInterval)` でサンプラーを起動する。`ctx` は `thief server` コマンドのシグナル連動 context で、サーバ終了時にサンプラーも終了する。
- `frontend` の変更はない。時系列 API の形も `ResourceCountChart` も変えていない。

完了条件の検証:

- `EC2CountRecorder.Keys()` が保持している組を返す: `backend/internal/aws/ec2_metrics_test.go` の `TestEC2CountRecorderKeys` で、組が無いとき空を返し、記録した 2 組を重複なく返すことを検証する。
- `EC2CountSampleInterval` と `ec2CountRingSize` の関係: 同ファイルの `TestEC2CountRingSizeCovers30Days` が `ec2CountRingSize >= 2 * int(Range30Days.Duration()/EC2CountSampleInterval)` を検証する。
- `runEC2CountSampler` の記録と失敗時の継続: `backend/internal/api/ec2_sampler_test.go` の `TestRunEC2CountSamplerRecordsOnEachTick` で複数周期の記録を、`TestSampleEC2CountsRecordsSuccessfulTargetsDespiteFailures` で失敗した組が記録されず成功した組が 2 周期分記録されることを検証する。
- `ctx` のキャンセルで終了する: `TestRunEC2CountSamplerRecordsOnEachTick` がキャンセル後の goroutine 終了を、`TestRunEC2CountSamplerReturnsWhenContextAlreadyCancelled` がキャンセル済み context での即時終了を検証する。
- `handleEC2` の一覧取得時の記録が従来どおり: 既存の `TestHandleEC2RecordsOnCacheMissOnly` が通ることを確認した。
- 競合が無い: `mise run backend:test` (`go test -race -cover ./...`) の通過を確認した。
- `mise run check` の通過: 通過を確認した。

## reopen の理由

close 直後に、実装方針を「backend がプロセス内に定期サンプリングして記録する」から「CloudWatch の `AWS/AutoScaling` `GroupInServiceInstances` を Auto Scaling グループごとに取得する」へ変更する指示を受けた。CloudWatch から時間窓を指定して過去の推移を取得できるため、プロセス内の記録は不要になる。元の「## 解決方法」の `EC2CountRecorder` と定期サンプラーはこの方針転換に伴って撤去する。元の完了条件のうち「定期サンプリング」「リングバッファ」「`Keys()`」「`EC2CountSampleInterval`」に関する行は、以下の再設計の完了条件で置き換える。

- `ListAutoScalingGroupNames` (`backend/internal/aws/autoscaling.go`) が `DescribeAutoScalingGroups` のページを跨いで、空でない Auto Scaling グループ名を返す。
- `ListEC2InstanceCountSeries` (`backend/internal/aws/ec2_metrics.go`) がグループごとに `AWS/AutoScaling` `GroupInServiceInstances` を MetricStat で取得し、グループ名の昇順で 1 グループ 1 系列を返す。欠測は `v: null` のままグリッドに並ぶ。
- `handleEC2Timeseries` が他の時系列と同じく `serveCached` で応答をキャッシュし、期間をキャッシュキーに含める。`cloudwatch:GetMetricData` または `autoscaling:DescribeAutoScalingGroups` の権限が無ければ 403 ACCESS_DENIED を返す。
- `handleEC2` の記録、`EC2CountRecorder`、定期サンプラーを削除する。
- frontend の EC2 グラフが Auto Scaling グループごとの系列を描き、グラフの右下に「Auto Scaling グループに属するインスタンスのみ」の注記を出す。
- `mise run check` の通過。

## 解決方法 (再対応)

方針転換後の実装。元の「## 解決方法」の定期サンプリングは撤去し、CloudWatch から直接取得する。

- `backend/internal/aws/autoscaling.go` (新規): `ListAutoScalingGroupNames` が `DescribeAutoScalingGroups` をページ送りし、空でないグループ名を返す。`listAutoScalingGroupNamesWith` にクライアント生成と分離したコアを置き、単体テストで固定できるようにした。
- `backend/internal/aws/ec2_metrics.go`: `ListEC2InstanceCountSeries` と `ec2InstanceCountSeries` を追加。グループ名の昇順に 1 グループ 1 クエリ (`MetricStat`、名前空間 `AWS/AutoScaling`、指標 `GroupInServiceInstances`、ディメンション `AutoScalingGroupName`、統計 `Average`、粒度は期間から) を組み、`timeseriesGrid` で欠測を null 埋めする。500 件でバッチ分割する。`SEARCH` 式ではなく構造化したディメンションで引くため、グループ名の文字種による式の破壊がない。`EC2CountRecorder` / `EC2CountTarget` / `Keys` / ring buffer / `EC2CountSampleInterval` / `ec2CountRingSize` / `CountRunningEC2` を削除した。
- `backend/internal/aws/metrics.go`: 記録用だった `TimeseriesRange.RecordedWindow` を削除した (CloudWatch のグリッドに合わせ `Window` のみを使う)。
- `backend/internal/api/handlers_timeseries.go`: `handleEC2Timeseries` を ECS と同型の `serveCached` (`ec2-timeseries` キー、期間を含む) に変更し、`s.ec2InstanceCountSeries` を呼ぶ。
- `backend/internal/api/handlers_aws.go`: `handleEC2` から台数の記録を削除した。
- `backend/internal/api/server.go` / `server_test.go`: `ec2Counts` フィールドとサンプラー起動を削除し、`ec2InstanceCountSeries` フィールドを追加した。`ec2_sampler.go` / `ec2_sampler_test.go` を削除した。
- `frontend`: `ResourceCountChart` に `caption` prop を追加し、グラフ右下に注記を表示する。`AccountView` の EC2 は見出し `In-service instances` と注記 `Only instances in an Auto Scaling group` を渡す。
- 新規依存: `github.com/aws/aws-sdk-go-v2/service/autoscaling` (クラウドプロバイダ公式 SDK)。IAM は `autoscaling:DescribeAutoScalingGroups` が新たに必要。

完了条件の検証:

- グループ名の列挙とページ送り: `backend/internal/aws/ec2_metrics_test.go` の `TestListAutoScalingGroupNamesWith` (空名と nil を除外し、2 ページを跨いで返す) と `TestListAutoScalingGroupNamesWithError` で検証する。
- クエリの形・系列・欠測・バッチ・エラー: 同ファイルの `TestEC2InstanceCountSeriesQueries` / `TestEC2InstanceCountSeriesPoints` / `TestEC2InstanceCountSeriesNoGroups` / `TestEC2InstanceCountSeriesBatches` / `TestEC2InstanceCountSeriesError` で検証する。
- ハンドラのキャッシュ・窓・権限: `backend/internal/api/handlers_timeseries_test.go` の `TestHandleEC2Timeseries` (同じ期間の 2 回目はキャッシュ、別期間は再取得) / `TestHandleEC2TimeseriesWindow` / `TestHandleEC2TimeseriesAccessDenied` で検証する。一覧の失敗は `TestHandleEC2ListError` で確認する。
- frontend の注記: `frontend/src/components/charts/ResourceCountChart.test.tsx` の `caption を渡すとグラフの右下に注記を出す` / `caption を渡さなければ注記を出さない` で検証する。
- `mise run check` の通過: 通過を確認した。

## 撤回 (2026-09-18)

Auto Scaling 方式の実値確認で、`AWS/AutoScaling` のグループメトリクス (`GroupInServiceInstances`) は ASG ごとの有効化 (`EnableMetricsCollection`) が必要なオプトインであり、実アカウント (`example-common` / `example-dev`) の ASG はいずれも無効 (`EnabledMetrics: []`) で CloudWatch に一切発行されていないことが分かった。インスタンスを持たない ASG ではそもそも発行されない。AWS/EC2 名前空間にもアカウントやリージョン単位の Running 台数を表す標準メトリクスが無いため、EC2 の台数集計そのものを諦め、実装を削除する。

- 削除したもの: `backend/internal/aws/ec2_metrics.go` / `backend/internal/aws/autoscaling.go` とそれぞれのテスト、`handleEC2Timeseries` と `/ec2/timeseries` ルート、`Server.ec2InstanceCountSeries`、`Server.ec2Counts` 系 (前回の再対応で削除済み)、frontend の EC2 グラフ設定 (`countChartTitle` / `countChartCaption`) と `ResourceCountChart` の caption 機構、`github.com/aws/aws-sdk-go-v2/service/autoscaling` 依存。
- 残すもの: ECS の Tasks per cluster の時系列グラフ (`handleECSTimeseries` / `TimeseriesChart` / `ResourceCountChart`) はそのまま。
- これに伴い、`## 解決方法 (再対応)` の ASG 実装は本撤回で取り消される。

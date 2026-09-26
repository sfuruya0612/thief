# Cloud Run の ListJobs 逐次実行を計測結果に基づいて並列化する

Created: 2026-07-18
Completed: 2026-07-18
Model: Claude Opus 4.8

## 背景

docs/issues/closed/0041-perf-cloudrun-slow-fetch.md で、Cloud Run のデータ取得が約 50 秒かかる件の
原因調査として `internal/gcp/cloudrun.go` の `ListCloudRun` に区間計測 (`log/slog`) を追加した。

サービス一覧取得、`listRunLocations`、ロケーションごとの `ListJobs` (逐次実行) のそれぞれの
所要時間がログに出力されるようになっている。本作業環境には実 GCP プロジェクトへの認証情報が
なく、実測して原因を特定するところまでは行えなかった。

## 目的

0041 で追加した計測ログを実環境で確認し、遅延の主因を特定したうえで改善する。

## 対応内容

### 計測

- 実際に遅い Cloud Run 対象プロジェクトに対して `GET /api/gcp/cloudrun` を呼び出し、以下の
  ログを確認する。
  - `cloud run list services done`
  - `cloud run list locations done`
  - `cloud run list jobs in location done` (ロケーションごと)
  - `cloud run list jobs done (all locations, sequential)`
  - `cloud run list all done`
- 逐次処理 (ロケーション数に比例した時間) と個々の API 応答のどちらが支配的かを切り分ける。

### backend (逐次処理が主因だった場合)

- ロケーションごとの `ListJobs` を `golang.org/x/sync/errgroup` で並列化する。キャンセルと
  エラー伝播を揃え、goroutine の終了経路を確保すること。
- サービス取得・ロケーション取得・ジョブ取得の同時実行が有効かも検討する。

## 設計上の論点

- 目標とする取得時間を定める必要がある。原因が GCP 側 API のロケーション単位のレイテンシなど
  並列化で縮まらない要因であれば、短縮できない可能性がある。この場合は「短縮」を完了条件に
  できないため、目標値または「有意な短縮 (before / after を明記)」として定量化する。

## 完了条件

- 実環境のログから遅延の原因が特定され、記録されている
- (改善を行う場合) 改善が入り、設計上の論点で定めた目標に対する before / after が記録されている

## 解決方法

### 計測結果

ユーザが実環境 (project_id=example-project-a、ロケーション 43 件) で計測したログから、
遅延の主因はロケーションごとの `ListJobs` の逐次実行であると特定した。

| フェーズ | 所要時間 |
| --- | --- |
| `ListServices` | 931ms |
| `listRunLocations` | 621ms |
| `ListJobs` (43 ロケーション、逐次) | 43360ms |
| 全体 | 44913ms |

`ListJobs` の逐次実行だけで全体の 96.6% (43360ms / 44913ms) を占めており、個々の API 応答自体
(129ms 〜 2291ms、平均約 1008ms) はロケーションあたりでは遅くない。ロケーション数に比例して
時間が伸びる逐次実行が支配的要因と判断した。

### backend

- `internal/gcp/cloudrun.go` の `ListCloudRun` で、ロケーションごとの `ListJobs` を
  `golang.org/x/sync/errgroup` で並列化した。
  - 各 goroutine は結果を `jobsByLocation[i]` (ロケーションごとに専用の index) へ書き込み、
    共有ミューテーションなしで競合を避ける (goroutine ごとにデータオーナーシップを分離)。
  - `errgroup.WithContext` でエラー伝播とキャンセルを揃え、`g.Wait()` で全 goroutine の終了を
    待ってから集約する (終了経路の確保)。
  - Cloud Run Admin API の QPS クオータに抵触しないよう `g.SetLimit(listJobsConcurrency)` で
    同時実行数の上限を設けた (ユーザ指定によりロケーション数 43 に対して 15)。

### 計測結果 (before / after)

ユーザが実環境 (project_id=example-project-b、ロケーション 43 件、同時実行数 10 で最初に検証、
その後 15 へ変更) で計測した結果:

| フェーズ | before (逐次) | after (並列、同時実行数 10) |
| --- | --- | --- |
| `ListJobs` (全ロケーション) | 43360ms | 4776ms |
| 全体 | 44913ms | 6317ms |

`ListJobs` は 43360ms → 4776ms (約 89% 短縮)、全体は 44913ms → 6317ms (約 86% 短縮) となった。
before/after は異なるプロジェクトでの計測 (いずれもロケーション数 43、対象リソースなしの
プロジェクト) だが、支配的要因だった逐次実行を並列化したことで大幅な短縮を確認できた。
同時実行数は計測後にユーザ指定で 15 に変更している (after 計測時点の 10 よりわずかに緩和)。

### 完了条件の確認

- 遅延の原因 (`ListJobs` の逐次実行、全体の 96.6%) が計測に基づいて特定され、記録されている。
- 改善 (`errgroup` による並列化) を行い、before / after (44913ms → 6317ms) を記録した。

# S3 バケット一覧取得をバケット間で並列化して高速化する

Created: 2026-07-23
Completed: 2026-07-23
Model: Claude Opus 4.8

## 背景

`docs/issues/TODO.md` に「S3 の一覧を取得する画面のロードが遅い。リージョンごとに取りに行っているなら、Cloud Run 同様に並列呼び出しにして」という要望がある。

`ListS3Resources` (backend/internal/aws/s3.go) は `ListBuckets` の後、各バケットについて `GetBucketLocation` / `GetBucketEncryption` / `GetPublicAccessBlock` の 3 本を逐次で呼ぶ。
これをバケット間の for ループで直列に繰り返すため、合計で「1 + 3N」本 (N はバケット数) の API 呼び出しを直列に行う。
バケットが数十件あると S3 コントロールプレーンの往復遅延が積み上がり、一覧取得が数十秒に達する。

これは issue 0043 で並列化した Cloud Run の ListJobs (ロケーション 43 件の逐次実行が全体 44.9 秒中 43.4 秒を占めていた) と同型の N+1 構造である。

## 目的

`ListS3Resources` のバケットごとの属性解決をバケット間で並列実行し、一覧取得を高速化する。

## 設計判断

- issue 0043 の `ListCloudRun` (backend/internal/gcp/cloudrun.go) と同じ errgroup パターンを踏襲する。結果スライスを事前確保し、各 goroutine は自分の index にのみ書き込む (ロック不要でデータ競合しない)。
- 同時実行数は `errgroup.Group.SetLimit` で上限を設ける (S3 のリクエストレート上限への配慮)。
- AWS SDK for Go v2 の `*s3.Client` は複数 goroutine から並行安全なため、単一の us-east-1 クライアントを共有する。クライアントの作り直しは不要。
- 並列化はバケット間 (for ループ) のみとする。各バケット内の 3 本 (region / encryption / public) も独立だが、主因はバケット間の直列であり、入れ子の並列化は最小差分を優先して見送る。
- CLI 経路の `ListS3BucketInfos` はバケットごとの解決を行わない軽量版のため対象外。
- `golang.org/x/sync/errgroup` は導入済み (cloudrun.go が使用) で新規依存はない。

## 完了条件

- `ListS3Resources` をバケット間で並列化する。
- `mise run check` が全て通過する。

## テストの扱い

`ListS3Resources` と `s3FromBucket`、各 resolver は具象 `*s3.Client` を受け取り interface 化されていない。実 S3 を叩く並列コードのユニットテストはモックなしには書けず、モック化は影響範囲が大きい。issue 0043 の Cloud Run 並列化も client を interface 化せず並列パスのテストを追加せずにマージされている。本 issue も同じ方針とし、並列化の正しさは index 書き込みパターン (順序保持・レースフリー) で担保する。既存テスト (純関数のみ) は並列化の影響を受けず通過する。

## 解決方法

- `backend/internal/aws/s3.go`: `ListS3Resources` のバケットループを `errgroup` で並列化する。結果スライス `resources` を事前確保し、各 goroutine が自分の index に `s3FromBucket` の結果を書き込む (ロック不要)。同時実行数の上限定数 `s3BucketConcurrency` (= 30) を `SetLimit` で設定し、単一の us-east-1 `*s3.Client` を全 goroutine で共有する。
- import に `golang.org/x/sync/errgroup` を追加 (既存依存、go.mod 変更なし)。
- `mise run backend:lint` (vet / staticcheck / govulncheck / golangci-lint): 0 件。`mise run backend:test` (-race): 全パッケージ pass。既存テストは並列化の影響を受けない。

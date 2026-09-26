# torow.go の到達不能な ToRow() 実装を削除する

Created: 2026-08-05
Model: Claude Fable 5
Completed: 2026-08-07

## 背景

TODO.md の「torow.go の到達不能な ToRow() 実装を削除するか CLI が使う型を統一したい (issue 0093 のスコープ外として送り)」に対応する。

`backend/internal/aws/torow.go` には 18 の `ToRow()` メソッドがあるが、呼び出し元があるのは 7 つだけで、次の 11 メソッドはリポジトリ内のどこからも呼ばれていない。

- `EC2Resource` / `RDSResource` / `ElastiCacheResource` / `ECSResource` / `ECRRepoResource` / `ECRImageResource` / `S3Resource` / `IAMResource` / `SSMParameterResource` / `SecretResource` / `CFNStackResource`

これらの CLI 表示は、Resource 型ではなく各サービスの Info / Summary 系の型 (レガシー CLI 互換とコメントされた型) が担っており、Resource 型の `ToRow()` は使われないまま残っている。
使われないメソッドは構造体のフィールド追加に追従せず腐る。
実例として `ElastiCacheResource` (`backend/internal/aws/elasticache.go` 14-28 行) は 13 フィールドを持つが、`ElastiCacheResource.ToRow()` は `Name` / `Port` / `ParameterGroup` / `ReplicationGroupID` / `NodeAvailabilityZones` / `CostMonthly` の 6 フィールドを既に欠いている。
docs/issues/closed/0093 で `NodeAvailabilityZones` を追加した際にこの腐敗が発見され、スコープ外として本 TODO に送られた。

## 目的

呼び出し元の無い 11 の `ToRow()` メソッドを削除し、「存在するのに追従されないコード」を無くす。

## 設計判断

- TODO が併記するもう一方の案「CLI が使う型を統一する」(Resource 型と Info / Summary 型の統合) は採らない。両者の分離は意図的なものだからである。例えば `backend/internal/aws/secretsmanager.go` の Info 型には「SecretResource と異なり値を含まない」というコメントがあり、API サーバ向けの Resource 型と CLI 向けの型で公開するフィールドを意図して変えている。統一すると CLI に出すべきでない情報 (シークレット値等) の扱いを型ごとに再設計する必要があり、refactor の範囲を超える。
- 削除対象は上記 11 メソッドのみとする。呼び出し元のある 7 メソッド (`LambdaResource` / `SSOAccountResource` / `KinesisResource` / `CloudFrontResource` / `ELBResource` / `CostResource` / `ForecastResource`) と Info / Summary 系の `ToRow()` は変更しない。
- 削除後に将来 CLI で Resource 型の表示が必要になった場合は、その時点の構造体定義から書き直す方が、腐った実装を復活させるより安全である。
- 追加の API 呼び出しや権限は不要。挙動を変えない削除のみである。

## 実装詳細の乖離

完了条件の「変更は `backend/internal/aws/torow.go` からのメソッド削除のみ」に対し、`backend/internal/aws/ec2.go` のヘルパー関数 `tagMapStr` とそれだけが使っていた `strings` import も削除した。
`tagMapStr` の呼び出し元は削除対象の `EC2Resource.ToRow()` だけであり、残すと staticcheck の未使用検出 (U1000) で `mise run check` が通らないためである。
到達不能なコードを削除するという方式そのものは変えていない。

## 完了条件

- `backend/internal/aws/torow.go` に残る `ToRow()` が、呼び出し元のある 7 メソッドのみになる。
- `mise run backend:build` と `mise run backend:test` が通る (build が本体コード、test がテストコードのコンパイルを通すことで、削除対象に呼び出し元が無いことの機械的な裏付けになる)。
- `internal/cli` のコードには変更を加えない (変更は `backend/internal/aws/torow.go` からのメソッド削除のみ)。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0093: 本 issue の起票元。`ElastiCacheResource.ToRow()` のフィールド欠落が発見された。
- docs/issues/0100: 到達可能な `ToRow()` と列定義の対応テスト。0100 はテスト対象を到達可能な型に限定しているため、本 issue の削除と実施順序に依存しない。

## 解決方法

- `backend/internal/aws/torow.go` から呼び出し元の無い 11 の `ToRow()` メソッド (`EC2Resource` / `RDSResource` / `ElastiCacheResource` / `ECSResource` / `ECRRepoResource` / `ECRImageResource` / `S3Resource` / `IAMResource` / `SSMParameterResource` / `SecretResource` / `CFNStackResource`) を削除し、呼び出し元のある 7 メソッドだけを残した。
- 削除対象の `EC2Resource.ToRow()` だけが使っていた `backend/internal/aws/ec2.go` の `tagMapStr` と、その削除で未使用になる `strings` import も併せて削除した (「実装詳細の乖離」を参照)。
- `internal/cli` には変更を加えていない。
- `mise run check` の通過を確認した (build と test のコンパイル通過が、削除対象に呼び出し元が無いことの機械的な裏付けになる)。

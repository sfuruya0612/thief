# Kinesis の一覧に Mode 列を追加する

Created: 2026-07-28
Completed: 2026-07-29
Model: Claude Fable 5

## 背景

`docs/issues/TODO.md` の「Kinesis の Mode (ON_DEMAND or PROVISIONED) を一覧に表示したい」に対応する。

backend `internal/aws/kinesis.go` の `KinesisResource` (16-25 行) は `ID`/`Name`/`State`/`ShardCount`/`RetentionHours`/`EncryptionType`/`Tags`/`CostMonthly` のみを持ち、Mode に相当するフィールドがない。
`ListKinesisResources` (38-96 行) は各ストリームに対して `client.DescribeStreamSummary` (74-76 行) を並列に呼び出し、その結果を `kinesisFromSummary` (98-110 行) で `KinesisResource` に変換しているが、`kinesisFromSummary` は `StreamModeDetails` を読んでいない。ストリーム一覧自体は `kinesis.NewListStreamsPaginator` (53 行) で全ページを走査済みであり、ページネーションは Mode 追加によって影響を受けない。

`backend/go.mod` は `github.com/aws/aws-sdk-go-v2/service/kinesis v1.43.6` を使用する。この SDK の `DescribeStreamSummary` レスポンスである `types.StreamDescriptionSummary` は `StreamModeDetails *StreamModeDetails` フィールド (types.go:603) を既に持つ。`StreamModeDetails` 構造体 (types.go:616 で宣言) の `StreamMode` フィールド (types.go:623、コメントに "This member is required" とあるため `StreamModeDetails` が非 nil なら必ず値を持つ) は `enums.go:187-192` で `StreamModeProvisioned = "PROVISIONED"` / `StreamModeOnDemand = "ON_DEMAND"` と定義された enum である。
現状のコードは `DescribeStreamSummary` を既に呼び出しており、レスポンスの一部を読んでいないだけなので、追加の API 呼び出しは不要である。

frontend も同様に Mode を持たない。`frontend/src/types/aws.ts` の `KinesisRaw` (741-750 行) と `KinesisRow` (752-761 行) にはいずれも Mode 相当のフィールドがなく、`frontend/src/lib/normalize.ts` の `kinesisFromRaw` (628-639 行) もこれらのフィールドをそのまま変換するだけである。`frontend/src/components/tables/columns.tsx` の `kinesisColumns` (1266-1299 行) は `name` → `state` → `shardCount` → `retentionHours` → `encryptionType` → `region` の順で列を定義しており、Mode 列がない。

DynamoDB には ON_DEMAND / PROVISIONED に相当する Billing Mode を表示する先行実装が既にある。`backend/internal/aws/dynamo.go` の `dynamoFromDescription` (123-148 行) は `t.BillingModeSummary` が nil の場合は `"provisioned"` を既定値とし (136 行のコメント「BillingModeSummary が未設定の場合、既定はプロビジョンド」、137 行で `mode = "provisioned"` を代入)、`dynamodbtypes.BillingModePayPerRequest` を `"on-demand"`、`dynamodbtypes.BillingModeProvisioned` を `"provisioned"` に変換して `DynamoResource.Mode` に格納する。frontend では `frontend/src/components/tables/columns.tsx` の `dynamoColumns` (1170-1211 行) のうち `mode` 列 (1178-1183 行) が `<span style={mutedMono}>{r.mode}</span>` という素のテキスト表示で、`StatusBadge` は使っていない。

## 目的

Kinesis の一覧で、各ストリームが ON_DEMAND か PROVISIONED かを一覧上で判別できるようにする。

## 設計判断

- 追加の API 呼び出しは行わない。`ListKinesisResources` が既に呼んでいる `DescribeStreamSummary` のレスポンスから `StreamModeDetails.StreamMode` を読むだけで実現できる。追加の IAM 権限も不要である。
- 値の表記は DynamoDB の `Mode` フィールドと同じ `"on-demand"` / `"provisioned"` (小文字・ハイフン区切り) に揃える。SDK の enum 値 (`"ON_DEMAND"` / `"PROVISIONED"`) をそのまま表示する案は採らない。同じ「モード」概念を持つ既存列 (DynamoDB) と表記が異なると一覧を横断的に見たときの一貫性が失われるため。
- `StreamModeDetails` が nil の場合は `"provisioned"` を既定値とする。DynamoDB の `BillingModeSummary` が nil のときの既定値の扱い (136-137 行) と同じ考え方である。`StreamMode` フィールド自体は `StreamModeDetails` 構造体内では必須 (types.go:623 のコメントで "This member is required") だが、`StreamModeDetails` へのポインタ自体は `StreamDescriptionSummary` 側で必須指定されていないため nil になり得る。オンデマンドキャパシティモードが導入される前の古いストリームで `StreamModeDetails` が返らないケースを想定した安全側のデフォルトである。
- `StreamMode` が `ON_DEMAND` でも `PROVISIONED` でもない未知の値を返した場合も `"provisioned"` にフォールバックする。SDK の enum は将来値が追加される可能性があり、既知の 2 値以外を未定義動作にしない。
- 列の表示形式は DynamoDB の `mode` 列 (1178-1183 行) と同じ `mutedMono` の素のテキスト表示にする。`StatusBadge` は状態 (正常/異常など状態遷移がある値) を表すために使われており、Mode は状態ではなく構成上の分類であるため、`StatusBadge` によるバッジ化は採らない。
- CLI (`thief kinesis`) にも Mode を表示する。Kinesis は ElastiCache (`ElastiCacheClusterInfo`/`ListElastiCacheClusterInfos`、`internal/aws/elasticache.go:131-164`) のようなレガシー CLI 互換の別型を持たず、`internal/cli/kinesis.go` は Web API と同じ `awsinternal.KinesisResource` を `ListConfig[awsinternal.KinesisResource]` に渡して CLI 出力を作っている。したがって Web API 側にのみ Mode を追加すると、CLI (`thief kinesis`) では表示されないという食い違いが生じる。これを避けるため、CLI の列定義と `ToRow()` にも Mode を含める。
- 列の位置は `state` 列の直後、`shardCount` 列の直前に追加する。DynamoDB の `dynamoColumns` で `mode` 列が `state` 列の直後に置かれている配置 (1177-1183 行) に揃える。
- `ColumnDef.width` は `kinesisColumns` の既存 6 列 (name/state/shardCount/retentionHours/encryptionType/region) の合計で既に 100% になっており (24/12/11/14/15/24)、新規列を追加するだけでは合計が 100% を超える。`e47f9a4` (ElastiCache に Replication Group 列を追加した際、既存 6 列の width を全て縮小し新規列を挿入して合計 100% を維持した前例) および `2a3bdd1` (RDS に Engine Version 列を追加した際も同様の再配分を行った前例) に倣い、既存 6 列の width を再配分した上で `mode` 列を追加する。再配分は、追加する `mode` 列の幅を差し引いた残りを既存 6 列の現在の比率で分配する (各列の新 width = 現 width ÷ 現在の合計 100% × (100% − mode 列の幅))。端数調整で合計が 100% からずれる場合は最も幅の大きい列で調整する。既存列の最小値 (`kinesisColumns` では 11%) を大きく下回る配分は避け、概ね 5% 以上を維持する。

## 完了条件

- backend: `KinesisResource` (`internal/aws/kinesis.go`) に `Mode string` (`json:"mode"`) を追加する。
- backend: `kinesisFromSummary` で `s.StreamModeDetails` を読み、nil の場合は `"provisioned"` を設定する。非 nil の場合は `StreamMode` が `ON_DEMAND` なら `"on-demand"`、`PROVISIONED` なら `"provisioned"` を設定し、それ以外の未知の値の場合も `"provisioned"` にフォールバックする。
- backend: `internal/aws/kinesis_test.go` の既存の `TestKinesisFromSummary` に、`StreamModeDetails` が `ON_DEMAND` のケース、`PROVISIONED` のケース、`StreamModeDetails` が nil のケース、`StreamMode` が未知の値のケースを追加する。
- backend: `internal/cli/kinesis.go` の `Columns` に Mode 列を追加し、`KinesisResource.ToRow()` (`internal/aws/torow.go:62`) の戻り値に `r.Mode` を含める。
- backend: `internal/aws/kinesis_test.go` に `KinesisResource.ToRow()` の戻り値に `Mode` の値が含まれることを検証するテストを追加する。既存の `TestKinesisFromSummary` は `kinesisFromSummary` のみを検証しており、`ToRow()` にはテストが一つもないため、`Mode` フィールド追加に伴い新規に用意する。
- frontend: `types/aws.ts` の `KinesisRaw` に `mode: string`、`KinesisRow` に `mode: string` を追加する。
- frontend: `lib/normalize.ts` の `kinesisFromRaw` で `raw.mode` を `row.mode` にそのまま変換する。
- frontend: `lib/normalize.test.ts` に `kinesisFromRaw` の `mode` 変換テストを追加する。
- frontend: `components/tables/columns.tsx` の `kinesisColumns` に `mode` 列を `state` 列の直後に追加し、DynamoDB の `mode` 列と同じ `mutedMono` のテキスト表示にする。既存 6 列 (name/state/shardCount/retentionHours/encryptionType/region) の `width` を再配分し、`mode` 列を含めた合計が 100% になるようにする。
- frontend: `components/tables/columns.test.tsx` に `kinesisColumns` の `width` 合計が 100 になることを検証するテストを追加する (`wafColumns`/`cloudfrontColumns` の同名テスト、24-27 行/119 行付近と同じ `reduce` + `expect(total).toBe(100)` のパターン)。
- `mise run check` が全て通過する。

## 解決方法

backend は `KinesisResource` (`internal/aws/kinesis.go`) に `Mode string` (`json:"mode"`) を `State` の直後に追加した。
`kinesisFromSummary` では `mode` を `"provisioned"` で初期化し、`StreamModeDetails` が非 nil でかつ `StreamMode` が `ON_DEMAND` の場合だけ `"on-demand"` へ上書きする。
`StreamModeDetails` が nil の場合と、`StreamMode` が `PROVISIONED` および将来追加されうる未知の値の場合は、初期値の `"provisioned"` のままになる。
`DescribeStreamSummary` は変更前から呼び出しているため、追加の API 呼び出しと IAM 権限は発生しない。
CLI は `internal/cli/kinesis.go` の `Columns` に `Mode` を `State` と `Shards` の間へ追加し、`KinesisResource.ToRow()` (`internal/aws/torow.go`) にも同じ位置で `r.Mode` を含めた。

frontend は `types/aws.ts` の `KinesisRaw` と `KinesisRow` の双方に `mode: string` を `state` の直後に追加し、`lib/normalize.ts` の `kinesisFromRaw` で `raw.mode` をそのまま写す。
`components/tables/columns.tsx` の `kinesisColumns` には `mode` 列を `state` 列の直後に追加し、DynamoDB の `mode` 列と同じ `mutedMono` の素のテキスト表示にした。
既存 6 列の `width` は 24/12/11/14/15/24 から 20/10/9/12/13/21 へ再配分し、`mode` 列の 15% を加えて合計 100% を保つ。

テストは backend の `TestKinesisFromSummary` に `StreamModeDetails` が nil のケース、`ON_DEMAND` のケース、`PROVISIONED` のケース、未知の値のケースの 4 件を追加し、`ToRow()` を検証する `TestKinesisResourceToRow` を新設した。
frontend は `lib/normalize.test.ts` に `kinesisFromRaw` の全フィールドを `toEqual` で検証するテストを新設し、`components/tables/columns.test.tsx` に `kinesisColumns` の列の並び順、`width` の合計が 100 になること、各列の `width` の対応の 3 件を追加した。

完了条件の `mise run check` の通過は、Step 1 で記録したベースラインからの新たな失敗が無いことと読み替えて検証した。
`backend:lint` の `govulncheck` は `google.golang.org/grpc` の `GO-2026-6061` で失敗するが、これは本 issue に着手する前から存在するベースラインの失敗であり、`docs/issues/0095-bug-grpc-vulnerable-dependency.md` として登録済みである。
検証結果は `frontend:lint` がエラー 0 件、`frontend:test` が 625 件すべて通過、`backend:test` が全パッケージ ok であり、`backend:lint` は `GO-2026-6061` のみで失敗してベースラインと同一である。

`CHANGES.md` の種別は `[ADD]` とした。
グローバル規約は `[ADD]` を「後方互換がある追加」、`[UPDATE]` を「後方互換がある変更」と定めており、本変更は既存の列と JSON フィールドの意味を変えずに列とフィールドを増やすだけなので追加にあたる。
一覧に列を追加した前例は `f376f58` (RDS の Cluster 列) と `e47f9a4` (ElastiCache の Replication group 列) が `[ADD]`、`656249d` (WAF の Description 列) が `[UPDATE]` と割れている。
3 件はいずれも一覧の列と Drawer の Overview 行を同時に追加しており変更の構造が同じであるため、前例からは分類の基準を導けない。
よって規約の定義に従って `[ADD]` とする。

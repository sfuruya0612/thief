# ElastiCache の一覧にノードの AZ を追加する

Created: 2026-07-28
Completed: 2026-07-29
Model: Claude Fable 5

## 背景

`docs/issues/TODO.md` の「Elasticache の各ノードのリージョンと AZ を一覧に表示したい」に対応する。

このうちリージョンについては、`ElastiCacheResource` (`internal/aws/elasticache.go` 13-26 行) 自体にはリージョンを保持するフィールドがない。frontend の `frontend/src/components/tables/columns.tsx` の `cacheColumns` (290-340 行) が表示している `region` 列は、`frontend/src/lib/normalize.ts` の `cacheFromRaw(raw, region)` (238-253 行) の第二引数として渡される値であり、これは ElastiCache 固有の実装ではなく、ユーザーが選択している問い合わせ対象リージョンを全 AWS サービス共通で表示している値である。`ListElastiCacheResources` (`internal/aws/elasticache.go:47`) は指定された 1 リージョンに対して `DescribeCacheClusters` を呼び出す構造であり、通常の (Global Datastore を使わない) クラスターであれば、返るクラスターとそのノードはすべてこの問い合わせリージョン内に存在する。したがって通常構成のノード単位のリージョンは、既存の `region` 列がすでに表している値と一致し、追加のフィールドを設けても表示内容は変わらない。この理由により、本 issue ではリージョンへの対応は不要と判断し、AZ のみを対象とする。Global Datastore (レプリケーショングループが複数リージョンに跨る構成) でのノード単位のリージョン表示は、本 issue の対象外とする。3 階層構成のシャード表示と同様に別のデータモデルの検討を要するため、対応する場合は別 issue とする。

AZ は現状表示されておらず、`ElastiCacheResource` (13-26 行) に AZ に相当するフィールドがない。`ListElastiCacheResources` (47-65 行) は `elasticache.NewDescribeCacheClustersPaginator` で `&elasticache.DescribeCacheClustersInput{}` を空のまま呼んでおり (54-55 行)、レスポンスを `elastiCacheFromCluster` (67-91 行) で変換しているが、`elastiCacheFromCluster` は `c.CacheNodes` を一切読んでいない。`ShowCacheNodeInfo: aws.Bool(true)` の追加はページごとに返るクラスターの件数やページ送り (`HasMorePages`) の挙動には影響しない。このフラグはクラスターごとのレスポンスに `CacheNodes` の内容を含めるかどうかのみを制御し、`DescribeCacheClusters` のページネーション自体 (何件ずつ返すか) はクラスター数に基づいて別途決まるため。`ListElastiCacheClusterInfos` (157-158 行、CLI 側の別ページネーター) も同じ理由で影響を受けない。

`backend/go.mod` は `github.com/aws/aws-sdk-go-v2/service/elasticache v1.51.10` を使用する。この SDK の `DescribeCacheClustersInput` (`api_op_DescribeCacheClusters.go:84-86`) は `ShowCacheNodeInfo *bool` フィールドを持ち、同ファイル 23-26 行のドキュメントコメントには、このフラグを true にしない限りノードレベルの情報が返らない旨が書かれている。現状のコードはこのフラグを設定していないため、たとえ `elastiCacheFromCluster` で `c.CacheNodes` を読んでも、AZ を含むノード詳細はレスポンスに含まれない。したがって本 issue には API 呼び出しパラメータの変更 (`ShowCacheNodeInfo: aws.Bool(true)` の追加) を伴う。新規の API 呼び出しや追加の IAM 権限は不要である。

AZ の格納先は `types.CacheCluster.CacheNodes []CacheNode` (types.go:189) であり、各要素の `types.CacheNode.CustomerAvailabilityZone *string` (types.go:472) にノードごとの AZ が入る。なお `types.CacheCluster.PreferredAvailabilityZone *string` (types.go:253) も AZ 情報を持つが、同フィールドのコメント (251-252 行) に「複数の AZ にまたがる場合は文字列 `"Multiple"` になる」とあり、個々のノードの AZ を判別できないため、本 issue では `PreferredAvailabilityZone` ではなく `CacheNodes[].CustomerAvailabilityZone` を使う。

frontend も同様に AZ を持たない。`frontend/src/types/aws.ts` の `CacheRaw`/`CacheRow` (182-210 行付近) に AZ 相当のフィールドがなく、`frontend/src/lib/normalize.ts` の `cacheFromRaw` (238-253 行) もこれらのフィールドをそのまま変換するだけである。

複数の AZ 値を一覧の 1 列に持たせる先行実装が既にある。ELB では `frontend/src/types/aws.ts` の `ELBRaw.azs: string[] | null` / `ELBRow.azs: string[]` (514, 527 行) が複数 AZ を配列で保持し、`lib/normalize.ts` の `elbFromRaw` (512 行) で `azs: raw.azs ?? []` と変換し、`components/tables/columns.tsx` の `elbColumns` の `azs` 列 (778-783 行付近) で `<span style={mutedMono}>{r.azs.join(', ') || '—'}</span>` として表示している。

配列で持たせる必要があるのは表示形式を揃えるためだけではない。`docs/issues/closed/0072-feat-elasticache-replication-group-view.md` の「設計判断」>「エンジンと構成による違い」(27-33 行) が示す通り、1 つの `CacheCluster` が持つノード数はエンジンによって異なる。Redis / Valkey はクラスターモード無効の場合でも 1 レプリケーショングループが 1 `CacheCluster` (= 1 ノード) に対応するが、Memcached は `NumCacheNodes` に応じて 1 つの `CacheCluster` が複数ノードを持つ。したがって 1 クラスターに対して AZ が複数個返る可能性が構造的にあり、`NodeAvailabilityZones` は単一の文字列ではなく配列で持つ必要がある。

ElastiCache のクラスターモードが有効な Redis / Valkey は、レプリケーショングループが複数のノードグループ (シャード) を持つ 3 階層構成になる。このシャード単位でのグルーピング表示 (シャードごとに AZ を分けて見せる、シャード境界を可視化するなど) は `docs/issues/closed/0072` の未確定論点として既に扱われており、本 issue でも同様に踏み込まない。一方、`DescribeCacheClusters` が返す個々の `CacheCluster` (クラスターモード有効時の各ノードもこの API で 1 行として返る) に対しては、シャード単位のグルーピングとは無関係に通常通り AZ を `NodeAvailabilityZones` に格納する。つまり本 issue のスコープ外は「シャード単位のグルーピング表示」のみであり、クラスターモード有効時の各行に AZ が表示されないという意味ではない。

## 目的

ElastiCache の一覧で、各クラスターに属するノードの AZ を一覧上で確認できるようにする。

## 設計判断

- `ListElastiCacheResources` (54 行) の `DescribeCacheClustersInput` に `ShowCacheNodeInfo: aws.Bool(true)` を追加する。新規の API 呼び出しではなく、既存呼び出しのパラメータ変更で実現する。このパラメータはレスポンスの `CacheNodes` フィールドの内容にのみ影響し、`ElastiCacheResource.NumNodes` (`internal/aws/elasticache.go:20`) には影響しない。`NumNodes` は `c.NumCacheNodes` (同 85 行) という別フィールドから導出されており、`CacheNodes` の件数を数えているわけではないため。
- AZ の集約元は `CacheCluster.CacheNodes[].CustomerAvailabilityZone` とする。`CacheCluster.PreferredAvailabilityZone` は複数 AZ 時に `"Multiple"` という非 AZ 値になり個々のノードの AZ を判別できないため採らない。
- 表示形式は ELB の `azs: string[]` パターンを踏襲し、`ElastiCacheResource` に `NodeAvailabilityZones []string` を追加してノードごとの AZ を配列として保持する。単一の文字列に連結して backend に持たせる案は採らない。配列で持たせておけば frontend 側の表示形式の変更 (件数表示への変更など) に backend の変更を要さないため。
- Memcached のように 1 クラスターが複数ノードを持つ場合、複数ノードが同一 AZ に属することがあるが、`NodeAvailabilityZones` は重複を除去せずノードごとの値をそのまま格納する。一意化すると「同一 AZ に何ノードあるか」という情報が失われるため。
- `CacheNode.CustomerAvailabilityZone` が nil または空文字列のノードは `NodeAvailabilityZones` への集約対象から除外する (空文字列が配列に混入すると、表示上意味のない要素になり、`.join()` 時に余分な区切り文字が残るため)。nil だけでなく空文字列も除外する必要があるのは、AWS SDK for Go v2 が XML の空要素を nil ではなく空文字列のポインタにデシリアライズするためである。`smithy-go` の `NodeDecoder.Value()` (`encoding/xml/xml_decoder.go`) は空要素・自己終了タグに対し nil ではなく `[]byte{}` を返し、`elasticache` の deserializer は `if val == nil { break }` のみを判定するため `[]byte{}` が通過して `ptr.String("")` が設定される。したがって nil チェックのみでは空文字列を弾けない。この除外は「重複を除去しない」方針と矛盾しない。両者は別の問題を扱っており、重複除去は「同じ値を持つ要素をまとめるか」の判断、この除外は「値が存在しない要素を含めるか」の判断である。したがって `NodeAvailabilityZones` の要素数は `CacheNodes` の要素数と必ずしも一致しない (除外した分だけ少なくなる) が、これは意図した挙動である。
- frontend の列表示は ELB の `azs` 列と同じ `mutedMono` のテキストに `.join(', ')` した文字列を表示し、空の場合は `'—'` を表示する。`FacetBar` にファセットとして追加する案は採らない。`docs/issues/closed/0072` で既に検討済みの通り、`FacetBar` の対象は 4 種にハードコードされ、絞り込みの実処理も `ServicePanel` の `filtered` に固定されているため。
- 列の位置は `region` 列の直後に追加する。TODO の文言が「リージョンと AZ」の順であることと、地理的な情報をまとめて配置する意図に沿う。
- `ColumnDef.width` は `cacheColumns` の既存 8 列 (name/state/engine/replicationGroupId/nodeType/region/numNodes/endpoint) の合計で既に 100% になっており (15/9/11/12/12/11/7/23)、新規列を追加するだけでは合計が 100% を超える。`e47f9a4` (同じ `cacheColumns` に Replication Group 列を追加した際、既存列の width を全て縮小し新規列を挿入して合計 100% を維持した前例、この issue と同一テーブルへの列追加である) および `2a3bdd1` (RDS に Engine Version 列を追加した際も同様の再配分を行った前例) に倣い、既存 8 列の width を再配分した上で `nodeAvailabilityZones` 列を追加する。再配分は、追加する `nodeAvailabilityZones` 列の幅を差し引いた残りを既存 8 列の現在の比率で分配する (各列の新 width = 現 width ÷ 現在の合計 100% × (100% − nodeAvailabilityZones 列の幅))。端数調整で合計が 100% からずれる場合は最も幅の大きい列で調整する。既存列の最小値 (`cacheColumns` では 7%) を大きく下回る配分は避け、概ね 5% 以上を維持する。
- CLI (`thief cache`) にも AZ を表示する。ElastiCache は Kinesis と異なり、`internal/cli/elasticache.go` の一覧サブコマンドが Web API と同じ `ElastiCacheResource` ではなく、レガシー CLI 互換の別型 `ElastiCacheClusterInfo` (`internal/aws/elasticache.go:131-164`) を使う。この型にも `NodeAvailabilityZones` を追加しないと、Web UI には AZ が出るが CLI には出ないという食い違いが生じる。`ListElastiCacheClusterInfos` (149 行以降) は `elasticache.NewDescribeCacheClustersPaginator` を独自に呼んでおり、`ListElastiCacheResources` とは別の `DescribeCacheClustersInput` を使っているため、こちらにも `ShowCacheNodeInfo: aws.Bool(true)` を追加する必要がある。
- クラスターモードが有効なレプリケーショングループのシャード単位でのグルーピング表示 (シャード境界の可視化など) は本 issue では扱わない。`docs/issues/closed/0072` の未確定論点と同じスコープ外事項であり、対応する場合は別 issue とする。ただし個々の `CacheCluster` 行の AZ 表示自体はクラスターモードの有効無効に関わらず本 issue の対象内であり、除外されない。

## 完了条件

- backend: `ElastiCacheResource` (`internal/aws/elasticache.go`) に `NodeAvailabilityZones []string` (`json:"node_availability_zones"`) を追加する。
- backend: `ListElastiCacheResources` の `DescribeCacheClustersInput` に `ShowCacheNodeInfo: aws.Bool(true)` を追加する。
- backend: `elastiCacheFromCluster` で `c.CacheNodes` を走査し、各要素の `CustomerAvailabilityZone` (nil でも空文字列でもないもの) を `NodeAvailabilityZones` に集約する。
- backend: `internal/aws/elasticache_test.go` に `TestElastiCacheFromClusterNodeAvailabilityZones` を追加し、複数ノードで AZ が異なるケース、単一ノードのケース、`CustomerAvailabilityZone` が nil のケース、`CustomerAvailabilityZone` が空文字列のケース、全ノードの `CustomerAvailabilityZone` が nil または空文字列のケース、`CacheNodes` が 0 件のケース、複数ノードが同一 AZ を持つケース (重複が除去されずそのまま格納されることを検証する) をテーブル駆動テストで検証する。
- backend: `ElastiCacheClusterInfo` (`internal/aws/elasticache.go:131-164`) に `NodeAvailabilityZones []string` を追加し、`ListElastiCacheClusterInfos` の `DescribeCacheClustersInput` に `ShowCacheNodeInfo: aws.Bool(true)` を追加し、`ToRow()` の戻り値に含める。
- backend: `internal/cli/elasticache.go` の `elasticacheColumns` に AZ 列を追加する。
- backend: `internal/aws/elasticache_test.go` に `ElastiCacheClusterInfo.ToRow()` の戻り値に `NodeAvailabilityZones` の値が含まれることを検証するテストを追加する。
- frontend: `types/aws.ts` の `CacheRaw` に `node_availability_zones: string[] | null`、`CacheRow` に `nodeAvailabilityZones: string[]` を追加する。
- frontend: `lib/normalize.ts` の `cacheFromRaw` で `nodeAvailabilityZones: raw.node_availability_zones ?? []` に変換する。
- frontend: `lib/normalize.test.ts` に `cacheFromRaw` の `nodeAvailabilityZones` 変換テストを追加する。
- frontend: `components/tables/columns.tsx` の `cacheColumns` に `nodeAvailabilityZones` 列を `region` 列の直後に追加し、ELB の `azs` 列と同じ `mutedMono` の `.join(', ') || '—'` 表示にする。既存 8 列 (name/state/engine/replicationGroupId/nodeType/region/numNodes/endpoint) の `width` を再配分し、`nodeAvailabilityZones` 列を含めた合計が 100% になるようにする。
- frontend: `components/tables/columns.test.tsx` に `cacheColumns` の `width` 合計が 100 になることを検証するテストを追加する (`wafColumns`/`cloudfrontColumns` の同名テスト、24-27 行/119 行付近と同じ `reduce` + `expect(total).toBe(100)` のパターン)。
- `mise run check` が全て通過する。

## スコープ外

- クラスターモードが有効なレプリケーショングループのシャード単位のグルーピング表示 (シャード境界の可視化など) は本 issue では扱わない。個々の `CacheCluster` 行への AZ 表示自体はクラスターモードの有効無効を問わず対象内である。
- Global Datastore (レプリケーショングループが複数リージョンに跨る構成) でのノード単位のリージョン表示は本 issue では扱わない。本 issue はノードの AZ のみを対象とし、リージョンは既存の問い合わせ対象リージョン表示 (`region` 列) と通常構成では一致するため追加しない。

## 関連

- `docs/issues/closed/0072-feat-elasticache-replication-group-view.md`: レプリケーショングループの 3 階層構成とシャード単位表示の未確定論点を共有する。

## 解決方法

### backend

- `internal/aws/elasticache.go` の `ElastiCacheResource` に `NodeAvailabilityZones []string` (`json:"node_availability_zones"`) を追加した。`ReplicationGroupID` の後、`CostMonthly` の前に配置している。
- `nodeAvailabilityZonesFromCluster` を新規に追加し、`CacheCluster.CacheNodes` を出現順に走査して各ノードの `CustomerAvailabilityZone` を集約する。同一 AZ の重複は除去しない (同じ AZ に何ノード配置されているかの情報が失われるため)。
- 除外条件は「nil または空文字列」とした。`ptrStr` (`internal/aws/ec2.go:211-216`) で nil を空文字列に正規化した上で `az == ""` を判定する形にしている。nil チェックのみでは不十分な理由は次の通りで、SDK のソースを読んで確認した。`smithy-go@v1.27.3` の `NodeDecoder.Value()` (`encoding/xml/xml_decoder.go`) は XML の空要素・自己終了タグに対し nil ではなく非 nil の空スライス `[]byte{}` を返す。`elasticache@v1.51.10` の `deserializers.go` (13916-13927 行) の `CustomerAvailabilityZone` のパースは `if val == nil { break }` のみを判定するため、`[]byte{}` がこの判定を素通りして `sv.CustomerAvailabilityZone = ptr.String("")` が設定される。空文字列が配列に混入すると、frontend の `.join(', ')` で `"ap-northeast-1a, "` のように末尾に区切り文字が残り、CLI の `strings.Join(..., ",")` で `"ap-northeast-1a,"` のように末尾カンマが残る表示崩れになる。
- `ListElastiCacheResources` と `ListElastiCacheClusterInfos` の `DescribeCacheClustersInput` に `ShowCacheNodeInfo: aws.Bool(true)` を追加した。このフラグを指定しないと `CacheNodes` が返らない。両関数はそれぞれ独立したページネーターを持つため両方に追加が必要である。追加の API 呼び出しは発生しない。
- レガシー CLI 互換型 `ElastiCacheClusterInfo` にも `NodeAvailabilityZones []string` を追加し、`ToRow()` の戻り値に `strings.Join(c.NodeAvailabilityZones, ",")` を末尾要素として含めた。区切り文字はカンマのみとし、backend の既存の `ToRow()` 実装 (`internal/aws/torow.go` の `ELBResource`/`IAMResource` 等) の慣習に合わせている。
- `internal/cli/elasticache.go` の `elasticacheColumns` に `{Header: "NodeAvailabilityZones"}` を末尾に追加した。列数 7 と `ToRow()` の要素数 7 が一致する。
- `internal/aws/elasticache_test.go` に `TestElastiCacheFromClusterNodeAvailabilityZones` (7 ケース) と `TestElastiCacheClusterInfoToRow` (3 ケース) を追加した。

### frontend

- `types/aws.ts` の `CacheRaw` に `node_availability_zones: string[] | null`、`CacheRow` に `nodeAvailabilityZones: string[]` を追加した。backend で Go の nil スライスが JSON の `null` にエンコードされるため Raw 側を nullable にしている。
- `lib/normalize.ts` の `cacheFromRaw` に `nodeAvailabilityZones: raw.node_availability_zones ?? []` を追加した。
- `components/tables/columns.tsx` の `cacheColumns` に `nodeAvailabilityZones` 列を `region` 列の直後に追加した。`header: 'AZs'`、`width: '14%'`、表示は ELB の `azs` 列と同じ `mutedMono` の `.join(', ') || '—'` である。既存 8 列の width を「旧 width ÷ 100 × 86」で再配分し、端数 1% は最大列の `endpoint` で吸収した (13/8/9/10/10/9/14/6/21 = 100)。
- `lib/normalize.test.ts` に `cacheFromRaw` の変換テスト 2 件、`components/tables/columns.test.tsx` に `cacheColumns` のテスト 3 件を追加した。`CacheRaw` リテラルを生成する既存フィクスチャ (`lib/normalize.test.ts` の 2 箇所、`components/Drawer/DrawerCacheParameters.test.tsx` の 1 箇所) にも `node_availability_zones` を追加した。

### 完了条件の最終項目の読み替え

完了条件の「`mise run check` が全て通過する」については、本リポジトリのベースラインに既知の失敗が 1 件あるため、「ベースラインからの新たな失敗が無いこと」と読み替えて判定した。ベースラインの失敗は `mise run backend:lint` の govulncheck が `google.golang.org/grpc` の GO-2026-6061 を報告するもので、`docs/issues/0095-bug-grpc-vulnerable-dependency.md` として別途登録済みである。govulncheck の report 上の到達経路は `internal/gcp/logging_tail.go` と `internal/aws/cloudwatchlogs.go` 経由のみで、本 issue の変更箇所である `internal/aws/elasticache.go` 経由ではない。

読み替えたうえでの実測結果は次の通りである。

- `mise run backend:test`: 全 14 パッケージ ok、FAIL 0 件。新規テストの 10 サブケースを含め全 PASS。
- `mise run frontend:test`: 70 ファイル 630 テスト全 PASS。
- `mise run frontend:lint`: eslint エラー 0 件。警告 10 件はいずれもベースラインと同一で、本変更のファイルとは無関係 (`App.tsx`/`Drawer.tsx`/`AwsIcons.tsx`/`GcpIcons.tsx`/`Icons.tsx`/`cells.tsx`/`AccountView.tsx`)。
- `gofmt -l` / `goimports -l` / prettier: いずれも差分なし。

新たな失敗はゼロである。

### 対応しなかった事項

`internal/aws/torow.go` の `ElastiCacheResource.ToRow()` (18-20 行) には `NodeAvailabilityZones` を追加していない。このメソッドは呼び出し元が存在しない到達不能コードであり、`ElastiCacheResource` の 13 フィールドのうち Name / Port / ParameterGroup / ReplicationGroupID / CostMonthly / NodeAvailabilityZones の 6 つを既に欠いた状態にある。AZ だけを追加しても構造体との整合は回復せず、「更新が維持されているコード」であるかのような誤解を招く分だけ有害と判断した。

直近の `docs/issues/closed/0092` (Kinesis の Mode 列追加) では `torow.go` の `KinesisResource.ToRow()` に `r.Mode` を追加しているが、これは前例にならない。Kinesis の CLI (`internal/cli/kinesis.go`) は `ListConfig[awsinternal.KinesisResource]` を使うため `KinesisResource.ToRow()` は実際に呼ばれる生きたコードであり、更新しなければ CLI の列数 6 と `ToRow()` の要素数 5 が食い違って表示がずれる。一方 ElastiCache の CLI (`internal/cli/elasticache.go`) は `ListConfig[awsinternal.ElastiCacheClusterInfo]` を使うため、`ElastiCacheResource.ToRow()` は呼ばれない。

`torow.go` には `EC2Resource` / `RDSResource` / `ECSResource` / `S3Resource` 等にも同種の未使用実装が存在する。正しい対応は死コードの削除か、CLI が使う型を `ElastiCacheResource` 系に統一することであり、いずれも本 issue のスコープを超えるため `docs/issues/TODO.md` に記録した。

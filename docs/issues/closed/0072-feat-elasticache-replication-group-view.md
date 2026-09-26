# ElastiCache の一覧でノードの所属レプリケーショングループが分かる表示にする

Created: 2026-07-24
Completed: 2026-07-24
Model: Claude Opus 4.8

## 背景

ElastiCache の Redis と Valkey は、レプリケーショングループの配下に複数のノードが紐づく。
現状の Web 一覧はこの所属関係を表現していない。

`internal/aws/elasticache.go` の `ListElastiCacheResources` は `DescribeCacheClusters` だけを呼ぶ。
`elastiCacheFromCluster` は `ReplicationGroupId` をレスポンスにマッピングしておらず、`ElastiCacheResource` に所属レプリケーショングループを指すフィールドがない。
CLI の `ListElastiCacheClusterInfos` は同じ `DescribeCacheClusters` から `ReplicationGroupId` を列として読んでいる。

frontend の DataTable は 1 行 1 リソースのフラット表示で、`CacheRow` にレプリケーショングループ識別子を持たない。

`docs/issues/TODO.md` の「RDS, Elasticache はクラスター、グループにインスタンス(ノード)が紐づく構成だから一覧の見え方を変えて欲しい」のうち ElastiCache 分に対応する。
RDS 分は `docs/issues/0070` で扱う。

## 目的

ElastiCache の Web 一覧で、各ノードがどのレプリケーショングループに属するかを判別できるようにする。

## 設計判断

### エンジンと構成による違い

`DescribeCacheClusters` が返す 1 件の意味はエンジンと構成で異なる。
Redis と Valkey では 1 つの `CacheCluster` が 1 ノードを表し、レプリケーショングループを作った場合は複数ノードがそのグループにまとまる。
Redis と Valkey でもレプリケーショングループを作らない単一ノード構成では `ReplicationGroupId` が空になる。
Memcached では 1 つの `CacheCluster` が複数ノード (`NumCacheNodes`) を束ねたクラスターであり、レプリケーショングループを持たない。
したがって `ReplicationGroupId` が空のノードやクラスター (単一ノードの Redis や Valkey、Memcached) は、レプリケーショングループに属さないものとして扱う。

### backend

`ReplicationGroupId` は既存の `DescribeCacheClusters` のレスポンス (`CacheCluster.ReplicationGroupId`) に含まれるため、追加の API 呼び出しなしで取得できる。
`elastiCacheFromCluster` で `c.ReplicationGroupId` を読み、`ElastiCacheResource` に `replication_group_id` フィールドを追加する。
`DescribeReplicationGroups` の追加呼び出しは行わない。
新たな IAM 権限も要求しない。

### frontend の表示方式

`CacheRow` に `replicationGroupId` を追加し、一覧に所属レプリケーショングループの列を加える。
絞り込みは `DataTable` が既に備える列単位のテキストフィルターを用い、列でソートすると同じレプリケーショングループのノードが隣接する。
全 AWS サービスが共有する `ServicePanel`、`FacetBar`、`DataTable` は改修しない。

`FacetBar` にファセットを足す案は採らない。
`FacetBar` の対象は 4 種にハードコードされ、絞り込みの実処理も `ServicePanel` の `filtered` に固定されているため、ファセットの追加は共有コンポーネントの改修を要する。
またレプリケーショングループ識別子は値の種類が多く、完全一致のファセットより列の部分一致フィルターに向く。

親のレプリケーショングループを見出し行として挿入する階層グルーピングも採らない。
`DataTable` は id の一意性、全選択、`StatsRow` の件数集計を実リソースのフラット配列で行うため、見出し行を混ぜるとこれらが破綻する。

### 未確定論点

- クラスターモードが有効な Redis と Valkey では、レプリケーショングループが複数のノードグループ (シャード) を持つ 3 階層になる。この 3 階層のシャードを列やファセットで表現するかを決める。本 issue ではシャードを扱わない。ノードとシャードの対応や役割 (`CurrentRole`) は `DescribeCacheClusters` からは取得できず、`DescribeReplicationGroups` の `NodeGroups[].NodeGroupMembers[]` の追加取得が必要になるため、無償取得の方針とは切り分けて別途判断する。

## 完了条件

- backend: `elastiCacheFromCluster` で `ReplicationGroupId` を読み、`ElastiCacheResource` に `replication_group_id` を追加する。追加の API 呼び出しは行わない。
- backend: `elastiCacheFromCluster` の変換テーブル駆動テストに、レプリケーショングループに属する Redis または Valkey のノード、`ReplicationGroupId` が空の単一ノードの Redis または Valkey、Memcached のクラスターの各ケースを追加する。
- frontend: `CacheRaw` と `CacheRow` に `replication_group_id` と `replicationGroupId` を追加し、`cacheFromRaw` を更新する。
- frontend: `cacheFromRaw` が `replication_group_id` を `replicationGroupId` に変換することを、値が空のケースを含めて `lib/normalize.test.ts` で検証する。
- frontend: 一覧に所属レプリケーショングループの列を追加し、`DataTable` の既存の列フィルターで絞り込め、列でソートすると同じレプリケーショングループのノードが隣接することを確認する。
- frontend: `replicationGroupId` を持たないノードやクラスター (単一ノードの Redis や Valkey、Memcached) が列、フィルター、ソートで破綻しないことを確認する。
- frontend: `cacheOverviewRows` に所属レプリケーショングループの行を追加するかを確定し、追加する場合は値が空のとき Dash 表示になることを確認する。
- CLI: `elasticache ls` は既に `ReplicationGroupId` を列に持つため、本 issue では変更しない。
- シャード (クラスターモードが有効なときの 3 階層) は本 issue では扱わない。
- `mise run check` が全て通過する。

## 関連

- `docs/issues/0070`: RDS のクラスター所属を一覧に出す issue。列と型と normalize は独立して実装するが、ファセットと階層グルーピングを採らない設計判断は本 issue と共通である。

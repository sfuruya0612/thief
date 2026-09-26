# RDS の一覧でインスタンスの所属 DB クラスターが分かる表示にする

Created: 2026-07-24
Completed: 2026-07-24
Model: Claude Opus 4.8

## 背景

RDS の Aurora や Multi-AZ DB クラスターは、DB クラスターの配下に複数の DB インスタンスが紐づく。
現状の Web 一覧はこの所属関係を表現していない。

`internal/aws/rds.go` の `ListRDSResources` は `DescribeDBInstances` だけを呼び、`RDSResource` にインスタンスが属するクラスターを指すフィールドがない。
そのため、同じクラスターに属するインスタンスを一覧で判別できない。
CLI は `rds cluster` が `DescribeDBClusters` でクラスターを取得するが、`rds ls` のインスタンス一覧とは別コマンドであり、両者の所属関係は表示していない。

frontend の DataTable は 1 行 1 リソースのフラット表示で、`RDSRow` にクラスター識別子を持たない。

`docs/issues/TODO.md` の「RDS, Elasticache はクラスター、グループにインスタンス(ノード)が紐づく構成だから一覧の見え方を変えて欲しい」のうち RDS 分に対応する。
ElastiCache 分は `docs/issues/0072` で扱う。

## 目的

RDS の Web 一覧で、各インスタンスがどの DB クラスターに属するかを判別できるようにする。

## 設計判断

### 親子判定の基準

クラスターへの所属は、エンジン種別 (aurora-mysql や aurora-postgresql など) ではなく `DBInstance.DBClusterIdentifier` の有無で判定する。
Aurora だけでなく mysql や postgres の Multi-AZ DB クラスター (プライマリと読み取り可能スタンバイ 2 台の 3 インスタンス構成) も `DBClusterIdentifier` を持ち、`DescribeDBClusters` に現れる正規の DB クラスターである。
一方、旧来の Multi-AZ DB インスタンス構成 (プライマリとスタンバイ 1 台) はクラスターではなく `DBClusterIdentifier` を持たない。
`DBClusterIdentifier` を持たないインスタンスだけを、クラスターに属さないインスタンスとして扱う。

### backend

`DBClusterIdentifier` は既存の `DescribeDBInstances` のレスポンスに含まれるため、追加の API 呼び出しなしで取得できる。
`rdsFromInstance` で `db.DBClusterIdentifier` を読み、`RDSResource` に `cluster_id` フィールドを追加する。
`DescribeDBClusters` の追加呼び出しは行わない。
新たな IAM 権限も要求しない。

`DescribeDBClusters` を呼んでクラスター自体を属性付きの行として一覧に加える案もある。
しかしこの案は一覧経路に新規 API 呼び出しと `rds:DescribeDBClusters` 権限依存を持ち込む。
既存サービス (ECR Images, ELB Targets, CFN Resources, 0066 の Parameters タブ) は、一覧に主 Describe が無償で返すフィールドだけを載せ、追加取得が要るものは Drawer で二段目取得する方針で統一している。
本 issue はこの方針に従い、クラスター自体の取得は行わない。

### frontend の表示方式

`RDSRow` に `clusterId` を追加し、一覧に所属クラスターの列を加える。
絞り込みは `DataTable` が既に備える列単位のテキストフィルターを用いる。
所属クラスターの列でソートすると、同じクラスターに属するインスタンスが隣接して並ぶ。
全 AWS サービスが共有する `ServicePanel`、`FacetBar`、`DataTable` は改修しない。

`FacetBar` にクラスターのファセットを足す案は採らない。
`FacetBar` の対象は `Env`、`state`、`region`、`Team` の 4 種にハードコードされ、絞り込みの実処理も `ServicePanel` の `filtered` に同じ 4 種で固定されている。
ファセットを 1 種増やすには共有の `FacetBar` と `ServicePanel` の両方を改修する必要があり、共有コンポーネントを改修しない方針と両立しない。
またクラスター識別子は値の種類が多く、ファセットのチップが大量に並んで実用に耐えない。
値の種類が多い識別子は、完全一致のファセットより列の部分一致フィルターに向く。

親のクラスターを見出し行として挿入する階層グルーピングも採らない。
`DataTable` は `T extends { id: string }` を前提に、`key`、チェックボックス選択、選択ハイライト、`StatsRow` の件数集計をすべて実リソースのフラット配列で行う。
見出し行を混ぜると id の一意性、全選択、件数集計が破綻し、2 サービスのために全 15 サービスの一覧が複雑性を負う。

### 未確定論点

- 所属クラスターの列を一覧の初期ソート列にして、同じクラスターのインスタンスを既定で隣接させるかを決める。`DataTable` に初期ソート列を指定する口があるかを実装時に確認し、無い場合の対応の要否も含めて判断する。
- 列とソートだけで TODO の要望を満たせない場合は、Drawer にクラスターのメンバー一覧を表示する案を次段の論点として検討する。
- Writer と Reader の役割を列に出すかを決める。出す場合の情報源は `DescribeDBClusters` の `DBClusterMembers[].IsClusterWriter` であり、インスタンス単体の `DescribeDBInstances` からは判定できない。役割表示を採るなら追加取得が必要になるため、本 issue の無償取得の方針とは切り分けて別途判断する。

## 完了条件

- backend: `rdsFromInstance` で `DBClusterIdentifier` を読み、`RDSResource` に `cluster_id` を追加する。追加の API 呼び出しは行わない。
- backend: `rdsFromInstance` の変換テーブル駆動テストに、クラスターに属するインスタンスと属さないインスタンスの両ケースを追加する。
- frontend: `RDSRaw` と `RDSRow` に `cluster_id` と `clusterId` を追加し、`rdsFromRaw` を更新する。
- frontend: `rdsFromRaw` が `cluster_id` を `clusterId` に変換することを、値が空のケースを含めて `lib/normalize.test.ts` で検証する。
- frontend: 一覧に所属クラスターの列を追加し、`DataTable` の既存の列フィルターで絞り込め、列でソートすると同じクラスターのインスタンスが隣接することを確認する。
- frontend: `clusterId` を持たないインスタンスが列、フィルター、ソートで破綻しないことを確認する。
- frontend: `rdsOverviewRows` に所属クラスターの行を追加するかを確定し、追加する場合は値が空のとき Dash 表示になることを確認する。
- CLI: 本 issue は Web 一覧を対象とし、CLI の `rds ls` は変更しない。
- `mise run check` が全て通過する。

## 関連

- `docs/issues/0071`: RDS のクラスターパラメータグループ参照を追加する issue。本 issue が追加する `cluster_id` を Drawer で利用する。本 issue を先に実装する。
- `docs/issues/0072`: ElastiCache のレプリケーショングループ所属を一覧に出す issue。列と型と normalize は独立して実装するが、ファセットと階層グルーピングを採らない設計判断は本 issue と共通である。
- `docs/issues/closed/0066`: RDS インスタンスの DB パラメータグループ参照を追加した issue。一覧に無償フィールドのみ載せる方針の先行例。

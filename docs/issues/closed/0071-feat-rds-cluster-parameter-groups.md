# RDS のクラスターパラメータグループの中身も参照できるようにする

Created: 2026-07-24
Completed: 2026-07-24
Model: Claude Opus 4.8

## 背景

`docs/issues/closed/0066` で RDS のパラメータグループの中身を参照できるようにした。
このとき対応したのは、DB インスタンスに紐づく DB パラメータグループ (`DescribeDBParameters`) だけである。

DB クラスター (Aurora や Multi-AZ DB クラスター) は、クラスター全体の設定を DB クラスターパラメータグループ (`DescribeDBClusterParameters`) に持つ。
例えば Aurora MySQL の `binlog_format` はインスタンスのパラメータグループには現れず、クラスターパラメータグループでのみ設定する。
現状ではこのクラスター単位の設定を参照する手段がない。

`internal/aws/rds.go` の Web 一覧 (`ListRDSResources`) はクラスターを取得しておらず、frontend の Parameters タブもインスタンスのパラメータグループだけを表示する。
CLI の `ListRDSClusterInfos` は `DescribeDBClusters` を呼ぶが、クラスターパラメータグループ名やその中身は扱っていない。

`docs/issues/TODO.md` の「RDS のパラメータグループは Cluster, Instance どちらも欲しい」に対応する。

## 目的

DB クラスターに属するインスタンスについて、所属クラスターの DB クラスターパラメータグループのパラメータを、既存の DB パラメータグループと区別して Web UI と CLI から参照できるようにする。

## 設計判断

### 0070 への依存

frontend の統合は、`docs/issues/0070` が `RDSResource` に追加する `cluster_id` (`DBClusterIdentifier`) を前提とする。
Drawer が一覧クエリ (`useResources`) のキャッシュからインスタンスの `clusterId` を引くためである。
一方、backend の `ListRDSClusterParameters`、API の `?cluster=` エンドポイント、CLI の `rds cluster-parameters` は `cluster_id` を明示的な入力として受け取るため、`docs/issues/0070` に依存せず単独で実装とテストができる。
実装順序は番号順とし、`docs/issues/0070` を先に実装する。

### backend

クラスターパラメータグループのパラメータは、Parameters タブを開いた時点で Drawer が二段目として取得する。
一覧経路 (`ListRDSResources`) には手を入れず、クラスターパラメータグループ名を一覧レスポンスに載せない。
これは ECR Images や ELB Targets と同じ遅延取得の方針であり、DB クラスターを使わないアカウントで無駄な API 呼び出しを避ける。

取得は次の経路で行う。
インスタンスの `cluster_id` から `DescribeDBClusters` でそのクラスターの `DBClusterParameterGroup` 名を得て、`DescribeDBClusterParameters` のページネータで全パラメータを取得する。
この 2 つの呼び出しを行う `ListRDSClusterParameters` を追加する。

API 経路は `GET .../rds/cluster-parameters?cluster=<cluster_id>` を新設する。
パスで分離することで、キャッシュキー、CLI コマンド、frontend の `queryKey` がインスタンス用と独立する。
既存の `GET .../rds/parameters?group=...` に `scope` を足す案もあるが、その場合は 1 つのハンドラが 2 種類の取得を分岐し、`handleRDSParameters` の `cacheKey` にも `scope` を加える改修が要る。
新エンドポイントの方がハンドラとキャッシュの責務が分かれる。

新エンドポイントは既存の `serveCached` と `writeAWSError` を経由し、SSO トークン期限切れ (401 と `SSO_TOKEN_EXPIRED`) の扱いを 0066 と揃える。
Web 経路が新たに要求する権限は `rds:DescribeDBClusterParameters` である。
`rds:DescribeDBClusters` は既存の CLI (`rds cluster`) が既に使用している。
これらが権限不足やスロットリングで失敗した場合は、Parameters タブのクラスター区分にエラーを表示し、一覧とインスタンス用パラメータの表示には影響させない。

パラメータの変換は既存の `rdsParameterFromSDK` (backend) と `rdsParameterFromRaw` (frontend) を流用する。
`DescribeDBClusterParameters` の戻り値がインスタンス用と同じ型のため、新しい変換関数は追加しない。

### frontend

Parameters タブで、インスタンスの DB パラメータグループ (複数) とクラスターの DB クラスターパラメータグループ (単一) を区別して表示する。
現在の `DrawerRDSParameters` はパラメータグループ名の配列をセグメントに並べ、選択された名前を一律にインスタンス用のエンドポイントへ渡している。
クラスターのパラメータは別のエンドポイントと別のフックで取得するため、セグメントを種別付き (`{ kind: 'instance' | 'cluster', name }`) に拡張し、種別で `useRDSParameters` と `useRDSClusterParameters` を出し分ける。
`cluster_id` を持たないインスタンスではクラスターの区分を表示せず、`enabled: !!clusterId` でクラスターパラメータの取得を発火させない。

### CLI

`rds cluster-parameters --cluster <cluster_id>` を新設する。
`ListRDSClusterParameterInfos` を追加し、インスタンス用の `rds parameters` と対をなすコマンドにする。

## 完了条件

- backend: `DescribeDBClusters` でクラスターパラメータグループ名を引き、`DescribeDBClusterParameters` の全ページを取得する `ListRDSClusterParameters` を追加する。パラメータの変換は既存の `rdsParameterFromSDK` を流用する。
- backend: `ListRDSClusterParameters` が `DescribeDBClusters` で得たグループ名を `DescribeDBClusterParameters` へ渡す経路を、モックを用いたテストで検証する。
- backend: `GET .../rds/cluster-parameters?cluster=...` を追加し、`serveCached` と `writeAWSError` を経由させる。
- backend: `DescribeDBClusters` と `DescribeDBClusterParameters` の失敗時に、一覧とインスタンス用パラメータの表示が影響を受けないことを確認する。
- CLI: `rds cluster-parameters --cluster <cluster_id>` を追加する。
- frontend: Parameters タブのセグメントを種別付きにし、クラスターとインスタンスのパラメータグループを区別して参照できるようにする。
- frontend: `cluster_id` を持たないインスタンスでクラスター区分が表示されず、クラスターパラメータの取得を発火させないことを確認する。
- frontend: クラスター区分のパラメータ取得が失敗しても、インスタンス区分のパラメータ表示と一覧が影響を受けないことを確認する。
- `mise run check` が全て通過する。

## 関連

- `docs/issues/0070`: 本 issue の frontend 統合が前提とする `cluster_id` を追加する基盤 issue。先に実装する。
- `docs/issues/closed/0066`: インスタンスの DB パラメータグループ参照を追加した先行 issue。Parameters タブと遅延取得の方針、変換関数を踏襲する。

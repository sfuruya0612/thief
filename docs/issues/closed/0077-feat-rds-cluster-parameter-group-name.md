# RDS の Cluster Parameters タブにクラスターパラメータグループ名を表示する

Created: 2026-07-25
Completed: 2026-07-26
Model: Claude Fable 5

## 背景

docs/issues/0076 (RDS の Parameters タブ分割) の設計中に判明した派生 issue であり、TODO.md 由来ではない。
タブ分割そのものは docs/issues/0076 が行う。
本 issue はタブの見出しをクラスター識別子からパラメータグループ名に具体化する。

RDS の Drawer のクラスターパラメータ表示は、見出しに DB クラスター識別子 (`clusterId`) を出すだけで、実際に紐づくクラスターパラメータグループ名を出す手段がない。
backend の `listRDSClusterParameters` (`backend/internal/aws/rds.go`) は `DescribeDBClusters` でグループ名を解決しているが、解決した名前を捨ててパラメータの配列 (`[]RDSParameter`) だけを返している。
グループ名はエラーメッセージにしか現れない。

`GET .../rds/cluster-parameters` エンドポイントの利用箇所は frontend の `getRDSClusterParameters` の 1 つ。
Go 関数 `ListRDSClusterParameters` の利用箇所は API ハンドラと CLI 用の `ListRDSClusterParameterInfos` の 2 つで、形状変更の影響はこの範囲に閉じる。

## 目的

`Cluster Parameters` タブの見出しで、クラスターパラメータグループ名が確認できる。

## 設計判断

- AWS 層の `ListRDSClusterParameters` の戻り値を、新設の構造体 `RDSClusterParameterGroup` (フィールド: `GroupName string` (JSON タグ `group_name`)、`Parameters []RDSParameter` (JSON タグ `parameters`)) に変える。`listRDSClusterParameters` が既に解決しているグループ名を捨てずに載せるだけで、追加の AWS API 呼び出しは不要。エンドポイントのレスポンスはオブジェクト形状 `{"group_name": "...", "parameters": [...]}` になる。
  - 後方互換のない変更として `CHANGES.md` は `[CHANGE]` で記載する。frontend と backend は同一リポジトリからローカルで起動する構成のため、旧形状と新形状の両対応の分岐は入れず、更新後に `thief server` を再起動することを前提にする。backend の in-memory キャッシュは同一プロセス内の Go 構造体を持つだけなので、キャッシュ由来の形状非互換は起こらない。
  - 却下案 1: 一覧レスポンス (`RDSResource`) に `cluster_parameter_group` フィールドを追加する。一覧経路 (`DescribeDBInstances` ベース) に `DescribeDBClusters` の追加呼び出しが要り、詳細情報は選択時に遅延取得する既存方針に反するため却下。
  - 却下案 2: グループ名をキャッシュキーにする (同一グループを共有する複数クラスターでキャッシュを共有できる)。一覧行にグループ名が無く frontend がキーを組み立てられないため却下。キーは現行の `cluster` ベースのままとし、グループ名を差し替えた直後は最大 1 時間 (キャッシュ TTL) 旧グループ名が表示されるが、パラメータ本体が同じ条件で stale になる現状と同等のため許容する。後続の docs/issues/0079 が close されれば Refresh で即時に取り直せるようになる。
- 既存の `TestListRDSClusterParameters` (`backend/internal/aws/rds_test.go`、モック `mockRDSClusterParameterClient` あり) は、成功系の 2 サブテストが戻り値型の変更の影響を受ける (エラー系はエラーのみを検証しており影響しない)。`group_name` が空になる経路は実運用では到達しない (グループ名が解決できない場合、空のグループ名での `DescribeDBClusterParameters` 呼び出しが AWS 側のエラーになり先に返る) ため、モックでのみ検証する。
- CLI の `ListRDSClusterParameterInfos` は新しい戻り値型に追随させる。CLI の出力はパラメータ表 (`rdsParameterColumns`) のみでグループ名を含まないため、表示内容は変わらない。
- frontend は `types/aws.ts` / `api/endpoints.ts` / `lib/normalize.ts` / `api/queries.ts` を新形状に追随させる。レスポンスが配列でなくなるため `apiGetList` の null 正規化が使えず、`parameters ?? []` の正規化を変換層で行う。`DrawerRDSClusterParameters` (docs/issues/0076 で新設) の見出しを `clusterId` からグループ名に変える。`group_name` が空のときは `clusterId` を表示する (防御的フォールバック)。
- 追加の権限は不要。

## 完了条件

- `GET .../rds/cluster-parameters` が `group_name` と `parameters` を持つオブジェクトを返す。`TestListRDSClusterParameters` の全サブテストが新形状に更新され、`group_name` の設定あり / 空 (防御的分岐) と `parameters` が nil のケースを検証している。
- `Cluster Parameters` タブの見出しにクラスターパラメータグループ名が表示される。グループ名が空の場合は `clusterId` が表示される。`parameters` が null で来ても 0 件として表示される。
- `DrawerRDSClusterParameters.test.tsx` に見出し (グループ名 / フォールバック) の検証が追加されている。
- 新形状のレスポンスを変換する `lib/normalize.ts` の関数 `rdsClusterParameterGroupFromRaw` のユニットテストが既存の `lib/normalize.test.ts` にあり、`parameters` が null / 欠落のとき空配列になることを検証している。
- CLI の表示内容が変わらない: `RDSParameterInfo` の `ToRow()` (`backend/internal/aws/rds.go`) のユニットテストを既存の `backend/internal/aws/rds_test.go` に追加して列の並びを固定し、`rdsParameterColumns` (`backend/internal/cli/rds.go`) に変更が無いことを確認して本 issue に記録する。
- 一覧レスポンス (`RDSResource`) へのフィールド追加は本 issue では扱わない。
- `CHANGES.md` の `## develop` に `[CHANGE]` エントリと担当者行が記載されている。
- `mise run check` が通過する。

## 解決方法

設計判断の通りに実装した。

- backend (`backend/internal/aws/rds.go`): 構造体 `RDSClusterParameterGroup` (`group_name` / `parameters`) を新設し、`ListRDSClusterParameters` / `listRDSClusterParameters` の戻り値を `[]RDSParameter` から `RDSClusterParameterGroup` に変更した。既に解決していたグループ名を載せるだけで追加の AWS API 呼び出しは無い。パラメータが 1 件も無い場合 `Parameters` は nil のまま (JSON では null) とし、正規化は frontend の変換層に置いた。`ListRDSClusterParameterInfos` は `group.Parameters` を走査する形に追随させた。API ハンドラ (`handlers_aws.go`) は `any` を返すため変更不要。
- backend テスト (`backend/internal/aws/rds_test.go`): `TestListRDSClusterParameters` の成功系 2 サブテストを新形状 (`GroupName` 付き) に更新し、`group_name` が空になる防御的分岐 (グループ名 nil + パラメータ 0 件 = `Parameters` nil) のサブテストを追加した。`TestRDSParameterInfoToRow` を追加して CLI の列順 (Name, Value, ApplyType, DataType, IsModifiable, Source) を固定した。
- CLI 表示の不変確認: `rdsParameterColumns` (`backend/internal/cli/rds.go`) は本 issue で一切変更していない (git diff 空を確認)。CLI の出力列は従来通り。
- frontend: `types/aws.ts` に `RDSClusterParameterGroupRaw` / `RDSClusterParameterGroupRow` を追加。`getRDSClusterParameters` を `apiGetList` から `apiGet<RDSClusterParameterGroupRaw>` に変更。`lib/normalize.ts` に `rdsClusterParameterGroupFromRaw` を追加し `parameters ?? []` で null を空配列に正規化。`useRDSClusterParameters` の queryFn を同関数経由に変更。`DrawerRDSClusterParameters` の見出しを `query.data?.groupName || clusterId` (空グループ名とローディング中は clusterId フォールバック)、行を `query.data?.parameters ?? []` に変更した。
- frontend テスト: `normalize.test.ts` に `rdsClusterParameterGroupFromRaw` の変換と null → 空配列のテストを追加。`DrawerRDSClusterParameters.test.tsx` のモックをオブジェクト形状に更新し、見出しがグループ名になるテストと `group_name` 空時の clusterId フォールバックのテストを追加した。
- `CHANGES.md` の `## develop` に `[CHANGE]` エントリを追加した。
- `mise run check` (fmt + lint + backend/frontend テスト 571 件) 通過を確認した。

## 関連

- docs/issues/0076: 本 issue の分離元。タブ分割が先 (番号順)。TODO L53 に紐づくのは docs/issues/0076 のみで、本 issue は派生として起票する。
- docs/issues/0079: 後続の Refresh 修正。close されればキャッシュ TTL 内の stale なグループ名を Refresh で即時に取り直せるようになる。
- docs/issues/closed/0071: `listRDSClusterParameters` の実装元。グループ名の解決ロジックはここで入った。

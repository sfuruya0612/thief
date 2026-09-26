# RDS の Drawer の Parameters タブを Instance と Cluster の 2 タブに分ける

Created: 2026-07-25
Completed: 2026-07-26
Model: Claude Fable 5

## 背景

TODO.md の次の項目に対応する。

> RDS の Parameter Group は現在1つのタブになってしまっているところを Cluster,  Instance の Parameter Group のタブを分ける

タブ見出しへのクラスターパラメータグループ名の表示は、本 issue の設計中に判明した派生として docs/issues/0077 で扱う。

RDS の Drawer のタブは `frontend/src/components/Drawer/Drawer.tsx` の `DRAWER_TABS` で `rds: ['Overview', 'Parameters', 'Tags']` と定義されており、パラメータグループは `Parameters` タブ 1 つに集約されている。
タブ本体の `DrawerRDSParameters` (`frontend/src/components/Drawer/DrawerRDSParameters.tsx`) は、インスタンスの DB パラメータグループ (`kind: 'instance'`) とクラスターパラメータグループ (`kind: 'cluster'`) を判別可能ユニオン `ParameterSegment` で表現し、同一のセグメントボタン列 (`.seg`) に並べて切り替えている (docs/issues/closed/0071 の設計判断)。

この構成には次の問題がある。

- セグメントラベルは cluster 側だけ `Cluster:` プレフィックスが付き、instance 側はグループ名のみで、種別の表現が非対称になっている。instance 側の単一グループのインスタンスでは、見出しにグループ名は出るものの、それが Instance 側のパラメータであることを示す語がどこにもない。
- 一覧キャッシュから該当行が引けていない間も `No parameter groups.` と表示され、未取得と 0 件が区別できない。
- docs/issues/closed/0071 は、クラスター区分の取得失敗はエラーを表示してインスタンス区分に影響させない、という設計を置いたが、現行の `DrawerRDSParameters` は query の `error` を使っておらず、取得失敗時は空のテーブルが出るだけになっている。

backend のエンドポイントは `GET .../rds/parameters?group=` (Instance) と `GET .../rds/cluster-parameters?cluster=` (Cluster) で既に分離済み (`backend/internal/api/routes.go`) であり、タブ分割に backend の変更は不要。

## 目的

RDS の Drawer で、インスタンスのパラメータグループとクラスターのパラメータグループが別々のタブで参照でき、どちらの種別を見ているかが常に判別できる。

## 設計判断

- `DRAWER_TABS['rds']` を `['Overview', 'Instance Parameters', 'Cluster Parameters', 'Tags']` に変更する。4 タブ構成は cfn (`['Overview', 'Events', 'Resources', 'Tags']`) の前例があり、タブバーの幅は問題にならない。タブ順は、クラスターに属さないインスタンスでも常に中身がある Instance 側を先にする (TODO の文言は「Cluster, Instance」の順だが、並びの指定ではなく対象の列挙と解釈する)。
  - 却下案 1: 現行の 1 タブ + セグメント方式の改善 (instance 側にも種別プレフィックスを付ける)。TODO の要望が「タブを分ける」であるため却下。
  - 却下案 2: タブ名を `Parameters (Instance)` / `Parameters (Cluster)` とする。より長くタブバーを圧迫するため短い名前を採る。
- ElastiCache (`DRAWER_TABS['cache']`) は対象外とする。ElastiCache のパラメータグループは 1 種別しかなく、分けるものがないため現状の `Parameters` タブのままにする。
- タブ名は英語ハードコードとし i18n に載せない。docs/issues/closed/0066 で確立した方針 (ECR Images / ELB Targets タブと同様) に従う。
- `DrawerRDSParameters` は Instance 専用 (`DrawerRDSInstanceParameters`) と Cluster 専用 (`DrawerRDSClusterParameters`) の 2 コンポーネントに分割する。
  - Instance 側: `parameterGroups` が複数のときのみ `.seg` セグメントで切り替える (現行踏襲)。0 件のときは既存文言 `No parameter groups.` の空表示を出す。見出しは選択中のグループ名。
  - Cluster 側: `clusterId` が空 (クラスターに属さないインスタンス) のときは `Not part of a DB cluster.` の空表示を出す。見出しは `clusterId` をそのまま表示し、現行の `Cluster:` プレフィックスは付けない (タブ名で種別が分かるため)。タブ自体は常に表示する (`DRAWER_TABS` はサービス単位の静的定義で、リソース単位の分岐機構がないため)。
  - 3 状態を区別する: 一覧キャッシュから該当行が引けていない間 (`row === undefined`) はローディング表示、行はあるが対象が無い場合は上記の空表示、query がエラーの場合はエラー表示。判定材料は `row` の有無、`clusterId` / `parameterGroups` の値、query の `error` で足りる (`useRDSClusterParameters` は `enabled` 制御があり空の `clusterId` では発火しない)。
  - エラー表示は docs/issues/0075 で導入する `DrawerError` 部品と、docs/issues/0075 で確定するエラー表示規則 (`data` が無いときはエラー表示のみ、あるときは既存表示の上部にエラー) を使う (docs/issues/closed/0071 が設計として記載したまま未実装のエラー表示を、分割時に実装する)。一方のタブのエラーがもう一方のタブの表示に影響しないことは、エンドポイントと query が分かれているため構造的に満たされる。
  - 却下案: `clusterId` の有無でタブを動的に出し分ける。リソース単位のタブ分岐機構を新設することになり、本 issue の範囲を超えるため却下。
- Cluster タブの見出しは `clusterId` の表示にとどめる。クラスターパラメータグループ名の表示は backend のレスポンス形状の変更が必要であり、docs/issues/0077 で扱う。
- 追加の AWS API 呼び出し、権限は不要。既存の 2 エンドポイントをタブ別に呼ぶだけで済む。

## 完了条件

- RDS の Drawer に `Instance Parameters` タブと `Cluster Parameters` タブが表示され、`Parameters` タブが存在しない。ElastiCache の Drawer は現状の `Parameters` タブのまま変わらない。タブ構成は `Drawer.test.tsx` で検証する。
- `Instance Parameters` タブで、インスタンスの DB パラメータグループのパラメータが表示される。グループが複数あるときはセグメントで切り替えられる。グループが 0 件のときは `No parameter groups.` が表示される。
- `Cluster Parameters` タブで、クラスターに属するインスタンスはクラスターパラメータグループのパラメータが表示され (見出しは `Cluster:` プレフィックスなしの `clusterId`)、属さないインスタンスは `Not part of a DB cluster.` が表示される。この見出しは docs/issues/0077 でグループ名に置き換わる。
- 両タブとも、一覧キャッシュに行が無い間はローディング表示、取得エラー時は空表示と区別できるエラーメッセージ (docs/issues/0075 の `DrawerError` 部品) が表示される。両タブの `DrawerError` の使用は grep (`<DrawerError`) で確認できる。
- テストは分割後のコンポーネントと 1:1 の `DrawerRDSInstanceParameters.test.tsx` / `DrawerRDSClusterParameters.test.tsx` に置き、`DrawerRDSParameters.test.tsx` は削除する。表示 / 空表示 / ローディング / エラー表示の各ケースを検証する。
- `Drawer.test.tsx` に、Cluster 側の取得をエラーにした状態で `Instance Parameters` タブにパラメータが表示される統合ケースがある (エラー分離の検証)。
- クラスターパラメータグループ名の表示は本 issue では扱わない (docs/issues/0077 が引受先)。
- `CHANGES.md` の `## develop` に `[UPDATE]` エントリと担当者行が記載されている。
- `mise run check` が通過する。

## 関連

- docs/issues/0075 (WAF Rules タブ): 本 issue と同じく `Drawer.tsx` の `DRAWER_TABS` と `dbody` を触る。番号順 (docs/issues/0075 → 本 issue) に実装し、エラー表示は docs/issues/0075 が導入する `DrawerError` 部品を使う (grep で判定できる)。
- docs/issues/0077: Cluster タブにクラスターパラメータグループ名を表示する後続 issue。本 issue の設計中に判明した派生 (TODO.md 由来ではない)。
- docs/issues/0082 (サブタブのエラー表示の横断修正): 本 issue は RDS の 2 タブのみを扱い、他サブタブは docs/issues/0082 が扱う。
- docs/issues/closed/0066: Parameters タブの初回実装。タブ名を i18n に載せない方針の出典。
- docs/issues/closed/0071: クラスターパラメータグループ対応。1 タブ内セグメント方式とエラー分離設計の出典。本 issue はセグメント方式をタブ分割に変更し、設計に記載されたまま未実装だったエラー表示を実装する。
- docs/issues/closed/0070: `RDSResource.cluster_id` の追加。Cluster タブの表示可否判定に使う。

## 解決方法

- `Drawer.tsx` の `DRAWER_TABS['rds']` を `['Overview', 'Instance Parameters', 'Cluster Parameters', 'Tags']` に変更し、dbody の `Parameters` 分岐を 2 タブの分岐に置き換えた。`cache` の `Parameters` タブは変更していない。
- `DrawerRDSParameters.tsx` を削除し、`DrawerRDSInstanceParameters.tsx` と `DrawerRDSClusterParameters.tsx` に分割した。
  - Instance 側: グループが複数のときのみ `.seg` セグメントで切り替える (現行踏襲)。0 件は `No parameter groups.`。見出しは選択中のグループ名 (種別プレフィックスなし)。
  - Cluster 側: `clusterId` が空のときは `Not part of a DB cluster.`。見出しは `Cluster:` プレフィックスなしの `clusterId`。
  - 両タブとも 3 状態を区別する: 一覧キャッシュに該当行が無い間 (`row === undefined`) は `DrawerLoading`、行はあるが対象が無い場合は空表示、query のエラーは `DrawerError` (docs/issues/0075 の部品と表示規則: `data` 無しはエラーのみ、`data` 有りは既存表示の上部)。docs/issues/closed/0071 で設計のまま未実装だったエラー表示をこの分割で実装した。
- テストは分割後のコンポーネントと 1:1 の `DrawerRDSInstanceParameters.test.tsx` (表示 / セグメント切り替え / 空表示 / ローディング / エラーの 5 ケース) と `DrawerRDSClusterParameters.test.tsx` (表示 / 空表示 / ローディング / エラーの 4 ケース) に置き、`DrawerRDSParameters.test.tsx` は削除した。
- `Drawer.test.tsx` にタブ構成の検証 (rds は 4 タブで `Parameters` 無し、cache は現状のまま) と、Cluster 側の取得を 403 にした状態で `Instance Parameters` タブにパラメータが表示される統合ケース (エラー分離の検証) を追加した。
- CHANGES.md の `## develop` に `[UPDATE]` エントリを記載した。
- `mise run check` 通過 (eslint の警告 9 件はすべて既存で、新規コードでの増加なし)。

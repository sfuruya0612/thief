# ElastiCache の Drawer の Overview にノードの AZ 行を追加する

Created: 2026-08-05
Model: Claude Fable 5
Completed: 2026-08-07

## 背景

TODO.md の「Elasticache の Drawer (overviewRows.tsx の cacheOverviewRows) にノードの AZ 行を追加したい (issue 0093 のスコープ外として送り)」に対応する。

docs/issues/closed/0093 で ElastiCache の一覧にノードごとの AZ を表示する `AZs` 列を追加した。
一覧側は `frontend/src/types/aws.ts` の `CacheRow.nodeAvailabilityZones` (string 配列、normalize 時に null を吸収済み) を表示する。
一方 Drawer 側の `frontend/src/components/Drawer/overviewRows.tsx` の `cacheOverviewRows` (78-90 行) は Resource ID, Engine, Engine version, Replication group, Node type, Nodes, Endpoint, Port, Region の 9 行で、AZ の行が無い。
同じファイルの `elbOverviewRows` (186 行付近) は `r.azs.join(', ')` で AZ を表示しており、AZ を持つリソースの Drawer 表示として不揃いである。

## 目的

ElastiCache クラスターの Drawer の Overview でノードの AZ を確認できるようにし、一覧と Drawer の表示項目の差、および ELB の Drawer との不揃いを解消する。

## 設計判断

- `cacheOverviewRows` に AZ の行を 1 行追加する。値は `r.nodeAvailabilityZones.join(', ')` とし、空配列のときは他の行と同じ dash 表示に落とす。`elbOverviewRows` の azs 行をテンプレートにする。
- ラベルは一覧の列ヘッダおよび ELB の Drawer と同じ `AZs` にする。`Availability zones` と綴る案は、同一画面内の他の表示と食い違うため採らない。
- ラベルは英語ハードコードにする。Drawer 系の文言を i18n に載せない方針 (docs/issues/closed/0066) に従う。
- 行の位置は Nodes の直後にする。ノード数とノードの配置という関連する情報が並ぶ。
- `nodeAvailabilityZones` は docs/issues/closed/0093 で normalize 済みの既存フィールドのため、追加の API 呼び出しや権限は不要。
- 変更は Drawer の Overview に限る。一覧側の列と CLI の AZ 表示は docs/issues/closed/0093 で対応済みのため変更しない。

## 完了条件

- `cacheOverviewRows` の返す行に `AZs` が含まれ、値が `nodeAvailabilityZones` のカンマ区切り連結である。
- `frontend/src/components/Drawer/overviewRows.test.tsx` に `cacheOverviewRows` のテストを追加し、行のラベルの順序、複数 AZ のカンマ区切り、空配列のときの dash 表示を検証する。
- 一覧側の列と CLI の AZ 表示には変更を加えない。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0093: 一覧への AZs 列の追加。本 issue はその Drawer 側の積み残し。
- docs/issues/0097: Kinesis の Drawer への Capacity mode 行の追加。同種の「一覧と Drawer の表示項目差」の解消だが、対象サービスと対象フィールドが独立して実装と検証ができるため別 issue とする。

## 解決方法

- `cacheOverviewRows` の Nodes 行の直後に `AZs` 行を追加した。値は `nodeAvailabilityZones.join(', ')` で、空配列のときは他の行と同じダッシュ表示に落とす (`elbOverviewRows` の azs 行と同じ形)。
- `overviewRows.test.tsx` に `cacheOverviewRows` のテストを追加し、行ラベルの順序 (AZs が Nodes の直後にあること)、複数 AZ のカンマ区切り連結、空配列のときのダッシュ表示を検証した。
- `mise run check` の通過を確認した。

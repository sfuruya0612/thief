# Kinesis の Drawer の Overview に Mode 行を追加する

Created: 2026-08-05
Model: Claude Fable 5
Completed: 2026-08-07

## 背景

TODO.md の「Kinesis の Drawer (overviewRows.tsx の kinesisOverviewRows) に Mode 行を追加したい (issue 0092 のスコープ外として送り)」に対応する。

docs/issues/closed/0092 で Kinesis の一覧にキャパシティモードの列を追加した。
一覧側は `frontend/src/components/tables/columns.tsx` の `kinesisColumns` に `mode` 列 (ヘッダ `Mode`) があり、`frontend/src/types/aws.ts` の `KinesisRow.mode` を表示する。
一方 Drawer 側の `frontend/src/components/Drawer/overviewRows.tsx` の `kinesisOverviewRows` は Resource ID, Shards, Retention, Encryption, Region の 5 行のみで、Mode 行が無い。
一覧にはある情報が Drawer の Overview で見られず、表示項目に差がある。

## 目的

Kinesis ストリームの Drawer の Overview でキャパシティモードを確認できるようにし、一覧と Drawer の表示項目の差を無くす。

## 設計判断

- `kinesisOverviewRows` に `['Capacity mode', r.mode]` の 1 行を追加する。値は `KinesisRow.mode` をそのまま使う。normalize 済みの既存フィールドのため、追加の API 呼び出しや権限は不要。`mode` が空文字列の場合もそのまま表示し、dash への変換はしない。既存行 (Shards や Encryption) がゼロ値をそのまま表示する作りに合わせる。
- ラベルは DynamoDB の `dynamoOverviewRows` が同じ意味の値に使っている `Capacity mode` にする。一覧のヘッダと同じ `Mode` にする案は、DynamoDB の Drawer とラベルが食い違うため採らない。
- ラベルは英語ハードコードにする。overviewRows.tsx の既存行は全て英語ハードコードで、Drawer 系の文言を i18n に載せない方針 (docs/issues/closed/0066) に従う。
- 行の位置は Resource ID の直後にする。`dynamoOverviewRows` が Resource ID の直後に Capacity mode を置いているのに揃える。
- 変更は Drawer の Overview に限る。一覧側の列 (`kinesisColumns`) と CLI の表示は docs/issues/closed/0092 で対応済みのため変更しない。DynamoDB 側の Drawer 表示も変更しない。

## 完了条件

- `kinesisOverviewRows` の返す行で、Resource ID の直後に `Capacity mode` の行があり、値が `KinesisRow.mode` である。
- `frontend/src/components/Drawer/overviewRows.test.tsx` に `kinesisOverviewRows` のテストを追加し、行のラベルの順序と Capacity mode の値を検証する。
- 一覧側の列 (`kinesisColumns`)、CLI の表示、DynamoDB 側の Drawer には変更を加えない。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0092: 一覧への Mode 列の追加。本 issue はその Drawer 側の積み残し。
- docs/issues/0103: ElastiCache の Drawer への AZ 行の追加。同種の「一覧と Drawer の表示項目差」の解消だが、対象サービスと対象フィールドが独立して実装と検証ができるため別 issue とする。

## 解決方法

- `frontend/src/components/Drawer/overviewRows.tsx` の `kinesisOverviewRows` で、`['Resource ID', r.id]` の直後に `['Capacity mode', r.mode]` の行を追加した。値は `KinesisRow.mode` をそのまま表示し、空文字列でもダッシュへ変換しない。
- `frontend/src/components/Drawer/overviewRows.test.tsx` に `kinesisOverviewRows` のテストを 3 件追加した。Resource ID の直後に Capacity mode 行があり既存行 (Shards 以降) の順序が変わらないこと、値が `KinesisRow.mode` をそのまま表示すること、空文字列でもダッシュへ変換しないことを検証する。レビューの指摘 (観点 2) を受け、行順序のテスト名を既存行の順序が変わらないことを検証する意図が伝わる名前にした。
- 一覧側の列 (`kinesisColumns`)、CLI の表示、DynamoDB 側の Drawer には変更を加えていない。
- `mise run check` の通過を確認した (ベースラインからの新たな失敗なし)。

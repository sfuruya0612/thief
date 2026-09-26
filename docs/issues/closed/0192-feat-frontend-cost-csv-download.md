# Web のコスト画面に CSV ダウンロードを追加する

Created: 2026-09-18
Model: deepseek-v4p1-flash
Completed: 2026-09-18

## 背景

Web のコスト表示 (Cost Explorer / Datadog / TiDB) は集計表を画面に描画するが、ファイルとして保存する導線が無い。`frontend/src/views/CostExplorerPanel.tsx` と `frontend/src/components/MonthlyCostPanel.tsx` のどちらにもダウンロード / エクスポートのボタンが無い。

既存の「Copy CSV」はクリップボードへ文字列を入れるだけで (`frontend/src/lib/queryFormat.ts:67` の `toCsv` + `navigator.clipboard`)、`createObjectURL` / `Blob` / `download` 属性を使ったファイル保存はリポジトリ内に存在しない。CLI の `cost` 系コマンドは `-o csv` で CSV を出力できており、Web だけが CSV をファイルとして保存できない。

## 目的

コスト画面の集計表を CSV ファイルとしてダウンロードできるようにし、CLI の CSV 出力と同等の利便性を Web に持たせる。

## 設計判断

- 集計済みのクロス表データ (`CostCrossTable` の `categories` と `CostCrossTableRow[]`) をそのまま CSV 化する。再取得はせず、画面に表示している値と一致させる。
- `toCsv` (`frontend/src/lib/queryFormat.ts`) を再利用し、文字列から Blob を作って `<a download>` を一時的に生成しクリックする共通ヘルパー `downloadCsv` を `frontend/src/lib/download.ts` に追加する。
  - 却下案：サーバ側に CSV 生成エンドポイントを足す案。集計ロジックがフロント側 (`lib/costAggregateCore.ts`) にあり、同じ集計を二重実装することになるため却下。
  - 却下案：`showSaveFilePicker` を使う案。対応ブラウザが限られ、既存の `<a download>` 方式 (S3 ダウンロード) とも不揃いになるため却下。
- 対象は `CostExplorerPanel` (AWS) と `MonthlyCostPanel` (Datadog / TiDB)。Pricing は単価見積りでありコスト集計表ではないため本 issue の対象外とする。
- ファイル名は `<view>-cost-<start>-<end>.csv` 形式とし、期間が分かるようにする。
- 追加の API 呼び出し・権限・依存パッケージは不要。

## 完了条件

- Cost Explorer のツールバーに Download CSV ボタンがあり、押すと Group / Total / 各期間列の CSV がダウンロードされる。
- Datadog / TiDB の Cost タブの `MonthlyCostPanel` に Download CSV ボタンがあり、同様に CSV がダウンロードされる。
- `downloadCsv` の単体テストがある (ヘッダ行と行が `toCsv` の規則で組み立てられること)。
- `mise run frontend:lint` と `mise run frontend:test` が通過する。
- `mise run check` が通過する。

## 解決方法

- `frontend/src/lib/download.ts` を追加した。`crossTableCsv` がクロス集計表を `Group / Total / 各期間` の列に変換し (金額は小数 2 桁固定)、`downloadCsv` が `toCsv` の文字列を Blob にして `<a download>` で保存させる。
- `frontend/src/views/CostExplorerPanel.tsx` の facets に Download CSV ボタンを追加した。ファイル名は `cost-<start>-<end>.csv`。
- `frontend/src/components/MonthlyCostPanel.tsx` に `csvBaseName` プロパティ (既定 `cost`) を追加し、Download CSV ボタンを追加した。`DatadogView` は `datadog`、`TiDBView` は `tidb` を渡す。ファイル名は `<base>-<startMonth>-<endMonth>.csv`。
- `frontend/src/lib/download.test.ts` に `crossTableCsv` の単体テストを追加した。
- `mise run check` (fmt / lint / test) が通過することを確認した。

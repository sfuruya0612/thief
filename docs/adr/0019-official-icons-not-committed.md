# 0019. AWS と Google Cloud の公式アイコンをリポジトリに含めない

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-07-08

## 状況

サイドバーには AWS と Google Cloud の公式アイコンを表示する。
公式アイコンの無改変のファイルをリポジトリに含めることは、ライセンス上できない (`AGENTS.md`、`README.md`)。

## 決定

- `frontend/public/assets/aws-icons/` と `frontend/public/assets/gcp-icons/` は `.gitignore` の対象にする。
- 利用者が公式の配布物 (zip) から、`mise run frontend:fetch-aws-icons <zip>` と `mise run frontend:fetch-gcp-icons <zip>` で展開する。
- サービスとファイル名の対応は `frontend/scripts/fetch-aws-icons.mjs` と `frontend/scripts/fetch-gcp-icons.mjs` の `ICON_FILENAMES` に置く。
- アイコンが無いときは代わりの表示を出す。

## 検討した代替案

記録なし。

## 結果

- アイコンを表示するには、利用者が公式の配布物を入手する必要がある。
  展開しなくても起動と表示はでき、アイコンが代わりの表示になる (`README.md`)。
- 公式の配布物のファイル名が変わると、`ICON_FILENAMES` の更新が要る。

## 根拠資料

- コミット 2aac22f (2026-07-08、AWS アイコンの取得手順の追加)
- コミット 2ac257e (2026-07-13、Google Cloud 対応。Google Cloud のアイコンの取得手順と `.gitignore` の追加)
- `README.md`、`AGENTS.md` の frontend 「コンポーネント設計」

# 0028. issue を docs/issues/ に移し、git で管理する

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-09-26

## 状況

2026-07-12 から、issue は git の管理の外にある (ADR 0027)。
2026-09-26 に、要求 (PRD) と設計判断 (ADR) をリポジトリに置くことにした (ADR 0001)。
PRD と ADR は issue を根拠資料として参照するが、issue が git の管理の外にあると、参照先がリポジトリに無い。
issue、PRD、ADR の置き場所を揃える必要があった。

## 決定

- issue を `issues/` から `docs/issues/` に移す。
- `docs/` 以下 (`docs/prd/`、`docs/adr/`、`docs/issues/`) をすべて git で管理する。
- issue には、組織を特定できる記述 (AWS のプロファイル名、Google Cloud のプロジェクト ID、社内のホスト名など) を書かない。
  実環境で確認した結果を書くときは、`example-common` のような仮の名前に置き換える。
- ソースコードのコメントと `CHANGES.md` からの issue への参照は、`issues/` を `docs/issues/` に置き換えた形で書く。
  `closed/` の有無は元の参照のままとし、`docs/issues/0075` の形の参照も残る。
- グローバルの規約とスキル (`todo-to-issue`、`implement-issues`、`write-prd`) が `issues/` と書く箇所は、このリポジトリでは `docs/issues/` と読み替える (`AGENTS.md` の「ドキュメントの配置」)。

## 検討した代替案

- `docs/issues/` だけを git の管理の外に残す。
  PRD と ADR の根拠資料の参照先がリポジトリに無くなる。
- issue を `issues/` に置いたまま git で管理する。
  要求、判断、issue の置き場所が分かれ、`docs/` を見ても文書が揃わない。

## 結果

- 2026-07-12 から 2026-09-26 までに書いた issue は、移動のコミットの時点の内容で履歴に入る。
  その間の書き換えの経過は残らない。
- issue を新規作成したときと 1 issue を完了したときのコミットに、issue のファイルが入る (グローバル規約の issue の運用がそのまま当てはまる)。
- 移動の前に、issue に書かれていた組織を特定できる記述を仮の名前に置き換えた。
  git の履歴に残る過去のコミットの内容は書き換えていない。
- コミットメッセージの末尾の issue の参照は `(docs/issues/closed/NNNN)` になる。
  移動をコミットするまでのコミットは `(issues/closed/NNNN)` のままである。

## 根拠資料

- ADR 0001、ADR 0027
- `AGENTS.md` の「ドキュメントの配置」

# issue の形式の規約をリポジトリの実務に揃える

Created: 2026-11-13
Model: DeepSeek V4.1 Flash 1017
Completed: 2026-11-13
Branch: feature/chore-align-issue-format-rule

## 背景

(docs/issues/closed/0242) の解決方法に、issue の節構成がグローバルの規約と合っていないため、テンプレートの整合をリポジトリ全体の判断として別に扱う記録がある。

グローバルの `~/.pi/agent/AGENTS.md` の「文書」は、issue に目的、現状、設計方針、完了条件、解決方法を書くことと、作成日、完了日、優先度、作成したモデル、ブランチ名をタイトルの後に置くことを定めている。
`issue-style` スキルも同じ節構成を Open の形式として定め、カテゴリを `bug`、`feature`、`other` に限っている。
`reviewer` の定義も `Requirements` を常時の観点として固定している。

このリポジトリの `docs/issues/closed/` の 241 件は別の形式で書かれている。
`Requirements` と `Priority` を持つ issue は 0 件、`Created` と `Model` を持つ issue は 241 件、`Completed` を持つ issue は 239 件である。
節は `背景` が 183 件 (`## 背景` が 169 件、`## 背景 / 根拠` が 14 件)、`完了条件` が 193 件、`解決方法` が 236 件で、`目的`、`関連`、`設計判断`、`再現手順`、`原因`、`修正方針` を種類に応じて足している。
カテゴリは `feat` 109 件、`bug` 53 件、`chore` 45 件、`refactor` 22 件、`perf` 11 件、`docs` 1 件で、スキルが定める値と一致しない。

PRD の要求 ID は `FR-N` (32 種) で、グローバルの「仕様駆動」が定める `REQ-<領域>-<3桁>` は 0 件である。PRD の優先度は `Must`、`Should`、`Could` で、`issue-style` の `High`、`Medium`、`Low` と尺度が違う。
PRD の要求を本文で引く issue は 17 件あるが、`Requirements` を持つものは 0 件である。

規約と実務が食い違ったままでは、レビューが毎回同じ指摘を出し、issue を書く人がどちらに従うかを毎回判断することになる。既存の issue を規約の形式へ書き換えると、241 件の履歴を書き換えることになり、得られる情報は増えない。

## 対応方針

- `AGENTS.md` の「ドキュメントの配置」に、このリポジトリの issue の形式を節として書く。グローバルの `AGENTS.md` と、`issue-style`、`prd-style`、`review-integration`、`delegation` スキル、`reviewer` の定義が定める形式より、この節を優先する。
- メタデータは、タイトルの後に `Created`、`Model` を置き、完了時に `Completed` を足す。作業ブランチを書く場合は `Branch` も置く。
- `Requirements` と `Priority` の欄は設けない。PRD の要求 ID は `FR-N` とし、グローバルの `REQ-<領域>-<3桁>` に代えて使う。優先度は PRD が `Must`、`Should`、`Could` で持つ。PRD の要求を扱う issue は本文で `FR-N` を引用し、完了条件をその要求の受け入れ基準に対応させる。
- 節は `背景`、`対応方針`、`完了条件` を置き、完了時に `解決方法` を足す。残りの節は issue の種類で決め、バグは `再現手順`、`原因`、`修正方針`、機能は `目的`、`設計判断`、`関連` を足す。
- pending の issue は止める理由と再開条件を、Draft の issue は確認したい点と未確定の判断を書く。
- `docs/issues/SEQUENCE` は次に使う番号を持つ。
- 参照は `(docs/issues/NNNN)` または `(docs/issues/closed/NNNN)` と書き、本文中では `issue NNNN` と書いてよい。コミットメッセージは、作業中の issue が `(docs/issues/NNNN)`、完了した issue が `(docs/issues/closed/NNNN)` とする。
- `AGENTS.md` が指すグローバルのパス (`~/.codex/AGENTS.md`) とスキル名 (`todo-to-issue`、`implement-issues`、`write-prd`) を、現行の `~/.pi/agent/AGENTS.md` と `issue-style`、`prd-style`、`review-integration` に直す。
- グローバルの `issue-style` に、プロジェクトが別の形式を定めるときはそちらを優先する一文と、Open 判定基準とレビューチェックリストをプロジェクトの形式へ置き換える注記を足す。
- グローバルの `reviewer` の定義で、`Requirements` を常時の観点に固定する 2 項目を、プロジェクトの形式に従う観点へ書き換える。
- グローバルの `review-integration` と `delegation` は書き換えず、`AGENTS.md` の優先宣言の対象に加えて解決する。
- 既存の `docs/issues/closed/` の 241 件は書き換えない。形式の統一による便益が、履歴の書き換えの費用に見合わないため。

## 完了条件

- `AGENTS.md` に、issue の形式 (メタデータ、節、カテゴリ、PRD の要求の引き方、`SEQUENCE` の意味、参照、pending と Draft の書き方) が書かれている。
- `AGENTS.md` の記述どおりに chore の issue を書くと、(docs/issues/closed/0241) と (docs/issues/closed/0242) と同じメタデータと節になる。
- `AGENTS.md` が指すグローバルのパスとスキル名が、現在の `~/.pi/agent/` の構成と一致する。
- グローバルの `issue-style` に、プロジェクトが別の形式を定めるときの優先順位と、Open 判定基準とレビューチェックリストの置き換えが書かれている。
- グローバルの `reviewer` の定義に、プロジェクトの形式に従う観点が書かれている。
- 既存の issue ファイルを書き換えていない。
- `mise run check` が通る。

## 解決方法

- `AGENTS.md` の「ドキュメントの配置」に「issue の形式」節を追加した。ファイル名、メタデータ (`Created`、`Model`、`Completed`、`Branch`)、`Requirements` と `Priority` を設けないこと、`FR-N` の引用と受け入れ基準の対応、節構成、pending と Draft、`docs/issues/SEQUENCE`、参照方法、完了行の参照を書いた。
- PRD の要求 ID は `FR-N`、優先度は `Must`、`Should`、`Could` を PRD が持つことを明記し、グローバルの `REQ-<領域>-<3桁>` に代えて使うことを書いた。PRD の 32 個の要求を `REQ-...` へ移行する案は採らない。PRD と 17 件の issue の参照を書き換える割に、得られる情報が増えないため。
- 節は `背景`、`対応方針`、`完了条件` を置き、完了時に `解決方法` を足す形にした。chore 45 件、refactor 22 件、perf 11 件、docs 1 件のうち `対応方針` を持つのは 50 件であり、種類を問わず置く節にした。
- `AGENTS.md` が指していたグローバルのパス `~/.codex/AGENTS.md` を `~/.pi/agent/AGENTS.md` に、スキル名 `todo-to-issue`、`implement-issues`、`write-prd` を現行の `issue-style`、`prd-style`、`review-integration` に直した (3 箇所のパスとテスト節の見出し名)。
- グローバルの `issue-style` に、プロジェクトの `AGENTS.md` またはプロジェクトのスキルが形式を定めるときはそちらを優先する一文と、Open 判定基準とレビューチェックリストをプロジェクトの形式に置き換える注記 (`Branch` の要否を含む) を足した。リポジトリの外にあるため、コミットには含まれない。
- グローバルの `reviewer` の定義で `Requirements` を固定していた 2 項目を、プロジェクトの形式に従い、PRD の要求を扱うときに要求 ID と受入基準を対応づける観点に書き換えた。同じくリポジトリの外にある。
- `review-integration` と `delegation` は書き換えず、`AGENTS.md` の優先宣言の対象に加えた。どちらも全プロジェクトで使うスキルであり、リポジトリ固有の形式を書き込むと他のプロジェクトの前提を変えるため。
- 既存の `docs/issues/closed/` の 241 件は書き換えていない。`Completed` を持たない 2 件 (0045、0232) は既存の欠落として残した。
- `mise run check` が通った。
- 扱わなかった範囲: グローバルの「仕様駆動」が定める「テストは守る要求 ID をテスト名か module doc に書く」と、このリポジトリの実装の乖離 (`FR-` を含むコードは 1 件)。要求 ID とテストの対応は PRD とテストの設計に関わるため、別の判断とする。`docs/issues/TODO.md` に積んだ。

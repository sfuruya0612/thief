# issue の形式の規約をリポジトリの実務に揃える

Created: 2026-11-13
Model: DeepSeek V4.1 Flash 1017
Branch: feature/chore-align-issue-format-rule

## 背景

レビューで、issue の形式がグローバルの規約と合っていないという指摘が 2 回続いた (docs/issues/closed/0241、docs/issues/closed/0242)。
グローバルの `~/.pi/agent/AGENTS.md` の「文書」は、issue に目的、現状、設計方針、完了条件、解決方法を書くことと、作成日、完了日、優先度、作成したモデル、ブランチ名をタイトルの後に置くことを定めている。
`issue-style` スキルも同じ節構成を Open の形式として定め、カテゴリを `bug`、`feature`、`other` に限っている。

このリポジトリの `docs/issues/closed/` の 241 件は別の形式で書かれている。
`Requirements` と `Priority` を持つ issue は 0 件、`Created` と `Model` を持つ issue は 241 件、`Completed` を持つ issue は 239 件である。
節は `背景`、`対応方針`、`完了条件`、`解決方法` が基本で、`目的`、`設計判断`、`関連` を足す issue もある。
カテゴリは `feat` 109 件、`bug` 53 件、`chore` 45 件、`refactor` 22 件、`perf` 11 件、`docs` 1 件で、スキルが定める値と一致しない。

規約と実務が食い違ったままでは、レビューが毎回同じ指摘を出し、issue を書く人がどちらに従うかを毎回判断することになる。既存の issue を規約の形式へ書き換えると、241 件の履歴を書き換えることになり、得られる情報は増えない。

## 対応方針

- `AGENTS.md` の「ドキュメントの配置」に、このリポジトリの issue の形式を節として書く。グローバルの `AGENTS.md` と `issue-style` スキルが定める形式より、この節を優先する。
- メタデータは、タイトルの後に `Created`、`Model` を置き、完了時に `Completed` を足す。作業ブランチを書く場合は `Branch` も置く。
- 節は `背景`、`対応方針`、`完了条件` を基本とし、完了時に `解決方法` を足す。issue の性質に応じて `目的`、`設計判断`、`関連` を足してよい。
- カテゴリは `bug`、`chore`、`docs`、`feat`、`perf`、`refactor` から選ぶ。
- 要件 ID と優先度は、PRD の要求に紐づく issue でだけ書く。
- `docs/issues/SEQUENCE` は次に使う番号を持つ。
- 完了した issue の参照は `(docs/issues/closed/NNNN)` と書き、open の間は `(docs/issues/NNNN)` と書く。
- 既存の `docs/issues/closed/` の 241 件は書き換えない。形式の統一による便益が、履歴の書き換えの費用に見合わないため。
- グローバルの `issue-style` スキルに、プロジェクトが別の形式を定めるときはそちらを優先する一文を足す。スキルはリポジトリの外にあり、このリポジトリのコミットには含まれない。

## 完了条件

- `AGENTS.md` に、issue の形式 (メタデータ、節、カテゴリ、要件 ID と優先度の扱い、`SEQUENCE` の意味、完了した issue の参照) が書かれている。
- `AGENTS.md` の記述が、docs/issues/closed/0241 と docs/issues/closed/0242 の実物と一致する。
- グローバルの `issue-style` に、プロジェクトが別の形式を定めるときの優先順位が書かれている。
- 既存の issue ファイルを書き換えていない。
- `mise run check` が通る。

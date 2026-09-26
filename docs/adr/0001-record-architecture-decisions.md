# 0001. 設計判断を ADR として後追いで記録する

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-09-26

## 状況

thief の設計判断の根拠は、issue の本文 (`docs/issues/closed/`)、コミットメッセージ、`AGENTS.md` に散っている。
issue は 2026-07-12 から 2026-09-26 まで git 管理の対象外であり (ADR 0027)、その間に issue に書いた判断はリポジトリの履歴に残っていない。
issue は作業の記録と判断が混ざっており、判断だけを取り出して読めない。
判断を変えたくなったとき、何を理由にその判断をしたのかを 1 か所で確かめる手段が無い。

## 決定

設計判断を 1 判断 1 ファイルの ADR として `docs/adr/NNNN-<kebab-case>.md` に記録し、git で管理する。

- 形式は Michael Nygard の形式 (状況、決定、結果) に、検討した代替案と根拠資料の節を加えたものとする。
- 番号は 4 桁の連番とし、欠番を作らない。
- ヘッダに `Created` (ADR を書いた日)、`Model` (書いた LLM)、`Status`、`Decided` (判断した日) を置く。
- 2026-09-26 より前の判断は、コード、issue、コミットから逆算して書く。
  `Decided` には、判断を確認できる最も古い資料の日付を書く。
- 判断を覆すときは、元の ADR を書き換えず、新しい ADR を書く。
  元の ADR の `Status` を `Superseded by NNNN` にする。

## 検討した代替案

- `AGENTS.md` に判断を追記する。
  `AGENTS.md` は規約 (何をするか) の置き場であり、判断の経緯と却下した代替案を書くと規約が読みにくくなる。
- issue に判断を書き続ける。
  issue は作業の記録と判断が混ざり、却下した代替案を判断ごとに引けない。
  判断した当時は、issue は git 管理の対象外でもあった (ADR 0027)。

## 結果

- 後追いの ADR は、代替案と根拠が記録に無い判断を含む。
  その場合は「記録なし」と書き、推測で埋めない。
- 製品の要求は `docs/prd/thief.md` に置き、ADR には判断だけを書く。

## 根拠資料

- `AGENTS.md` の「ドキュメントの配置」
- Michael Nygard, "Documenting Architecture Decisions" (2011): https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions

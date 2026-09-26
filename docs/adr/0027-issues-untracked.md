# 0027. issue を git で管理しない

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Superseded by 0028
Decided: 2026-07-12

## 状況

thief は issue を `{seqnum}-{category}-{short-description}.md` の Markdown ファイルで管理する。
2026-07-12 まで、issue は `issues/` に置いて git で管理していた。

## 決定

- `issues/` を git の管理から外す (コミット b4ca037)。
  理由はコミット b4ca037 のメッセージにあり、運用中の issue 管理ファイルをリポジトリの履歴に混ぜないためである。
- `issues/` は `.gitignore` で除外する。

## 検討した代替案

代替案を比べた記録は無い。

## 結果

- 2026-07-12 以降に書いた issue の本文は、リポジトリの履歴に残らない。
  b4ca037 の時点で追跡していた当時の issue 3 件 (`issues/closed/0001`〜`0003`。ターミナルの resize observer、S3 のアップロードのキャッシュと Content-Length) は、履歴に残る。
  現在の `docs/issues/closed/0001`〜`0003` は、これとは別の issue である。
- 2026-09-26 に ADR 0028 で覆した。

## 根拠資料

- コミット b4ca037 (2026-07-12)

# npm audit が報告する minimatch の脆弱性を overrides で解消する

Created: 2026-11-13
Model: DeepSeek V4.1 Flash 1017
Completed: 2026-11-13
Branch: feature/chore-fix-minimatch-audit

## 背景

`frontend` の `npm audit` は、開発用の依存に由来する moderate 10 件を報告していた。
内訳はすべて `minimatch` の GHSA-g3hv-75wg-m249 (glob パターンの展開でメモリと CPU を消費する DoS) で、影響範囲は `7.0.0` から `11.0.0` である。

依存元は 3 つで、いずれも `minimatch` の 10 系を要求する。

- `eslint` 10.14.0 が直接依存する `minimatch` 10.2.6
- `@eslint/config-array` 0.23.5 が依存する `minimatch` 10.2.6
- `@typescript-eslint/typescript-estree` 8.72.2 が依存する `minimatch` 10.2.6

修正版は `11.0.1` 以降であり、3 つの依存元はいずれも `^10` の範囲を要求するため、通常の `npm update` では修正版に到達しない。
`npm audit fix` は `eslint` 9.39.5 へのダウングレードを提示するが、採用すると Lint の設定と検証の前提が古い版に戻る。

docs/issues/closed/0241 は、この 10 件を扱わない範囲として記録した。

## 対応方針

- `frontend/package.json` に `overrides` を置き、`minimatch` を解決済みの `^11.0.8` に固定する。npm の `overrides` は依存ツリー全体の該当パッケージの版を置き換えるため、3 つの依存元の要求範囲より優先される。
- `minimatch` 11 は Node.js 20 系または 22 以上を要求する。`mise.toml` の Node.js は 26.13.1 であり、CI も同じ固定を使うため条件を満たす。
- `mise run frontend:lint`、`mise run frontend:test`、`mise run frontend:build` を実行し、`minimatch` 11 で ESLint の glob 解決 (`ignores` と `files`) が動くことを確かめる。Lint の警告数は変更前から増やさない。`typescript-estree` が `minimatch` を使うのは `allowDefaultProject` の経路だけで、この設定では実行されない。
- `eslint` と `typescript-eslint` が `minimatch` 11 系を要求するようになった時点で、この `overrides` は不要になる。外す作業を `docs/issues/TODO.md` に積む。
- `eslint` を 9 系へダウングレードする案は採らない。修正のために検証基盤の版を下げると、Lint の規則と `typescript-eslint` の対応範囲が古い前提に戻るため。
- 脆弱性を許容して `npm audit` の失敗を無視する案は採らない。修正版が存在し、利用側の検証で互換を確認できるため。

## 完了条件

- `frontend/package.json` に `overrides` の `minimatch` があり、値が `^11.0.8` である。
- `frontend/package-lock.json` が `minimatch` 11.0.8 を解決し、`npm ls minimatch` が 11.0.8 だけを報告する。
- `npm audit` が既知の脆弱性 0 件を報告する。
- `frontend` で `npm ci --dry-run` が失敗せず、lock から引き直す経路でも依存が解決する。
- `mise run frontend:lint` のエラーが 0 件で、警告数が変更前から増えない。
- `mise run frontend:test` が全テストに成功する。
- `mise run frontend:build` が成功する。
- `CHANGES.md` の `## develop` の `### misc` に、`minimatch` の固定と `npm audit` の解消を説明するエントリと担当者行 `  - @sfuruya0612` がある。
- `docs/issues/TODO.md` に、`eslint` と `typescript-eslint` が `minimatch` 11 系へ上がった後に `overrides` を外し、lock を更新して `npm ls minimatch` と `npm audit` を再確認する作業が `[ ]` で残っている。

## 解決方法

- `frontend/package.json` に `"overrides": { "minimatch": "^11.0.8" }` を追加し、`npm install` で `frontend/package-lock.json` を更新した。`npm ls minimatch` は eslint、@eslint/config-array、@typescript-eslint/typescript-estree の 3 経路すべてで 11.0.8 を報告する。
- `npm audit` は 0 件になり、`frontend` で `npm ci --dry-run` も失敗しない。
- `mise run check` が通り、frontend:lint は 0 errors / 9 warnings (変更前と同じ)、frontend のテストは 121 ファイル 1355 件すべて成功し、frontend:build も成功した。
- `CHANGES.md` の `## develop` の `### misc` にエントリを追加し、`docs/issues/TODO.md` に `overrides` を外す条件を `[ ]` で積んだ。
- 判断した点:
  - CHANGES の misc のエントリには種別タグを付けない。このファイルの misc は既存の 20 件以上がタグ無しで並んでおり、1 件だけ付けると同じ節の中で表記が揃わない。種別順の整理は `docs/issues/TODO.md` の既存の項目で扱う。
  - issue 本文の節構成 (背景、対応方針、完了条件) はリポジトリの既存 issue に揃え、issue-style の `Requirements`、`Priority`、`目的`、`現状`、`設計方針`、`解決方法` の見出しは使わない。テンプレートの整合はリポジトリ全体の判断になるため、本 issue では扱わない。
- 扱わなかった範囲: CI に `npm audit` を組み込むこと。現在の CI は `mise run check` だけを実行する。依存を追加したときに失敗させる変更になるため、別の判断とする。

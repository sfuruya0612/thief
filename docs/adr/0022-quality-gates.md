# 0022. 品質ゲートを mise run check と pre-commit フックに置く

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Superseded by 0031
Decided: 2026-07-08

## 状況

thief の品質は自動テストと静的解析で担保する。
検査の道具を増やすと、検査の結果と規約の食い違い、ツールの導入の手間が増える。
過去に次の問題があった。

- `AGENTS.md` は golangci-lint を検査に含めると書いていたが、`mise.toml` のタスクは実行していなかった (issue 0139)。
- `mise.toml` が Go を 1.25.12 に固定したままだったため、govulncheck が標準ライブラリ由来の脆弱性を報告した (issue 0144)。
  修正で `backend/go.mod` に `toolchain` 行を新設した。
- frontend のテストで jsdom をファイルごとに作り直し、テストに 71.98 秒かかっていた (issue 0182)。
- mockery のタスクが設定ファイルの不在で必ず失敗していた (issue 0199)。

## 決定

- `mise run check` が fmt、lint、test を順に実行する。
  pre-commit フック (`.pre-commit-config.yaml`、`prek` 経由) が同じ検査をコミットの前に実行する (コミット 2aac22f、2026-07-08)。
  フック自体はコミット 6b4f175 (2026-02-20) で `mise test` の実行として入った。
- backend の Lint は `go vet`、`staticcheck`、`govulncheck` とし、golangci-lint を使わない (issue 0139 の案 B)。
- backend のテストは標準の `testing` で書き、モックは規約どおり `go.uber.org/mock` か手書きとする。
  現状は手書きのモックだけであり、mockery のタスクとツールを削除した (issue 0199)。
- frontend のテストは vitest の `pool: 'vmThreads'` で実行する (issue 0182)。
- `mise.toml` の Go と `backend/go.mod` の `toolchain` 行は同じバージョンに同時に上げる (issue 0144)。

## 検討した代替案

- issue 0139: golangci-lint を足して検出を直す (案 A)。
  errcheck の検出それぞれに無視の理由が要るため採らなかった。
  issue の本文の件数 (37 件、うち errcheck 36 件) は表示の上限で切り捨てられた値で、上限を外すと errcheck は 57 件相当だった。
  golangci-lint を足して errcheck を無効にする (案 C)。
  採らなかった。
  利用者に確認して案 B に決めた。
- issue 0182: `isolate: false`。
  18.33 秒と速いが、10〜11 ファイルが実行順に依存して失敗するため採らなかった。
  `vmThreads` は 9.58 秒で全テストが通った。
- issue 0199: mockery の設定ファイルを足す。
  規約はモックの生成に `go.uber.org/mock` か手書きのモックを指定しており、mockery を含まないため採らなかった。

## 結果

- CI は無い (`.github/workflows/` にワークフローは無い)。
  CI を置くかどうかを決めた記録は無い。
- 品質ゲートは各開発者の端末の pre-commit フックに依存する。
  フックを入れていない端末からのコミットは検査されない。
- CI の扱いは `docs/prd/thief.md` の「未確定論点」に残した。

## 根拠資料

- コミット 6b4f175 (2026-02-20)、2aac22f (2026-07-08)
- `docs/issues/closed/0139`、`0144`、`0182`、`0199`
- `mise.toml`、`.pre-commit-config.yaml`、`frontend/vite.config.ts`
- `AGENTS.md` の「pre-commit」と backend 「ビルドと CI」

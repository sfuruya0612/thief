# Node.js を 26.13.1 に、Go を 1.27.4 に更新し、backend と frontend の依存パッケージを更新する

Created: 2026-11-12
Model: DeepSeek V4.1 Flash 1017
Completed: 2026-11-12
Branch: feature/chore-update-nodejs-go-and-dependencies

## 背景

2026-11-11 に Node.js の security release が公開され、26.x、24.x、22.x の各系統に修正版が出た。
26 系統の最新版は v26.13.1 (2026-11-10 公開、security release) である。
本リポジトリは `mise.toml` の `[tools].node` で `26.4.0` を固定しており、`node:http2`、`node:https`、TLS の脆弱性 (CVE-2026-123863、CVE-2026-123864、CVE-2026-123873 など) の修正を受け取っていない。

Go は `mise.toml` の `[tools].go` が `1.26.10`、`backend/go.mod` の `toolchain` 行が `go1.26.10` で揃っている。
go1.27.4 は 2026-11-10 に公開され、security fixes を含む 1.27 系の最新パッチである。
docs/issues/closed/0231 は、go1.27 系への更新を別の判断として本 issue へ送った。

依存パッケージの現状は、`backend` の `go list -m -u all` が 156 モジュールの更新を報告し、`frontend` の `npm outdated` が 30 パッケージの更新を報告する。
`npm audit --omit=dev` は 0 件で、既知の脆弱性は無い。

## 対応方針

- `mise.toml` の `[tools].node` を `26.13.1` にする。
- `mise.toml` の `[tools].go` を `1.27.4` に、`backend/go.mod` の `toolchain` 行を `go1.27.4` に同時に更新する (docs/issues/closed/0145 の運用)。
- `backend/go.mod` の `go` ディレクティブを `go 1.27.0` に上げ、言語バージョンを toolchain と揃える。
- `AGENTS.md` の「対象言語/バージョン」が定める Go 1.26 系の前提を 1.27 系に書き換える。
- `AGENTS.md` の「frontend (React)」の対象スタックと `docs/prd/thief.md` の「制約と前提」にある Go と React のバージョンを、更新後の値に揃える。
- backend は `go get -u ./...` で直接依存と間接依存を同じメジャーバージョンの中で最新に上げ、`go mod tidy` で `go.mod` と `go.sum` を整理する。
- frontend は `npm update` で `package.json` の範囲内の更新を当てたうえで、メジャー更新を `package.json` に反映する。対象は `react` と `react-dom` の 19、`@types/react` と `@types/react-dom` の 19、`@testing-library/jest-dom` の 7、`@xterm/xterm` の 6 と `@xterm/addon-fit` の 0.11、`i18next` の 27 と `react-i18next` の 18、`jsdom` の 31、`typescript` の 6 である。
- `apache-arrow` は duckdb-wasm が `^17.0.0` を要求するため 17 系に留める。21 系に上げると duckdb-wasm が同梱する arrow と実体が分かれ、`lib/duckdb.ts` と `lib/duckdbResult.ts` の型が合わなくなる。
- `typescript` は typescript-eslint 8 の peer 範囲 `>=4.8.4 <6.1.0` に合わせて `~6.0.3` に留める。7 系には typescript-eslint が利用する JavaScript API が無く、ESLint が起動時に停止する。
- `@duckdb/duckdb-wasm` は `1.32.0` に固定したままにする。npm の `latest` は `1.33.1-dev57.0` であり、安定版が無いため。
- メジャー更新で API の差異が出た箇所は、呼び出し側を追随させる。追随の結果、画面の見た目と振る舞いを変える修正が要る場合は、その場で直さずに選択肢を報告する。
- 依存の更新で `mise run check` が落ちた場合、落ちた原因を直してから完了とする。テストの削除、skip、lint 規則の緩和では通さない。

## 完了条件

- `mise.toml` の `[tools].node` が `26.13.1` で、`node --version` が `v26.13.1` を返す。
- `mise.toml` の `[tools].go` が `1.27.4`、`backend/go.mod` の `toolchain` 行が `go1.27.4`、`go` ディレクティブが `go 1.27.0` であり、`go version` が `go1.27.4` を返す。
- `frontend/package.json` と `frontend/package-lock.json` が上記の更新を反映し、`apache-arrow` が `^17.0.0`、`typescript` が `~6.0.3` で、`npm ls` が依存の不整合を報告しない。
- `npm audit --omit=dev` の既知の脆弱性が 0 件である。
- `backend/go.mod` と `backend/go.sum` が `go mod tidy` の結果と一致する。
- `mise run check` が通る。fmt の実行後に `git status --porcelain` が空か、差分が生じたファイルを整形済みにして残す。
- `mise run backend:vuln` が終了コード 0 で、影響する脆弱性が 0 件と報告される。
- `AGENTS.md` の「backend (Go)」の「対象言語/バージョン」が Go 1.27 系を前提とする記述になっている。
- `AGENTS.md` の「frontend (React)」の対象スタックと `docs/prd/thief.md` の「制約と前提」が、Go 1.27 系と React 19 を指している。
- `CHANGES.md` の `## develop` の `### misc` に、Node.js、Go、backend、frontend の更新を説明するエントリを追加し、担当者行 `  - @sfuruya0612` を持つ。
- 扱わない範囲: `@duckdb/duckdb-wasm` の開発ビルドへの更新、Go の `go` ディレクティブが要求する言語バージョンを超える書き換え、CI のジョブ構成の変更。

## 解決方法

- `mise.toml` の `[tools].node` を `26.13.1`、`[tools].go` を `1.27.4` にして `mise install` を実行した。`node --version` は `v26.13.1`、`go version` は `go1.27.4` を返す。
- `backend/go.mod` の `toolchain` 行を `go1.27.4`、`go` ディレクティブを `go 1.27.0` にした。`go get -u ./...` で 156 モジュールを更新し、`go mod tidy` の後に `go mod tidy -diff` の差分が無いことを確認した。
- `frontend` は `npm update` で `package.json` の範囲内の更新を当てた後、react と react-dom を 19.3.1、@types/react と @types/react-dom を 19.3.0、@testing-library/jest-dom を 7.0.1、@xterm/xterm を 6.0.0 と @xterm/addon-fit を 0.11.0、i18next を 27.1.1 と react-i18next を 18.0.1、jsdom を 31.0.0、typescript を 6.0.3 にした。
- 依存更新で API と型が変わった箇所を直した。
  - `backend/internal/tui/tab_list.go` と `model.go`: bubbles v1.0.0 の `table.Model.SetRows` が直前の行数より大きいカーソルだけを丸めるようになり、`SetRows(nil)` の後に行を入れてもカーソルが -1 のまま残る。一覧のカーソルを 0 に戻し、取り直しの前後でカーソルを復元するようにした。修正前は `TestModel_ReloadKeepsCursorOnSelectedRow`、`TestModel_TagFetcherFetchesRowSelectedDuringReload`、`TestListDetailTabImmediateDetailAndIntent`、`TestListDetailTabAsyncDetailAndStaleResult`、`TestECSInteractiveTabs`、`TestELBWAFAndRDSTabs` が失敗する。
  - `frontend/src/components/icons/AwsIcons.tsx`、`GcpIcons.tsx`、`Icons.tsx`: React 19 の型でグローバルな `JSX` 名前空間が無くなったため、`react` から `JSX` 型を import する。
  - `frontend/src/components/logviewer/LogList.tsx`: `useRef<HTMLDivElement>(null)` の型が `RefObject<HTMLDivElement | null>` になったため、`bodyRef` の型を合わせる。
  - `frontend/src/components/Drawer/DrawerCFNEvents.test.tsx`: jsdom 31 が `color: var(--err)` をカスタムプロパティの値に解決しないため、インライン style の値を直接検証する。
  - `frontend/src/components/Terminal/TerminalDock.test.tsx`: jsdom 31 では `hidden` を付けた要素の `display` が `flex` を返し、29.1.1 の quirk が解消している。表示を判定しない理由を、CSS の import が空文字列に差し替えられて jsdom に規則が無いことに書き直した。
  - `frontend/vite.config.ts`: 規則のテキストを検証するテストのパスを、実在する `src/styles/styles.test.ts` に直した。
- `AGENTS.md` の「対象言語/バージョン」を Go 1.27 系、「frontend (React)」の対象スタックを React 19 に、`docs/prd/thief.md` の「制約と前提」を Go 1.27 系と React 19 に揃えた。
- `mise run check` が通り、`mise run backend:vuln` は 0 件、frontend のテストは 119 ファイル 1350 件すべて成功した。
- 計画との差分:
  - `apache-arrow` は 21 ではなく 17 系のままにした。duckdb-wasm 1.32.0 が `^17.0.0` を要求し、21 系に上げると duckdb-wasm が同梱する arrow と実体が分かれて `lib/duckdb.ts` の型が合わなくなる。
  - `typescript` は 7 ではなく `~6.0.3` にした。typescript-eslint 8.72.2 の peer 範囲が `>=4.8.4 <6.1.0` であり、7.0 では ESLint が起動せず、6.1 以降も受け付けない。
  - 完了条件の「上記のメジャー更新を反映し」を「上記の更新を反映し」に直し、`apache-arrow` と `typescript` の値を条件に加えた。
- 扱わなかった範囲: `npm audit` (開発依存を含む) が報告する minimatch の moderate 10 件。eslint 10.14.0 と typescript-eslint 8.72.2 が minimatch 10 系に依存し、修正版の 11 系は両者の依存範囲の外になるため、この更新では解消しない。`npm audit --omit=dev` は 0 件である。

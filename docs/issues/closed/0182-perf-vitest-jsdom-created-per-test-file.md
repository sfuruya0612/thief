# vitest のテスト環境をワーカーごとの生成に変えて実行時間を短縮する

Created: 2026-09-16
Model: Claude Opus 5
Completed: 2026-09-17

## 背景

`docs/issues/TODO.md` の次の項目に対応する。

> - [ ] frontend のテストで jsdom 環境が実行ごとに作り直されておりテスト全体の実行時間の大半を占めている (issue 0174 のレビューで判明。vitest の `pool: 'vmThreads'` または `isolate: false` の採用でファイルごとの分離を保ったまま高速化できる可能性がある)

`mise run frontend:test` は `frontend` ディレクトリで `npm run test` を実行し、これは vitest 5.0.0 を呼ぶ。
`frontend/vite.config.ts` の `test` は次のとおりで、`pool` も `isolate` も指定していない。

```ts
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/setupTests.ts'],
    css: { include: [/\.css\?raw$/] },
  },
```

`pool` の既定は `forks`、`isolate` の既定は `true` であるため、テストファイルごとに環境が作り直される。

起票時にコンテナ内で `npx vitest run` を実行した結果は次のとおりである。

| 項目 | 値 |
| --- | --- |
| テストファイル数 | 95 |
| テスト数 | 987 |
| Duration | 71.98 秒 |
| 内訳 | environment 74%、setup 19%、import 2%、tests 2%、transform 2% |
| jsdom の生成回数 | 95 回 |
| jsdom の生成に費やした時間 | 247.69 秒 (計測対象時間の 74%、並列実行のため実時間を超える) |

テストコードそのものの実行 (tests) は全体の 2% であり、実行時間の大半は jsdom の生成が占める。
vitest 自身も出力の末尾で `pool: 'vmThreads'` (ファイルごとの分離を保ったままワーカーごとに 1 回だけ生成する) と `isolate: false` (ファイル間で共有する) を案内している。

## 目的

`mise run frontend:test` の実行時間を短縮し、テストを回す間の待ち時間を減らす。
テストの結果 (95 ファイル 987 テストがすべて通ること) は変えない。

## 計測手段

起票時に、同じコンテナで 3 つの設定を実行して比較した。設定ファイルは変更せず、コマンドラインのオプションで切り替えた。

| 設定 | コマンド | Duration | 結果 |
| --- | --- | --- | --- |
| 現状 (`pool: 'forks'`、`isolate: true`) | `npx vitest run` | 71.98 秒 | 95 ファイル 987 テストすべて成功 |
| `pool: 'vmThreads'` | `npx vitest run --pool=vmThreads` | 9.58 秒 | 95 ファイル 987 テストすべて成功 |
| `isolate: false` | `npx vitest run --no-isolate` | 18.33 秒 | 10 ファイル 71 テストが失敗 |

`--pool=vmThreads` は再実行しても 8.82 秒で 987 テストすべてが成功し、結果が安定した。

`pool: 'vmThreads'` がファイルごとの分離を保つことは、`frontend/node_modules/vitest` の型定義にある `isolate` の説明「Run tests in an isolated environment. This option has no effect on vmThreads pool.」で裏付けられる。
`vmThreads` はワーカーごとに VM コンテキストを用意し、ファイルごとにその中で環境を作り直すため、`isolate` の指定を受け付けない。
計測で 95 ファイル 987 テストがすべて成功したことも、分離が保たれていることと整合する。

`--no-isolate` の失敗は実行するたびに顔ぶれが変わる。
1 回目は 10 ファイル 71 テスト、2 回目は 11 ファイル 71 テストが失敗し、失敗したファイルの集合が一致しなかった。
2 回目に失敗したのは `src/App.test.tsx`、`src/components/SSOExpiredBanner.test.tsx`、`src/components/Sidebar.test.tsx`、`src/hooks/useDatadogLogin.test.tsx`、`src/views/AccountView.test.tsx`、`src/views/CostExplorerPanel.test.tsx`、`src/views/PricingPanel.test.tsx`、`src/components/Drawer/DrawerECSTasks.test.tsx`、`src/components/Drawer/DrawerRDSInstanceParameters.test.tsx`、`src/components/Drawer/DrawerWAFRules.test.tsx`、`src/components/session/DatadogOrgSessionTabs.test.tsx` である。
実行順に依存してファイル間で状態が干渉していることを示す。

したがって計測に基づく結論は次のとおりである。**`isolate: false` は採用できず、`pool: 'vmThreads'` を採用する。**

実装時は、設定を変更したうえで `npx vitest run` を 3 回実行し、Duration と成功したテスト数を記録する。
比較の基準は上表の現状の値とする。

## 最適化方針

`frontend/vite.config.ts` の `test` に `pool: 'vmThreads'` を追加する。`isolate` は設定しない (`vmThreads` では効かないため)。

「## 計測手段」の計測結果をそのまま採用する。現状 (`pool: 'forks'`、`isolate: true`) の 71.98 秒に対し、`pool: 'vmThreads'` は 9.58 秒 (再実行時 8.82 秒) で 95 ファイル 987 テストすべて成功し、結果も安定していた。`isolate: false` は 18.33 秒だが、実行するたびに顔ぶれの変わる 10〜11 ファイル 71 テストが実行順依存で失敗するため採らない。`vmThreads` がファイルごとの分離を保つことは vitest の型定義の説明 (`isolate` は `vmThreads` プールに効果が無い) と、計測で全テストが成功したことの両方で裏付けられている。

`vite.config.ts` には、`pool: 'vmThreads'` を選んだ理由と `isolate: false` を採らなかった理由 (計測値と失敗したテスト数) を日本語コメントで残す。

## 完了条件

- `frontend/vite.config.ts` の `test` に `pool: 'vmThreads'` が設定されている。
- `isolate: false` を設定しない。`frontend/node_modules/vitest` の型定義は `isolate` について「This option has no effect on vmThreads pool.」と記しており、`pool: 'vmThreads'` の下ではこの項目を書いても効かない。計測でも `isolate: false` は 71 テストを失敗させており、採る理由が無い。
- `vite.config.ts` に、`pool: 'vmThreads'` を選んだ理由と `isolate: false` を採らなかった理由 (計測値と失敗したテスト数) が日本語のコメントで残っている。
- `mise run frontend:test` が 95 ファイル 987 テストすべて成功する。
- `mise run frontend:test` を 3 回連続で実行し、3 回とも成功し、かつ 3 回とも Duration が 30 秒未満である (現状の 71.98 秒に対する改善を客観的に判定するための閾値。計測値 9.58 秒に対して十分な余裕を取った値とする)。
- 既存のテストファイルとテストコードを変更しない。設定の変更だけで上記を満たす。変更が必要になった場合は、なぜ必要かを issue に追記したうえで、変更したファイルと理由を記録する。
- `mise run check` が通る。

### 扱わない範囲

- `isolate: false` を採用するために失敗する 11 ファイルを修正することは本 issue では行わない。`pool: 'vmThreads'` が分離を保ったまま `isolate: false` より速いという計測結果が出ているため、テストの書き換えを伴う選択肢を追う理由が無い。
- `setupFiles` (`frontend/src/setupTests.ts`) の内容は変更しない。内訳の setup 19% はこのファイルの実行に相当するが、`pool: 'vmThreads'` 適用後の実測で全体が 9.58 秒まで下がっており、追加の最適化は不要である。
- backend のテスト実行時間は本 issue の対象ではない。

## 関連

- `docs/issues/closed/0174-feat-persistent-terminal-dock.md` のレビューで検出された指摘が本 TODO の出所である。
- 同じレビューで検出されたもう 1 件 (`App.tsx` の未使用 eslint-disable) は別 issue で扱う (docs/issues/closed/0181)。

## 解決方法

`frontend/vite.config.ts` の `test` に `pool: 'vmThreads'` を追加した。`isolate` は設定していない。追加した設定の直前に、選定理由 (現状 71.98 秒に対し vmThreads は 9.58 秒、再実行時 8.82 秒) と `isolate: false` を採らなかった理由 (18.33 秒だが実行順依存で 10〜11 ファイル 71 テストが失敗する) を記した日本語コメントを残した。`isolate` を指定しないのは vmThreads プールでは効果が無いためである。

実装時の `mise run check` (実装エージェントの worktree 内、以下の集計方法は解決方法の書き方に合わせて統合後の値を主とする) と、統合後にこの作業ツリーで実行した `npx vitest run` を 3 回連続実行した結果は次のとおりですべて成功し、Duration はすべて 30 秒未満だった。

| 回 | ファイル | テスト | Duration |
| --- | --- | --- | --- |
| 1 | 96 | 997 | 7.48 秒 |
| 2 | 96 | 997 | 9.88 秒 |
| 3 | 96 | 997 | 7.99 秒 |

`mise run check` も通過した (frontend は 96 ファイル 997 テストすべて成功、eslint は 9 件の既存警告のみで新規警告なし、backend は govulncheck を含め 0 vulnerabilities)。

起票時の計測 (95 ファイル 987 テスト) との差は、issue 0177〜0181 が本 issue の実装より先に着地し、その間にテストが 1 ファイル 10 テストぶん増えたためである。設定変更のみで全テストが成功するという完了条件の趣旨は変わらないため、起票時の計測値そのもの (`## 計測手段` の表) は書き換えていない。

3 観点の多観点レビュー (完了条件との突き合わせ、回帰と整合、成果物の規約整合) はいずれも指摘 0 件だった。回帰と整合のレビューでは、`isolate: false` で見られた実行順依存の flaky さが vmThreads では再現しないことを複数回の実行で確認している。

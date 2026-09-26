# App.tsx の未使用 eslint-disable ディレクティブを削除する

Created: 2026-09-16
Model: Claude Opus 5
Completed: 2026-09-17

## 背景

`docs/issues/TODO.md` の次の項目に対応する。

> - [ ] `App.tsx` 100 行目付近の `no-console` の eslint-disable ディレクティブが未使用になっている (issue 0174 のレビューで判明。対象のコードが既に無くなっており警告として検出されている)

`frontend/src/App.tsx` の 98 行目から 103 行目は次のとおりである。

```tsx
  useEffect(() => {
    if (error) {
      // eslint-disable-next-line no-console
      console.error('failed to load profiles', error);
    }
  }, [error]);
```

`npx eslint src/App.tsx` を frontend ディレクトリで実行すると次の警告が出る。

```
  100:7  warning  Unused eslint-disable directive (no problems were reported from 'no-console')
```

`npm run lint` は `eslint . && tsc --noEmit` であり、ESLint の警告では終了コードが 0 のままなので Lint は失敗しない。
`frontend/AGENTS.md` の「Lint / Format」は「警告は許容するが、新規コードで警告を増やさないよう努める」と定めており、この警告は既存分として残っている。
リポジトリ全体の ESLint の警告は 10 件で、そのうち 1 件がこの未使用ディレクティブである。

### TODO の記述と実際の原因が異なる

TODO は原因を「対象のコードが既に無くなっており」としているが、これは誤りである。
`console.error('failed to load profiles', error)` は 101 行目に現在も存在する。

実際の原因は、`frontend/eslint.config.js` が `no-console` ルールを設定していないことである。
同ファイルの `rules` に並ぶのは `react-hooks/rules-of-hooks`、`react-hooks/exhaustive-deps`、`react-refresh/only-export-components`、`@typescript-eslint/no-unused-vars` の 4 つで、`extends` に指定するのは `js.configs.recommended` と `tseslint.configs.recommended` である。
`no-console` はどちらの recommended にも含まれない。
有効でないルールを抑止しているため、ディレクティブが不要と判定される。

警告として現れるのは ESLint 9 以降で `linterOptions.reportUnusedDisableDirectives` の既定値が `"warn"` になったためである。
`frontend/eslint.config.js` はこの項目を明示していない。
このリポジトリの ESLint は 10.10.0 である。

### 他の eslint-disable は削除対象ではない

`frontend/src` にある `eslint-disable` は次の 5 か所で、`App.tsx` 以外の 4 か所はすべて `frontend/src/views/PricingPanel.tsx` の `react-hooks/exhaustive-deps` に対するものである (121、141、155、177 行目)。
`react-hooks/exhaustive-deps` は `eslint.config.js` で `warn` として有効であり、未使用ディレクティブの警告は出ていない。これらは削除しない。

## 対応方針

`frontend/src/App.tsx` の 100 行目の `// eslint-disable-next-line no-console` を削除する。
`console.error` の呼び出しは残す。

`no-console` ルールを `eslint.config.js` に追加してディレクティブを有効にする方法は採らない。
ルールを有効にすると、この 1 か所を抑止し続けるために同じディレクティブが必要になり、警告が消えるだけで状況は変わらない。
加えて frontend には `console` を使う箇所が他にもありうるため、ルールの追加はプロジェクト全体の方針判断になる。TODO が求めているのは未使用ディレクティブの解消であり、ルールの追加は求めていない。

`linterOptions.reportUnusedDisableDirectives` を `off` にして警告を止める方法も採らない。
未使用のディレクティブを検出できなくなり、同種の残骸が今後見つからなくなる。

## 完了条件

- `frontend/src/App.tsx` から `// eslint-disable-next-line no-console` が削除されている。
- `frontend/src/App.tsx` の `console.error('failed to load profiles', error)` が残っている。
- frontend ディレクトリでの `npx eslint .` の警告が 10 件から 9 件に減り、`Unused eslint-disable directive` の警告が 0 件になる。
- `frontend/eslint.config.js` を変更しない。
- `frontend/src/views/PricingPanel.tsx` の 4 か所の `eslint-disable-next-line react-hooks/exhaustive-deps` を変更しない。
- `mise run check` が通る。

## 関連

- `docs/issues/closed/0174-feat-persistent-terminal-dock.md` のレビューで検出された指摘が本 TODO の出所である。同 issue は baseline から存在する問題として扱い、対応を TODO.md へ送った。
- 同じレビューで検出されたもう 1 件 (vitest の jsdom 生成が実行時間の大半を占める) は別 issue で扱う。

## 解決方法

`frontend/src/App.tsx` の 100 行目にあった `// eslint-disable-next-line no-console` を削除した。`console.error('failed to load profiles', error)` の呼び出しはそのまま残した。`frontend/eslint.config.js` と `frontend/src/views/PricingPanel.tsx` の 4 か所の `eslint-disable-next-line react-hooks/exhaustive-deps` は変更していない。

変更後、frontend ディレクトリでの `npx eslint .` の警告は 10 件から 9 件に減り、`Unused eslint-disable directive` の警告は 0 件になった。残る 9 件は本 issue と無関係な既存の警告 (`Drawer.tsx`、`AwsIcons.tsx`、`GcpIcons.tsx`、`Icons.tsx`、`cells.tsx`、`AccountView.tsx`) である。

`mise run check` は通過した (frontend は 96 ファイル 997 テストすべて成功、backend は govulncheck を含め 0 vulnerabilities)。3 観点の多観点レビュー (完了条件との突き合わせ、回帰と整合、成果物の規約整合) はいずれも指摘 0 件だった。

# 英語 UI でターミナルドックのリサイズハンドルのツールチップが日本語のまま表示される

Created: 2026-09-29
Model: Claude Fable 5.1

## 症状

Tweaks パネルで Language を English にしても、常駐ターミナルドック (`TerminalDock`) の上端にあるリサイズハンドル (`.terminal-dock-resizer`) にマウスを載せたときのツールチップが日本語の「ドラッグして高さを変更する」のまま表示される。
同じドックの他の文言 (Collapse terminal / Expand terminal / Close session など) は英語に切り替わる。

## 再現手順

1. `mise run backend:run` と `mise run frontend:run` で起動し、Tweaks パネルの Language を English にする。
2. EC2 または ECS の Drawer からターミナルを接続し、画面下部のターミナルドックを展開した状態にする。
3. ドックの上端のリサイズハンドル (`title` 属性を持つ `div.terminal-dock-resizer`) にマウスを載せる。
4. 期待する結果: 英語のツールチップが出る。実際の結果: 「ドラッグして高さを変更する」が出る。

ブラウザを使わずに確認することもできる。`frontend/` で ja / en の `drawerStorage.json` のキー集合を比べると、ja にだけ `terminal.resizeDock` がある (2026-09-28 に node で実測。差分はこの 1 キーだけである)。

## 原因

`frontend/src/i18n/locales/en/drawerStorage.json` の `terminal` に `resizeDock` キーが無い。

- `frontend/src/components/Terminal/TerminalDock.tsx` はリサイズハンドルの `title` に `t('terminal.resizeDock')` を使う。
- `frontend/src/i18n/locales/ja/drawerStorage.json` の `terminal.resizeDock` は、常駐ターミナルドックの高さをドラッグで変更できるようにした docs/issues/closed/0176 の実装 (コミット `1507806`) で ja にだけ追加され、en には追加されなかった。en のロケールはそれより前のコミット `4dc47c2` で既に存在していた。
- `frontend/src/i18n/index.ts` は `fallbackLng: 'ja'` を設定しているため、en にキーが無くてもキー名は表示されず、日本語の文言が表示される。この fallback がキーの欠落を隠している。
- ja と en のキー集合の一致を検査するテストは無い (`frontend/src/i18n/` にテストファイルは無く、`locales/en` を読むテストも無い)。

## 修正方針

- `frontend/src/i18n/locales/en/drawerStorage.json` の `terminal` に `resizeDock` を追加する。文言は他のドック操作の文言 (Collapse terminal / Expand terminal) と同じ形の英語にする。
- 再発を防ぐため、ja と en の全ネームスペースについてキー集合の一致 (双方向) を検査するテストを `frontend/src/i18n/locales.test.ts` に追加する。`index.ts` と同じ `import.meta.glob` の形でロケールを読み、`lang` ごとにネームスペースとキーのパスを平坦化して比べる。

採らなかった案と却下の理由は次のとおりである。

- `fallbackLng` を外して欠落をキー名で表示させる案は採らない。ユーザーに `terminal.resizeDock` のようなキー名を見せることになり、症状が別の形で残る。
- `TerminalDock.tsx` の `title` に英語をハードコードする案は採らない。ドックの他の文言は i18n に載っており、1 つだけ外す理由が無い。

新しい API と権限は不要で、backend の変更も無い。

## 完了条件

- `frontend/src/i18n/locales/en/drawerStorage.json` の `terminal.resizeDock` が英語の文言で存在する。
- ja と en の全ネームスペースのキー集合が一致することを検査するテストが `frontend/src/i18n/locales.test.ts` にあり、通過する。このテストは、どちらかのロケールからキーを 1 つ消すと失敗する。
- `TerminalDock.tsx` の `title` は `t('terminal.resizeDock')` のままとし、文言のハードコードや `fallbackLng` の変更は行わない。
- `mise run check` が通過する。

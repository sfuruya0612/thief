# 英語 UI でターミナルドックのリサイズハンドルのツールチップが日本語のまま表示される

Created: 2026-09-29
Model: Claude Fable 5.1
Completed: 2026-10-02

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

## 実装詳細の乖離 (2026-10-02)

修正方針は en への追加を `drawerStorage.json` の `terminal.resizeDock` だけとしていたが、`frontend/src/i18n/locales/en/drawerAws.json` にも 2 キーを追加した。方式 (en のロケールにキーを足し、全ネームスペースのキー集合をテストで比べる) は変えていない。

- 理由: 全ネームスペースのキー集合を比べるテストを加えると、`drawerAws` でも ja にだけあるキーが 2 つ見つかり、テストが通らなかった。完了条件の「全ネームスペースのキー集合が一致する」テストを通すには、この 2 キーの追加が要る。
- 追加したキー:
  - `fetchFailedWarning.tooltip`: `frontend/src/components/primitives/FetchFailedWarning.tsx` がツールチップに使う。英語 UI でも ja の「取得に失敗しました (権限不足または一時的なエラーの可能性があります)」が表示されていた。
  - `drawerTags.fetchFailedHeading`: `frontend/src/components/Drawer/DrawerTags.tsx` が見出しに使う。英語 UI でも ja の「Tags (取得失敗)」が表示されていた。
- 2 キーはコミット `27fc8b0` (2026-08-07) で ja にだけ追加された。再現手順の「差分はこの 1 キーだけ」は `drawerStorage.json` の比較の結果であり、`drawerAws.json` は比べていなかった。

## 解決方法

## 解決方法

英語 UI で `TerminalDock` のリサイズハンドルのツールチップが日本語のままになる原因 (en のロケールに `terminal.resizeDock` が無く、`fallbackLng: 'ja'` が欠落を隠していた) を、en へのキーの追加で直した。同じ欠落の再発を防ぐため、ja と en の全ネームスペースのキー集合を比べるテストを加えた。

変更したファイルとシンボル。

- `frontend/src/i18n/locales/en/drawerStorage.json`: `terminal` に `resizeDock` (`"Resize terminal"`) を `collapseDock` と `expandDock` の間に追加した。文言は同じドックの `Collapse terminal` / `Expand terminal` と同じ「動詞 + terminal」の形にした。
- `frontend/src/i18n/locales/en/drawerAws.json`: `fetchFailedWarning.tooltip` と `drawerTags.fetchFailedHeading` を追加した (「## 実装詳細の乖離 (2026-10-02)」)。
- `frontend/src/i18n/locales.test.ts` (新規): `index.ts` と同じ `import.meta.glob('./locales/*/*.json', { eager: true })` でロケールを読み、`flattenKeyPaths` でキーを `terminal.resizeDock` の形に平坦化し、`collectKeyPathsByLang` で言語とネームスペースごとに集める。次の 3 つのテストを持つ。
  - `ja と en の両方のロケールが読み込まれている`: glob の不一致で両方が空になり、比較が素通りすることを防ぐ。
  - `ja と en のネームスペース集合が一致する`: 片方にしか無いネームスペースのファイルを `locales/en/<ns>.json does not exist` の形で挙げる。
  - `ja と en の全ネームスペースでキー集合が双方向に一致する`: 片方にしか無いキーを `locales/en/drawerStorage.json is missing "terminal.resizeDock"` の形で挙げる。

`TerminalDock.tsx` の `title={t('terminal.resizeDock')}` と `frontend/src/i18n/index.ts` の `fallbackLng` は変えていない。

完了条件ごとの検証。

- en の `terminal.resizeDock`: `frontend/src/i18n/locales/en/drawerStorage.json` に `"Resize terminal"` で存在する。
- キー集合の一致を検査するテスト: `frontend/src/i18n/locales.test.ts` の 3 テストが `mise run test` で通過した (frontend は 119 ファイル、1350 テストが通過)。en から `terminal.resizeDock` を一時的に消すと `locales/en/drawerStorage.json is missing "terminal.resizeDock"` で、ja から消すと `locales/ja/drawerStorage.json is missing "terminal.resizeDock"` で失敗することを確かめ、消したキーを戻した。
- `TerminalDock.tsx` と `fallbackLng`: 差分に含まれない (変更したファイルは上の 3 つだけ)。
- `mise run check`: 同じ内容の 5 段 (backend の gofmt / goimports の check、frontend の `prettier --check .`、`mise run lint`、`mise run backend:vuln`、`mise run test`) がすべて終了コード 0 で通った。eslint の warning は変更前と同じ 9 件で、新たな失敗は無い。

再現確認。修正前は、`i18n.changeLanguage('en')` の後に `i18n.t('terminal.resizeDock', { ns: 'drawerStorage' })` を読む一時的なテストが「ドラッグして高さを変更する」を返して失敗した。node による ja と en のキー集合の比較でも、ja にだけ `drawerStorage` の `terminal.resizeDock` と `drawerAws` の 2 キーがあった。修正後は同じテストが `Resize terminal` を返して通過し、キー集合の差分は無くなった。一時的なテストは削除し、回帰テストは `locales.test.ts` のキー集合の比較が担う。

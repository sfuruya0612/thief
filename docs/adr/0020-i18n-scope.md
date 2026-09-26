# 0020. UI の固定の文言を翻訳し、Drawer のタブ名と外部サービスのメッセージは英語のままにする

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-07-21

## 状況

UI を日本語と英語で切り替えたいという要望があった (issue 0050)。
一方、Drawer のタブ名 (Overview、Images、Targets など) は AWS のコンソールと同じ英語の方が分かりやすい。
AWS などが返すエラーメッセージは英語で、thief の側で訳すと元の意味を損なう。

## 決定

- react-i18next を使い、日本語で書かれていた UI の固定の文言を機能ごとの名前空間 (`frontend/src/i18n/locales/<lang>/`) に外に出す (issue 0050、2026-07-21)。
  もともと英語で書かれていた文字列 (テーブルの列見出し、Theme、Dark、Light など) は翻訳の対象にしない。
  言語は日本語 (`ja`) と英語 (`en`) とする。
- Drawer の参照専用のタブのタブ名は英語で直に書き、翻訳の対象にしない (issue 0066、2026-07-23)。
  issue 0076 と 0157 がこれを踏襲した。
- 外部サービスが返したメッセージを表示する部品 (`DrawerError`) は、ラベルも英語で直に書く (issue 0075)。
  issue 0082 がこれを他のタブに広げた。
- 言語の選択は表示設定 (`useTweaks.ts` の `lang`) に保存する。

## 検討した代替案

issue 0050 は次の案を比べた。

- 翻訳の方式: `react-i18next` の導入と、自前の軽量な辞書とフック。
  補間、複数形、名前空間の分割が組み込みで揃う利点が依存を最小にする方針を上回るとして、`react-i18next` を採った。
- 言語の保存先: `Tweaks` と、`PersistedState` の独立したフィールド。
- 切り替えの UI の配置: `TweaksPanel` と `TopBar`。

## 結果

- 日本語の表示でも、Drawer のタブ名とエラーメッセージは英語になる。
- `AGENTS.md` の frontend 「基本方針」は翻訳リソースの場所を `src/i18n/locales/ja/` とだけ書いており、`en` の存在を書いていない。

## 根拠資料

- `docs/issues/closed/0050`、`0066`、`0075`、`0076`、`0082`、`0157`
- `frontend/src/i18n/`、`frontend/src/hooks/useTweaks.ts`

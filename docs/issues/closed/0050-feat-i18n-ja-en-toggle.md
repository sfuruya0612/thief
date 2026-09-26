# UI 文字列を日本語と英語で切り替えられるようにする

Created: 2026-07-19
Completed: 2026-07-21
Model: Claude Opus 4.8

## 背景

frontend の UI 文字列は現在すべて日本語のハードコードである。

`AGENTS.md` の frontend 節は多言語対応について「現状は日本語 UI 文字列のハードコードのみ。i18n ライブラリは未導入 (需要が出るまで導入しない)」と定めている。
`docs/issues/TODO.md` に「日本語、英語で表記を切り替えられるようにして」という要望があり、この需要が顕在化した。
本 issue はこの要望を実現するための設計判断を整理する。

## 現状

- 日本語文字列を含む非テストの `.ts` / `.tsx` ファイルは 135 個ある。UI 文字列は各コンポーネントに直接埋め込まれており、外部化されていない。
- UI 設定は `components/TweaksPanel.tsx` に集約され (Theme / Detail panel / Accent の 3 項目)、`hooks/useTweaks.ts` と `lib/storage.ts` の `PersistedState.tweaks` で永続化する。
- 言語を切り替える仕組みと、言語設定を保持するフィールドは存在しない。
- エラーメッセージの一部はバックエンド由来である (`api/client.ts` が `ApiError` として受ける `message`)。これらはフロントの辞書では切り替えられない。

## 目的

- UI 文字列を日本語と英語で切り替えられるようにする。
- 選択した言語を永続化し、リロード後も保持する。

## 設計上の論点

着手前に次の点を決める必要がある。とくに文字列外部化の方式は `AGENTS.md` の既存方針の転換を含むため、ユーザーの判断を要する。

### 文字列外部化の方式

2 つの選択肢がある。

- **i18n ライブラリの導入** (`react-i18next` 等): 補間、複数形、名前空間分割などの機能が揃うが、依存が重い。`AGENTS.md` は状態管理ライブラリやアイコンパッケージなど外部依存の追加を抑制する方針で、i18n も「需要が出るまで導入しない」としているため、導入はこの方針の転換になる。
- **自前の軽量な辞書とフック**: キーから `{ ja, en }` を引く辞書と `useLang` 相当のフックを自作する。依存を増やさず `AGENTS.md` の YAGNI 方針と整合するが、補間や複数形は自前で実装する必要がある。UI 文字列が固定の短い語句が中心であれば、この方式で足りる可能性が高い。

どちらを採るかで実装量とファイル構成が変わる。

### スコープ

135 ファイルの全 UI 文字列を一度に対象とするか、主要画面から段階的に外部化するかを決める。
一度に外部化する場合、翻訳漏れの検出方法 (未翻訳キーのフォールバック挙動、Lint での検出可否) を定める。

### 言語設定の永続化先

`Tweaks` 型 (`types/common.ts`) に `lang` を追加して `TweaksPanel` に載せるか、`PersistedState` に独立したフィールドを追加するかを決める。
Theme / Accent と同じ「表示に関するユーザー設定」であるため、`Tweaks` に含める方が既存パターンと揃う。

### 言語切り替え UI の配置

`TweaksPanel` に Language 行を追加する案が既存 UI と最も整合する。
`TopBar` に独立したトグルを置く案もある。

### 動的文字列とロケール依存フォーマット

- バックエンド由来のエラーメッセージ (`ApiError.message`) はフロントの辞書では切り替えられない。エラーコード (`code`) を辞書のキーに用いてフロント側で文言を持つか、日本語のまま表示するかを決める。
- 日付と数値のフォーマット (`lib/format.ts`、`primitives/Money.tsx`) をロケールに追従させるかを決める。金額は USD 固定表示のため、当面は言語切り替えの対象外にできる。

## pending にした理由 (解消済み)

新規依存の追加可否 (i18n ライブラリ) と、`AGENTS.md` の既存方針 (「需要が出るまで導入しない」) の転換という設計判断を要した。
2026-07-21 にユーザーへ確認し、以下の通り方針が決定したため `issues/` へ戻す。

## 決定した方針

- **文字列外部化の方式**：`react-i18next` + `i18next` を導入する。135 ファイル規模の全画面を対象にするため、補間・複数形・名前空間分割が組み込みで揃うライブラリの利点が YAGNI の依存最小化方針を上回ると判断した。`AGENTS.md` の「i18n は需要が出るまで導入しない」は、この issue の起票 (`docs/issues/TODO.md` の要望) によって需要が顕在化したため転換する。
- **スコープ**：段階分割はせず、対象の 135 ファイル (JP 文字列を含む非テスト `.ts`/`.tsx`) を一度に外部化する。
- **バックエンド由来エラーメッセージの扱い**：`ApiError.code` をキーにフロント側 (`errors` namespace) で ja/en の文言を持つ。`code` が辞書に無い場合のみ `ApiError.message` (日本語) をそのまま表示するフォールバックとする。
- **言語設定の永続化先**：`Tweaks` 型 (`types/common.ts`) に `lang: 'ja' | 'en'` を追加し、`useTweaks`/`lib/storage.ts` の既存パターンで永続化する (Theme/Accent と同じ「表示に関するユーザー設定」のため)。
- **言語切り替え UI**：`TweaksPanel` に Language 行を追加する (Theme/Detail panel と同じセグメントコントロール)。

## 完了条件 (方式決定後に確定する)

- UI 文字列を日本語と英語で切り替えられる。
- 選択した言語が永続化され、リロード後も保持される。
- 翻訳漏れの扱い (未翻訳キーのフォールバック) が定義される。
- `CHANGES.md` の `## develop` に `[ADD]` エントリを追記する。
- `mise run check` が全て通過する。

## 解決方法

### 基盤

- `frontend/src/i18n/index.ts` で `react-i18next` + `i18next` を初期化した。翻訳リソースは機能領域別の名前空間 JSON (`frontend/src/i18n/locales/{ja,en}/{namespace}.json`) に置き、`import.meta.glob('./locales/*/*.json', { eager: true })` で自動収集する (namespace ファイルを追加するだけで有効になり、初期化コード側の登録は不要)。`initAsync: false` を指定し、全リソースがバンドル同梱で非同期バックエンド読み込みが無いことを利用して init() を同期完了させ、Suspense 無しで初回レンダーから翻訳を確定させている。
- `types/common.ts` に `Lang = 'ja' | 'en'` と `Tweaks.lang` を追加。デフォルトは `'ja'` (既存の日本語ハードコード UI と挙動を変えない)。`hooks/useTweaks.ts` の副作用で `tweaks.lang` を `i18n.changeLanguage` に反映する。
- `TweaksPanel.tsx` に Theme/Detail panel と同じセグメントコントロールで Language 行を追加した (`日本語`/`English` の表示名自体は言語に依らず固定)。
- `setupTests.ts` で i18n を初期化し、`afterEach` で `i18n.changeLanguage('ja')` にリセットして言語切り替えを検証するテストが他のテストファイルへ言語設定を漏らさないようにした。

### バックエンド由来エラーメッセージの扱い

調査の結果、`ApiError.code` を分岐に使っている箇所はフロント全体で `SSO_TOKEN_EXPIRED` の 1 箇所 (`SSOExpiredBanner` の表示条件) のみだった。バックエンドのエラーメッセージ自体 (`backend/internal/api/errors.go` 等) はすべて英語のハードコードまたは AWS/GCP SDK の生テキストであり、日本語は含まれていなかった。このため「エラーコードをキーにした汎用の翻訳辞書関数」は作らず、`SSOExpiredBanner.tsx` の固定文言 (フロント側が保持する UI テキスト) を `errors` namespace の通常の `t()`/`<Trans>` 呼び出しに置き換える形で、決定した方針を実現した。`ErrorBanner` が表示する `error.message` (バックエンド由来の生テキスト) は方針通り翻訳せず英語のまま表示する。

### 82 ファイルの外部化

対象の 135 ファイル (JP 文字列を含む非テスト `.ts`/`.tsx`) を機能領域別に 12 グループ (app / topbar / sidebar / session / drawerAws / drawerStorage / pricing / query / logviewer / cost / account / gcp) に分割し、並行して外部化した。各グループは専用の namespace JSON ファイルを持ち、ファイル集合も重複しないため衝突なく並行作業できた。

- 実際にユーザー向け日本語文字列を含んでいたのは 135 ファイル中の一部で、残りはコメントのみ (規約によりコメントは日本語のまま維持) か、既に英語ハードコードの UI 文字列 (Theme/Dark/Light、テーブル列見出し等、言語に依らず同じ表記のもの) だった。
- 各グループの ja.json の値は、既存のハードコード文字列と句読点まで完全一致させ、既存 vitest テストの日本語アサーションを変更せずに通過させている。
- 動的な値を含む文言は i18next の補間 (`{{value}}`) を、太字等のネストした要素を含む文言は `<Trans>` コンポーネントを使った。
- React コンポーネント/hook の外 (`lib/sessionMeta.ts`、`lib/logTimeRange.ts`、`lib/monthRange.ts`、`lib/normalizeQuery.ts`、`lib/objectPreview.ts`、`lib/format.ts` の `formatPricingUnit`) にあった文言は、`i18n.t('<namespace>:<key>')` を直接呼ぶ形か、呼び出し元のコンポーネントで解決した `t` を渡す形に変更した。

### 検証

- `cd frontend && npx tsc --noEmit`: エラーなし。
- `cd frontend && npx vitest run`: 57 ファイル / 506 テスト全て pass (既存テストの日本語アサーションは無変更のまま通過)。
- `cd frontend && npx eslint .`: 0 エラー、警告はこの変更前と同数の 9 件 (新規警告は発生時に修正済み: `AthenaView.tsx` の `useCallback` 依存配列漏れ 2 件)。
- `mise run check` (backend + frontend の fmt/lint/test 一式) が全て通過することを確認した。
- 開発サーバ (`npm run dev`) を起動し、バンドルが 200 で配信されエントリポイントが問題なく変換されることを確認した (ブラウザでのクリック操作による目視確認は環境上未実施。`TweaksPanel.test.tsx` の言語切り替えテストと `SSOExpiredBanner.test.tsx` の ja/en 両方の表示テストで、Language 行のクリックが実際に i18next の表示言語を切り替えることを自動テストで確認済み)。

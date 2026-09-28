# frontend の使われていない設定・トークン・フォント読み込みを削除する

Created: 2026-09-28
Model: Claude Fable 5.1

## 背景

刷新案 A (docs/issues/0201 の背景を参照) の第 5 段。2026-09-28 時点 (コミット 3562f56) で、定義だけがあって参照の無いものが次のとおりある。

- `Tweaks.layout` (`types/common.ts` の `Layout = 'tabs-top'`、`hooks/useTweaks.ts` の `DEFAULT_TWEAKS.layout`)。`localStorage` の `cloudlens:v1` に `tweaks.layout` として保存されるが、CSS (`data-layout` 属性は無い) からも TSX からも読まれていない。`TweaksPanel` に切り替えの UI も無い。
- `--shadow-drawer` (`tokens.css`)。`.drawer` は `--shadow-pop` を使っており、参照が無い。
- `--sp-5` (`tokens.css`、20px)。参照が無い。
- `[data-theme='light']` ブロックの `--accent-hi` (`#4f5abd`)。常に適用される `[data-accent]` ブロックが上書きするため、実効値になることが無い (docs/issues/0202 の背景を参照)。
- `index.html` の Google Fonts の読み込みに含まれる `Instrument+Serif`。CSS のどこからも参照が無く、起動のたびに使わないフォントを取得している。

## 目的

参照の無い定義を消し、「定義があれば使われている」状態に戻す。見た目と挙動は変えない。

## 設計判断

- `Tweaks.layout` は型 (`Layout`、`Tweaks.layout`) と `DEFAULT_TWEAKS.layout` から削除する。`useTweaks.ts` の `getSnapshot` は `{ ...DEFAULT_TWEAKS, ...persisted }` で永続化データを取り込むため、保存済みの `layout` はそのまま state に残り、次の保存でも書き戻される。これを避けるため、取り込み時に `layout` を落とす (`const { layout: _legacy, ...rest } = persisted` の形で 1 度読み捨てる)。`PersistedState` の型は `tweaks?: Tweaks` のままで、旧データの読み込みは docs/issues/closed/0020 の `migrateSessions` と同じく冪等な読み捨てにする。
  - 却下案: `layout` を残して将来の切り替え (タブの位置など) に備える。刷新案 B〜E のどれも `tabs-top` 以外の値を使わず、YAGNI で消す。
- `--shadow-drawer` と `--sp-5` は `tokens.css` から削除する。`[data-theme='light']` の `--accent-hi` も削除し、`[data-accent]` が `--accent-hi` を持つことをコメントで書く。
- `index.html` の Google Fonts の URL から `family=Instrument+Serif` を外す。`Geist` と `Geist Mono` は残す。
- 削除の検証は grep で行う: `layout` (`Tweaks` に関するもの)、`shadow-drawer`、`sp-5`、`Instrument` が `frontend/` (node_modules を除く) に無いこと。

## 完了条件

- `frontend/src` に `Layout` 型、`Tweaks.layout`、`DEFAULT_TWEAKS.layout` が無い。保存済みの `tweaks.layout` を持つ `cloudlens:v1` を読み込んでも、state に `layout` が現れず、次の保存で書き戻されないことを `useTweaks.test.tsx` で確認している。
- `tokens.css` に `--shadow-drawer`、`--sp-5`、`[data-theme='light']` の `--accent-hi` が無い。
- `index.html` の Google Fonts の URL に `Instrument+Serif` が無い。
- 開発サーバで Tweaks パネルの各切替 (Theme / Language / Detail panel / Accent) が動き、再読み込み後も残ることを目視する。
- `CHANGES.md` の `## develop` の `### misc` に変更が記載されている。
- `mise run check` が通過する。

## 関連

- docs/issues/0201〜0204 (先に実装する)。
- docs/issues/closed/0020: 永続化データの冪等な移行の前例。
- docs/issues/closed/0062 / 0064: Tweaks パネルの現在の項目。

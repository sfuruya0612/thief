# frontend の直書きの色とテーマ上書きをトークンに寄せ、既定テーマの対比が弱い組を直す

Created: 2026-09-28
Completed: 2026-09-28
Model: Claude Fable 5.1

## 背景

刷新案 A (docs/issues/0201 の背景を参照) の第 2 段。docs/issues/0201 で `app.css` を層に分けた後、色の値がトークン (`styles/tokens.css`) の外に残っている箇所と、テーマごとの部品の上書きを片付ける。

2026-09-28 時点 (コミット 3562f56) の `app.css` で、`:root` / `[data-theme='light']` / `[data-accent]` の外にある色の直書きは次の 30 か所である (行番号は分割前の `app.css`)。

- 16 進 21 か所: `.terminal-container` の `#0a0a0a` (1873)、`.toggle .tg::after` の `#fff` (2141)、`.qe-run` の `#fff` (2674)、ログビューアの Live Tail 切替 `.lv-live-toggle*` / `.lv-live-dot*` の `#dc2626` / `#16a34a` / `#15803d` (3334〜3344)、`.lv-run-btn` の `#16a34a` / `#fff` (3499〜3500)、ヒストグラムの `.lv-hbar-*` / `.lv-hseg-*` の `#9aa4b2` / `#e0972f` / `#dc2626` (3547〜3565)、行と重要度バッジの `.lv-row-msg.tint-*` / `.lv-sev-*` / `.lv-sev-badge.*` の `#dc2626` / `#b45309` (3762〜3871)。
- `rgba()` 9 か所: `.drawer-backdrop` の `rgba(15, 15, 20, 0.25)` (1534)、`.toggle .tg::after` の影 `rgba(0, 0, 0, 0.25)` (2144)、`.sso-banner` の `rgba(242, 201, 76, 0.12)` (2190)、`.error-banner` の `rgba(235, 87, 87, 0.12)` (2211)、`.lv-live-toggle.on` の `rgba(22, 163, 74, 0.12)` (3339)、`.lv-sev-badge.*` の `rgba(220, 38, 38, 0.15)` / `rgba(224, 151, 47, 0.18)` (3868〜3872)、Pricing の注意枠 `rgba(229, 165, 75, 0.12)` / `rgba(229, 165, 75, 0.3)` (4023〜4024)。

ログビューアの重要度の色は「両テーマ共通の固定色」とコメントで宣言されており、テーマに追随しないこと自体は意図である。しかし値が 6 つの規則に散っているため、1 つ変えるときに漏れる。

テーマごとの部品の上書き `[data-theme='dark'] .xxx { background: var(--bg-0) }` は 7 ブロックある (`.main` 942、`.stat` 1055、`table.dt thead th` 1219、`table.dt thead tr.dt-filter-row th` 1271、`table.dt.cost-cross-table` の固定列 1349 と 1365、`.lv-table thead th` 3732)。すべて「ライトでは `bg-1`、ダークでは `bg-0`」を部品ごとに書き直している (`.stat` はライトもダークも `bg-1` で、上書きの効果が無い)。本文の地の色という 1 つの意味が、7 か所に分かれている。

既定の Tweaks (`theme: light`、`accent: green`) で、文字と地の対比が WCAG 2 の 4.5:1 に届かない組が 4 つある (値は実測)。

| 組 | 対比 | 使われる場所 |
| --- | --- | --- |
| `--text-3` `#8b8b95` の文字 / `bg-1` `#ffffff` | 3.4:1 | th、`.section-label`、`.stat .label`、`.kv .k` など (11〜12px) |
| `--accent` (green `#12a66c`) の文字 / 白地 | 3.1:1 | `.facet.active` とその `.k` / `.v` |
| `--accent-text` `#fff` の文字 / `--accent` (green) の面 | 3.1:1 | `.btn.primary` |
| `--err` `#e5484d` の文字 / 白地 | 3.9:1 | `.error-banner`、`.qe-error`、`.session-card-error` など 16 か所 |

ダークテーマは `--text-3` 4.8:1、`--err` 5.2:1 と満たしている。また `[data-accent]` の 6 色のうち、白地で 4.5:1 を満たす文字色は indigo (4.7:1) だけである。

なお `[data-theme='light']` ブロックの `--accent-hi` (`#4f5abd`) は、常に適用される `[data-accent]` ブロックが `--accent-hi` を上書きするため使われていない。`[data-accent='indigo']` の `#7b86e2` が実効値になる。

## 目的

- 色の値が `styles/tokens.css` にだけある状態にする。feature の CSS は `var(--…)` だけを書く。
- 本文の地の色を 1 つのトークン `--surface-main` にし、`[data-theme='dark'] .xxx` の部品上書きを無くす。
- 既定テーマで 4.5:1 に届かない 4 組を、面の色を変えずに文字色側で直す。それ以外の見た目は変えない。

## 設計判断

- **両テーマ共通の固定色** を `tokens.css` の `:root` に「固定色 (両テーマ共通)」としてまとめる。意図 (テーマに追随しない) をコメントで残す。
  - `--terminal-bg: #0a0a0a` (xterm の背景)
  - `--sev-info: #9aa4b2`、`--sev-warn: #e0972f`、`--sev-err: #dc2626` (ログの重要度。バー、セグメント、Live Tail の off の点)
  - `--sev-warn-text: #b45309`、`--sev-err-text: #dc2626` (重要度の文字)
  - `--sev-warn-dim: rgba(224, 151, 47, 0.18)`、`--sev-err-dim: rgba(220, 38, 38, 0.15)` (重要度バッジの地)
  - `--live-on-text: #15803d`、`--live-on-dim: rgba(22, 163, 74, 0.12)` (Live Tail の on。面の緑は既存の `--qe-run` `#16a34a` と同じ値なので `var(--qe-run)` を使う。`.lv-run-btn` の `#16a34a` も同じ)
  - `--overlay: rgba(15, 15, 20, 0.25)` (Drawer の backdrop)
  - `--shadow-knob: 0 1px 3px rgba(0, 0, 0, 0.25)` (トグルのつまみの影)
  - `--banner-warn-bg: rgba(242, 201, 76, 0.12)` (`.sso-banner`)、`--err-dim: rgba(235, 87, 87, 0.12)` (`.error-banner`)、`--warn-dim: rgba(229, 165, 75, 0.12)`、`--warn-line: rgba(229, 165, 75, 0.3)` (Pricing の注意枠)
  - `#fff` の 3 か所 (トグルのつまみ、`.qe-run` と `.lv-run-btn` の文字) は既存の `--accent-text` (`#fff`、両テーマ同値) を使う。
  - 却下案: 固定色をテーマ追随の `--warn` / `--err` に置き換える。値が変わり (`#e0972f` → `#d9a514` など)、コメントが宣言している「固定」の意図に反するため採らない。値の変更は別の判断にする。
- **`--surface-main`** を `tokens.css` に足す (ライト `#ffffff`、ダーク `#15171c`。`bg-1` / `bg-0` の別名ではなく実値で書き、テーマの面の割り当てをここで読めるようにする)。`.main`、`table.dt thead th`、`table.dt thead tr.dt-filter-row th`、`table.dt.cost-cross-table` の固定列 (td と th)、`.lv-table thead th` の `background` を `var(--surface-main)` にし、7 つの `[data-theme='dark']` ブロックを削除する。`.stat` の上書きは効果が無いので削除だけする。
- **対比の修正** は文字色のトークンを足して行い、面の色は変えない。
  - `[data-theme='light']` の `--text-3` を `#8b8b95` → `#6f7079` にする (白地 4.9:1、`bg-0` 4.8:1)。`--muted` (状態の点) は `#8b8b95` のまま。
  - `--err-ink` を足す (ライト `#d2373d`、白地 4.8:1。ダークは `--err` と同じ `#eb5757`)。`color: var(--err)` の 16 か所を `var(--err-ink)` にする。点や枠 (`background` / `border-color`) の `--err` は変えない。
  - `--accent-ink` を足し、`[data-accent]` の 6 ブロックにライト用とダーク用の値を持たせる (`--accent-ink-light` / `--accent-ink-dark` を各 accent が定義し、`:root` が `--accent-ink: var(--accent-ink-dark)`、`[data-theme='light']` が `var(--accent-ink-light)` を選ぶ。3 つとも `html` 要素の変数なので参照が解決する)。ライトの値は白地・`bg-0`・`bg-2` の上で 4.5:1 以上まで暗くしたもの: indigo `#5a66ca`、amber `#8b6a0d`、blue `#286ecc`、green `#0e7e52`、purple `#7d53dd`、pink `#b24a7d`。ダークの値は既存の `--accent-hi` と同じ。`color: var(--accent)` / `var(--accent-hi)` を文字に使う 4 か所 (`.facet.active` とその `.k` / `.v`、`.session-tabs-more.holds-active`、`.lv-tree-leaf.checked`) を `var(--accent-ink)` にする。ソート矢印 (`th.sorted .sort`) はアイコンなので変えない。
  - `--accent-strong` を足し (両テーマとも `--accent-ink-light` と同じ値)、`.btn.primary` の `background` / `border-color` を `var(--accent-strong)` にする。白文字との対比は 6 色すべてで 5.0:1 以上になる。hover は現行の `--accent-hi` のままにする。
  - 却下案: 面の色 (`--accent` 自体) を暗くする。選択行やアクティブな nav の薄い面 (`--accent-dim`) にも波及し、見た目が変わる範囲が広いため採らない。
  - 却下案: `.btn.primary` の文字を暗くする。indigo / blue / purple では暗い文字が 4.5:1 に届かず、accent ごとに文字色を切り替えることになるため採らない。
- `[data-theme='light']` の使われていない `--accent-hi` は削除せず、コメントで「`[data-accent]` が上書きする」と書く (削除は docs/issues/0205 の範囲)。
- 検証は、`styles/` 配下の `tokens.css` 以外に `#[0-9a-f]{3,8}` と `rgba(` / `rgb(` の直書きが無いことを grep で判定する。対比は WCAG 2 の相対輝度の式で計算した値を `解決方法` に記録する。

## 完了条件

- `frontend/src/styles/` のうち `tokens.css` 以外のファイルに、`#` で始まる 16 進の色と `rgb(` / `rgba(` の直書きが無い (grep で判定する)。
- `frontend/src/styles/` に `[data-theme='dark']` で始まるセレクタが `tokens.css` 以外に無い。
- `--surface-main` / `--err-ink` / `--accent-ink` / `--accent-strong` / 固定色のトークンが `tokens.css` にあり、それぞれの用途がコメントに書かれている。
- 既定テーマ (light + green) で、`--text-3` の文字 / `bg-1`、`--accent-ink` の文字 / `bg-1`、`--accent-text` の文字 / `--accent-strong`、`--err-ink` の文字 / `bg-1` の対比がいずれも 4.5:1 以上である。6 つの accent すべてで `--accent-ink` / 白地と `--accent-text` / `--accent-strong` が 4.5:1 以上である。ダークテーマの同じ組も 4.5:1 以上である。
- 上の 4 組以外の色の値は変わっていない (トークンへの置き換えは同じ値への置き換えである)。
- 開発サーバで EC2 一覧 (facet を選び、primary ボタンを表示)、Cloud Logging (Live Tail の切替とヒストグラム)、Pricing をライト / ダーク、accent を green と indigo で目視する。
- `CHANGES.md` の `## develop` に `[UPDATE]` として記載されている (既定テーマの文字色 3 か所と primary ボタンの面の色が僅かに変わるため)。
- `mise run check` が通過する。

## 関連

- docs/issues/0201 (先に実装する)。本 issue は分割後のファイルを対象にする。
- 刷新案 A の「対比が弱い 4 組」。
- `frontend/src/components/charts/chartTheme.ts` は `--text-2` 相当の値を直値で持つ (コメントに理由がある)。ECharts はカスタムプロパティを読めないため本 issue では扱わない。
- docs/issues/0205: 使われていない `[data-theme='light']` の `--accent-hi` の削除。

## 解決方法

- `styles/tokens.css` の `:root` に「固定色 (両テーマ共通)」の群 (`--terminal-bg`、`--overlay`、`--shadow-knob`、`--banner-warn-bg`、`--err-dim`、`--warn-dim`、`--warn-line`、`--sev-info` / `--sev-warn` / `--sev-err` / `--sev-warn-text` / `--sev-err-text` / `--sev-warn-dim` / `--sev-err-dim`、`--live-on-text` / `--live-on-dim`) を足し、`tokens.css` の外にあった直書き 30 か所 (16 進 21、`rgba()` 9) をすべて `var(--…)` に置き換えた。`#fff` 3 か所は `--accent-text`、Live Tail と `.lv-run-btn` の緑 `#16a34a` 3 か所は `--qe-run` を使った。設計判断のとおり値は変えていない。
- `--surface-main` (ライト `#ffffff`、ダーク `#15171c`) を足し、`.main`、`table.dt thead th`、`table.dt thead tr.dt-filter-row th`、`table.dt.cost-cross-table` の固定列 (td / th)、`.lv-table thead th` の `background` を置き換えて、`[data-theme='dark']` の部品上書き 7 ブロックを削除した (`.stat` の上書きは効果が無かったので削除のみ)。`tokens.css` 以外に `[data-theme='dark']` は残っていない。
- 対比の修正: `[data-theme='light']` の `--text-3` を `#6f7079` にした (白地 4.9:1、`bg-0` 4.8:1、`bg-2` 4.6:1。解決される参照は 79 か所で、`.cb:hover` の枠線 1 か所も同じ変数を使うため一緒に変わる)。`--err-ink` (ライト `#d2373d` 4.8:1、ダーク `#eb5757`) を足して `color: var(--err)` の 16 か所を置き換えた。`--accent-ink` を足し、`[data-accent]` の 6 ブロックに `--accent-ink-light` (indigo `#5a66ca` / amber `#8b6a0d` / blue `#286ecc` / green `#0e7e52` / purple `#7d53dd` / pink `#b24a7d`) と `--accent-ink-dark` (`--accent-hi` と同じ。indigo だけは `bg-2` の上で 4.3:1 だったため `#8590e6` に上げた) を持たせ、`.facet.active` (とその `.k` / `.v`)、`.session-tabs-more.holds-active`、`.lv-tree-leaf.checked` の文字を `var(--accent-ink)` にした。`--accent-strong` (両テーマとも `--accent-ink-light`) を足し、`.btn.primary` の `background` / `border-color` を置き換えた。`[data-accent]` が未設定の初回描画に備え、`--accent-ink` / `--accent-strong` には indigo の値をフォールバックとして書いた。
- 対比の実測 (WCAG 2 の相対輝度): ライトの `--text-3` 4.9 / 4.8 / 4.6:1 (bg-1 / bg-0 / bg-2)、`--err-ink` 4.8 / 4.7:1、`--accent-ink` は 6 色とも `bg-1` / `bg-0` / `bg-2` の上で 4.6:1 以上、`--accent-text` (白) / `--accent-strong` は 6 色とも 5.0:1 以上。ダークの `--text-3` 4.8 / 5.4:1、`--err-ink` 4.6 / 5.2:1、`--accent-ink` は 6 色とも `bg-1` / `bg-0` / `bg-2` の上で 4.6:1 以上。
- 検証: 分割前の `app.css` (コミット 5d248c6) と新しい `styles/` を、`:root` / `[data-theme]` / `[data-accent]` の変数を解決した上で規則ごとに突き合わせる一時的なスクリプトで比較した (ライト / ダーク × 6 accent)。差分は設計判断に書いた 4 組 (`--text-3` の参照 79 + `.cb:hover` の枠線、`--err` の文字 16、`.facet.active` 系 3 と `.session-tabs-more.holds-active` / `.lv-tree-leaf.checked` の文字、`.btn.primary` の面と枠) だけで、それ以外の解決後の値はすべて同じだった。ダークでは `.facet.active` 系と上の 2 か所の文字が `--accent` から `--accent-hi` 相当 (明るい側) に変わる (対比は上がる)。
- 完了条件の「開発サーバでの目視」はこの環境では未実施 (docs/issues/closed/0201 と同じ)。frontend の `npm run lint` (エラー 0、警告 9 は既存分)、`npm run test` (113 ファイル 1,234 テスト成功)、`npm run build` の通過を確認した。backend には変更が無い。

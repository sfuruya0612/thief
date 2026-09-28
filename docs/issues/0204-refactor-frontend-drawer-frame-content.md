# frontend の Drawer を配置 (DrawerFrame) と中身 (Drawer) に分け、ドックの高さの参照を 1 か所にする

Created: 2026-09-28
Model: Claude Fable 5.1

## 背景

刷新案 A (docs/issues/0201 の背景を参照) の第 4 段。`frontend/src/components/Drawer/Drawer.tsx` (416 行) は、次の 2 種類の責務を 1 つのコンポーネントで持っている。

1. **配置**: 右 / 下のどちらに出すか (`position`)、分割表示中にペインの中に収めるか (`contained`)、backdrop、ESC で閉じるか (`closeOnEscape`)、ドラッグでのリサイズとその永続化 (`cloudlens:drawerSize`)、永続化した寸法のクランプ。
2. **中身**: サービスごとのタブ (`DRAWER_TABS`)、見出し (サービス、プロファイル、リージョン、名前、状態、ID)、タブごとの本文 (`DrawerTags` / `DrawerECSTasks` など 20 個の部品への振り分け)。

配置の側は、docs/issues/closed/0003 (下配置が画面全体を覆う)、0174 (ドックの高さの分だけ閉じ位置を下げる)、0175 (分割中はペインの中に収める)、0176 (ドックのリサイズ) と、issue のたびに条件が増えてきた。CSS (`styles/features/drawer.css`) 側も同じで、`--terminal-dock-h` (常駐ターミナルドックの高さ) を `.drawer` の `bottom`、`.drawer.pos-bottom` の `bottom` と `transform` の 3 か所で足し算している。docs/issues/closed/0174 の reopen は、この 3 か所のうち 1 か所だけを直したことで起きた。

`Drawer.test.tsx` の 24 テストも、配置 (サイズのクランプ 4、ESC 3、contained 6、開閉クラス 3) と中身 (タブ構成 3、Tags 2、Behaviors 1、RDS 1、ECS Exec 1) が同じファイルにある。

## 目的

配置を `DrawerFrame`、中身を `Drawer` に分け、配置の条件を足すときに中身を読まなくて済むようにする。ドックの高さの参照を CSS の 1 か所にする。呼び出し側 (`views/AccountView.tsx`、`views/GcpView.tsx`) と見た目、挙動は変えない。

## 設計判断

- `components/Drawer/DrawerFrame.tsx` を新設する。Props は `open: boolean`、`position?: DrawerPos`、`contained?: boolean`、`closeOnEscape?: boolean`、`onClose: () => void`、`children: ReactNode`。backdrop、`.drawer` 要素 (クラス `pos-bottom` / `open` / `contained`)、リサイズハンドルと `startResize`、`cloudlens:drawerSize` の読み書きとクランプ、ESC のリスナーを `Drawer.tsx` からそのまま移す。`children` は `.drawer` の中 (リサイズハンドルの後) に描画する。
- `Drawer.tsx` の `Drawer` は公開の名前と Props (`DrawerProps`) を変えない。中で `<DrawerFrame open={!!resource} position={position} contained={contained} closeOnEscape={closeOnEscape} onClose={onClose}>` を描画し、`resource` があるときだけ見出し・タブ・本文を子として渡す。`DRAWER_TABS`、タブの state、`DrawerOverview` はそのまま残す。
  - 却下案: 中身を `DrawerContent` という別ファイルにして `Drawer` を合成だけにする。呼び出し側の名前を保つなら `Drawer` が合成を持つ必要があり、中身をさらに別ファイルへ出しても `Drawer.tsx` は 20 個の部品の import と振り分けを持ち続ける。ファイルを 1 つ増やす効果が無いので、`Drawer.tsx` = 中身、`DrawerFrame.tsx` = 配置の 2 ファイルにする。
  - 却下案: 配置の state を `App` へ持ち上げる。分割表示では 2 つの Drawer が別々の寸法を持ちうるため、配置は Drawer ごとに閉じたままにする。
- CSS は `.drawer` に `--drawer-lift: calc(var(--terminal-dock-h, 0px) + 8px)` を 1 度だけ定義し、`bottom: var(--drawer-lift)`、`.drawer.pos-bottom` の `transform: translateY(calc(100% + var(--drawer-lift) + 8px))` (= 現行の `100% + var(--terminal-dock-h, 0px) + 16px`) にする。`.drawer.pos-bottom` の `bottom` の再宣言は削除する (`.drawer` の宣言が効く)。`contained` の変種も同じ変数を使う。これで `--terminal-dock-h` を参照する規則は `.drawer` の 1 つになる。
  - `styles/styles.test.ts` の「下配置の Drawer の閉じ位置」のテストは、文字列の完全一致から「`.drawer` が `--drawer-lift` を `--terminal-dock-h` から定義し、`.drawer.pos-bottom` の `transform` が `--drawer-lift` を参照する」の 2 点に書き換える。固定したい不変条件 (ドックの高さの分だけ下がる) は同じ。
- `Drawer.test.tsx` の配置のテスト 16 件を `DrawerFrame.test.tsx` に移し、`DrawerFrame` を直接描画する形に書き換える (子には `<div>body</div>` のような目印を渡す)。中身のテスト 8 件は `Drawer.test.tsx` に残す。`Drawer` が `DrawerFrame` に props を渡していることを確認するテストを 1 件足す (`contained` と `position` が `.drawer` のクラスに現れることを `Drawer` 経由で確認する)。
- `components/Drawer/index.ts` は `Drawer` を再輸出したままにし、`DrawerFrame` は足さない (呼び出し側は無い)。

## 完了条件

- `components/Drawer/DrawerFrame.tsx` があり、backdrop / 位置クラス / リサイズ / 寸法の永続化とクランプ / ESC を持つ。`Drawer.tsx` にこれらのコードが残っていない。
- `Drawer` の公開 Props と、`AccountView.tsx` / `GcpView.tsx` の呼び出しが変わっていない。
- `styles/features/drawer.css` で `--terminal-dock-h` を参照する規則が `.drawer` の 1 つだけである (grep で判定する)。閉じた下配置の Drawer の位置が、ドックの有無・高さにかかわらず現行と同じである (`--drawer-lift` の式が現行の式と等価であることをコメントに書く)。
- `DrawerFrame.test.tsx` に配置のテスト 16 件があり、`Drawer.test.tsx` に中身のテスト 8 件と合成の確認 1 件がある。全部通る。
- `styles/styles.test.ts` が新しい不変条件を検証している。
- 開発サーバで、右配置 / 下配置、ターミナルドックの折りたたみ / 展開 / 高さ変更、分割表示の各組み合わせで Drawer の開閉と閉じ位置を目視する (docs/issues/closed/0174 / 0176 の手動確認と同じ観点)。
- `CHANGES.md` の `## develop` の `### misc` に変更が記載されている。
- `mise run check` が通過する。

## 関連

- docs/issues/0201〜0203 (先に実装する)。
- docs/issues/closed/0003、0174、0175、0176: 配置の条件の出所。
- docs/adr/0012 (常駐ドック)、docs/adr/0025 (分割表示)。本 issue は方針を変えない。

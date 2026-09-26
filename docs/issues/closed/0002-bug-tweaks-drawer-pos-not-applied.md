# Tweaks の Detail panel 切り替えがブラウザリロードまで反映されない

Created: 2026-07-16
Completed: 2026-07-16
Model: Claude Fable 5 claude-fable-5

## 症状

Tweaks パネルで Detail panel (Right / Bottom) を切り替えても、Drawer の表示位置が変わらない。ブラウザをリロードすると切り替え後の位置で表示される。

## 再現手順

1. `mise run frontend:run` でアプリを起動する
2. 任意のリソース行をクリックして Drawer を開く
3. TopBar から Tweaks パネルを開き、Detail panel を Right → Bottom (または逆) に切り替える
4. Drawer の位置が変わらない (リロードすると反映される)

## 原因

`frontend/src/hooks/useTweaks.ts` の `useTweaks` がコンポーネントローカルな `useState` で実装されており、呼び出し元ごとに独立した state を持つ。

- `App.tsx` の `useTweaks()` インスタンスが `tweaks.drawerPos` を props で AccountView / GcpView → Drawer へ渡している
- Tweaks パネルのボタンは `TweaksPanel.tsx` 内の別の `useTweaks()` インスタンスの `update({ drawerPos })` を呼ぶ
- 更新は TweaksPanel 側インスタンスの `useEffect` で localStorage に永続化されるが、App 側インスタンスへは伝わらないため再レンダリングが起きない。リロード後は App 側が localStorage から初期値を読み直すため反映される

theme / density / accent が即時反映されるのは、React state 経由ではなく `useEffect` が `document.documentElement` の `data-*` 属性へ直接書き込み、CSS が属性セレクタで反応するため。React 値として配られる `drawerPos` だけがインスタンス分断の影響を受ける。

## 同根の潜在バグ

`CostChart.tsx` も独立した `useTweaks()` を呼び、`tweaks.theme` を React 値として参照して ECharts の文字色を決めている。TopBar でテーマを切り替えても CostChart 側インスタンスは古い theme のままのため、チャートの文字色が再マウントまで更新されない。

## 解決方法

- `frontend/src/hooks/useTweaks.ts` をモジュールレベルの共有ストア + `useSyncExternalStore` に書き換えた。どこから `useTweaks()` を呼んでも同一 state を購読するため、TweaksPanel での `update({ drawerPos })` が App 側の props (AccountView / GcpView → Drawer) へ即時反映される。永続化と `data-*` 属性反映の副作用は現行どおり hook 内の `useEffect` で行う (冪等)。API (`{ tweaks, setTweaks, update }`) は不変のため呼び出し元の変更は不要。CostChart の theme 陳腐化 (同根の潜在バグ) も同時に解消した。
- `frontend/src/hooks/useTweaks.test.tsx` を新規追加し、複数インスタンス間の即時反映 (回帰テスト)、data-* 属性反映、localStorage 永続化と読み戻し、他フィールド保持マージを検証した。
- テスト基盤の修正: Node 22+ の experimental localStorage グローバル (--localstorage-file 未指定時は undefined) が jsdom の localStorage をシャドウし、テストで localStorage が使えなかったため、`frontend/src/setupTests.ts` に in-memory Storage 実装のフォールバックを追加した。
- `mise run check` 全通過を確認した。

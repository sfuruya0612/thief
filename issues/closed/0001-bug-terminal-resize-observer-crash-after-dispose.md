# Terminal タブで xterm.js 内部の setTimeout が dispose 後に発火しクラッシュする

Created: 2026-07-09
Completed: 2026-07-09
Model: Claude Sonnet 4.6

## 再現手順

1. `mise run frontend:run` (Vite dev server, React 18 `StrictMode` 有効) で frontend を起動する。
2. `mise run backend:run` で backend を起動する。
3. EC2 一覧から実行中のインスタンスを選び、Drawer の Terminal タブを開く。

## 現象

- ブラウザ Console に `WebSocket connection to '...' failed: WebSocket is closed before the connection is established.` という warning が出る (StrictMode の effect 二重実行による無害な警告で、2 回目の接続は正常に確立される)。
- それに加えて `Uncaught TypeError: Cannot read properties of undefined (reading 'dimensions')` が発生する。Playwright で採取したスタックは以下の通り:
  ```
  TypeError: Cannot read properties of undefined (reading 'dimensions')
      at get dimensions (.../@xterm_xterm.js:1885:41)
      at t2.Viewport.syncScrollArea (.../@xterm_xterm.js:831:70)
      at .../@xterm_xterm.js:808:1498
  ```

## 原因

`@xterm/xterm` v5.5.0 内部の `Viewport` クラスは、コンストラクタで `requestAnimationFrame` と `setTimeout` の両方から `this.syncScrollArea()` を呼ぶコールバックを無条件に登録している。この登録は xterm.js のプライベート実装であり、公開 API からキャンセルする手段がない。

`frontend/src/components/Terminal/Terminal.tsx` の `useEffect` クリーンアップは `term.dispose()` を同期的に呼んでおり、これは内部の `_renderService` を即座に破棄する。StrictMode による effect の「マウント→即クリーンアップ→再マウント」のように、`Viewport` 生成から `dispose()` までの間隔が極めて短いタイミングでは、上記の未キャンセル `setTimeout`/`requestAnimationFrame` が `dispose()` 後に発火する。発火時に `Viewport.syncScrollArea()` → `RenderService.get dimensions()` が `this._renderer.value.dimensions` を読み取るが、`_renderer.value` は `dispose()` によって既に `undefined` になっているためクラッシュする。

なお、初期調査では `ResizeObserver` の初回通知が `disconnect()` 後に配送されるケースを疑い、`ResizeObserver` コールバックのみに dispose 済みガードを追加したが、再現テストではクラッシュが解消しなかった。実際の発火源は `ResizeObserver` ではなく xterm.js 内部の `Viewport` タイマーであることを Playwright のスタックトレース採取で確認した。

## 影響

- 開発時 (Vite dev server + StrictMode) のみ再現する。`docker compose` の本番ビルド (StrictMode の effect 二重実行が発生しない) では再現しない。
- 未捕捉の例外としてクラッシュするが、xterm 自体は再マウント後の 2 回目のインスタンスで正常に動作するため、ユーザー操作としては (エラーが Console に出る以外は) 支障が出にくい。ただし ErrorBoundary 等を導入した場合に画面全体がクラッシュしうる。

## 解決方法

`frontend/src/components/Terminal/Terminal.tsx` を以下の 2 点で修正した。

1. `disposed` フラグを、effect 内で登録するすべてのコールバック (`ws.onopen` / `ws.onmessage` / `ws.onerror` / `ws.onclose` / `sendResize()` / `term.onData` / `ResizeObserver` コールバック) の先頭でチェックするように拡張し、クリーンアップ後に配送された遅延コールバックが dispose 済みの `term` や `ws` に触れないようにした。
2. クリーンアップ内の `term.dispose()` を同期呼び出しから `setTimeout(() => term.dispose())` に変更し、次のマクロタスクへ遅延させた。これにより、xterm.js が内部で登録した未キャンセルの `setTimeout`/`requestAnimationFrame` (`Viewport.syncScrollArea`) が先に (まだ生きている内部状態に対して無害に) 発火してから `dispose()` が実行されるようになり、破棄後の状態にアクセスすることがなくなった。

Playwright で Vite dev server (StrictMode 有効) 上で Terminal タブの開閉を 3 回連続実行し、すべて JS エラー 0 件を確認した。`docker compose` の本番ビルドでも同様に開閉・Escape の連続操作でエラー 0 件を確認済み。

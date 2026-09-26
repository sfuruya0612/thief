# Bottom Drawer がウィンドウサイズ次第で画面全体を覆い操作不能になる

Created: 2026-07-16
Completed: 2026-07-16
Model: Claude Fable 5 claude-fable-5

## 症状

Tweaks の Detail panel を Bottom にすると、ブラウザのウィンドウサイズによっては Drawer が下から画面いっぱいに開く。上部のテーブル・ツールバーが覆われるだけでなく、Drawer 自身のヘッダー (閉じるボタン・タブ) も画面上端の外に押し出され、backdrop も全面覆われるため、操作不能になる。

## 再現手順

1. `mise run frontend:run` でアプリを起動し、Detail panel を Bottom にする
2. 高さの大きいウィンドウで Drawer を開き、リサイズハンドルで上端近くまでドラッグして高さを最大化する (px 絶対値が localStorage `cloudlens:drawerSize` に保存される)
3. ブラウザウィンドウの高さを大幅に縮める (または保存値より低い解像度の画面で開く)
4. 任意のリソース行をクリックして Drawer を開くと、Drawer が画面全体を覆い、閉じる手段がなくなる

## 原因

`frontend/src/components/Drawer/Drawer.tsx` の `sizeStyle` (153-160 行) に非対称なクランプ漏れがある。

- ドラッグリサイズした高さ/幅は px 絶対値で localStorage に永続化される。ドラッグ中は `[220, window.innerHeight * 0.85]` にクランプされるが、これはその時点のウィンドウ高さ基準の値
- right (幅) 側は描画時に `Math.min(size.width, window.innerWidth * 0.85)` で再クランプしているのに、bottom (高さ) 側は `{ height: size.height }` と生の永続値をそのまま適用している
- インライン style が CSS 側の安全なデフォルト `height: min(46vh, 520px)` (`.drawer.pos-bottom`) を上書きする
- Drawer は `position: fixed; bottom: 8px` で上方向に伸びるため、保存値が現在のウィンドウ高さを超えるとヘッダーが画面外に出る。ESC ハンドラも存在しないため復旧手段がない

## 解決方法

- `frontend/src/components/Drawer/Drawer.tsx`: `sizeStyle` の bottom 分岐を width 側と対称にし、永続化された height を `Math.min(size.height, window.innerHeight * 0.85)` で現在のウィンドウ高さに再クランプするようにした (0.85 はドラッグ時クランプと同じ係数)。
- `frontend/src/app.css`: インラインの px 指定が描画後のウィンドウ縮小を追い越さないよう、`.drawer` に `max-width: 85vw`、`.drawer.pos-bottom` に `max-height: 85vh` の CSS ガードレールを追加した (pos-bottom は `max-width: none` で解除)。ウィンドウリサイズにもライブに追従する。
- `frontend/src/components/Drawer/Drawer.tsx`: open 中に Escape キーで `onClose` を呼ぶ keydown ハンドラを追加し、レイアウト崩れ時の復旧手段を確保した。
- `frontend/src/components/Drawer/Drawer.test.tsx` を新規追加し、height/width の描画時クランプ (回帰テスト)、永続値なし時の inline 未設定、Escape での close 動作を検証した。
- `mise run check` 全通過を確認した。

# 0025. 分割表示は 2 ペインまでとし、frontend だけで実装する

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-09-25

## 状況

EC2 や ECS のシェルにつないだまま、Parameter Store や Secrets Manager の値を見たいという要望があった (issue 0175)。
1 つのサービスしか表示できない画面では、行き来するたびに選択と絞り込みが失われる。

## 決定

- AWS と Google Cloud のビューで、2 つのペインを左右に並べられるようにする。
- プロファイルとリージョンはペインの間で共有し、サービス、選択中の行、絞り込み、Drawer のタブはペインごとに持つ。
- 状態は純関数 (`frontend/src/lib/splitPanes.ts`) と `useReducer` (`frontend/src/hooks/useSplitPanes.ts`) で持ち、永続化しない。
- 分割中の Drawer はペインの中に収める (`Drawer` の `contained` と `closeOnEscape`)。
- (2026-09-29 追記、issue 0213) Tweaks の Layout = Workbench では Drawer をオーバーレイではなく docked (表の右または下に列として並ぶ、`DrawerFrame` の `mode='docked'`) にする。docked はペインの `.main-row` の中で流れの中にあるため、分割中も `contained` の位置計算を使わずにペインの中に収まる (`contained` は Layout = Standard のオーバーレイだけが使う)。分岐は `DrawerFrame` の `mode` の 1 か所で、Standard の振る舞いは変えない。
- backend は変えない。

## 検討した代替案

issue 0175 は次の案を採らなかった。

- 同じサービスを両方のペインに表示する。
  Athena と BigQuery の `localStorage` のキーが衝突するため。
- 3 ペイン以上。
  要望に無いため (YAGNI)。
- Drawer の位置の設定 (`DrawerPos`) に分割用の値を足す。
  好みの設定と分割は別の軸であるため。
- Drawer を画面に固定したまま重ねる。
  背景がクリックを奪うため。Workbench の docked (issue 0213) は背景 (backdrop) を持たず表と同時に操作できるので、この理由は当たらない。

## 結果

- 分割の状態は再読み込みで失われる。
- 同じサービスを 2 つのペインで開けない。

## 根拠資料

- `docs/issues/closed/0174`、`0175`、`0213` (Workbench の docked)
- `frontend/src/lib/splitPanes.ts`、`frontend/src/views/AccountView.tsx`、`frontend/src/views/GcpView.tsx`

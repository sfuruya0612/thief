# Cost Explorer の絞り込み state をリージョン切り替えで初期化する

Created: 2026-08-05
Model: Claude Fable 5
Completed: 2026-08-07

## 背景

TODO.md の「Cost Explorer の絞り込み state を profile/region の切り替えで初期化したい (issue 0094 のスコープ外として送り)」に対応する。

`frontend/src/views/CostExplorerPanel.tsx` の `CostExplorerPanel` は、granularity / groupBy / 日付レンジ / metric / サービス名とアカウント ID の入力値と確定値の 9 つの `useState` を持つ (103-116 行)。
`frontend/src/views/AccountView.tsx` の 471 行はこれを `<CostExplorerPanel profile={profile} region={region} />` と key 無しでレンダリングするため、リージョンを切り替えてもこれらの state は保持される。

TODO はプロファイル切り替えも対象に挙げるが、プロファイル側は既に初期化される。
`frontend/src/App.tsx` の 135-136 行が `<AccountView key={activeProfile} ...>` としており、プロファイル切り替えで `AccountView` ごと再マウントされるため、`CostExplorerPanel` の state もこの時点で破棄される。
残る差分はリージョン切り替えだけである。

なお Cost Explorer のデータ自体はリージョンに依存しない。
`backend/internal/aws/cost.go` の `GetCost` (162 行) は受け取った region 引数を使わず、Cost Explorer クライアントを us-east-1 固定で生成し (164 行)、返るコストはアカウント全体の値である。
したがって本 issue はデータの正誤の修正ではなく、「リージョンを切り替えたのに前の絞り込みが残る」という TODO の要望に応える表示状態の初期化である。
前プロファイルの絞り込み値が残って「コストが 0」に見えるという TODO 記載の誤解の筋は、プロファイル側の再マウントで既に防がれている。

他の AWS サービスパネル (`AccountView.tsx` の `ServicePanel`) もリージョン切り替えで state を初期化していない。
`ServicePanel` 内部の `filters` (113 行) も、`AccountView` 本体が持ち `ServicePanel` へ props で渡す `selectedId` (201 行) も、key 無しの `useState` のためリージョンを切り替えても保持される。
本 issue の変更は「他パネルとの一貫性」を根拠にしない (設計判断を参照)。

## 目的

リージョン切り替え時に Cost Explorer の絞り込み state (granularity / groupBy / 日付レンジ / metric / サービスとアカウントのフィルタ) が初期値に戻る。

## 設計判断

- `AccountView.tsx` の 471 行を `<CostExplorerPanel key={region} profile={profile} region={region} />` にし、React の再マウントで state を破棄する。9 つの `useState` に個別のリセット処理 (useEffect 等) を書く案は、リセット漏れの余地を残す上、state を追加するたびに配線が要るため採らない。再マウントによるリセットは `App.tsx` の `key={activeProfile}` が既に採っている手法で、リポジトリ内の先例に揃う。
- `AccountView` 自体の key に region を足す案は採らない。全サービスパネルが再マウントされ、Cost Explorer 以外のパネルの状態 (Drawer の開閉等) まで破棄されてスコープを超える。
- key は region のみとする。profile は `App.tsx` の再マウントで既に担保されており、key に重ねて入れても挙動が変わらない。
- 他のサービスパネル (`ServicePanel`) にも `key={region}` を付けてリージョン切り替えで一斉に初期化する案は採らない。TODO は Cost Explorer を名指ししており、他パネルの初期化には要望実績が無い。他パネルのリージョン切り替え時の state 初期化はスコープ外とし、必要になった時点で個別に起票する。
- 追加の API 呼び出しや権限は不要。

## 完了条件

- リージョンを切り替えると `CostExplorerPanel` の 9 つの state が全て初期値に戻ることをコンポーネントテストで検証する。テストは `key={region}` を適用した親要素経由で region を変更し、再マウントの経路を通すこと (props の変更だけでは `useState` は初期化されないため、`CostExplorerPanel` 単体への props 差し替えでは検証にならない)。
- 変更箇所が `AccountView.tsx` の `CostExplorerPanel` をレンダリングする行への `key={region}` の追加に限られることを diff で確認する。`App.tsx` と `ServicePanel` には変更を加えない。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0094: 本 issue の起票元。サービスとアカウントの絞り込み追加時に、切り替え時の state 保持がスコープ外として送られた。

## 解決方法

- `frontend/src/views/AccountView.tsx` の `CostExplorerPanel` をレンダリングする行に `key={region}` を追加し、リージョン切り替え時に React の再マウントで 9 つの絞り込み state を初期値へ戻すようにした。再マウントの意図を説明するコメントを同じ箇所に添えた。
- `frontend/src/views/CostExplorerPanel.test.tsx` に、AccountView をレンダリングしてリージョンを切り替えると 9 つの state (granularity / groupBy / 日付レンジ / metric / サービスとアカウントの入力値と確定値) が全て初期値に戻ることを検証するコンポーネントテストを追加した。入力値と選択値はフォーム要素の値で、UI に表示されない確定値は getCost の呼び出し引数で検証する。AccountView 側の `key={region}` を外すとこのテストが落ちることを確認した。
- `App.tsx` と `ServicePanel` には変更を加えていない。
- `mise run check` が通ることを確認した。

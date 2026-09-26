# EC2 / ECS のターミナルを Drawer から独立した常駐パネルへ移し、画面遷移をまたいで接続を維持する

Created: 2026-09-15
Model: Claude Fable 5.1
Completed: 2026-09-16

## 背景

TODO.md の次の項目に由来する。

> AWS, Google Cloud で特定のサービスを開いた状態で、別のサービスの状態を画面をセパレートするなどして同時に 2 つ以上のサービスを参照できるようにしたい
> - 例えば、AWS EC2 や ECS の Session Manager を繋いだ状態で Parameter Store や Secret Manager の値を確認したい時に、一度ページ遷移を挟む必要があり、session の繋ぎ直しが都度必要
> - EC2, ECS の Session Manager を開いた状態を別画面に遷移しても維持できればそれだけでもいいかも

本 issue は子項目の「Session Manager を開いた状態を別画面に遷移しても維持できれば」を扱う。
親項目の「画面をセパレートして 2 つ以上のサービスを同時に参照する」分割表示は docs/issues/pending/0175 で扱う。

### ターミナルのマウント位置

EC2 の Session Manager と ECS Exec のブラウザ側ターミナルは `frontend/src/components/Terminal/Terminal.tsx` の `Terminal` であり、`useEffect` の中で xterm.js のインスタンスと WebSocket を生成する。
`Terminal` は `components/Drawer/DrawerTerminal.tsx` の `DrawerTerminal` を経由して、`components/Drawer/Drawer.tsx` の `Drawer` が `tab === 'Terminal'` のときだけ描画する。
`Drawer` は `views/AccountView.tsx` の `ServicePanel` の子であり、`ServicePanel` は `AccountView` が `activeService` ごとの条件レンダリングで 1 つだけ描画する。
`AccountView` 自体は `App.tsx` の `App` が `view === 'aws'` のときだけ、`key={activeProfile}` を付けて描画する。

このマウント位置のため、次の操作のいずれでも `Terminal` はアンマウントされる。

- トップバーでのビュー切替 (`App.tsx` の `view` state。AWS 以外を選ぶと `AccountView` が描画されなくなる)
- プロファイルタブの切替 (`key={activeProfile}` により `AccountView` が再マウントされる)
- サイドバーでのサービス切替 (`AccountView` の条件レンダリングで前サービスの `ServicePanel` が描画されなくなる。加えて `activeService` の変化で `selectedId` を `null` に戻す `useEffect` がある)
- 同じサービス内で別の行を選択する (`Drawer` の `useEffect` が `resource?.id` の変化で `tab` を `'Overview'` に戻す)
- Drawer を閉じる (`resource` が `null` になり `Drawer` の本文が描画されなくなる)
- Drawer の Terminal 以外のタブへ切り替える

### アンマウントが AWS 側のセッションを終了させる経路

`Terminal` の `useEffect` のクリーンアップは `ws.close()` を呼ぶ。
backend では `backend/internal/api/handlers_session.go` の `handleEC2Session` と `handleECSExec` が `StartSSMSession` / `ExecuteECSCommand` (`backend/internal/aws/ssm_session.go`、`backend/internal/aws/ecs_exec.go`) でセッションを開始したあと、`runSessionBridge` で `session.Bridge` を起動する。
`backend/internal/session/bridge.go` の `Bridge.Run` は、ブラウザ側 WebSocket の正常クローズを `pumpBrowserToDataChannel` の正常終了として扱い、`cleanup` で `Terminate` (`TerminateSSMSession`) を必ず呼ぶ。
backend にはセッションのレジストリ、再接続、出力のバッファリングは無く、1 リクエスト = 1 ブリッジである。

つまり、TODO の「session の繋ぎ直しが都度必要」という現象は backend の不具合ではない。
フロントの UI 遷移が `Terminal` を不要にアンマウントし、その結果として backend が設計どおりにセッションを終了させている。
Parameter Store (`ssm`) や Secrets Manager (`secrets`) の値を Drawer の Value タブで確認するにはサイドバーでサービスを切り替える必要があり、上記のサービス切替に該当する。

### 既存の類似実装

- `frontend/src/hooks/useTweaks.ts`: モジュールレベルの共有ストアを `useSyncExternalStore` で購読するフック。どのコンポーネントから呼んでも同じ状態を参照できる。
- `frontend/src/lib/sessionTabsState.ts`: `{ open: string[], active: string }` 形の状態に対する純関数 (`openSession`、`closeSession`、`activateSession`、`moveSession`)。`closeSession` はアクティブなタブを閉じたときの次のアクティブの選び方を含む。
- `frontend/src/App.tsx` の `usePersistedSidebarWidth`: レイアウト寸法を CSS 変数 (`--sidebar-w`) として `document.documentElement` に反映する実装。

## 目的

EC2 / ECS のターミナルを一度開けば、ビュー、プロファイル、サービス、リソース選択、Drawer の開閉のどれを変えても接続が維持され、同じ画面上で別サービス (Parameter Store、Secrets Manager など) の内容を参照しながらシェルを操作できる。

## 設計判断

### 1. フロントだけで解決し、backend は変更しない

接続が切れる原因はフロントのマウント位置にあるため、`Terminal` を `view` / `activeProfile` / `activeService` / `selectedId` のどれにも依存しない位置 (`App` 直下) にマウントし続ける。
backend のブリッジ (`runSessionBridge`、`Bridge.Run`) と 2 つの WebSocket エンドポイント (`backend/internal/api/routes.go`) は変更しない。
追加の AWS API 呼び出しや IAM 権限は不要である。

却下した案: backend にセッションレジストリを設け、ブラウザ切断後も SSM セッションを保持して再接続できるようにする。
同一ページ内の遷移では WebSocket を閉じなければ接続が保てるため、レジストリは要らない。
レジストリを設けると、閲覧者のいない SSM セッションを保持し続けるため、孤立セッションの TTL 管理と出力バッファの上限管理が新たに必要になる。
この案で得られるページのリロードをまたいだ維持は、TODO で求められていない。

### 2. 常駐ターミナルパネル (以下、ターミナルドック) を `App` 直下に置く

`frontend/src/components/Terminal/TerminalDock.tsx` (新規) を `App.tsx` の `.app` 要素の末尾の子として、`view` の値に関係なく描画する。
`.app` は `height: 100vh` の flex column で、`.app > .body` が `flex: 1` を持つため (`frontend/src/app.css`)、ドックは本文の下に固定高さで並び、本文がその分縮む。
ドックは開いているセッションが 1 つ以上あるときだけ描画し、0 のときは描画しない。

ドックは次の構成とする。

- 上部にタブバーを置き、セッションごとに 1 タブ (ラベル、接続状態、閉じるボタン) を並べる。タブバーの右端にドックの折りたたみ / 展開ボタンを置く。
- 本文 (`div.terminal-dock-body`) には開いている全セッションの `Terminal` を、セッションごとのラッパー (`div.terminal-dock-pane`) に包んで同時にマウントし、アクティブでないセッションのラッパーに `hidden` 属性を付けて非表示にする。アンマウントはしない。
- 折りたたみは `div.terminal-dock-body` に `hidden` を付けるだけで、`Terminal` はアンマウントしない。
- `hidden` は `Terminal` が返す `.terminal-panel` には付けない。`app.css` の `.terminal-panel` は `display: flex` を宣言しており、CSS のカスケードでは author 由来の宣言が UA スタイルシートの `[hidden] { display: none }` に詳細度と無関係に勝つため、`.terminal-panel` に `hidden` を付けても非表示にならない。2 つのラッパーには `display` を宣言せず、あわせて `app.css` に `.terminal-dock-body[hidden], .terminal-dock-pane[hidden] { display: none; }` を明示して、ラッパーに後から `display` が付いても非表示が壊れないようにする。
- jsdom 29.1.1 の `getComputedStyle` は、author の `display: flex` を持つ要素に `hidden` を付けた場合でも `display` を `none` と返す (起票時に `frontend/node_modules/jsdom` で確認)。ブラウザと結果が異なるため、テストでは表示状態を `getComputedStyle` で判定せず、`hidden` 属性の有無と `app.css` の規則の存在を確認し、実際の表示は手動確認で見る。
- セッションを新しく開いたときは、そのセッションをアクティブにし、ドックが折りたたまれていれば展開する。
- タブの閉じるボタンで、そのセッションの `Terminal` だけをアンマウントする。既存のクリーンアップ (`ws.close()`) がそのまま backend の `TerminateSSMSession` を呼ぶ。これがセッションを終了させる唯一の UI 操作になる。
- アクティブなタブを閉じたときの次のアクティブは `sessionTabsState.ts` の `closeSession` に従う。
- ドックの高さは固定とし、タブバーの高さ 32px と本文 (`div.terminal-dock-body`) の高さ 320px を `TerminalDock.tsx` の定数として持つ。本文の 320px は `app.css` の `.terminal-panel` の `min-height: 320px` と同じ値にする。`.terminal-panel` は `height: 100%` と `min-height: 320px` を持つため、本文がこれより低いとパネルが本文からあふれる。ルート要素の高さは、展開時が両者の和の 352px、折りたたみ時がタブバーだけの 32px であり、ルート要素とタブバーの `style.height` と設計判断 6 の CSS 変数の値はこの 2 つの定数から導く。ルート要素は `display: flex; flex-direction: column` とし、タブバーは `flex: none`、`div.terminal-dock-body` は `flex: 1; min-height: 0`、`div.terminal-dock-pane` は `height: 100%` として、`.terminal-panel` の `height: 100%` が本文の高さ 320px に解決されるようにする。`.terminal-panel` の既存規則は変更しない。

TODO の子項目は 1 セッションの維持だけを求めているが、次の 2 点は TODO の場面を成立させるために本 issue に含める。

- 複数セッションのタブ: ターミナルを Drawer から切り離すと、1 つ目のセッションを開いたまま別のインスタンスやタスクの Terminal タブから Connect を押せるようになる。ドックを 1 セッションに限ると、2 つ目の Connect で 1 つ目を終了させる (UI 操作がセッションを切るという TODO の問題を別の形で再現する) か、Connect を拒否するかのどちらかになる。タブの状態管理は `sessionTabsState.ts` の純関数で済むため、タブ UI の追加分は小さい。
- 折りたたみ: ドックは展開時に `.app > .body` の高さを 352px 分奪う。TODO の場面 (シェルを繋いだまま Parameter Store / Secrets Manager の一覧と Value タブを見る) では本文の面積が要り、折りたたみが無ければ面積を取り戻す手段がセッションを閉じることだけになる。実装は本文への `hidden` の切替 1 つである。

ドックの高さをドラッグで変える機能は、TODO の場面に必要なく、上の 2 点とも結合しないため本 issue に含めない (設計判断 8)。

本 issue を「状態管理と 1 セッションの維持」「タブ UI」「Drawer のランチャー化」に分けない。`Terminal` をドックへ移すと `DrawerTerminal` は `Terminal` を描画できなくなる (同じセッションのマウント位置が 2 つになる) ため、ランチャー化はドックの導入と同じ変更である。タブ UI は上記のとおり 1 セッション制限の代わりに要り、先行 issue だけを close した状態では 2 つ目の Connect の挙動が未定義になる。

却下した案: タブバー UI に `components/session/SessionTabs.tsx` の `SessionTabs` をそのまま使う。
`SessionTabs` は `window` に Ctrl+1 から 9 の `keydown` リスナーを登録してタブを切り替える。プロファイルタブの `AwsSessionTabs` が既に同じリスナーを登録しているため、ドックにも `SessionTabs` を置くと 1 回のキー入力でプロファイルタブとドックのタブが同時に切り替わる。このリスナーは `.xterm` にフォーカスがあるときは何もしないため、ターミナル入力中の誤発火は起きないが、二重登録の問題は残る。
また `SessionTabs` は追加ピッカー (`picker`、`addLabel`)、並べ替え (`onReorder`)、オーバーフロー時の入れ替え (`onSwapToVisible`) を必須 props として要求する。ドックのセッションは Drawer 側から開くため追加ピッカーが無く、これらにダミーを渡すことになる。
純関数 (`sessionTabsState.ts`) だけを再利用し、タブバー UI はドック専用に書く。

### 3. セッションの状態はモジュールレベルの共有ストアで持つ

`frontend/src/hooks/useTerminalSessions.ts` (新規) に、`useTweaks.ts` と同じ `useSyncExternalStore` ベースの共有ストアを実装する。

- セッション情報の型は `TerminalSession = { id: string; kind: 'ec2' | 'ecs'; profile: string; region: string; label: string; wsUrl: string }` とする。`wsUrl` は既存の `api/terminal.ts` の `ec2SessionUrl` / `ecsExecUrl` で開く時点に組み立てる。
- ストアの状態は `{ tabs: SessionTabsState; sessions: Record<string, TerminalSession>; collapsed: boolean }` とし (`SessionTabsState` は `sessionTabsState.ts` の `{ open, active }`)、`useTerminalSessions()` フックで購読する。
- `id` はストア内の連番から生成する。同じ対象 (同じインスタンス、または同じタスクとコンテナ) へ Connect を繰り返すと、そのたびに新しいセッションが増える。
- `label` は EC2 では `resource.name` (空なら `resource.id`)、ECS では `cluster / タスク id の末尾 / コンテナ名` とする。
- 操作は React の外からも呼べるモジュール関数として次のシグネチャで公開し、`Drawer` 側のイベントハンドラとテストから直接呼べるようにする。
  - `openTerminalSession(input: Omit<TerminalSession, 'id'>): string` は id を採番してセッションを追加し、それをアクティブにし、`collapsed` を `false` にして、採番した id を返す。
  - `closeTerminalSession(id: string): void` はセッションを削除し、次のアクティブを `sessionTabsState.ts` の `closeSession` で選ぶ。
  - `activateTerminalSession(id: string): void` は `tabs.active` を変える。
  - `setTerminalDockCollapsed(collapsed: boolean): void` は `collapsed` を変える。
  - `resetTerminalSessionsForTest(): void` は `useTweaks.ts` の `resetTweaksForTest` と同様に状態を初期値へ戻す。
- 状態は `localStorage` に永続化しない。

却下した案 1: `App` の `useState` で持ち、`AccountView` と `ServicePanel` (15 分岐) を経由して `Drawer` へ props で配る。
`ServicePanel` の全分岐と `Drawer` の props を書き換える必要があり、変更が広がる。
`useTweaks.ts` に同じ課題を共有ストアで解決した前例があるため、それに合わせる。

却下した案 2: 開いているセッションを `lib/storage.ts` の `PersistedState` に永続化し、リロード後に復元する。
SSM セッションはブラウザの WebSocket が閉じた時点で backend が終了させるため、復元は同じ対象へ新しいセッションを開き直す操作になり、利用者の操作なしに AWS API を呼んでセッションを開始することになる。
リロードをまたいだ維持は TODO で求められていない。

却下した案 3: 同じ対象への Connect は既存のセッションをアクティブにするだけにし、対象ごとに 1 セッションに制限する。
1 つのインスタンスでログを追いながら別のシェルでコマンドを実行する使い方があるため、UI 側では制限を設けない。
backend は 1 リクエストごとに `StartSSMSession` / `ExecuteECSCommand` を呼び、同じ対象への既存セッションを確認しない。
AWS 側が同じ対象への追加セッションを拒否した場合は、これらの呼び出しが失敗し、`handleEC2Session` / `handleECSExec` が WebSocket へのアップグレード前に `writeAWSError` で HTTP エラーを返す。ブラウザ側では WebSocket の接続に失敗し、`Terminal` の `ws.onerror` が `status` を `'error'` にして、そのタブに接続エラーが表示される。利用者はそのタブを閉じるボタンで個別に閉じられる。
AWS 側の上限について本リポジトリに一次情報は無い。AWS の公開ドキュメントで確認できる内容は EC2 と ECS で異なる。Session Manager のサービスクォータ (https://docs.aws.amazon.com/general/latest/gr/ssm.html) には管理ノードごとの同時セッション数の項目が無く、同じインスタンスへの複数セッションを禁じる記載は見つからなかった。ECS Exec の考慮事項 (https://docs.aws.amazon.com/AmazonECS/latest/developerguide/ecs-exec.html) は PID 名前空間ごとに ECS Exec セッションは 1 つだけと記載し、タスク定義パラメータ (https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task_definition_parameters.html) の `pidMode` は未指定時の既定をコンテナごとの private な名前空間としている。このため `pidMode` を指定しない既定のタスクでは、同じコンテナへの 2 つ目の ECS Exec セッションは AWS 側で拒否されると読める。拒否がどの段階で起きるか (`ExecuteCommand` API の失敗か、セッション開始後のデータチャネルの切断か) は記載が無い。前者なら上記の `writeAWSError` の経路でそのタブに接続エラーが表示される。後者なら backend のブリッジが終了し、`Terminal` は `Bridge.notifyExit` が送る `{"type":"exit"}` の受信または WebSocket のクローズ (`ws.onclose`) で `status` を `'closed'` にする。どちらの場合も利用者はそのタブを閉じるボタンで個別に閉じられ、他のタブには影響しない。本 issue は UI 側で対象ごとの制限を設けず、EC2 と ECS のどちらについてもこの可否に依存しない設計とする。

### 4. Drawer の Terminal タブは接続を開始するランチャーにする

`DrawerTerminal.tsx` から `Terminal` の描画を外し、ターミナルドックにセッションを開く操作だけを担わせる。

- EC2: Connect ボタンを置き、押下でドックに EC2 セッションを開く。
- ECS: 既存のタスク / コンテナのドロップダウン (`ECSExecTerminalDropdown`) を残し、タスクとコンテナが確定したら Connect ボタンを有効にする。押下でドックに ECS セッションを開く。現状の「確定と同時に接続する」挙動は、選択を変えるたびにセッションが開くのを避けるため、明示的なボタンに変える。
- ECS の Tasks タブ (`DrawerECSTasks` の `onExec`) は、Terminal タブへ切り替えずにドックへ直接セッションを開く。これに伴い `Drawer.tsx` の `pendingExecTarget` state と `DrawerTerminal` の `execTarget` prop は不要になるため削除する。
- Drawer の Open CLI ボタンは現状どおり Terminal タブへ切り替える。
- Drawer のタブ名 `Terminal` は英語ハードコードのまま変更しない (docs/issues/closed/0066 の方針)。

却下した案: Terminal タブを開いた瞬間に自動でセッションを開く。
タブを見るだけの操作の副作用として SSM セッション (AWS API 呼び出し) が発生し、タブを出入りするたびにセッションが増える。

### 5. Terminal コンポーネントの変更

`Terminal.tsx` には次の 2 点を加える。

- `ResizeObserver` のコールバックで、コンテナの `clientWidth` または `clientHeight` が 0 のときは `fitAddon.fit()` を呼ばない。`@xterm/addon-fit` 0.10.0 の `proposeDimensions` は、レンダラのセル寸法 (`dims.css.cell`) が 0 のときは早期 return し、そうでなければ親要素の `getComputedStyle` の `height` / `width` を `parseInt` し、列数と行数を `Math.max(2, ...)` と `Math.max(1, ...)` で下限に丸める (`frontend/node_modules/@xterm/addon-fit/src/FitAddon.ts`)。セル寸法は xterm の `CharSizeService` (`frontend/node_modules/@xterm/xterm/src/browser/services/CharSizeService.ts` の `_validateAndSet`) が計測値 0 のとき前回値を保持するため、一度表示された端末が非表示になってもセル寸法は 0 にならず、この早期 return は非表示の判定として働かない。非表示の要素で寸法が 0 と評価された場合、ターミナルが 2 列 1 行に縮み、`term.onResize` 経由で backend にその寸法が送られてリモートの端末が再レイアウトされる。`display: none` の要素では `getComputedStyle` の `height` が `auto` を返し `parseInt` が `NaN` になるため `fit()` は早期 return するが、この挙動は `.terminal-container` に高さ指定が無いことに依存する。CSS に依存しないガードを `Terminal.tsx` 側に置く。
- `active?: boolean` prop を追加し、`false` から `true` に変わったときに `fitAddon.fit()` と `term.focus()` を呼ぶ。タブの切替でアクティブになったセッションへ入力フォーカスを移すためであり、あわせて表示に戻った要素の寸法を `ResizeObserver` の発火を待たずに合わせる。

### 6. Drawer とドックの重なり

`Drawer` は `position: fixed` で配置され、`app.css` では `.drawer` が `bottom: 8px` を宣言し、下配置の `.drawer.pos-bottom` も `bottom: 8px` を再宣言している。`.drawer.pos-bottom` の方が詳細度が高いため、`.drawer` だけを変えても下配置には反映されない。既定の配置は `hooks/useTweaks.ts` の `DEFAULT_TWEAKS.drawerPos` の `'bottom'` である。
ドックのルート要素の高さ (展開時 `352px`、折りたたみ時 `32px`。設計判断 2 の定数から導く) を CSS 変数 `--terminal-dock-h` として `document.documentElement` に設定し (`usePersistedSidebarWidth` の `--sidebar-w` と同じ方法)、`.drawer` と `.drawer.pos-bottom` の両方の `bottom` を `calc(var(--terminal-dock-h, 0px) + 8px)` に変える。
これにより、右配置と下配置のどちらの Drawer もターミナルドックの上に収まり、ドックを覆わない。
ドックが描画されていないときは変数を `0px` にし、現状の配置を保つ。

### 7. i18n

ドックとランチャーの文言 (Connect ボタン、折りたたみ / 展開、閉じる、接続状態) は、`Terminal.tsx` の既存キーがある `drawerStorage` ネームスペースの `terminal` グループに ja / en の両方で追加する。
新しいネームスペースは追加しない (既存の `terminal.*` キーの移動を伴い、本 issue の範囲を超える)。

### 8. 扱わない範囲

- ページのリロードやブラウザタブを閉じた後のセッション維持 (backend の再接続機構が必要。設計判断 1 で却下)。
- Google Cloud の Cloud Logging Live Tail (`frontend/src/components/logviewer/useLiveTail.ts` の WebSocket) の維持。`views/GcpView.tsx` の条件レンダリングで同じ構造の問題があるが、対象が別サービスであり、本 issue の変更に依存しないため別 TODO として扱う。
- 親項目の分割表示 (docs/issues/pending/0175)。
- ドックの高さをドラッグで変える機能と、高さ / 折りたたみ状態の永続化。必要になれば TODO に追加して別 issue で扱う。
- Drawer の Escape キーによる閉じる処理 (`Drawer.tsx` の `document` への `keydown` リスナー) とターミナル入力の衝突。ドックへ移すことで Drawer と一緒にターミナルが閉じることは無くなるが、Drawer が開いているときにターミナルで Escape を押すと Drawer が閉じる挙動は現状のままとする。

## 完了条件

- `frontend/src/hooks/useTerminalSessions.ts` が新設され、設計判断 3 に列挙したシグネチャで `useTerminalSessions`、`openTerminalSession`、`closeTerminalSession`、`activateTerminalSession`、`setTerminalDockCollapsed`、`resetTerminalSessionsForTest` と、型 `TerminalSession` を export している。
- `useTerminalSessions.test.tsx` で次が確認できる。`openTerminalSession` が返した id が `tabs.active` と一致する。`collapsed` が `true` の状態で開くと `collapsed` が `false` になる。同じ `input` で 2 回開くと異なる id で 2 セッションになる。アクティブなセッションを閉じると `sessionTabsState.ts` の `closeSession` の規則で次のアクティブが選ばれる。最後のセッションを閉じると `tabs.open` が空になる。
- `frontend/src/components/Terminal/TerminalDock.tsx` が新設され、`App.tsx` の `.app` 要素の子として `view` の値に関係なく描画されている。
- `TerminalDock.test.tsx` で次が確認できる。セッションが 0 のときドックは描画されない。2 セッション開いた状態で両方の `.terminal-container` が DOM に存在し、アクティブでない方の `.terminal-dock-pane` だけに `hidden` 属性が付き、どちらの `.terminal-panel` にも `hidden` が付かない。タブをクリックすると `hidden` の付く `.terminal-dock-pane` が入れ替わり、どちらの `.terminal-container` も DOM から消えない。折りたたみボタンで `.terminal-dock-body` に `hidden` が付き、両方の `.terminal-container` が DOM に残る。閉じるボタンで対象のセッションの `.terminal-container` だけが DOM から消える。
- `frontend/src/App.test.tsx` (新規。`api/queries` 等のフックは `views/AccountView.test.tsx` と同じ `vi.mock` の方法で差し替える。`hooks/useProfiles` は `profiles` と `openProfiles` に 2 つのプロファイルを持ち、`activateProfile` で `activeProfile` が切り替わる偽の実装に差し替える) で次が確認できる。`globalThis.WebSocket` を `vi.stubGlobal` で置き換えた偽の WebSocket クラス (コンストラクタで渡された URL と `close()` の呼び出し回数を記録する) を用い、セッションを 1 つ開いた状態で次の 3 つの操作を順に行っても、偽 WebSocket の `close()` が呼ばれず `.terminal-container` が DOM に残る。(1) プロファイルタブをもう一方のプロファイルへ切り替えて元に戻す (`AccountView` が `key={activeProfile}` で再マウントされる)。(2) トップバーのビューを `aws` から `gcp` に切り替えて `aws` に戻す。(3) サイドバーでサービスを `ec2` から `ssm` に切り替える。その後、ドックのタブの閉じるボタンを押すと `close()` が 1 回呼ばれる。
- `frontend/src/components/Terminal/Terminal.test.tsx` (新規) で次が確認できる。マウント時に prop の `wsUrl` で WebSocket が生成され、アンマウント時に `close()` が 1 回呼ばれる。`ResizeObserver` を発火させられるスタブに差し替え、コンテナの `clientWidth` / `clientHeight` が 0 のときに発火させると `FitAddon.prototype.fit` (`vi.spyOn`) が呼ばれない。`active` prop が `false` から `true` に変わると `fit` と xterm の `Terminal.prototype.focus` (`vi.spyOn`) が呼ばれる。テストでは `DrawerTerminal.test.tsx` と同じ `matchMedia` のスタブを用いる。
- `DrawerTerminal.tsx` は `Terminal` を描画せず、`DrawerTerminal.test.tsx` で次が確認できる。EC2 では Connect ボタンの押下でストアに `kind: 'ec2'` のセッションが 1 つ追加される。ECS ではタスクとコンテナが確定するまで Connect ボタンが `disabled` であり、確定後の押下で `kind: 'ecs'` のセッションが追加される。`DrawerTerminal` のどの状態でも `.terminal-container` は描画されない。
- `Drawer.tsx` から `pendingExecTarget` が削除され、`Drawer.test.tsx` で ECS の Tasks タブの Exec ボタン押下によりストアに `kind: 'ecs'` のセッションが追加され、Drawer のタブが Terminal へ切り替わらないことが確認できる。
- `app.css` に `.terminal-dock-body[hidden], .terminal-dock-pane[hidden] { display: none; }` の規則があり、`.terminal-dock-body` と `.terminal-dock-pane` に `display` を宣言する規則が無い。
- `app.css` に `.terminal-dock-body` の `flex: 1` と `min-height: 0`、`.terminal-dock-pane` の `height: 100%` の宣言があり、`.terminal-panel` の規則 (`height: 100%`、`min-height: 320px`) が変更されていない。
- `app.css` の `.drawer` と `.drawer.pos-bottom` の両方で `bottom` が `calc(var(--terminal-dock-h, 0px) + 8px)` になっており、`TerminalDock.test.tsx` で `document.documentElement` の `--terminal-dock-h` が、ドックの展開中は `352px`、折りたたみ中は `32px`、非描画時は `0px` になり、ドックのルート要素の `style.height` が展開中は `352px`、折りたたみ中は `32px` になることが確認できる。
- `drawerStorage` ネームスペースの ja / en の両方に新規キーが揃っており、キー集合が一致する。
- 手動確認: `mise run backend:run` と `mise run frontend:run` で起動し、実 AWS 環境で EC2 インスタンスへのセッションを開いたまま、サイドバーで Parameter Store と Secrets Manager の Value タブを開いてもシェルの入力とスクロールが続けられること、ドックを折りたたんで展開し直した後もシェルの入力が続けられることを確認する。展開中のドックでターミナルの最終行がドックの本文の中に表示され、本文からはみ出さないことを確認する。ECS Exec についても Tasks タブの Exec ボタンから同じ確認を行う。2 セッションを開き、アクティブでないタブのターミナルが画面に表示されないこと、折りたたみ中はドックの本文が表示されないことを確認する。Drawer の配置を TweaksPanel で右と下のそれぞれにし、開いた Drawer がドックに重ならないことを確認する。実 AWS 環境が使えない場合は、未実施であることと理由を issue に記録する。
- `mise run check` が通過する。

## 関連

- docs/issues/pending/0175: 同じ TODO 項目の親項目 (分割表示) を扱う。本 issue のドックで TODO の具体例 (Session Manager を繋いだまま Parameter Store / Secrets Manager を見る) が満たされるため、分割表示が引き続き必要かの判断を 0175 の調査タスクに含める。
- docs/issues/closed/0020: `sessionTabsState.ts` と `SessionTabs` を導入した issue。本 issue は純関数のみ再利用する。
- docs/issues/closed/0142: CLI (`backend/internal/cli/ec2.go`) 側で EC2 セッション開始失敗時のセッションリークを修正した issue。Web UI が使う `runSessionBridge` / `Bridge.cleanup` とは別の経路であり、本 issue はどちらの経路も変更しない。

## 解決方法

`frontend/src/hooks/useTerminalSessions.ts` を新設し、`useTweaks.ts` と同じ `useSyncExternalStore` ベースの共有ストアで、開いているターミナルセッション (`TerminalSession`) の集合と `sessionTabsState.ts` の `SessionTabsState`、ドックの折りたたみ状態を持つようにした。`openTerminalSession` / `closeTerminalSession` / `activateTerminalSession` / `setTerminalDockCollapsed` / `resetTerminalSessionsForTest` をモジュール関数として export した。

`frontend/src/components/Terminal/TerminalDock.tsx` を新設し、`App.tsx` の `.app` 要素の末尾の子として `view` の値に関係なく描画されるようにした。セッションが 0 のときは何も描画せず、1 つ以上のときはタブバーと `div.terminal-dock-body` (各セッションの `div.terminal-dock-pane` に包んだ `Terminal` を全セッション同時にマウント) を描画する。非アクティブなセッションは `div.terminal-dock-pane` に `hidden` 属性を付けて隠し、`Terminal` が返す `.terminal-panel` には `hidden` を付けない。折りたたみは `div.terminal-dock-body` への `hidden` の切替だけで行い、`Terminal` のマウント状態は変えない。

`DrawerTerminal.tsx` から `Terminal` の描画を外し、EC2 は Connect ボタン、ECS はタスク / コンテナ確定後の Connect ボタンで `openTerminalSession` を呼ぶランチャーにした。`Drawer.tsx` の ECS Tasks タブの Exec ボタン (`onExec`) も Terminal タブへ切り替えずに直接 `openTerminalSession` を呼ぶようにし、不要になった `pendingExecTarget` state と `DrawerTerminal` の `execTarget` prop を削除した。

`Terminal.tsx` には、`ResizeObserver` のコールバックでコンテナの `clientWidth` / `clientHeight` が 0 のときに `fitAddon.fit()` を呼ばないガードと、`active` prop が `false` から `true` に変わったときに `fit()` と `term.focus()` を呼ぶ処理を追加した。

`app.css` に `.terminal-dock-body { flex: 1; min-height: 0; }` と `.terminal-dock-pane { height: 100%; }`、`.terminal-dock-body[hidden], .terminal-dock-pane[hidden] { display: none; }` を追加し、`.terminal-panel` の既存規則 (`height: 100%`、`min-height: 320px`) は変更していない。`.drawer` と `.drawer.pos-bottom` の `bottom` を `calc(var(--terminal-dock-h, 0px) + 8px)` に変え、`--terminal-dock-h` を `usePersistedSidebarWidth` と同じ方法で `document.documentElement` に設定するようにした。

`drawerStorage` ネームスペースの ja / en に、ドックとランチャーの文言 (Connect、折りたたみ / 展開、閉じる、接続状態) を追加した。

完了条件の各行の検証は次のとおり。

- `useTerminalSessions.ts` の export とシグネチャ: ファイル本体で確認済み。ストアの挙動 5 点は `useTerminalSessions.test.tsx` (10 件、全 pass) で確認した。
- `TerminalDock.tsx` の描画位置と DOM 挙動: `App.tsx` の該当箇所と `TerminalDock.test.tsx` (6 件、全 pass) で確認した。
- 画面遷移をまたいだ接続維持: `App.test.tsx` (2 件、全 pass) で、偽 WebSocket を用いてプロファイル切替、ビュー切替、サイドバーのサービス切替のいずれでも `close()` が呼ばれず、ドックの閉じるボタンでのみ呼ばれることを確認した。
- `Terminal.tsx` の 2 点の変更: `Terminal.test.tsx` (5 件、全 pass) で確認した。
- `DrawerTerminal.tsx` のランチャー化: `DrawerTerminal.test.tsx` (4 件、全 pass) で確認した。
- `Drawer.tsx` の `pendingExecTarget` 削除: `Drawer.test.tsx` (15 件、全 pass) で確認した。
- `app.css` の `[hidden]` 規則、`.terminal-dock-body` / `.terminal-dock-pane` の宣言、`.terminal-panel` の無変更: `app.css` の該当行 (1467、1492、1744-1750、1873-1885 付近) を目視で確認した。実装エージェントは `node:fs` によるファイル読み込みでの自動検査を試みたが、このリポジトリに `@types/node` が無く `tsc --noEmit` が失敗したため、既存パッケージの追加を伴わない範囲では自動テスト化できなかった (下記の乖離 2 参照)。
- `--terminal-dock-h` の値: `TerminalDock.test.tsx` で展開時 352px、折りたたみ時 32px、非描画時 0px になることを確認した。
- ja / en のキー集合一致: `drawerStorage.json` の両言語の diff で確認した。
- `mise run check` の通過: 統合後の作業ツリーで実行し、`EXIT=0`、frontend のテストは最終的に 93 ファイル 957 件全 pass、backend は全パッケージ pass、baseline (失敗ゼロ) からの新規失敗は無かった。lint の warning 10 件はすべて baseline から存在する既存のもので、本 issue による増加は無い (統合直後は `DrawerTerminal.tsx` の `react-refresh/only-export-components` 新規警告 2 件を含む 12 件だったが、`openEC2TerminalSession` / `openECSTerminalSession` を `lib/terminalLaunchers.ts` へ分離して baseline と同じ 10 件に戻した。乖離 3 参照)。テスト数は多観点レビューの 3 ラウンド (下記) の反映で 952 → 953 → 957 と増えている。
- 手動確認: 実 AWS 環境がこの実行環境から利用できないため未実施。ドック常駐、折りたたみ後の再フィット、複数セッションの表示切替、Drawer との重なり回避は上記の自動テストと目視確認で担保しているが、実 EC2 / ECS Exec への接続を伴う確認は行っていない。

多観点レビュー (Step 7) はラウンド 1、その反映に伴う追加ラウンド (ラウンド 2)、ラウンド 2 の反映に伴う追加ラウンド (ラウンド 3、最大 2 回の追加レビュー枠の 2 回目にあたる最終ラウンド) の計 3 ラウンドを行った。

ラウンド 1 (3 観点) の指摘は次の低優先度 2 件のみで、いずれも反映した。

1. `TerminalDock.tsx` の `statuses` state (`Record<string, ConnectionStatus>`) が `closeTerminalSession` 後もエントリを保持し続け、セッションの開閉を繰り返すたびに際限なく蓄積する指摘 (堅牢性の観点)。`tabs.open` の変化を監視する `useEffect` を追加し、`tabs.open` に無い id のエントリを `statuses` から削除するようにした (`components/Terminal/TerminalDock.tsx`)。ids は連番採番で再利用されないため、この蓄積は動作の誤りではなくメモリ上の未解放だけが問題だった。
2. `useTerminalSessions.ts` の `closeTerminalSession` が持つ「存在しない id なら早期 return する」防御的分岐に対応するテストが無い指摘。`useTerminalSessions.test.tsx` に「存在しない id を closeTerminalSession に渡しても状態が変わらない」テストを追加し、`result.current` の参照同一性 (早期 return により再レンダーが起きないこと) まで確認した。

ラウンド 1 の反映 (上記 2 点) に限定したラウンド 2 (3 観点) では、次の低優先度 1 件が出て、反映した。

3. ラウンド 1 の指摘 1 で追加した `statuses` のプルーニング処理が `TerminalDock.tsx` の `useEffect` 内にインラインで書かれており、単体テストで直接検証できない指摘 (テストと堅牢性の観点)。このリポジトリは `lib/sessionTabsState.ts` のように状態遷移ロジックを純関数へ切り出してテストするパターンを既に確立しているため、それに揃えた。`components/Terminal/terminalDockStatuses.ts` (新規) に `pruneStatuses(statuses, openIds)` として切り出し、`TerminalDock.tsx` の `useEffect` からはこれを呼ぶだけにした。`terminalDockStatuses.test.ts` (新規、4 件) で、削除対象がある場合の挙動、削除対象が無い場合と `statuses` が空の場合に同じ参照を返すこと (React の再レンダー抑制に必要)、`openIds` が空なら全削除されることを検証した。ロジック自体はラウンド 1 の反映時から変更していない。

ラウンド 2 の反映 (指摘 3) に限定したラウンド 3 (3 観点、最終) では、次の低優先度 1 件のみが出て、反映した。

4. 本「## 解決方法」節にラウンド 2 とラウンド 3 の説明文を追記した際、本来「完了条件の各行の検証」リストの最終項目だった「手動確認」の箇条書きが、間に挟まったレビュー履歴の段落によって元のリストから分離され、孤立した箇条書きになっていた指摘 (規約と整合の観点)。「手動確認」の項目を完了条件検証リストの末尾 (本節の上記) へ戻し、レビュー履歴の説明はその後の独立した段落として続くように整理した (本追記そのものが反映結果である)。実装とテストへの変更は無い。

ラウンド 3 が最大 2 回の追加レビュー枠を使い切った最終ラウンドであり、指摘は低優先度のみで反映済みのため、多観点レビューを完了した。

方針セクションからの実装詳細の乖離 (方式は変更していない) は次の 3 点。

1. `Terminal.tsx` に `onStatusChange?: (status: ConnectionStatus) => void` prop を追加した。設計判断 5 は 2 点の変更を挙げているが、設計判断 2 のタブの接続状態表示と設計判断 7 のための i18n キーの追加を実現するため、`Terminal` から接続状態を通知する経路が要った。状態は `TerminalSessionsState` (設計判断 3) には持たせず、`TerminalDock` 側の `useState` に閉じたため、ストアのシグネチャは変えていない。`ConnectionStatus` 型を新たに export した。
2. `TerminalDock.test.tsx` から、`app.css` の原文を `node:fs` で読んで規則を直接検査するテストブロックを削除した。このリポジトリに `@types/node` が無いため `tsc --noEmit` が `TS2307` で失敗し、`?raw` インポートや `import.meta.glob` も Vitest 上で CSS が空モジュールに差し替わり検証にならないことを実測で確認した。`@types/node` の追加は外部依存の追加に当たるため、この場では追加せず、該当する完了条件の 2 行は `app.css` の目視確認で満たした。CSS 規則の自動回帰検査が必要になった場合は `@types/node` 追加の可否を別途判断する。
3. `openEC2TerminalSession` と `openECSTerminalSession` を `DrawerTerminal.tsx` から `frontend/src/lib/terminalLaunchers.ts` (新規) へ移した。統合後の `mise run check` で `DrawerTerminal.tsx` に `react-refresh/only-export-components` の新規警告 2 件 (コンポーネント以外の export が同居することによるもの) が baseline 比で増えていたため、リポジトリ規約 (frontend Lint/Format 節「新規コードで警告を増やさないよう努める」) に沿って分離した。呼び出し元 (`DrawerTerminal.tsx`、`Drawer.tsx`) の import 先を変えただけで、シグネチャと挙動は変えていない。`DrawerTerminal.test.tsx`、`Drawer.test.tsx` は挙動を UI 経由で検証しているため無修正で通過する。

本 issue は feat であり再現手順は無い。元の症状 (画面遷移でセッションが切れる) に対応する回帰テストは `App.test.tsx` に置いた。perf issue ではないため計測は無い。

スコープ外で見つかった問題 (未修正、`docs/issues/TODO.md` へ別途追記する)。

1. `frontend/src/App.tsx` の `no-console` 用 `eslint-disable-next-line` が、このリポジトリの eslint 設定に `no-console` ルールが無いため未使用ディレクティブ警告になっている (baseline から存在)。
2. `mise run frontend:test` が jsdom を 92 回生成しており、実行時間の大半を占めている。Vitest が `pool: 'vmThreads'` または `isolate: false` を提案している (テスト基盤の課題)。

## reopen の理由

2026-09-16 にユーザーが実 AWS 環境で手動確認 (完了条件の「手動確認」の行。close 時にはこの実行環境から実 AWS 環境が使えず未実施だった) を行い、次の 2 点が満たされていないことを観測した。

- 観測 1: ドックに表示された Terminal をクリックしてもフォーカスが当たらず、キーボード入力ができない。完了条件の「EC2 インスタンスへのセッションを開いたまま、サイドバーで Parameter Store と Secrets Manager の Value タブを開いてもシェルの入力とスクロールが続けられること」が満たされていない。
- 観測 2: Drawer (既定の下配置) を閉じると、Terminal 分の領域を占有したまま Drawer が閉じ切らない。完了条件の「開いた Drawer がドックに重ならないこと」は開いた状態だけを見ており、閉じた Drawer がドックに重ならないことを含めていなかった。

原因はいずれも `frontend/src/app.css` のレイヤ構造にあり、コードの読解で特定した (ブラウザでの再現はこの実行環境からは行えない)。

1. 閉じた Drawer がドックの上に残る。設計判断 6 で `.drawer.pos-bottom` の `bottom` を `calc(var(--terminal-dock-h, 0px) + 8px)` に持ち上げた一方で、閉じ位置の `transform` は `translateY(calc(100% + 16px))` のまま (`app.css` の `.drawer.pos-bottom` と `Drawer.tsx` の inline style の 2 か所) にした。閉じ位置は自身の高さ + 16px だけ下がるため、ドックの高さ (展開時 352px) の分だけ画面内に残り、Drawer の上端がドック上端の 8px 下に来る。Drawer は `position: fixed; z-index: 11` で不透明な背景を持つため、ドックの上に被さって見え (観測 2)、ドック上のクリックを全て受け取る (観測 1 のうち Drawer を閉じた状態の原因)。右配置の閉じ位置 `translateX(calc(100% + 16px))` は水平方向の移動で `bottom` の影響を受けないため、この問題は下配置だけで起きる。既定の配置は `hooks/useTweaks.ts` の `DEFAULT_TWEAKS.drawerPos` の `'bottom'` である。
2. Drawer を開いている間、backdrop がドックを覆う。`.drawer-backdrop` は `position: fixed; inset: 0; z-index: 10` で、`.open` 時に `pointer-events: auto` になる。`.terminal-dock` は `position` と `z-index` を持たないため backdrop より背面に描かれ、Terminal をクリックすると backdrop の `onClick={onClose}` が発火して Drawer が閉じるだけで、xterm にフォーカスが渡らない (観測 1 のうち Drawer を開いた状態の原因)。本 issue の背景にある「Parameter Store / Secrets Manager を見ながらシェルを使う」ことができない。

前回の実装は `--terminal-dock-h` を `bottom` に反映するところで止まり、閉じ位置の `transform` と、ドックと backdrop / Drawer の重なり順を見ていなかった。閉じ位置の定義が `app.css` と `Drawer.tsx` の inline style の 2 か所にあることが、片方だけを直す余地を生んだ。Escape キーについては `@xterm/xterm` が処理したキーの keydown で `stopPropagation()` を呼ぶ (`node_modules/@xterm/xterm/src/browser/Terminal.ts` の `cancel`) ため、Terminal 上の Escape が `Drawer.tsx` の document の keydown リスナーに届くことはなく、問題は無い。

### reopen 後の修正方針

- 閉じ位置と開き位置の定義を `app.css` の `.drawer` / `.drawer.open` / `.drawer.pos-bottom` / `.drawer.pos-bottom.open` に一元化し、`Drawer.tsx` の inline style から `transform` を除く。`.drawer.pos-bottom` の閉じ位置を `translateY(calc(100% + var(--terminal-dock-h, 0px) + 16px))` にして、`bottom` の持ち上げ分だけ余分に下げる。
- `.drawer` が `open` クラスを持たないときは `pointer-events: none` にし、閉じた Drawer が位置にかかわらず入力を受け取らないようにする (閉じ位置の計算が再び崩れた場合の保険)。
- `.terminal-dock` に `position: relative` と `z-index: 12` (`.drawer` の 11 と `.drawer-backdrop` の 10 より前面) と `flex: none` を与え、Drawer を開いている間も Terminal がクリックとフォーカスを受け取れるようにする。下配置の Drawer が閉じるときはドックの背面へ滑り込む。backdrop の範囲をドック分だけ狭める案 (`.drawer-backdrop` の `bottom` を `var(--terminal-dock-h)` にする) は、ドックが常駐の殻であり一時的なオーバーレイより前面にあるべきという関係を z-index で表す方が、backdrop 以外の fixed 要素を追加したときにも崩れないため採らなかった。
- jsdom はレイアウトと `transform` の計算を行わないため、規則そのものは `app.css` のテキストを読むテスト (`frontend/src/app.css.test.ts`) で検証する。0174 の設計判断 2 では表示状態の検証を手動確認に委ねたが、手動確認が行えない環境で close した結果として今回の不具合が出たため、閉じ位置と重なり順の 2 つの不変条件に限って自動テストで固定する。
- 追加の AWS API 呼び出しや IAM 権限は不要である。変更は `app.css`、`Drawer.tsx`、`vite.config.ts` とそのテストに限られ、backend とセッションブリッジは変更しない。

### reopen で追加する完了条件

- `Drawer.tsx` が `transform` を inline style で設定しない。`Drawer.test.tsx` で、閉じた状態 (`resource` が `null`) の下配置の Drawer に `pos-bottom` クラスが付き `open` クラスが付かず `style.transform` が空であること、開いた状態で `open` クラスが付き `style.transform` が空であることが確認できる。
- `app.css` の `.drawer.pos-bottom` の `transform` が `translateY(calc(100% + var(--terminal-dock-h, 0px) + 16px))` であり、`.drawer:not(.open)` に `pointer-events: none` の規則がある。
- `app.css` の `.terminal-dock` が `position: relative`、`flex: none`、および `.drawer` と `.drawer-backdrop` の `z-index` より大きい `z-index` を持つ。
- `frontend/src/app.css.test.ts` (新規) で、`app.css` のテキストから次が確認できる。`.drawer.pos-bottom` ブロックの `transform` 宣言に `var(--terminal-dock-h, 0px)` が含まれる。`.drawer:not(.open)` ブロックに `pointer-events: none` がある。`.terminal-dock` ブロックの `z-index` が `.drawer` ブロックと `.drawer-backdrop` ブロックの `z-index` より大きく、`position: relative` を持つ。`.terminal-dock` ブロックの `flex` が `none` である。
- 手動確認: 実 AWS 環境で Drawer を下配置と右配置のそれぞれにし、(1) Drawer を開いたまま Terminal をクリックするとフォーカスが当たって入力でき、Drawer が閉じないこと、(2) Drawer を閉じると Drawer が画面外へ完全に消えてドックの全域が見えること、(3) 閉じた後に Terminal をクリックすると入力できることを確認する。実 AWS 環境が使えない場合は未実施であることと理由を記録する。
- `mise run check` が通過する。

## 解決方法

reopen 後の対応 (2026-09-16)。前回の「## 解決方法」はそのまま残し、ここには「## reopen の理由」に書いた 2 つの原因への対応だけを書く。

`frontend/src/app.css` の `.drawer.pos-bottom` の閉じ位置を `translateY(calc(100% + var(--terminal-dock-h, 0px) + 16px))` に変え、`bottom` で持ち上げたドックの高さ分だけ余分に下げるようにした。`.drawer:not(.open) { pointer-events: none; }` を追加し、閉じた Drawer が位置にかかわらず入力を受け取らないようにした。`.terminal-dock` に `position: relative`、`z-index: 12`、`flex: none` を追加し、`.drawer-backdrop` (10) と `.drawer` (11) より前面に置いた。これで Drawer を開いている間も Terminal へのクリックが backdrop に奪われず、xterm にフォーカスが渡る。下配置の Drawer はドックの背面へ滑り込んで閉じる。

`frontend/src/components/Drawer/Drawer.tsx` の inline style から `transform` を除き、`style={sizeStyle}` (永続化されたサイズだけ) にした。開閉位置の定義は `app.css` の `.drawer` / `.drawer.open` / `.drawer.pos-bottom` / `.drawer.pos-bottom.open` の 4 規則だけになる。

`frontend/vite.config.ts` の `test.css.include` に `/\.css\?raw$/` を追加した。vitest は既定で CSS の import を空文字列に差し替え (`vitest:css-disable` プラグイン)、その判定 `\.css(?:$|\?)` が `app.css?raw` にも一致するため、新設の `app.css.test.ts` が規則のテキストを読めなかった。Node の `fs` で読む案は `@types/node` が無く `tsc --noEmit` (`mise run frontend:lint`) を通らないため採らなかった (`@types/node` の追加は外部依存の追加になる)。この設定は既存のテストに影響しない。`app.css` を import するのは `main.tsx` だけで、テストからは読み込まれない。

reopen で追加した完了条件の検証。

- `Drawer.tsx` が `transform` を inline で設定しない: `Drawer.test.tsx` の describe「Drawer の開閉クラスと transform の定義元」の 3 ケース (閉じた下配置、閉じた右配置、開いた状態) で、クラスが期待どおりで `style.transform` が空であることを確認した。
- `app.css` の閉じ位置、`pointer-events`、`.terminal-dock` の `position` / `z-index` / `flex`: 新設の `frontend/src/app.css.test.ts` の describe「app.css の Drawer とターミナルドックの重なり」の 4 ケースで、対象の規則ブロックが 1 つだけ存在すること、`.drawer.pos-bottom` の `bottom` と `transform` が同じ `var(--terminal-dock-h, 0px)` を参照すること、`.drawer:not(.open)` の `pointer-events` が `none` であること、`.terminal-dock` の `z-index` が `.drawer` と `.drawer-backdrop` より大きく `position` が `relative`、`flex` が `none` であることを確認した。規則ブロックの抽出はセレクタが行頭から始まり直後に ` {` が続くものに限り、`.drawer` と `.drawer-backdrop` のような接頭辞を共有するセレクタの誤一致を防いでいる。コメントはブロックの抽出より前に取り除き、コメント内の行頭の `}` をブロックの終端と誤認しないようにしている。抽出規則そのものは、同じファイルの describe「規則ブロックの抽出」の 3 ケースで、合成した CSS (コメント内の行頭の `}`、接頭辞を共有するセレクタと子孫セレクタ、同じセレクタの重複) に対して固定した。
- 手動確認: 実 AWS 環境がこの実行環境から利用できないため未実施。閉じ位置は幾何で確認した。下配置でドックの高さを D、Drawer の高さを H とすると、閉じた Drawer の上端は修正前が「ビューポート下端 - D + 8px」で画面内 (ドック上端の 8px 下)、修正後が「ビューポート下端 + 8px」で画面外になる。右配置は水平移動のため修正前後で変わらない。ユーザーによる実環境での確認 (reopen で追加した手動確認の 3 点) を依頼する。
- `mise run check`: 通過 (frontend 94 ファイル 967 テスト、lint は 0 errors 10 warnings で baseline と同じ、backend は全パッケージ ok)。テスト数は 957 → 967 (`Drawer.test.tsx` +3、`app.css.test.ts` +7)。レビューの反映で実装が変わるたびに再実行し、最終の実行も同じ結果だった。

方針からの乖離: 「reopen 後の修正方針」のとおりで、方式の乖離は無い。方針で定めていなかった実装詳細として、`app.css.test.ts` の CSS の読み取り方法 (`?raw` import と `vite.config.ts` の `test.css.include`) をここに記録する。

多観点レビュー (3 観点) の指摘と反映。ラウンド 1 は高が 0 件、中が 1 件、低が 4 件だった。

- 中 (テストと堅牢性): `app.css.test.ts` の `declarationsOf` がブロックの終端を行頭の `}` で判定するため、ブロック内のコメントに行頭の `}` を含む行が書かれると、そこを終端と誤認して以降の宣言を取りこぼす。反映: コメントをブロックの抽出より前に取り除く形に変えた。同じ指摘のうち、値に `!important` が付くと `toBe` の完全一致が落ちる点は、カスケードを変える変更として落とすのが意図であるため変えず、その旨をテストのコメントに書いた。
- 低 (完了条件の充足): 手動確認が前回の close と今回で 2 回続けて未実施である。反映: 実 AWS 環境がこの実行環境から使えないため未実施のままとし、ユーザーに実環境での確認 (reopen で追加した手動確認の 3 点) を依頼する。
- 低 (規約と整合) 3 件: 「reopen 後の修正方針」に API と権限の要否が無い。「reopen で追加する完了条件」の `app.css.test.ts` の列挙から `flex: none` が漏れている。`CHANGES.md` の追記が `.drawer:not(.open)` の `pointer-events: none` に触れていない。反映: いずれも本文に追記した。

追加ラウンド 1 (ラウンド 1 の反映で変わった箇所に限る) は高が 0 件、中が 1 件、低が 1 件だった。

- 中 (テストと堅牢性): コメント除去の順番を元に戻しても、現在の `app.css` の対象ブロックには行頭の `}` を含むコメントが無いため 4 ケースが通り続け、回帰を検出できない。反映: `declarationsOf` と `declarationOf` が CSS のテキストを引数で受け取れるようにし (省略時は `app.css`)、コメント除去を `declarationsOf` の中に置き、合成した CSS で抽出規則を検証する 3 ケースを describe「規則ブロックの抽出」として追加した。旧実装 (抽出後にコメントを除く) を合成 CSS に当てると `z-index` を取りこぼすことを確認したので、順番を戻す変更はこのケースで落ちる。
- 低 (テストと堅牢性): テストのコメントで `}` の前後の半角スペースが行によって不統一である。反映: 鍵括弧を使わない言い方に直した。

追加ラウンド 2 (追加ラウンド 1 の反映で変わった箇所に限る) は 3 観点とも指摘が 0 件で、レビューを終えた。

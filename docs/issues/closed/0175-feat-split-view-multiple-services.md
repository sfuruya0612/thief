# AWS / Google Cloud の複数サービスを 1 画面に分割表示する方式を調査する

Created: 2026-09-15
Model: Claude Fable 5.1
Completed: 2026-09-25

## 背景

TODO.md の次の項目に由来する。

> AWS, Google Cloud で特定のサービスを開いた状態で、別のサービスの状態を画面をセパレートするなどして同時に 2 つ以上のサービスを参照できるようにしたい
> - 例えば、AWS EC2 や ECS の Session Manager を繋いだ状態で Parameter Store や Secret Manager の値を確認したい時に、一度ページ遷移を挟む必要があり、session の繋ぎ直しが都度必要
> - EC2, ECS の Session Manager を開いた状態を別画面に遷移しても維持できればそれだけでもいいかも

本 issue は親項目の「画面をセパレートして 2 つ以上のサービスを同時に参照する」分割表示を扱う。
子項目の具体例 (Session Manager を繋いだまま Parameter Store / Secrets Manager を見る) は docs/issues/0174 のターミナルドックで満たす。
ファイル名のカテゴリは、調査 issue に将来の変更のカテゴリを付けた前例 (docs/issues/pending/0059 は feat、docs/issues/pending/0083 は perf) に従い、feat とする。

現状の画面構成は、1 ビューにつき 1 サービスだけを表示する前提で組まれている。

- `frontend/src/App.tsx` の `App` は `activeService` (AWS) と `activeGcpService` (Google Cloud) をそれぞれ 1 つの文字列の `useState` で持ち、`view` の値に応じて `AccountView` か `GcpView` のどちらか 1 つを描画する。
- `frontend/src/views/AccountView.tsx` の `AccountView` は `activeService` の条件レンダリングで `ServicePanel` を 1 つだけ描画し、`selectedId` (Drawer で開くリソース) も 1 つだけ持つ。`views/GcpView.tsx` の `GcpView` も同じ構造である。
- `frontend/src/components/Sidebar.tsx` の `Sidebar` はサービスの単一選択 (`activeService` / `onService`) を前提にしている。
- `frontend/src/components/Drawer/Drawer.tsx` の `Drawer` は `ServicePanel` ごとに 1 つ描画され、`position: fixed` でビューポート右または下に重なる (`frontend/src/app.css` の `.drawer` / `.drawer.pos-bottom`)。配置は `types/common.ts` の `DrawerPos = 'right' | 'bottom'` の 2 値で、`components/TweaksPanel.tsx` から切り替える。
- `frontend/src/lib/storage.ts` の `PersistedState` は `view`、`region`、セッションタブ (`awsSessions` / `gcpSessions` / `datadogOrgSessions`)、`sidebarWidth`、`tweaks` を永続化するが、表示中のサービスや選択中のリソースは永続化していない。

2 つ以上のサービスを同時に表示するには、`activeService` と `selectedId` を表示領域 (ペイン) ごとに持ち、`Sidebar` の選択がどのペインに向くかを決め、`position: fixed` の `Drawer` が複数同時に開いたときの配置を決める必要がある。
これらは実装方針を 1 つ選ばなければ決まらず、決まるまで実装に着手できない。

## 目的

分割表示を実装するかどうか、実装する場合にどのレイアウトモデルと状態分割を採るかの判断材料を揃える。

## pending にした理由

TODO の子項目が「Session Manager を開いた状態を別画面に遷移しても維持できればそれだけでもいいかも」と述べており、具体例は docs/issues/0174 のターミナルドックで満たされる。
分割表示が引き続き必要かは 0174 を使ったうえでの利用者の判断に依存し、その前にレイアウトモデルを確定できない。
また、ペインごとの状態を永続化するかどうかは `PersistedState` へのフィールド追加を伴い、リポジトリの frontend 規約 (永続化する状態フィールドの追加は事前に質問する) により利用者の確認が要る。

## pending 論点の決着 (2026-09-25)

2026-09-16 の調査結果 (分割表示は不要) を、2026-09-25 に利用者が撤回した。分割表示は必要であり、調査タスクの各論点について次の回答を得た。

- レイアウトモデル：案 A (`AccountView` 相当のペインを左右に 2 つ並べ、各ペインが独自のサービス、選択リソース、Drawer を持つ)。
- 共有する状態：プロファイルとリージョンは 2 ペインで共有する。ペインごとに持つのはサービス、選択リソース、フィルタ、Drawer のタブである。ビュー (`AppView`) も共有する。
- `Sidebar` の操作モデル：クリックはフォーカス中のペインに反映する。
- 永続化：ペインの状態は `PersistedState` に永続化しない (現状の `activeService` と同じ扱い)。
- Google Cloud：`GcpView` も本 issue で同時に対応する。
- `Drawer` の配置：分割中は各ペインの中に収め、`DrawerPos` の値は増やさない (次の「## 設計判断」の 5。利用者に提示し、異論が無かった)。

本 issue は調査 issue から実装 issue に変わった。タイトルは起票時の記述として残す。

## 設計判断

変更は frontend だけで、backend、API、AWS / Google Cloud の権限の追加は無い。

### 1. ペインの状態は純関数で遷移させ、App がビューごとに持つ

`frontend/src/lib/splitPanes.ts` (新規) に状態の型と遷移の純関数を置く。

- 状態：`SplitPanesState { services: (string | null)[]; focused: number }`。`services` の長さは 1 (分割なし) または 2 (分割中) で、`null` はサービス未選択のペインを表す。`focused` はフォーカス中のペインの添字。
- `selectService(state, service)`：`service` を他のペインが表示中ならフォーカスをそのペインへ移すだけにする。それ以外はフォーカス中のペインのサービスを `service` に置き換える。
- `openSplit(state)`：1 ペインのときだけ、2 つ目のペインを `null` で追加し、フォーカスを 2 つ目へ移す。2 ペインのときは同じ状態を返す。
- `closePane(state, index)`：2 ペインのときだけ `index` のペインを取り除いて 1 ペインにし、`focused` を 0 にする。1 ペインのときは同じ状態を返す。
- `focusPane(state, index)`：範囲外の `index` は同じ状態を返す。
- 変化が無い遷移は引数と同じ参照を返す (React の再描画を抑えるため。`components/Terminal/terminalDockStatuses.ts` の `pruneStatuses` と同じ扱い)。

`frontend/src/hooks/useSplitPanes.ts` (新規) が `useReducer` でこの純関数を包む。`App.tsx` の `activeService` / `activeGcpService` の `useState` を、AWS 用 (初期値 `ec2`) と Google Cloud 用 (初期値 `cloudrun`) の 2 つの `useSplitPanes` に置き換える。どちらも `App` に置くため、プロファイル切り替え (`AccountView` の `key={activeProfile}` による再マウント) とビュー切り替えをまたいで分割とペインのサービスが残る。これは現状の `activeService` の振る舞いと同じである。

- 却下：同じサービスを両ペインに表示できるようにする。Athena と BigQuery のエディタタブは `lib/queryEditorStorage.ts` の `scopedKey(service, scope, kind)` が作るサービスとスコープ単位の localStorage キーに保存され、同じサービスの 2 インスタンスが同じキーを上書きし合う。
- 却下：3 ペイン以上。要望は 2 つのサービスの同時参照で、ペインの幅も狭くなる (YAGNI)。
- 却下：2 つ目のペインの初期サービスを 1 つ目と同じにする。同じサービスの二重表示を禁じるため取れない。サイドバー順の先頭サービスなどを選ぶ規則は、利用者の意図と無関係なサービスの一覧取得 (AWS API 呼び出し) を起こす。

### 2. ペインの中身はペイン単位のコンポーネントに切り出す

`AccountView.tsx` のサービスごとの条件描画 (`ServicePanel` の 19 分岐と `AthenaView` / `CloudWatchLogsView` / `CostExplorerPanel` / `PricingPanel`) を `AwsServicePane` (同じファイル内) に移す。`AwsServicePane` は `service`、`profile`、`region`、`onRegionChange`、`drawerPos`、Drawer の `contained` と `closeOnEscape` (設計判断 5) を受け取り、`selectedId` を自分で持つ。サービス切り替え時に `selectedId` を `null` に戻す現在の `useEffect` も `AwsServicePane` に移す。`GcpView.tsx` も同じく `GcpServicePane` を切り出す。ペインごとにコンポーネントのインスタンスが分かれるため、フィルタ (`ServicePanel` / `GcpRowsPanel` の `filters`) と Drawer のタブ (`Drawer` の `tab`) は自然にペインごとになる。

`AccountViewProps` と `GcpViewProps` の `activeService` / `onServiceChange` を、`panes: SplitPanesState` と `onSelectService`、`onFocusPane`、`onClosePane` に置き換える (破壊的変更。呼び出し元は `App.tsx` とテストだけである)。

- 1 ペインのときの DOM は現状と同じにする (`.body` の直下に `Sidebar` とパネルの `.main` を置き、ラッパー要素を足さない)。既存のテストと CSS を分割なしの状態で変えないためである。
- 2 ペインのときは `.body` に `split` クラスを付け、各ペインを `div.pane` で包む。フォーカス中のペインは `div.pane.focused` にする。`div.pane` の `onPointerDownCapture` で `onFocusPane` を呼び、ペイン内のどこを操作してもフォーカスが移るようにする。
- 各 `div.pane` の先頭に `div.pane-bar` を置き、ペイン番号 (1 / 2) とペインを閉じるボタンを出す。
- サービスが `null` のペインは、サイドバーからサービスを選ぶよう促す文言を出す。

### 3. 分割の開始と終了は TopBar のボタンで行う

`TopBarProps` に省略可能な `split?: { active: boolean; onToggle: () => void }` を追加し、渡されたときだけ分割ボタンを出す。`App.tsx` は `view` が `aws` でセッションがある (`activeProfile` がある) とき、または `view` が `gcp` でセッションがある (`gcpProject` がある) ときだけ渡す。Datadog と TiDB のビューには出さない。

- 1 ペインで押すと `openSplit`、2 ペインで押すとフォーカスしていないペインを `closePane` で閉じる (見ている方のペインを残す)。
- `div.pane-bar` の閉じるボタンは、そのペインを `closePane` で閉じる。
- 却下：`Sidebar` の修飾キー付きクリックで右ペインに開く。利用者がフォーカス方式を選んだ。

### 4. Sidebar はフォーカス中のペインを基準に強調し、各ペインの表示中サービスに印を出す

`Sidebar` と `GcpSidebar` の `activeService` はフォーカス中のペインのサービスを受け取る (`null` のペインがフォーカス中なら `.nav-item.active` の項目は無い)。型を `string | null` に広げる。`onService` は `onSelectService` につなぐ。

省略可能な `paneServices?: (string | null)[]` を追加し、長さが 2 のとき、各ペインが表示中のサービスの `.nav-item` にペイン番号の印 (`span.pane-mark`) を出す。長さが 1 または未指定のときは印を出さない (現状と同じ描画)。

### 5. 分割中の Drawer はペインの中に収め、ESC はフォーカス中のペインだけが受け取る

`Drawer` に省略可能な props を 2 つ追加する。どちらも既定値で現状の振る舞いになる。

- `contained?: boolean` (既定値 `false`)：`true` のとき `.drawer` と `.drawer-backdrop` に `contained` クラスを付ける。`app.css` の `.drawer.contained` と `.drawer-backdrop.contained` は `position: absolute` とし、`.main` (既に `position: relative` と `overflow: hidden` を持つ) を包含ブロックにする。`.main` はターミナルドックより上にあるため、`bottom` は `--terminal-dock-h` を足さず 8px とする。幅と高さの既定値と上限は `vw` / `vh` ではなくペイン基準の `%` (幅 `min(600px, 85%)`、上限 85%。下配置の高さ `min(46%, 520px)`、上限 85%) にし、下配置の閉じ位置は `translateY(calc(100% + 16px))` にする。リサイズ (`startResize`) は `window.innerWidth` / `window.innerHeight` の代わりに包含ブロック (`.drawer` 要素の `offsetParent`) の `getBoundingClientRect()` を基準に位置と上限を計算する。
- `closeOnEscape?: boolean` (既定値 `true`)：`false` のとき document の `keydown` リスナーを登録しない。現状の `Drawer` は開いている間 document に ESC のリスナーを登録するため、2 ペインで両方の Drawer が開いていると ESC で両方が閉じる。

分割中は両ペインの `Drawer` に `contained` を渡し、`closeOnEscape` はフォーカス中のペインだけ `true` にする。1 ペインのときはどちらも渡さない (現状の `position: fixed` の Drawer のまま)。

- 却下：`position: fixed` のまま 2 つ目の Drawer をずらして重ねる。2 つの Drawer がビューポートの右端 (下配置では下端) で重なり、`.drawer-backdrop` (`position: fixed; inset: 0`) がもう一方のペインへのクリックを奪って Drawer を閉じる。
- 却下：`DrawerPos` に分割用の値を足す。`DrawerPos` は `TweaksPanel.tsx` で利用者が選ぶ好みであり、ペインに収めるかどうかは分割の有無で決まる別の軸である。
- 永続化された Drawer のサイズ (`loadDrawerSize` / `saveDrawerSize`) は両ペインで共有する。片方のペインでのリサイズはもう片方の Drawer に次のマウントまで反映されないが、どちらのサイズも `%` の上限で切られるため表示は崩れない。

### 6. レイアウト

`app.css` に次を追加する。

- `.body.split`：`grid-template-columns: var(--sidebar-w, 216px) minmax(0, 1fr) minmax(0, 1fr)`。
- `.pane`：`display: flex; flex-direction: column; min-width: 0; min-height: 0;`。2 つ目のペインに左の境界線を引く。`.pane > .main` は `flex: 1; min-height: 0;`。
- `.pane.focused`：アクセントカラー (`--accent`) の内側の枠線でフォーカス中を示す。
- `.pane-bar` と `.sidebar .nav-item .pane-mark`。

ペインの幅は 1:1 で固定する。

### 7. 文言

分割ボタンの `title` (`topbar` ネームスペース)、ペイン番号と閉じるボタンの `aria-label`、サービス未選択のペインの案内 (`app` ネームスペース) を `frontend/src/i18n/locales/ja/` と `locales/en/` の両方に追加する。

### 8. テスト

- `lib/splitPanes.test.ts` (新規)：設計判断 1 の 4 つの遷移の全分岐 (他ペイン表示中のサービスの選択でフォーカスだけが移ること、2 ペインでの `openSplit` と 1 ペインでの `closePane` が同じ参照を返すこと、`closePane` の添字 0 と 1、範囲外の `focusPane`) を固定する。
- `views/AccountView.test.tsx`：既存のテストを新しい props で通したうえで、分割時の描画 (2 つの `div.pane`、`.body.split`)、フォーカス中のペインへの `Sidebar` のクリックの反映、ペインの pointerdown によるフォーカスの移動、ペインごとの Drawer の独立 (片方のペインでの選択がもう片方に影響しないこと)、ESC でフォーカス中のペインの Drawer だけが閉じることを検証する。
- `views/GcpView.test.tsx` (新規)：`GcpView` について同じ分岐を検証する。
- `components/Drawer/Drawer.test.tsx`：`contained` のクラス付与、`closeOnEscape={false}` のとき ESC で `onClose` が呼ばれないこと、`contained` のリサイズが包含ブロックの矩形を基準に上限で切られることを検証する。
- `components/Sidebar.test.tsx`：`paneServices` の印の表示と非表示を検証する。
- `App.test.tsx`：分割ボタンが `aws` と `gcp` のビューにだけ出ること、押すと 2 ペインになり、もう一度押すとフォーカスしていないペインが閉じること、プロファイル切り替え後も分割が残ることを検証する。

## 方針からの乖離の記録 (2026-09-25)

### 設計判断 2 の変更: 1 ペインでも `div.pane` で包む (ユーザー確認済み)

implement-issues の Step 4 で、設計判断 2 の「1 ペインのときの DOM は現状と同じにする (ラッパー要素を足さない)」と、完了条件の「どちらの場合も残ったペインの選択リソースと Drawer の状態は保たれる」が両立しないことが分かった。

- 観測: `AccountView.test.tsx` のテスト「ペインの閉じるボタンでそのペインだけが閉じ、残ったペインの選択リソースと Drawer が保たれる」で、残ったペインのサービス (`h1` が S3) は保たれるが、`.drawer.open` が偽になった。
- 原因: 2 ペインから 1 ペインに戻ると、`.body` の子が `div.pane` の配列から `AwsServicePane` そのものに変わる。React は要素の型が変わった位置のサブツリーを作り直すため、`AwsServicePane` の `selectedId`、`Drawer` のタブ、フィルタが初期値に戻る。`GcpView` / `GcpServicePane` も同じ構造である。

2026-09-25 にユーザーが「常に `div.pane` で包む」案を選んだ。変更後の方式は次のとおり。

- `AccountView` と `GcpView` は、分割の有無にかかわらず、各ペインを `div.pane` で包んで描画する。1 ペインのときの `div.pane` は `display: contents` とし、レイアウトに影響させない。`app.css` に `.body > .main` のような直下の子を前提にするセレクタは無い (2026-09-25 に確認)。
- `div.pane-bar` は `div.pane` の最初の子として分割中だけ描画し、分割していないときは同じ位置を空け (`{split && ...}`)、後ろのペインの中身の位置を変えない。
- ペインの React の `key` はペインの添字ではなく、ペインごとに固定の識別子にする。左のペインを閉じると右のペインが添字 0 に移るため、添字を `key` にすると閉じたペインの状態が残ったペインに引き継がれる。設計判断 1 の `SplitPanesState` に `ids: number[]` (`services` と同じ長さ) を加え、`openSplit` は既存の識別子と重ならない値を追加し、`closePane` は残ったペインの識別子を保つ。
- 分割していないときの `onPointerDownCapture` と `focused` のクラスは付けない (1 ペインではフォーカスが常に 0 のため)。
- 完了条件の「分割していないときの DOM ... は変更前と同じ」の行は、この変更に合わせて「分割していないときは `display: contents` の `div.pane` が 1 つ増える以外の DOM、`Drawer` の配置 (`position: fixed`)、ESC の振る舞いが変更前と同じ」と読み替える。
- 却下: `selectedId` をペインの添字ごとに `AccountView` / `GcpView` へ持ち上げる案。選択リソースと Drawer の開閉は戻るが、Drawer のタブとフィルタは初期化されたままになる。
- 却下: 残ったペインのサービスだけを保ち、選択リソースと Drawer の初期化を許容する案。完了条件の「残ったペインの選択リソースと Drawer の状態は保たれる」を弱めることになる。

### 方針の方式を保った実装の詳細 (2026-09-25)

実装エージェントの報告に基づき、方針の方式を変えずに実装で決めた詳細を記録する。

- `openSplit` が追加する識別子は `Math.max(...ids) + 1` とした (`ids` が空なら 0)。採番用のカウンタを `SplitPanesState` に足す案は、状態のフィールドを増やすわりに得るものが無いため採らなかった。
- 1 ペインのとき、`AwsServicePane` / `GcpServicePane` へ渡す `contained` と `closeOnEscape` は `undefined` とした。`closeOnEscape={false}` を渡すと `Drawer` の既定値 (true) を打ち消して ESC で閉じなくなるため、未指定にして既定値に任せる (設計判断 5 の「1 ペインのときはどちらも渡さない」と同じ意味)。
- `contained` のリサイズは `Drawer` の包含ブロック (`offsetParent`、ペインの `.main`) の矩形を基準に計算する。`offsetParent` が無い環境 (jsdom) ではウィンドウの矩形にフォールバックする。
- 永続化された Drawer のサイズを読み込むときの丸めは、従来どおりウィンドウの大きさを基準にしたままとした。`contained` では CSS の `max-width: 85%` / `max-height: 85%` でペインの中に収める。設計判断 5 はリサイズの基準だけを定めており、永続化の丸めは変えていない。
- サービス未選択のペインの案内 (`div.main.pane-empty`) は、1 ペインでも `services[0]` が `null` なら出る。`App.tsx` は 1 ペイン目を `'ec2'` / `'cloudrun'` で初期化し、`closePane` は残ったペインのサービスを保つ。そのため 1 ペインで `null` になるのは、2 ペイン目をサービス未選択のまま残して 1 ペイン目を閉じた場合だけである。

## 完了条件

- `frontend/src/lib/splitPanes.ts` に `SplitPanesState` と `selectService` / `openSplit` / `closePane` / `focusPane` があり、設計判断 1 の規則どおりに遷移する。変化の無い遷移は引数と同じ参照を返す。
- `frontend/src/hooks/useSplitPanes.ts` があり、`App.tsx` の `activeService` / `activeGcpService` の `useState` が AWS 用と Google Cloud 用の `useSplitPanes` に置き換わっている。
- AWS と Google Cloud のビューで、`TopBar` の分割ボタンで 2 ペインにでき、左右のペインが別々のサービスを表示し、それぞれ独立に選択リソース、フィルタ、Drawer のタブを持つ。分割ボタンは Datadog と TiDB のビュー、およびセッションが無いときには出ない。
- 分割中、`Sidebar` / `GcpSidebar` のクリックはフォーカス中のペインのサービスを変える。もう一方のペインが表示中のサービスをクリックするとフォーカスがそのペインへ移り、どちらのペインのサービスも変わらない。ペイン内の pointerdown でフォーカスが移る。フォーカス中のペインは `div.pane.focused` になり、各ペインの表示中サービスの `.nav-item` にペイン番号の印が出る。
- 2 つ目のペインは開いた直後はサービス未選択で、案内の文言が出る。
- 分割中の `Drawer` は各ペインの `.main` の中に収まり (`.drawer.contained`)、一方のペインの Drawer や背景をクリックしてももう一方のペインの Drawer は閉じない。ESC はフォーカス中のペインの Drawer だけを閉じる。右配置と下配置の両方で、`contained` のリサイズがペインの矩形を基準に計算される。
- `div.pane-bar` の閉じるボタンでそのペインが閉じ、`TopBar` の分割ボタンを 2 ペインで押すとフォーカスしていないペインが閉じる。どちらの場合も残ったペインの選択リソースと Drawer の状態は保たれる。
- 分割していないときの DOM、`Drawer` の配置 (`position: fixed`)、ESC の振る舞いは変更前と同じで、既存のテストが (props の置き換えに伴う直接の修正を除き) 通る。
- `PersistedState` (`lib/storage.ts`) と `DrawerPos` (`types/common.ts`) と `TweaksPanel.tsx` が変更されていない。
- 新規の文言が `locales/ja/` と `locales/en/` の両方にある。
- 設計判断 8 のテストが追加されている。
- `mise run check` が通過する。
- 扱わない範囲：3 ペイン以上、上下の分割、ペイン幅のリサイズ、ペインごとのプロファイルとリージョン、同じサービスの両ペインでの表示、分割状態の永続化、Datadog と TiDB のビューの分割。

## 調査結果 (2026-09-16)

調査タスクの 1 つ目 (docs/issues/closed/0174 の close 後に分割表示が引き続き必要かを利用者に確認する) を実施した。
利用者の判断は「不要」である。docs/issues/closed/0174 のターミナルドック (接続を維持したまま他サービスへ遷移できる) で TODO 項目の用途は満たされた。
調査タスクの定めに従い、本 issue は pending に残す (close はしない)。レイアウトモデル以降の調査タスクは、分割表示が再び必要と判断された時点で行う。

## 備考

- グローバル規約により、pending の issue は修正せずそのまま残す (close しない)。調査タスクの実施と結果の追記は修正に当たらない。

## 関連

- docs/issues/0174: 同じ TODO 項目の子項目 (ターミナルの接続維持) を扱う。本 issue の判断は 0174 の close 後に行う。
- docs/issues/pending/0059、docs/issues/pending/0083: pending の調査 issue の前例。

## 解決方法

AWS と Google Cloud のビューに、左右 2 ペインの分割表示を追加した。方式は設計判断 1 から 8 と「## 方針からの乖離の記録 (2026-09-25)」のとおりである。

### 変更内容

- `frontend/src/lib/splitPanes.ts` (新規): `SplitPanesState` (`services`、`ids`、`focused`) と遷移の純関数 `selectService` / `openSplit` / `closePane` / `focusPane` を置いた。
  - `selectService` は、もう一方のペインが表示中のサービスならフォーカスだけを移し、それ以外はフォーカス中のペインのサービスを置き換える。
  - `openSplit` は 2 つ目のペインをサービス未選択 (`null`) と未使用の識別子で追加し、フォーカスを 2 つ目へ移す。
  - `closePane` は残ったペインのサービスと識別子を保ち、フォーカスを 0 にする。
  - 変化の無い遷移は引数と同じ参照を返す。
- `frontend/src/hooks/useSplitPanes.ts` (新規): 上記を `useReducer` で包むフック。初期状態は `{ services: [initialService], ids: [0], focused: 0 }`。
- `frontend/src/App.tsx`: `activeService` / `activeGcpService` の `useState` を `useSplitPanes('ec2')` / `useSplitPanes('cloudrun')` に置き換えた。`TopBar` の `split` prop は、セッションがある AWS / Google Cloud のビューにだけ渡す。2 ペインで分割ボタンを押すと、フォーカスしていないペインを `closePane` で閉じる。
- `frontend/src/components/TopBar.tsx`: 任意の `split` prop (`active` / `onToggle`) を追加し、渡されたときだけ分割ボタン (`Icons.split`、`aria-pressed`) を出す。`frontend/src/components/icons/Icons.tsx` に `split` アイコンを追加した。
- `frontend/src/views/AccountView.tsx`: 1 ペイン分の表示を `AwsServicePane` に切り出し、`selectedId` をペインごとに持たせた。`AccountView` は `panes` / `onSelectService` / `onFocusPane` / `onClosePane` を受け取る。
  - 各ペインを、分割の有無にかかわらず `div.pane` で包み、`key` に `panes.ids[index]` を使う。1 ペインのときは `div.pane.single` (`display: contents`) になる。
  - 分割中だけ `.body.split` と `div.pane-bar` (ペイン番号と閉じるボタン) を出す。フォーカス中のペインに `focused` を付け、`onPointerDownCapture` でフォーカスを移す。
  - サービス未選択のペインには `div.main.pane-empty` の案内を出す。
- `frontend/src/views/GcpView.tsx`: `GcpServicePane` を切り出し、`AccountView` と同じ構造にした。
- `frontend/src/components/Sidebar.tsx` / `frontend/src/views/GcpSidebar.tsx`: `activeService` を `string | null` にした。任意の `paneServices` を追加し、長さが 2 のときだけ、各ペインが表示中のサービスの `.nav-item` にペイン番号の印 (`span.pane-mark`) を出す。
- `frontend/src/components/Drawer/Drawer.tsx`: 任意の `contained` (既定 false) と `closeOnEscape` (既定 true) を追加した。
  - `contained` のとき、`.drawer` と `.drawer-backdrop` に `contained` を付ける。リサイズは包含ブロック (`offsetParent`) の矩形を基準に計算し、上限は矩形の 85% とする。
  - `closeOnEscape` が false のときは ESC のリスナーを登録しない。
- `frontend/src/app.css`: `.body.split`、`.pane`、`.pane.single`、`.pane.focused`、`.pane-bar`、`.main.pane-empty`、`.sidebar .nav-item .pane-mark`、`.topbar .iconbtn.active`、`.drawer.contained`、`.drawer-backdrop.contained` を追加した。
- `frontend/src/i18n/locales/{ja,en}/app.json` に `panes.label` / `panes.close` / `panes.empty`、`locales/{ja,en}/topbar.json` に `split` を追加した。
- `frontend/src/views/CostExplorerPanel.test.tsx`: `AccountView` の props の置き換えに合わせて、`activeService` / `onServiceChange` を `panes` などに直した。
- `frontend/src/app.css.test.ts`: `.pane.single` の `display: contents` を規則のテキストで固定するテストを追加した。

### 完了条件の検証

- 遷移の純関数: `lib/splitPanes.test.ts` の 17 件 (`selectService` 7 件、`openSplit` 4 件、`closePane` 4 件、`focusPane` 2 件) が、各遷移の結果と、変化の無い遷移が同じ参照を返すことを検証している。範囲外と負のフォーカス、空の `ids` に対する防御の分岐も含む。
- フックと `App.tsx` の置き換え: `hooks/useSplitPanes.ts` があり、`App.tsx` が `useSplitPanes('ec2')` / `useSplitPanes('cloudrun')` を使う。`App.test.tsx` の「分割ボタンで 2 ペインになり、もう一度押すとフォーカスしていないペインが閉じる」「プロファイルを切り替えても分割と各ペインのサービスが残る」で検証した。
- 2 ペイン、独立した選択リソース・フィルタ・Drawer のタブ、分割ボタンの出し分け: `AccountView.test.tsx` / `GcpView.test.tsx` の「ペインごとに選択リソースと Drawer を持ち、片方の背景クリックでもう片方は閉じない」「ペインごとにフィルタと Drawer のタブが独立する」で検証した。`TopBar` の分割ボタンで 2 ペインになることは、AWS を `App.test.tsx` の「分割ボタンで 2 ペインになり、もう一度押すとフォーカスしていないペインが閉じる」、Google Cloud を「Google Cloud のビューでも分割ボタンで 2 ペインになり、もう一度押すとフォーカスしていないペインが閉じる」で検証した。ボタンの出し分けは「分割ボタンは AWS と Google Cloud のビューにだけ出る」「セッションが無いビューには分割ボタンを出さない」で検証した。
- サイドバーとフォーカス: `AccountView.test.tsx` / `GcpView.test.tsx` の「サイドバーのクリックはフォーカス中のペインのサービスだけを変える」「ペイン内の pointerdown でフォーカスが移り、もう一方のペインが表示中のサービスのクリックではフォーカスだけが移る」、`Sidebar.test.tsx` の「paneServices の長さが 2 のとき、各ペインが表示中のサービスに番号の印が出る」「paneServices が未指定または長さ 1 のときは印を出さない (分割なしの現状の描画)」で検証した。
- 2 つ目のペインの案内: `AccountView.test.tsx` / `GcpView.test.tsx` の「分割を開始すると 2 ペインになり、2 つ目のペインはサービス未選択の案内を出してフォーカスする」で検証した。サービス未選択のペインを残して 1 ペインに戻ったときの案内とサイドバーでの復帰は、`AccountView.test.tsx` の「サービス未選択のペインを残して左のペインを閉じると 1 ペインで案内を出し、サイドバーの選択で復帰する」で検証した。
- 分割中の Drawer と ESC: `AccountView.test.tsx` / `GcpView.test.tsx` の「ペインごとに選択リソースと Drawer を持ち、片方の背景クリックでもう片方は閉じない」(両ペインの `.drawer` に `contained` が付くことを含む)、「ESC はフォーカス中のペインの Drawer だけを閉じる」、`Drawer.test.tsx` の「contained を渡すと .drawer と .drawer-backdrop に contained が付き、既定値では付かない」「closeOnEscape=false のとき ESC で onClose を呼ばない (フォーカスしていないペイン用)」と、右配置と下配置それぞれの包含ブロック基準の計算と 85% 上限の 4 件で検証した。
- ペインを閉じたときの状態の維持: `div.pane-bar` の閉じるボタンで左のペインを閉じる場合を、`AccountView.test.tsx` / `GcpView.test.tsx` の「左のペインを閉じても、残ったペインの選択リソース・Drawer・タブ・フィルタが保たれる」で検証した。`TopBar` の分割ボタンで右のペインを閉じる場合を、`App.test.tsx` の「TopBar の分割ボタンで右のペインを閉じても、残った左のペインの選択リソースと Drawer が保たれる」(選択リソース、Drawer の開閉、Drawer のタブ) で検証した。
- 分割していないときの DOM と振る舞い: 「## 方針からの乖離の記録 (2026-09-25)」の読み替えのとおり、`display: contents` の `div.pane.single` が 1 つ増える以外は変更前と同じである。`AccountView.test.tsx` / `GcpView.test.tsx` の「分割していないときは .body に split が付かず、ラッパーが display: contents の .pane.single になる」で、`.pane.single` のクラス、`focused` と `pane-bar` が無いこと、`.drawer` に `contained` が付かないこと、サイドバーにペイン番号の印が出ないこと (`GcpView.test.tsx`。`Sidebar` 側は `Sidebar.test.tsx`) を検証した。`.pane.single` の `display: contents` は jsdom が解釈しないため、`app.css.test.ts` の「.pane.single は display: contents で .body のグリッドに影響させない」で規則のテキストを固定した。`Drawer.test.tsx` の既存の ESC と配置のテストは変更なしで通る。
- `lib/storage.ts`、`types/common.ts`、`components/TweaksPanel.tsx` は変更していない (`git status` に含まれない)。
- 新規の文言は `locales/ja/` と `locales/en/` の両方の `app.json` と `topbar.json` にある。
- 設計判断 8 のテスト: 上記のテストを `lib/splitPanes.test.ts`、`views/AccountView.test.tsx`、`views/GcpView.test.tsx` (新規)、`components/Drawer/Drawer.test.tsx`、`components/Sidebar.test.tsx`、`App.test.tsx`、`app.css.test.ts` に追加した。
- `mise run check`: exit 0。frontend は 100 ファイル / 1074 件が pass し、backend は全パッケージが ok。lint は 0 errors / 9 warnings で、warning は既存の行だけである。ベースライン (失敗なし) から新たな失敗は無い。
- 扱わない範囲: 3 ペイン以上、上下の分割、ペイン幅のリサイズ、ペインごとのプロファイルとリージョン、同じサービスの両ペインでの表示、分割状態の永続化、Datadog と TiDB のビューの分割は実装していない。

### 方針からの乖離

- 設計判断 2 の「1 ペインのときはラッパー要素を足さない」を、ユーザーの確認を得て「1 ペインでも `display: contents` の `div.pane` で包む」に変えた。ペインの `key` には添字ではなく `SplitPanesState.ids` の識別子を使う。理由は「## 方針からの乖離の記録 (2026-09-25)」に書いた。
- 方式を変えない実装の詳細は「### 方針の方式を保った実装の詳細 (2026-09-25)」に書いた。

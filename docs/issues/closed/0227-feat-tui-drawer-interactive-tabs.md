# TUI の Drawer に一覧と詳細を持つ対話型タブを追加する

Created: 2026-10-26
Model: DeepSeek V4.1 Flash 1017
Completed: 2026-10-27

## 背景

`docs/issues/TODO.md` の次の項目に由来する。

> TUI の Drawer に一覧と詳細を持つサービス固有タブを追加し、ELB の Listeners / Targets (Rules / Health)、WAF の Rules の JSON、ECS の Tasks / Instances、RDS の Instance Parameters のパラメータグループ切替を表示できるようにしたい

issue 0226 は Drawer のタブ基盤を追加するが、タブ本文は読み取り専用の表とテキストだけで、行の選択、詳細ペイン、子リソースの取得、フォーム入力を持たない。Web UI の次のタブは、一覧の行を選ぶと詳細を表示するか、パラメータグループを切り替える操作を持つ。

- `frontend/src/components/Drawer/DrawerECSTasks.tsx` はタスク一覧と、選択したタスクの詳細（コンテナ一覧を含む）を表示し、コンテナごとに ECS Exec のボタンを持つ。
- `frontend/src/components/Drawer/DrawerECSContainerInstances.tsx` はコンテナインスタンスごとにタスクをまとめて表示する。
- `frontend/src/components/Drawer/DrawerELBListeners.tsx` はリスナー一覧と、選択したリスナーの Rules を表示する。
- `frontend/src/components/Drawer/DrawerELBTargets.tsx` はターゲットグループ一覧と、選択したグループの Target health を表示する。
- `frontend/src/components/Drawer/DrawerWAFRules.tsx` はルール一覧と、選択したルールの JSON を表示する。
- `frontend/src/components/Drawer/DrawerRDSInstanceParameters.tsx` は複数の DB パラメータグループを切り替えてパラメータを表示する。

これらの操作を TUI に足すため、issue 0226 の `Tab` サブモデルの上に、一覧と詳細を持つ共通タブを追加する。

## 目的

TUI の Drawer で ECS の Tasks / Instances、ELB の Listeners / Targets、WAF の Rules、RDS の Instance Parameters を、Web UI と同じ単位で確認できるようにする。ECS のタスク詳細からは選択中のコンテナへ ECS Exec を開始できるようにする。

## 設計判断

- 一覧と詳細を持つタブは `listDetailTab` という共通の `Tab` 実装で扱う。
  - 一覧は `bubbles/table`、詳細は `viewport` または選択可能な `table` で描画する。
  - 一覧で `enter` を押すと、選択中の行の詳細を開いて詳細へフォーカスを移す。`esc` で詳細を閉じて一覧へ戻る。
  - 詳細を開いている間も一覧は上部に残し、一覧の選択行を強調する。詳細は一覧の下に表示する。
  - 詳細を非同期で取得する場合は、開いた時点で取得を開始し、読み込み中、失敗、成功の状態を区別する。詳細の取得結果は `detailSeq` で世代管理し、一覧の選択が変わった後、`esc` で詳細を閉じた後、別の行の詳細を開き直した後、タブが作り直された後の結果を捨てる。詳細を開くときと閉じるときの両方で `detailSeq` を進める。
  - 詳細内で選択可能な表を持つ場合は、`enter` または `s` で選択行に対する `TabIntent` を返す。ECS のコンテナ一覧がこれに当たる。
  - 詳細を一覧の下に置く案を採用する。詳細で一覧を置き換える案は、詳細を見ながら別の行を選び直すときに `esc` で戻る操作が毎回必要になり、Web UI の「一覧の下に詳細」という構成とも異なる。
  - 一覧と詳細の共通実装 `listDetailTab` を使う。タブごとに一覧と詳細の描画を書き分ける案は、行選択、フォーカス、詳細の世代管理を 6 タブで重複させる。ECS Instances と RDS Instance Parameters は一覧の作り方が異なるため、共通実装を土台にした専用のタブにする。
- ECS Tasks タブは `aws.ListECSTasks(ctx, profile, region, clusterName, "")` の結果を使う。
  - 一覧の列は Web UI の `ecsTaskColumns` と同じ Group、Containers、Last status、Desired status、Launch type、Exec enabled、ARN とする。
  - 詳細には ARN、Group、Last status、Desired status、Launch type、CPU、Memory、Started at、Stopped at、Stopped reason と、`ECSTaskResource.Containers` の Name、Image、Last status、Health、CPU、Memory、Exit code、Reason、Exec を表示する。
  - コンテナの CPU が `0` のときは `-`、Memory は hard と soft のうち未指定でない値を ` / ` で連結し、両方未指定なら `-`、Exit code が nil なら `-`、Reason が空なら `-` にする。コンテナが 0 件なら `No containers` を表示する。表示規則は `frontend/src/components/Drawer/DrawerECSTasks.tsx` の `formatContainerMemory` と `isUnsetResource` に合わせる。
  - 詳細のコンテナ行で `s` または `enter` を押すと、`TabIntent` でクラスタ名、タスク ID（ARN の最後の `/` より後ろ）、コンテナ名を返す。Model はその intent で既存の ECS Exec のフロートセッションを開始する。
  - ECS Exec が無効なコンテナでは開始せず、通知行に `container <name> does not support ECS Exec` を表示する。`ECSTaskResource.Containers` の `ExecEnabled` を使う。
- ECS Instances タブは `aws.ListECSContainerInstances(ctx, profile, region, clusterName)` と `aws.ListECSTasks(ctx, profile, region, clusterName, "")` を使う。
  - Web UI の `DrawerECSContainerInstances.tsx` はコンテナインスタンスの一覧表ではなく、インスタンスごとの `InstanceSection` とタスク表を並べる。TUI では一覧をインスタンスの要約（EC2 インスタンス ID、Status、Agent connected、Running tasks、Pending tasks、CPU の remaining / registered、Memory の remaining / registered）とし、詳細を Web UI の `InstanceSection` の KV とタスク表に合わせる。
  - 詳細のタスク表の列は Web UI の `TaskTable` と同じ Group、Last status、CPU、Memory、Started at とする。`ecsTaskColumns` は使わない。該当タスクが無ければ `No tasks` を表示する。
  - `ContainerInstanceArn` が空の Fargate のタスクは、Web UI の `groupTasksByContainerInstance` と同じく一覧にも詳細にも表示しない。`ContainerInstanceArn` が空でなく、一覧のどのインスタンスにも一致しないタスクは、`Unknown container instance` の行にまとめて表示する。
  - このタブの詳細からは Exec を開始しない。Web UI の Instances タブにも Exec の導線が無いためである。
- ELB Listeners タブは `aws.ListELBListeners(ctx, profile, region, loadBalancerArn)` を使う。
  - 一覧の列は `elbListenerColumns` と同じ Protocol、Port、Default action、Default target group、ARN とする。リスナーが 0 件なら一覧に `No listeners` を表示する。
  - 詳細は `aws.ListELBRules(ctx, profile, region, listenerArn)` を呼び、`elbRuleColumns` と同じ Priority、Conditions、Action、Target group、ARN を表示する。ルールが 0 件なら `No rules` を表示する。
- ELB Targets タブは `aws.ListELBTargetGroups(ctx, profile, region, loadBalancerArn)` を使う。
  - 一覧の列は `elbTargetGroupColumns` と同じ Name、Protocol、Port、Target type、Health check、VPC、ARN とする。ターゲットグループが 0 件なら一覧に `No target groups` を表示する。
  - 詳細は `aws.DescribeELBTargetHealth(ctx, profile, region, targetGroupArn)` を呼び、`elbTargetHealthColumns` と同じ Target、Port、State、AZ、Reason、Description を表示する。ターゲットが 0 件なら `No targets` を表示する。
- WAF Rules タブは `aws.ListWAFRules(ctx, profile, region, scope, name, id)` を使う。
  - 一覧の列は `wafRuleColumns` と同じ Name、Priority、Action、Statement とする。
  - 詳細は `WAFRule.RuleJSON` を整形したテキストを `viewport` に表示する。JSON の整形は `encoding/json` の `Indent` を使い、失敗したときは元の文字列を表示する。
- RDS Instance Parameters タブは `RDSResource.ParameterGroups` を一覧にし、選択中のグループのパラメータを `aws.ListRDSParameters(ctx, profile, region, groupName)` で取得する。
  - パラメータグループが 1 つ以上あるときは、タブ本文の先頭に `Group: <name>` を出し、2 つ以上あるときは `Group: <name> (1/2)` のように現在位置を添える。`←` / `→` で切り替え、先頭で `←`、末尾で `→` を押したときは止まる。グループを切り替えたら取得をやり直し、古い結果を捨てる。
  - グループが 0 個のときは `No parameter groups.` を表示する。
  - 列は `rdsParameterColumns` と同じ Name、Value、Apply type、Data type、Modifiable、Source とする。
- 子リソースの取得は `errgroup` を使わず、詳細を開いたときの 1 リクエストに限定する。一覧の全行について子リソースを先回りして取得する案は、リスナーやターゲットグループが多いときに API 呼び出しが増えるため採らない。
- 詳細の取得中に一覧の選択を変える操作はできない（詳細にフォーカスがあるため）。`esc` で一覧へ戻ってから選択を変える。これにより、詳細を取得する対象と表示中の行がずれない。
- 行の選択と詳細のフォーカスはタブ内の状態で持つ。Model に選択行を持たせる案は、タブごとのキー操作と状態が Model に集中するため採らない。
- 追加の API 呼び出しは `ListECSTasks`、`ListECSContainerInstances`、`ListELBListeners`、`ListELBRules`、`ListELBTargetGroups`、`DescribeELBTargetHealth`、`ListWAFRules`、`ListRDSParameters`、`ExecuteCommand`、`TerminateSSMSession` で、Web UI と CLI が既に使う権限の範囲に収まる。新しい IAM 権限は要らない。
- ECS の Exec は issue 0225 のフロートセッションを使う。この issue は issue 0225 と issue 0226 の完了後に実装する。0225 が未完了の状態で `TabIntent` だけを追加するフォールバックは採らない。完了条件がセッション開始の動作を含むため、依存が満たされないまま close できてしまう形にしない。
- ECS Tasks タブから Exec を開始する経路を足すため、PRD の FR-30 と「決定した論点」の「TUI の ECS のタスクとコンテナの選び方」を更新する。クラスタ行の `s` と選択肢による接続に加え、Tasks タブのタスク詳細から選択中のコンテナへ接続できることを決定に含める。従来の「Drawer のタブで選ぶ案は採らない」という記述と、「TUI の Drawer を Overview と Tags に固定しており」という前提は、この issue の決定で置き換える。
- この issue では S3 の階層移動、DynamoDB の検索フォーム、Value の表示、列ごとのフィルタ、リソースの変更操作は扱わない。

## 完了条件

- `backend/internal/tui` に一覧と詳細を持つ共通タブの実装があり、ECS Tasks / Instances、ELB Listeners / Targets、WAF Rules、RDS Instance Parameters の 6 タブがその実装または専用の実装で追加されている。
- ECS Tasks タブで行を選んで `enter` を押すと、タスクの詳細とコンテナ一覧が表示され、`esc` で一覧に戻る。Exec が有効なコンテナ行で `s` または `enter` を押すと、クラスタ名、タスク ID、コンテナ名を持つ `TabIntent` が Model に渡り、issue 0225 のフロートセッションが開始される。
- ECS Tasks タブのコンテナ一覧で、CPU が `0` のときは `-`、Memory は hard と soft の未指定でない値を ` / ` で連結し、両方未指定なら `-`、Exit code が nil なら `-`、Reason が空なら `-` になる。コンテナが 0 件なら `No containers` が表示される。
- ECS Tasks タブで Exec が無効なコンテナ行に対して `s` または `enter` を押してもセッションを開始せず、通知行に `container <name> does not support ECS Exec` が表示される。
- ECS Instances タブでコンテナインスタンスを選ぶと、そのインスタンスのタスクだけが Group、Last status、CPU、Memory、Started at の表で詳細に表示される。タスクが無いインスタンスでは `No tasks` が表示される。
- ECS Instances タブで `ContainerInstanceArn` が空の Fargate のタスクは表示されない。`ContainerInstanceArn` が空でなく、どのインスタンスにも一致しないタスクがあるとき、`Unknown container instance` の行にまとめて表示される。
- ELB Listeners タブで `ListELBListeners` が 0 件のとき、一覧に `No listeners` が表示される。リスナーを選んで `enter` を押すと、`ListELBRules` の結果が Priority、Conditions、Action、Target group、ARN の表で表示される。ルールが 0 件なら `No rules` が表示される。取得失敗時は詳細に理由が表示され、一覧は残る。
- ELB Targets タブで `ListELBTargetGroups` が 0 件のとき、一覧に `No target groups` が表示される。ターゲットグループを選んで `enter` を押すと、`DescribeELBTargetHealth` の結果が Target、Port、State、AZ、Reason、Description の表で表示される。ターゲットが 0 件なら `No targets` が表示される。取得失敗時は詳細に理由が表示され、一覧は残る。
- WAF Rules タブでルールを選んで `enter` を押すと、ルールの JSON がインデントされたテキストで表示される。JSON の整形に失敗したときは元の文字列が表示される。`RuleJSON` が空のときは `—` が表示される。
- RDS Instance Parameters タブでパラメータグループが 1 つのときは `Group: <name>` を表示し、2 つ以上のときは `Group: <name> (1/2)` のように現在位置を表示する。`←` / `→` で切り替わり、先頭と末尾では止まる。選択したグループのパラメータが Name、Value、Apply type、Data type、Modifiable、Source の表で表示される。グループが 0 個のときは `No parameter groups.` が表示される。
- 詳細の取得結果が、一覧の選択変更、`esc` で詳細を閉じた後、別の行の詳細を開き直した後、タブ切替、行変更、`r` の再取得の後に届いた場合、その結果を表示にも保持にも使わない。
- 一覧と詳細の表は Drawer の内側の幅に収まり、列の見出しと値が Web UI の同じ列定義の表示文字列と一致する。ECS Instances は Web UI に対応する一覧表が無いため、一覧の要約は TUI 独自とし、詳細の KV とタスク表の値を Web UI の `InstanceSection` と `TaskTable` に合わせる。`minColumnWidth` まで縮めても収まらないときは右端の列を切り落とし、末尾の `…` で示す。
- `backend/internal/tui` の単体テストに、各タブの一覧取得、ELB の一覧が 0 件のときの `No listeners` / `No target groups`、詳細の開閉、詳細の読み込み中と失敗、`esc` で閉じた後の古い詳細結果の破棄、ECS コンテナの Exec 対象と無効時の通知、ECS コンテナの表示規則、ECS Instances のタスクの絞り込みと `Unknown container instance`、RDS の 1 グループ表示、2 グループの切替、グループ切替後の古い結果の破棄、先頭と末尾での停止の各ケースがある。
- `backend/internal/cli/tui.go` の `newTUIDeps` が上記の取得関数を各サービスのコンストラクタへ渡していることを `backend/internal/cli/tui_test.go` で確認している。
- `docs/prd/thief.md` の FR-30 と「決定した論点」の「TUI の ECS のタスクとコンテナの選び方」が、クラスタ行の `s` と選択肢による接続に加えて、ECS Tasks タブのタスク詳細から選択中のコンテナへ接続できることを含む形に更新されている。FR-30 の受け入れ基準に、ECS Tasks タブのタスク詳細で Exec が有効なコンテナを選び、`s` または `enter` でフロートセッションを開始して `echo ok` を確認する手順が追加されている。従来の「Drawer のタブで選ぶ案は採らない」という記述と、「TUI の Drawer を Overview と Tags に固定しており」という前提は残っていない。
- 実 AWS アカウントで、ECS Exec を有効にしたタスクのコンテナを ECS Tasks タブから選ぶと、フロートウィンドウにシェルが表示され、`echo ok` が `ok` を返す。`Ctrl-]` の後に `aws ssm describe-sessions --state Active` にセッションが残らない。
- 実 AWS アカウントで、ELB の Listeners / Targets、WAF の Rules の詳細が、上記の列と表示文字列の基準で Web UI と同じ内容になる。
- `CHANGES.md` の `## develop` に、TUI の対話型 Drawer タブの [ADD] エントリが 1 つある。担当者行は `- @sfuruya0612` とする。
- `mise run check` が通る。
- 扱わない範囲: S3 の階層移動、DynamoDB の検索フォーム、Value の表示、列ごとのフィルタ、リソースの変更操作、ECS Instances からの Exec。

## 関連

- PRD `docs/prd/thief.md` の FR-29、FR-30
- `docs/issues/0225`、`docs/issues/0226`
- `frontend/src/components/Drawer/DrawerECSTasks.tsx`、`DrawerECSContainerInstances.tsx`、`DrawerELBListeners.tsx`、`DrawerELBTargets.tsx`、`DrawerWAFRules.tsx`、`DrawerRDSInstanceParameters.tsx`
- `backend/internal/aws/ecs_exec.go`、`elb.go`、`waf.go`、`rds.go`

## 解決方法

- `backend/internal/tui/tab_list.go` に一覧と詳細を持つ `listDetailTab` を追加した。`enter` で詳細を開き、`esc` で一覧へ戻る。詳細は即時作成と非同期取得の両方に対応し、`detailSeq` で古い結果を捨てる。Drawer の高さに収まるよう一覧を 1/4、詳細を 3/4 に配分し、選択可能な詳細では KV の行数を制限してコンテナ表のヘッダと 1 行以上が見えるようにした。
- ECS の Tasks タブを追加した。タスクの詳細にコンテナの CPU、Memory、Exit code、Reason、ECS Exec の可否を表示し、Exec が有効なコンテナで `s` または `enter` を押すと `TabIntent{ECSExec}` を返す。Exec が無効なコンテナでは `container <name> does not support ECS Exec` を通知する。コンテナが 0 件なら `No containers` を表示する。
- ECS の Instances タブを追加した。コンテナインスタンスごとにタスクをまとめ、`ContainerInstanceArn` が空の Fargate のタスクを除き、一致しないタスクを `Unknown container instance` にまとめた。タスクが無いインスタンスでは `No tasks` を表示する。詳細の Agent と ECS reported の表記を Web UI に合わせ、このタブからは Exec を開始しない。
- ELB の Listeners タブと Targets タブを追加した。一覧が 0 件のときは `No listeners` と `No target groups`、詳細が 0 件のときは `No rules` と `No targets` を表示する。詳細の取得に失敗したときは理由を表示し、一覧は残す。
- WAF の Rules タブを追加した。`RuleJSON` を `json.Indent` で整形し、整形できないときは元の文字列、空のときは `—` を表示する。
- RDS の Instance Parameters タブを `rdsInstanceParametersTab` として追加した。`Group: <name>` と `(1/2)` の現在位置を表示し、`←` / `→` で切り替え、端で止まる。グループを切り替えたら取得をやり直し、古い結果を捨てる。グループが 0 個なら `No parameter groups.`、パラメータが 0 件なら `No parameters.` を表示する。
- `startECSExecTarget` に開始中とセッション中のガードを追加し、Tasks タブから `s` を二度押してもセッションを二重に開始しないようにした。
- `backend/internal/tui` の単体テストに、各タブの一覧取得と詳細の開閉、空と失敗、古い結果の破棄、ECS の表示規則と Exec の対象 / 無効時の通知、ECS Instances の絞り込みと `Unknown container instance`、ELB の空状態、WAF の JSON 整形、RDS のグループ切替と端での停止、Tasks タブから `OpenECSExecSession` が 1 回だけ呼ばれることを追加した。80 桁 x 24 行でタスク詳細とコンテナ行が Drawer に収まることも確認している。
- `docs/prd/thief.md` の FR-29、FR-30 と「決定した論点」を更新し、`CHANGES.md` に [ADD] エントリを追加した。
- 実 AWS アカウントで、ECS Tasks タブのタスク詳細から ECS Exec が有効なコンテナを選んで接続し、`echo ok` が返ること、`Ctrl-]` の後に `aws ssm describe-sessions --state Active` にセッションが残らないことを確認した。Exec が無効なコンテナでは接続しないことも確認した。
- 実 AWS アカウントで、ELB の Listeners / Targets と WAF の Rules の詳細が Web UI と同じ内容になることを確認した。
- `mise run check` が通る。

# TUI の ECS の一覧から ECS Exec でタスクのコンテナに接続する

Created: 2026-10-25
Model: Claude Fable 5.1
Completed: 2026-10-26

## 背景

`docs/issues/TODO.md` の次の項目に由来する。

> TUI から ECS Exec でタスクのコンテナに接続できるようにしたい (docs/prd/thief.md の FR-30。`backend/internal/tui/model.go` の `startSession` は `Service.SupportsSession` が真のサービスだけを扱い、EC2 の Session Manager しか実装が無い。クラスタ、タスク、コンテナの選び方は実装時に決める)

現状の実装は次のとおり。

- `backend/internal/tui/model.go` の `Deps.StartSession` は `func(ctx context.Context, profile, region, target string) (*exec.Cmd, func() error, error)` で、`target` は 1 本の文字列である。`startSession` は `Service.SupportsSession()` が偽なら `does not support sessions`、`Deps.StartSession` が nil なら `sessions are not available` を通知し、それ以外では `m.deps.StartSession(ctx, profile, region, row.ID())` を呼ぶ。`sessionReadyMsg` で `tea.ExecProcess` が子プロセスを実行し、`sessionExitMsg` で `finalize` を呼ぶ。
- `backend/internal/cli/tui.go` の `sessionManagerCommand` は EC2 専用で、`awsinternal.StartSSMSession(ctx, profile, region, target)` を呼び、`{"Target": target}` と `https://ssm.<region>.amazonaws.com` で `session-manager-plugin` の引数を組み立てる。
- CLI の `thief ecs exec` (`backend/internal/cli/ecs.go` の `ecsExecuteCommandWith`) は `--cluster`、`--task`、`--container` を必須のフラグで、`--command` (既定 `/bin/sh`) を任意のフラグで受け取り、`awsinternal.ExecuteECSCommandSession(ctx, profile, region, cluster, task, container, command)` (`backend/internal/aws/ecs_cli.go`) で `ecs.ExecuteCommand` (`Interactive: true`) を呼ぶ。返る `ECSExecSession` の `Target()` が `ecs:<cluster>_<task-id>_<runtime-id>` を `ClusterArn`、`TaskArn`、`ContainerArn` から組み立て、`{"Target": ...}` と `https://ecs.<region>.amazonaws.com` でプラグインを起動し、終了後に `awsinternal.TerminateSSMSession` で切断する。プラグインの引数の並び (7 引数) と `sessionManagerSessionJSON` (`backend/internal/cli/session_plugin.go`) は EC2 と共通で、違うのは開始 API、`Target` の文字列、エンドポイントの 3 点である。`ecsExecuteCommandWith(cmd, deps)` は `ecsExecSessionDeps` (`executeSession`、`lookupPlugin`、`execPlugin`、`terminateSession`) で外部処理を差し替えられ、`backend/internal/cli/run_test.go` の `context.Background()` を許す関数の一覧には `ecs.go:ecsExecuteCommandWith` が載っている。
- Web UI の ECS の一覧はクラスタ行 (`backend/internal/aws/ecs.go` の `ListECSResources`、`ECSResource` の `ID` は `ClusterArn`、`Name` は `ClusterName`) である。接続は Drawer の Terminal タブ (`frontend/src/components/Drawer/DrawerTerminal.tsx` の `ECSExecLauncher`) が行い、`useECSTasks` でタスクを、`useECSContainers` で `execEnabled` が真のコンテナを取り、1 件なら自動で選び、複数なら `<select>` で選ばせる。接続先は `awsinternal.ListECSTasks(ctx, profile, region, cluster, service)` (`backend/internal/aws/ecs_exec.go`、`service` は空でよい) と `awsinternal.ListECSContainers(ctx, profile, region, cluster, task)` (同じファイル、`ECSContainerResource` は `Name`、`RuntimeID`、`ExecEnabled` を持つ) で取る。Web UI は `cluster` に `resource.name` (クラスタ名。`/` を含む ARN はパスのセグメントに使えない) を、`task` に `arnSuffix` (タスクの ARN の最後の `/` より後ろのタスク ID) を渡し、タスクが 0 件のときは `noRunningTasks` の文言を出す。
- TUI の ECS の一覧 (`Service` と `Row`) は issue 0219 で足す。その行は Web UI と同じクラスタ行である。

`Deps.StartSession` の `target` 1 本では ECS Exec の接続先 (クラスタ、タスク、コンテナ) を渡せず、クラスタ行からタスクとコンテナを選ぶ画面も無いため、TUI から ECS Exec を開始できない。

## 目的

TUI の ECS の一覧でクラスタの行を選んで `s` を押すと、タスクとコンテナを選んで ECS Exec のシェルに入れる。
既定のコマンドは CLI と Web UI と同じ `/bin/sh` とする。

## 設計判断

- issue 0219 は ECS の `Service.SupportsSession()` を `false` で入れる。この issue でそれを `true` にし、`startSession` が `Service.ID()` が `ecs` のときにこの issue の経路へ分岐するようにする。
- タスクとコンテナは、`backend/internal/tui/picker.go` の選択肢 (絞り込み付き) で順に選ぶ。クラスタ行で `s` を押すと `ListECSTasks` でタスクを取り、1 件なら自動で選び、複数なら選択肢を出す。選んだタスクの `ListECSContainers` から `ExecEnabled` が真のコンテナだけを候補にし、同じく 1 件なら自動で選ぶ。これは Web UI の `ECSExecLauncher` と同じ選び方である。PRD の「未確定論点」の「TUI の ECS のタスクとコンテナの選び方」は「ECS Exec の issue の実装時に開発者が決める」としているので、この方式を第一候補とし、実装時に開発者が確認したうえで PRD の「決定した論点」に決めた日付で追記する。
  - Drawer に Services と Tasks のタブを足して選ばせる案は採らない。TUI の Drawer は Overview と Tags の 2 タブに固定しており (`drawerTab`)、サービス固有のタブを置くかは PRD の別の未確定論点である。Web UI でも接続先の選択は Terminal タブの中の `<select>` であり、タブで行を選ぶ形ではない。
  - ECS の一覧をタスク行にして `s` で直接つなぐ案は採らない。Web UI の一覧はクラスタ行であり、FR-29 の列の一致 (`ecsColumns` の `Cluster` 以下) を崩す。
- `Deps.StartSession` の `target` 文字列に `ecs:<cluster>_<task-id>_<runtime-id>` を入れて流用する案は採らない。`ExecuteECSCommandSession` はクラスタ、タスク、コンテナを別々の引数で受け取り、`Target()` は `ExecuteCommand` の応答から組み立てるので、Model が持つ 3 つの値を 1 本の文字列に詰めて `cli/tui.go` で分解し直す往復が増えるだけで、`target` が EC2 のインスタンス ID と ECS の合成文字列の二重の意味を持つことになる。
- `tui.Deps` に ECS 用の関数を足す: `ListECSTasks func(ctx context.Context, profile, region, cluster, service string) ([]aws.ECSTaskResource, error)` (`awsinternal.ListECSTasks` と同じシグネチャ。TUI は `service` に空文字列を渡す)、`ListECSContainers func(ctx context.Context, profile, region, cluster, task string) ([]aws.ECSContainerResource, error)`、`StartECSExec func(ctx context.Context, profile, region, cluster, task, container, command string) (*exec.Cmd, func() error, error)`。`cluster` には `Row.Name()` (クラスタ名) を、`task` には `ECSTaskResource.ARN` の最後の `/` より後ろ (タスク ID) を、`container` には `ECSContainerResource.Name` を渡す。Web UI の `ECSExecLauncher` と同じ識別子である。`Row.ID()` (クラスタ ARN) を渡す案は、Web UI が ARN を使わず、CLI の `--cluster` もクラスタ名を受け取るので採らない。`model.go` の `startSession` は `Service.ID()` が `ecs` のときにこの経路を使う。`Service` に接続の手順を実装させるインターフェースを足す案は、選択肢の表示が `Model` の状態 (非同期の取得と `picker`) に依存し、`Service` から画面の遷移を駆動できないので採らない。
- `backend/internal/cli/tui.go` に `ecsExecCommandWith(ctx, profile, region, cluster, task, container, command string, deps ecsExecSessionDeps)` と、それを `defaultECSExecSessionDeps()` で呼ぶ `ecsExecCommand` を足す。`ecsExecCommandWith` は `deps.executeSession` の結果の `ECSExecSession.Target()` と `https://ecs.<region>.amazonaws.com` で `exec.Cmd` を組み立て、`sessionManagerCommand` と同じく短命の context で `deps.terminateSession` を呼ぶ `finalize` を返す。開始後の失敗 (`deps.lookupPlugin` の失敗など) はその場で `finalize` を呼んで切断してから返す。`ecsExecSessionDeps` の `execPlugin` は、プロセスの実行を bubbletea に委ねるので使わない。`backend/internal/cli/run_test.go` の `context.Background()` を許す関数の一覧に `tui.go:ecsExecCommandWith` を足す。TUI 用に別の依存の構造体を作る案は、同じ外部処理をもう 1 組持つことになるので採らない。
- `ListECSTasks` と `ListECSContainers` の失敗は通知行に出す (FR-29 の通知)。タスクが 0 件のときは `no running tasks` を、`ExecEnabled` のコンテナが無いときは `no container with ECS Exec enabled` を通知し、セッションを開始しない。`Deps.StartECSExec` が nil のときは EC2 と同じ `sessions are not available` を通知する。
- 追加の API 呼び出しは `ListTasks` / `DescribeTasks` (`ListECSTasks`)、`DescribeTasks` (`ListECSContainers`)、`ExecuteCommand`、`TerminateSession` で、CLI と Web UI が既に使う権限の範囲に収まる。新しい依存は無い。

## 完了条件

- `backend/internal/tui/ecs.go` の `SupportsSession()` が `true` を返し、`ecs_test.go` の期待値を issue 0219 の `false` から更新している。
- `tui.Deps` に `ListECSTasks`、`ListECSContainers`、`StartECSExec` があり、`backend/internal/cli/tui.go` の `tui.Deps` を組み立てる関数が `awsinternal.ListECSTasks`、`awsinternal.ListECSContainers`、`ecsExecCommand` を渡す。
- `backend/internal/tui/model_test.go` に、ECS の行で `s` を押したときの次の経路を偽の関数で確かめるテストがある: `ListECSTasks` が `profile`、`region`、クラスタ名 (`Row.Name()`)、空の `service` で呼ばれる。タスク 1 件とコンテナ 1 件で選択肢を出さずに `StartECSExec` が `profile`、`region`、クラスタ名、タスク ID (ARN の最後の `/` より後ろ)、コンテナ名、`/bin/sh` で呼ばれる。タスク複数で選択肢が出て、選んだタスクのタスク ID で `ListECSContainers` が呼ばれる。`ExecEnabled` が偽のコンテナは候補に出ない。`ExecEnabled` が真のコンテナが複数あるタスクで選択肢が出て、選んだコンテナ名で `StartECSExec` が呼ばれる。タスクが 0 件のとき `no running tasks` が通知され `ListECSContainers` と `StartECSExec` が呼ばれない。候補が無いとき `no container with ECS Exec enabled` が通知され `StartECSExec` が呼ばれない。`ListECSTasks` の失敗と `ListECSContainers` の失敗が通知行に出る。`Deps.StartECSExec` が nil のとき `sessions are not available` が通知される。
- `backend/internal/cli/tui_test.go` (無ければ作る) に、偽の `ecsExecSessionDeps` を渡した `ecsExecCommandWith` を確かめるテストがある: 組み立てる `exec.Cmd` の引数が `backend/internal/cli/ecs_test.go` の `TestECSExecuteCommandSendsPluginArgsAndTerminates` と同じ並び (`{"Target":"ecs:<cluster>_<task-id>_<runtime-id>"}`、`https://ecs.<region>.amazonaws.com`) である。`finalize` が `terminateSession` をセッション ID で呼ぶ。`lookupPlugin` が失敗したとき `terminateSession` が呼ばれてから失敗が返る。`run_test.go` の一覧に `tui.go:ecsExecCommandWith` がある。
- PRD の FR-30 の受け入れ基準 (実 AWS の ECS Exec を有効にしたタスクで `echo ok` が `ok` を返す、`exit` の後に `aws ssm describe-sessions --state Active` にセッションが残らない) を開発者が実 AWS で確認し、結果をこの issue の「## 動作確認」に記録する (`example/` の floci では確認できない)。確認できなかった項目は、その旨と理由を書く。
- `docs/prd/thief.md` の「未確定論点」から「TUI の ECS のタスクとコンテナの選び方」を消し、「決定した論点」に選択肢で選ぶ決定を決めた日付で追記し、FR-30 の本文の「タスクとコンテナの選び方 (Drawer のタブか選択肢か) は開発者が決める (「未確定論点」)」を選択肢で選ぶ記述に書き換えて「未確定論点」への参照を消し、FR-30 の本文の「2026-10-23 時点のプロトタイプは EC2 のセッションだけを持ち、ECS Exec は後続の issue で足す (「決定した論点」)」を消し、FR-30 の ECS の受け入れ基準の手順 (クラスタ行で `s`、タスクとコンテナの選択肢) を確定する。
- `CHANGES.md` の `## develop` に、この issue の [ADD] のエントリが 1 つある。
- `mise run check` が通る。
- 扱わない範囲: Drawer のサービス固有のタブ、EC2 と ECS 以外のサービスのセッション、`/bin/sh` 以外のコマンドの指定。

## 関連

- docs/prd/thief.md の FR-30、「未確定論点」の「TUI の ECS のタスクとコンテナの選び方」
- issue 0218 (プロトタイプの検証。`Deps` の組み立ての関数)、issue 0219 (TUI の ECS の一覧。この issue はその `ecs` の `Service` と `Row` を前提にする)

## 動作確認

- 設計判断の「TUI の ECS のタスクとコンテナの選び方」は、issue の設計判断が第一候補とした選択肢 (`picker`) の方式で実装した (2026-10-26)。設計判断は「実装時に開発者が確認したうえで PRD に追記する」としているが、開発者が離席中のため確認を経ずに実装し、PRD の「決定した論点」に「2026-10-26 に issue 0224 の実装で決めた」として追記した。開発者の確認は実装の報告 (implement-issues の Step 12) で得る。多観点レビューの指摘を受け、PRD の「決定した論点」の該当行の「後続の作業」に開発者の確認と実 AWS の確認を書き、確定の形にしないようにした。
- 配線の読み合わせ: `backend/internal/cli/tui.go` の `newTUIDeps` は `ListECSTasks: awsinternal.ListECSTasks`、`ListECSContainers: awsinternal.ListECSContainers`、`StartECSExec: ecsExecCommand` を渡す。`ecsExecCommand` は `backend/internal/cli/ecs.go` の `ecsExecuteCommand` と同じ `defaultECSExecSessionDeps()` を `ecsExecCommandWith` に渡し、`ecsExecCommandWith` はその `deps.executeSession` (`awsinternal.ExecuteECSCommandSession`) の応答の `Target()` と `https://ecs.<region>.amazonaws.com` でプラグインの引数を組み立てる。`backend/internal/cli/tui_test.go` の `TestNewTUIDeps_ServicesFollowWebUIOrder` に、ECS 用の 3 関数が `awsinternal.ListECSTasks`、`awsinternal.ListECSContainers`、`ecsExecCommand` そのものであることの確認 (関数の同一性の比較) を足した。
- 単体テスト (`backend/` で `go test -race -cover ./internal/tui/ ./internal/cli/`、2026-10-26): `internal/tui` ok (coverage 96.7%)、`internal/cli` ok (coverage 51.3%)。追加したテストは次のとおり。
  - `backend/internal/tui/model_test.go`: `TestECSTaskID` (ARN の最後の `/` より後ろ、`/` 無し、空文字列)、`TestModel_ECSExecSingleTaskAndContainerStartsWithoutPicker` (`ListECSTasks` が `first`、`ap-northeast-1`、`demo`、空の `service` で呼ばれ、タスク 1 件とコンテナ 1 件で選択肢を出さずに `StartECSExec` が `first/ap-northeast-1/demo/abcdef1234567890/app//bin/sh` で呼ばれ、`sessionReadyMsg` の経路でセッションに入り、セッションの終了後に `s` で再び開始できる)、`TestModel_ECSExecMultipleTasksOpensPicker` (タスク 2 件で `Select a task in demo` の選択肢が出て、2 件目を選ぶとそのタスク ID で `ListECSContainers` が呼ばれる)、`TestModel_ECSExecContainerPickerOnlyListsExecEnabled` (`ExecEnabled` が偽のコンテナは候補に出ず、真の 2 件で `Select a container in task-a` の選択肢が出て、選んだコンテナ名で `StartECSExec` が呼ばれる)、`TestModel_ECSExecNoCandidatesNotifies` (タスク 0 件で `no running tasks`、ECS Exec が有効なコンテナ無しで `no container with ECS Exec enabled` を通知し、後段を呼ばず、`s` を再び受け付ける)、`TestModel_ECSExecListFailuresAreNotified` (`list tasks: AccessDeniedException` と `list containers: describe ecs task task-a: boom` が通知行に出て後段を呼ばない)、`TestModel_ECSExecWithoutDepsSetsStatus` (`StartECSExec` が nil なら `sessions are not available` を通知し `ListECSTasks` を呼ばない)、`TestModel_ECSExecWithoutListDepsSetsStatus` (`ListECSTasks` または `ListECSContainers` だけが nil でも同じ通知を出し取得を始めない)、`TestModel_ECSExecNoRowSelected`、`TestModel_ECSExecPickerCancelAllowsRetry` (タスクの選択肢の `esc` で開始をやめ、`s` を再び受け付ける)、`TestModel_ECSExecContainerPickerCancelAllowsRetry` (コンテナの選択肢の `esc` でも同じ)、`TestModel_ECSExecSecondSKeyIgnoredWhileSelecting` (取得中の 2 度目の `s` を無視する)、`TestModel_ECSExecTargetSwitchDropsStaleResults` (取得中にプロファイルを切り替えると選択をやめ、旧い対象のタスクの結果を捨てる)、`TestModel_ECSExecTargetSwitchDropsStaleContainers` (コンテナの取得中の切替でも旧い結果を捨て `StartECSExec` を呼ばない)、`TestModel_ECSExecStartFailureAllowsRetry` (`StartECSExec` の失敗を `session:` に続けて通知し、状態を残さず `s` を再び受け付ける)。メッセージが次の取得を連鎖させる経路を追うため、テストの補助関数 `applyChain` を足した。
  - `backend/internal/cli/tui_test.go`: `TestECSExecCommandWith_Success` (引数が `/usr/local/bin/session-manager-plugin`、セッション JSON、`ap-northeast-1`、`StartSession`、`test-profile`、`{"Target":"ecs:my-cluster_abcdef1234567890_runtime-id"}`、`https://ecs.ap-northeast-1.amazonaws.com` の並びで `ecs_test.go` の `TestECSExecuteCommandSendsPluginArgsAndTerminates` と同じ。`finalize` が `sess-1` を `test-profile` / `ap-northeast-1` で切断する)、`TestECSExecCommandWith_Failures` (`ExecuteCommand` の失敗では切断を呼ばない。`lookupPlugin` の失敗で `sess-1` の切断を呼んでから失敗を返す。切断も失敗すれば切断を 1 回呼んだうえで両方の理由を返す)、`TestECSExecCommandWith_FinalizeError`。`backend/internal/cli/run_test.go` の一覧に `tui.go:ecsExecCommandWith` を足した。
- floci での確認 (2026-10-26): `HOME="$(pwd)/example/home" thief tui -p floci` を pty で起動する検証スクリプト (5 項目) を流し、すべて通った。`S` で `ecs` を選ぶと `ECS (1)` に seed の `thief-example-cluster` が出る。その行で `s` を押すと通知行に `no running tasks` が出て選択肢は開かず、TUI の画面はそのまま残り、`r` で一覧を読み直せる。floci の seed はタスクを持たないため、選択肢の表示と接続は floci では確認できない。
- 実 AWS での確認は行っていない。完了条件の「PRD の FR-30 の受け入れ基準を開発者が実 AWS で確認する」は、実 AWS アカウントの SSO ログインと ECS Exec を有効にしたタスクが要り、開発者が離席中で認証とアカウントの指定を得られないため、この実装の時点では確認できなかった。確認できていない項目は次の 2 つで、確認は開発者が行う。
  1. ECS Exec を有効にしたタスクを持つクラスタの行で `s` を押し、タスクとコンテナの選択肢で接続先を選ぶと `session-manager-plugin` が起動し、シェルで `echo ok` を実行すると `ok` が返る。
  2. `exit` の後、`aws ssm describe-sessions --state Active` の結果にそのセッションが残らない。
  - プラグインに渡す引数の並びとエンドポイントは CLI の `thief ecs exec` (`ecsExecuteCommandWith`) と同じで、`TestECSExecCommandWith_Success` が並びを固定している。CLI 側が実 AWS で動くことはこの issue では確認しておらず、この issue の材料に記録も無い (CLI と同じ引数なら TUI も同じく接続できるというのは仮説である)。TUI 側はその引数を `tea.ExecProcess` に渡す点だけが違い、EC2 の Session Manager (issue 0218 で実 AWS 未確認のまま同じ構造で入れた経路) と同じ構造である。
- 完了条件の「`mise run check` が通る」は、implement-issues の Step 5 の読み方 (既知の失敗に無い新たな失敗が無いこと) で判定した。段ごとの実行 (2026-10-26): `gofmt -l .` と `goimports -l .` は出力無し、`mise run backend:lint` は exit 0、`mise run backend:test` は 19 パッケージすべて ok、`npx prettier --check .` は exit 0、`mise run frontend:lint` は 0 errors (9 warnings は既存)、`mise run frontend:test` は 119 ファイル 1350 件すべて通過。`mise run backend:vuln` (govulncheck) は issue 0218 の検査から続く既知の 12 件 (GO-2026-6809、6617、6613、6612、6611、6610、6608、6607、6605、6603、6600、6599) で exit 3 となり、新たな検出は無い (`go.mod` と `go.sum` は変えていない)。`mise run frontend:test` の 1 回目は 4 件が 5 秒のタイムアウトで落ちた。frontend のファイルは変えておらず、同じ内容の 2 回目は全件通過したため、判定には 2 回目の結果を使った。この 4 件は既知の失敗には加えていない (最終実行で解消した一時的な失敗であり、環境要因の扱いについてユーザーの指示は得ていない。原因は特定していない)。
- 多観点レビュー (テストと堅牢性、完了条件の充足、規約と整合の 3 観点、2026-10-26) の指摘と反映。却下した指摘は無い。
  - `example/README.md` の ECS の確認手順が `ECS does not support sessions` の通知のままだった (規約と整合、高): `no running tasks` の通知に直し、選択肢と接続は実 AWS で確認する旨を足した。
  - 選び方の決定が開発者の確認待ちなのに PRD と「## 解決方法」が確定の形になっていた (完了条件の充足と規約と整合、中): PRD の「決定した論点」の該当行に確認待ちと実 AWS の確認を「後続の作業」として書いた。
  - コンテナの選択肢の `esc` と、旧い `ecsContainersMsg` を捨てる分岐が未テスト (テストと堅牢性、中): `TestModel_ECSExecContainerPickerCancelAllowsRetry` と `TestModel_ECSExecTargetSwitchDropsStaleContainers` を足した。
  - `StartECSExec` の失敗経路、`ListECSTasks` と `ListECSContainers` が個別に nil の分岐、`newTUIDeps` の配線の同一性が未テスト (テストと堅牢性、低): `TestModel_ECSExecStartFailureAllowsRetry`、`TestModel_ECSExecWithoutListDepsSetsStatus` を足し、`TestNewTUIDeps_ServicesFollowWebUIOrder` を関数の同一性の比較にした。
  - `TestECSExecCommandWith_Failures` の 3 番目のケースが切断の呼び出しを記録していなかった (テストと堅牢性、低): 記録したうえで失敗する `terminateSession` に変え、切断 1 回と `sess-1` を確かめるようにした。
  - `ecsExec` と `reloadForNewTarget` のコメントが世代管理の実態と違っていた (テストと堅牢性と完了条件の充足、低): `ecsSeq` は選択の開始時だけ進み、切替は `ecsExec` を空にする、と直した。
  - `CHANGES.md` のエントリに対象の切替での中止、選択中の `s` の無視、README の修正が無かった (規約と整合、低): 足した。
  - この節の配線の読み合わせの文、「no exec-enabled container」の `s` の再受付の記述、CLI の実 AWS での動作の断定、frontend のタイムアウトの扱いの記述が実態とずれていた (完了条件の充足と規約と整合、低): 上の各行を直し、「no exec-enabled container」の後の `s` の再受付をテストにも足した。
- レビューの反映後の再検査 (2026-10-26): `backend/` で `gofmt -l .` と `goimports -l .` は出力無し、`go vet` と `staticcheck` は `./internal/tui/ ./internal/cli/` で exit 0、`go test -race -cover ./internal/tui/ ./internal/cli/` は `internal/tui` ok (coverage 96.8%)、`internal/cli` ok (coverage 51.3%)。
- レビューの反映後の段ごとの再実行 (2026-10-26): `gofmt -l .` と `goimports -l .` は出力無し、`mise run backend:lint` は exit 0、`mise run backend:vuln` は既知の 12 件だけで新たな検出は無し (exit 3)、`mise run backend:test` は 19 パッケージすべて ok、`npx prettier --check .` は exit 0、`mise run frontend:lint` は 0 errors (9 warnings は既存)、`mise run frontend:test` は 119 ファイル 1350 件すべて通過 (1 回で通過)。floci の pty の検証スクリプト (5 項目) も再実行してすべて通った。

## 解決方法

## 解決方法

ECS のクラスタ行で `s` を押すと、タスクとコンテナを選択肢で順に選んで ECS Exec で接続する経路を `backend/internal/tui/model.go` に足し、プラグインの起動と切断の組み立てを `backend/internal/cli/tui.go` に足した。

- `backend/internal/tui/ecs.go`: `SupportsSession()` を `true` にした。サービス ID を定数 `ecsServiceID`、既定のコマンドを定数 `ecsExecDefaultCommand` (`/bin/sh`) にし、タスク ARN の最後の `/` より後ろを返す `ecsTaskID` を足した。`ecs_test.go` の期待値を `true` に更新した。
- `backend/internal/tui/picker.go`: 選択肢の種類に `pickerECSTask` と `pickerECSContainer` を足した。
- `backend/internal/tui/model.go`: `Deps` に `ListECSTasks`、`ListECSContainers`、`StartECSExec` を足した。`startSession` は `Service.ID()` が `ecs` のとき `startECSExec` へ分岐し、`ListECSTasks(profile, region, クラスタ名, "")` を `tea.Cmd` で呼んで `ecsTasksMsg` を受ける。0 件なら `no running tasks` を通知し、1 件ならそのタスクで `listECSContainers` を呼び、複数なら `Select a task in <クラスタ名>` の選択肢を出す。`ecsContainersMsg` では `ExecEnabled` が真のコンテナだけを候補にし、0 件なら `no container with ECS Exec enabled` を通知し、1 件なら `startECSExecProcess` を呼び、複数なら `Select a container in <タスク ID>` の選択肢を出す。`startECSExecProcess` は `StartECSExec(profile, region, クラスタ名, タスク ID, コンテナ名, "/bin/sh")` を呼び、EC2 と同じ `sessionReadyMsg` の経路でセッションに入る。進行中の状態は `ecsExecState` (世代 `seq`、クラスタ名、タスク ID) で持ち、選択肢の `esc` とプロファイル、リージョン、サービスの切替 (`reloadForNewTarget`) で `cancelECSExec` が状態と `sessionStarting` を消し、届いていない結果は `ecsExec` が空になったことで捨てる (世代 `seq` は選択を始めるたびに進め、同じ選択の結果だけを使う)。取得の失敗は `list tasks:` と `list containers:` に続けて通知行に出す。`ListECSTasks`、`ListECSContainers`、`StartECSExec` のいずれかが nil のときは EC2 と同じ `sessions are not available` を通知する。
- `backend/internal/cli/tui.go`: `newTUIDeps` に `ListECSTasks: awsinternal.ListECSTasks`、`ListECSContainers: awsinternal.ListECSContainers`、`StartECSExec: ecsExecCommand` を足した。`ecsExecCommand` は `defaultECSExecSessionDeps()` で `ecsExecCommandWith(ctx, profile, region, cluster, task, container, command, deps)` を呼ぶ。`ecsExecCommandWith` は `deps.executeSession` で `ExecuteCommand` を呼び、その応答の `Target()` と `https://ecs.<region>.amazonaws.com` で `session-manager-plugin` の `exec.Cmd` を組み立て、短命の context で `deps.terminateSession` を呼ぶ `finalize` を返す。開始後の失敗 (`lookupPlugin` の失敗など) は `finalize` で切断してから返す。`backend/internal/cli/run_test.go` の `context.Background()` を許す一覧に `tui.go:ecsExecCommandWith` を足した。
- テスト: `backend/internal/tui/model_test.go` に ECS Exec の経路のテスト 15 本と、連鎖する `tea.Cmd` を追う補助関数 `applyChain` を足した。`backend/internal/cli/tui_test.go` に `ecsExecCommandWith` のテスト 3 本と、`newTUIDeps` の ECS 用の 3 関数が本番の関数そのものである確認を足した。
- `docs/prd/thief.md`: 「未確定論点」から「TUI の ECS のタスクとコンテナの選び方」を消し、「決定した論点」に 2026-10-26 の決定 (選択肢で選ぶ) を足した。選び方は issue の設計判断の第一候補のとおりに決めたが、設計判断が求める開発者の確認は得ておらず、「後続の作業」に開発者の確認と実 AWS での FR-30 の確認を書いた。FR-30 の本文を選択肢で選ぶ記述に書き換え、プロトタイプの時点の記述を消し、ECS の受け入れ基準の手順 (クラスタ行で `s`、タスクとコンテナの選択肢) を確定した。
- `example/README.md`: ECS の確認手順の `s` の結果を `ECS does not support sessions` から `no running tasks` の通知に直した (floci の seed はタスクを持たない)。
- `CHANGES.md` の `## develop` に [ADD] のエントリを 1 つ足した。

完了条件の検証は「## 動作確認」に記録した。floci ではタスクの無いクラスタでの `no running tasks` の通知まで確かめた。実 AWS での接続の確認と、選び方の方式の確認は開発者が離席中のため行えておらず、どちらも開発者の確認を待つ。`mise run check` に新たな失敗は無く、`mise run backend:vuln` の既知の 12 件だけが出る。

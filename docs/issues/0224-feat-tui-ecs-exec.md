# TUI の ECS の一覧から ECS Exec でタスクのコンテナに接続する

Created: 2026-10-25
Model: Claude Fable 5.1

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

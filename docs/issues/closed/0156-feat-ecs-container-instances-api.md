# ECS コンテナインスタンス一覧 API とタスクへの `container_instance_arn` を追加する

Created: 2026-08-26
Model: Claude Fable 5
Completed: 2026-08-27

## 背景

docs/issues/TODO.md の次の項目に由来する。

> ECS on EC2 の場合は EC2 ごとにどのタスクが動いているかをみたい

この要望の実現は 2 つの issue に分割した。本 issue はその第 1 段階で、backend にコンテナインスタンス (クラスタに登録された EC2) の一覧を返す API と、タスクがどのコンテナインスタンスで動いているかを示すフィールドを追加する。frontend の Drawer にタブを追加して表示するのは第 2 段階 (docs/issues/0157-feat-ecs-container-instances-drawer-tab.md) で扱う。
frontend の変更は、契約検査 (contract golden) を通すために `ECSTaskRaw` へ 1 フィールドを追加する分だけを本 issue に含める (詳細は設計判断の「契約検査 (contract golden) との整合」)。

ECS クラスタの Drawer (frontend/src/components/Drawer/Drawer.tsx) の Overview は `Registered EC2` として `registeredEc2` (backend/internal/aws/ecs.go の `ECSResource.RegisteredEC2`、`DescribeClusters` の `RegisteredContainerInstancesCount`) の件数だけを表示する。

backend は `ListContainerInstances` と `DescribeContainerInstances` をどこからも呼んでいない (リポジトリ全体で 0 件)。
タスク側でも、backend/internal/aws/ecs_exec.go の `ecsTaskFromSDK` は SDK の `Task.ContainerInstanceArn` を `ECSTaskResource` にコピーしておらず、構造体にフィールドも無い。

EC2 の一覧も、タスクがどの EC2 で動いているかの対応も、現状の API からは組み立てられない。
これが要望が満たされていない理由である。

AWS SDK for Go v2 の `ecs/types.ContainerInstance` (`DescribeContainerInstances` の応答) は `ContainerInstanceArn` / `Ec2InstanceId` / `Status` / `AgentConnected` / `RunningTasksCount` / `PendingTasksCount` / `RegisteredResources` / `RemainingResources` を持つ。
`RunningTasksCount` は件数のみで、どのタスクが動いているかは返さない。
タスクとの対応は `Task.ContainerInstanceArn` でしか取れない。

## 目的

frontend が、クラスタのコンテナインスタンス一覧と、各タスクが載っているコンテナインスタンスの ARN を API から取得できる。
第 2 段階の Drawer タブはこの 2 つのデータを結合して EC2 ごとのタスク一覧を表示する。

## 設計判断

### コンテナインスタンス一覧エンドポイントを追加する

- `GET /api/aws/profiles/{profile}/ecs/{cluster}/container-instances` を追加し、`ListContainerInstances` (`NewListContainerInstancesPaginator` で全ページ取得) の後に `DescribeContainerInstances` を呼ぶ。`DescribeContainerInstances` の `ContainerInstances` は 1 回あたり 100 件までのため、100 件ごとに分割して呼ぶ。分割数は新しい定数 `ecsDescribeContainerInstancesBatchSize = 100` として置き、既存の `ecsDescribeTasksBatchSize` (backend/internal/aws/ecs_cli.go) は名前が Tasks 用のため流用しない。実装は `listECSTasks` (`ListTasks` から `DescribeTasks`) と同じ 2 段構成で backend/internal/aws/ecs_exec.go に `ListECSContainerInstances` として置く。`ListContainerInstances` の結果が 0 件のときは `listECSTasks` と同じく `DescribeContainerInstances` を呼ばずに空の結果を返す (`DescribeContainerInstancesInput.ContainerInstances` は SDK godoc で必須パラメータであり、空のリストで呼ぶ理由が無い)。
- `DescribeContainerInstances` の応答の `Failures` (`ListContainerInstances` と `DescribeContainerInstances` の間に登録解除された ARN 等) はエラーにせず、該当 ARN と `Reason` を `slog.Warn` で 1 件ずつ記録して結果から除く。一覧の途中で 1 台が消えたことで全体を 500 にする理由が無く、`readSSOCacheStatuses` が読み飛ばしを `slog.Warn` で記録する規約に揃える。
- `ListContainerInstances` の `Status` は指定せず既定 (INACTIVE 以外) のままにする。`ContainerInstance.Status` の値は SDK godoc で REGISTERING / REGISTRATION_FAILED / ACTIVE / INACTIVE / DEREGISTERING / DRAINING の 6 つであり、既定では ACTIVE と DRAINING のほかに REGISTERING (awsvpcTrunking の ENI 準備中)、REGISTRATION_FAILED、DEREGISTERING も返る。スケールアウトとスケールインの途中に現れる状態であり、いずれも除外せず `status` で区別できるようにする。DRAINING の EC2 は新規タスクの配置を止め、サービスのタスクを可能な範囲で順次退避させている途中の状態で (SDK godoc `ContainerInstance.Status`)、退避が終わるまでは既存タスクが動いており、スケールインの前にどのタスクが載っているかを見たいという要望の対象そのものであるため、ACTIVE のみへの絞り込みは採らない。frontend が区別できるよう `status` をレスポンスに含める。`status` は `ecsServiceFromSDK` の `Status` や `ecsTaskFromSDK` の `LastStatus` と同じく `DisplayState(ptrStr(ci.Status))` を通し、小文字ハイフン区切り (`active` / `draining` / `registration-failed` 等) で返す。SDK の生値 (大文字) のまま返すと state 系フィールドの既存の慣習から外れ、frontend の `StatusBadge` の `MAP` (小文字キー) にも一致しない。
- レスポンスの要素 `ECSContainerInstanceResource` は `arn` / `ec2_instance_id` / `status` / `agent_connected` / `running_tasks_count` / `pending_tasks_count` / `registered_cpu` / `registered_memory` / `remaining_cpu` / `remaining_memory` を持つ。`RegisteredResources` / `RemainingResources` は `Name` が `CPU` / `MEMORY` かつ `Type` が `INTEGER` の要素の `IntegerValue` を取り出し、`PORTS` 等のそれ以外は捨てる。`Resource` は `Type` で有効な値フィールドが決まる構造であり、`Type` を見ずに `IntegerValue` を読むと型が変わったときにゼロ値の 0 が「残り 0」として出るため、`Type` が `INTEGER` 以外の CPU / MEMORY は該当要素無しとして扱う。`arn` / `ec2_instance_id` 以外のフィールドは TODO の字義 (EC2 とタスクの対応) を超えるが、いずれも同じ `DescribeContainerInstances` の応答から追加の API 呼び出し無しに得られ、第 2 段階の一覧で EC2 の状態 (退避中か、エージェントが切れていないか、資源が残っているか) を判断する文脈情報として表示するために含める。該当する要素が無いときは JSON で `null` にする。型は `*int32` とし、`omitempty` は付けない (`omitempty` は nil のときキー自体を省略するため `null` にならない)。0 を入れると「残り 0」と区別できないため nil にする。
- ハンドラ `handleECSContainerInstances` は backend/internal/api/handlers_aws.go に置き、`handleECSTasks` と同じく `serveCached` でキャッシュキー `ecs-container-instances` を使う。`handlers_cache_test.go` のセグメント一覧に `ecs-container-instances` を追加する。
- External (ECS Anywhere) のコンテナインスタンスは `Ec2InstanceId` に SSM のマネージドインスタンス ID が入る (SDK godoc `ContainerInstance.Ec2InstanceId`)。この ID は `mi-` 始まりである (SSM のマネージドインスタンス ID の命名規則で、godoc の記述ではない)。本 issue はこれを区別せず `ec2_instance_id` にそのまま入れる。TODO の対象は ECS on EC2 であり、External が混在するクラスタでの表示の区別は本 issue と 0157 のスコープ外とする。
- 採らなかった案: EC2 一覧 (`DescribeInstances`) と `Ec2InstanceId` で突合して EC2 の名前やインスタンスタイプも返す。EC2 一覧は region 全体の取得で、クラスタごとの API 呼び出しに全 EC2 の取得が加わる。TODO の要望は「EC2 ごとにどのタスクが動いているか」であり、EC2 の属性は要望に含まれないため却下する。要望として出ていない推測の機能のため docs/issues/TODO.md には追加せず、要望が出た時点で起票する (0153 の `sso:Logout` は設計中に見つかった既存動作の不足のため TODO.md に記載する。扱いの差はこの理由による)。

### タスクに `container_instance_arn` を追加する

- `ECSTaskResource` に `ContainerInstanceArn` (JSON `container_instance_arn`) を追加し、`ecsTaskFromSDK` で `Task.ContainerInstanceArn` をコピーする。Fargate のタスクでは空文字列になる。
- 採らなかった案: コンテナインスタンスごとに `ListTasks` を `ContainerInstance` フィルタで呼ぶ API を作る。EC2 の台数ぶん `ListTasks` と `DescribeTasks` が増え、既存の tasks エンドポイントと同じデータを二重に取る。タスク側に ARN を 1 フィールド足せば frontend でグルーピングできるため却下する。
- `listECSTasks` は `ListTasksInput.DesiredStatus` を指定しておらず、ECS の既定で `desiredStatus` が RUNNING のタスクだけを返す。第 2 段階のタブはこの既定に依存して「動いているタスク」を表示するため、`listECSTasks` にステータスの指定を追加する場合はタブの前提が崩れることを `listECSTasks` のコメントに書き残す。

### 契約検査 (contract golden) との整合

- backend/internal/contract/contract.go の `Registry` (60 型) に登録された構造体は、backend/internal/contract/contract_test.go の `TestGolden` / `TestTagsGolden` が frontend/src/types/__contract__/<型名>.json のゴールデンと突き合わせ、frontend/src/types/contract.check.ts が同じゴールデンと Raw 型のキー集合の双方向一致を `tsc --noEmit` (`npm run lint` に含まれる) で検査する。
- `ECSTaskResource` は `Registry` に登録済みのため、`container_instance_arn` の追加でゴールデンの再生成 (`UPDATE_GOLDEN=1 go test ./internal/contract/`) が必須になる。
- 再生成後は `ECSTaskRaw` (frontend/src/types/aws.ts) に `container_instance_arn` が無いと `Contract<typeof ecsTask, ECSTaskRaw>` が型エラーになる。
- このため、本 issue は frontend の変更を `ECSTaskRaw` への `container_instance_arn: string` の 1 フィールド追加と、それに伴う既存テストのリテラル修正に限って行う。`ECSTaskRow` / `ecsTaskFromRaw` / フック / Drawer タブは変更せず 0157 に残す。
- `ecsTaskFromRaw` は Raw のフィールドを個別にコピーするため関数本体は変更不要である。frontend/src/lib/normalize.test.ts の `ecsTaskFromRaw` に `ECSTaskRaw` のオブジェクトリテラルを直接渡す既存テスト 2 件はフィールドが必須のため型エラーになるので、この 2 件のリテラルに `container_instance_arn` を足す (テストの期待値は変えない)。
- フィールドは optional (`?`) にしない。backend 側に `omitempty` を付けないためキーは常に出力され、frontend/src/types/contract.check.ts の冒頭コメントが定める「optional は backend の `omitempty` と対応させる」規約に従うと必須にするのが正しい。optional にすれば既存テストの修正は不要になるが、応答に常に含まれるキーを optional と宣言する食い違いを作るため採らない。
- 新設する `ECSContainerInstanceResource` も `Registry` に登録し、ゴールデン `ECSContainerInstanceResource.json` を本 issue で生成する。`TestRegistryNamesUnique` が `len(Registry) != 60` で件数を固定しているため 61 に更新する。`contract.check.ts` への import と `Contract<...>` 行の追加は `ECSContainerInstanceRaw` を作る 0157 で行う (`contract.check.ts` は全ゴールデンの参照を強制していないため、Raw が無い間もゴールデンだけ先に置ける)。
- 採らなかった案: `ECSContainerInstanceResource` の `Registry` 登録とゴールデン生成を、Raw 型を作る 0157 まで遅らせる。contract.go の除外規則は「frontend に消費者が無い型」で、0156 の close 直後はこの型に消費者が無い。しかし除外の実例 (`SSOAccountResource` / `SSMValueResponse`) は恒久的に frontend が消費しない型であり、本型は次の issue で消費が決まっている。本 issue で登録すれば、backend が返す JSON の形状 (`null` になるフィールド、キー名) を本 issue の PR でゴールデンとして固定でき、0157 は Raw 型を書いた時点で `tsc --noEmit` により形状の食い違いを検出できる。遅らせると 0157 の PR に backend の変更 (Registry と件数の更新) が混ざり、0157 の「backend の変更は行わない」と矛盾するため却下する。
- 採らなかった案: 0156 と 0157 を 1 issue に統合する。統合すれば契約検査の分割問題は消えるが、backend の 2 段構成 API とテスト、frontend の 2 クエリ合成とタブ、i18n を 1 issue に抱えて 1 issue で検証と close ができる大きさを超える。Raw への 1 フィールド追加を本 issue に含めるだけで分割を保てるため却下する。
- 採らなかった案: `ECSTaskResource` を `Registry` から外して契約検査を回避する。contract.go の除外規則は「frontend に消費者が無い型」であり、`ECSTaskResource` は該当しないため却下する。

### 権限

- 追加で `ecs:ListContainerInstances` と `ecs:DescribeContainerInstances` の IAM 権限が必要になる。README や docs に IAM ポリシーの記載は無いため、リポジトリ内のドキュメント更新は不要である。権限不足のときは既存の `writeAWSError` の経路で 403 `ACCESS_DENIED` が返る。
- 新規の依存追加は無い (`service/ecs` は既存)。

## 完了条件

- `GET /api/aws/profiles/{profile}/ecs/{cluster}/container-instances` が routes.go に登録され、`ListContainerInstances` の全ページと `DescribeContainerInstances` (100 件ずつ分割) の結果から `ECSContainerInstanceResource` の配列を返す。
- `ECSContainerInstanceResource` が `arn` / `ec2_instance_id` / `status` / `agent_connected` / `running_tasks_count` / `pending_tasks_count` / `registered_cpu` / `registered_memory` / `remaining_cpu` / `remaining_memory` を持ち、`registered_*` / `remaining_*` は該当要素が無いとき `null` になる。
- `ListContainerInstances` に `Status` フィルタを渡さず、DRAINING / REGISTERING / REGISTRATION_FAILED / DEREGISTERING のコンテナインスタンスも一覧に含まれる。`status` は `DisplayState` を通した小文字ハイフン区切りである。
- `ec2_instance_id` は SDK の `Ec2InstanceId` をそのまま入れ、External (`mi-` 始まり) を判別する処理を追加しない。
- backend/internal/aws/ecs_exec_test.go に、ページング (2 ページ)、101 件時の `DescribeContainerInstances` の 2 分割、`RegisteredResources` から CPU と MEMORY のみを取り出すこと、`RegisteredResources` に CPU の要素が無いとき `registered_cpu` が `null` になること、`RemainingResources` に MEMORY の要素が無いとき `remaining_memory` が `null` になること、`RegisteredResources` に `Name` が `CPU` で `Type` が `DOUBLE` の要素があるとき `registered_cpu` が `null` になること、DRAINING の 1 台が `status` `draining` (小文字) で返ること、REGISTERING / REGISTRATION_FAILED / DEREGISTERING の各 1 台が除外されずに `registering` / `registration-failed` / `deregistering` で返ること、0 件時に `DescribeContainerInstances` を呼ばずに空配列を返すこと、`Failures` に 1 件入っているとき残りの件だけが返りエラーにならないことのテストが追加されている。
- `ECSTaskResource` に `container_instance_arn` が追加され、`ecsTaskFromSDK` がコピーし、Fargate (nil) のとき空文字列になるテストが backend/internal/aws/ecs_exec_test.go に追加されている。
- `listECSTasks` に、`DesiredStatus` 未指定 (RUNNING のみ) に第 2 段階のタブが依存している旨のコメントが追加されている。
- `handlers_cache_test.go` のセグメント一覧に `ecs-container-instances` が追加され (セグメント総数を検証する定数 `wantSegments` も更新する)、`POST /api/cache/invalidate?view=aws` で破棄される。
- `ECSContainerInstanceResource` が backend/internal/contract/contract.go の `Registry` に追加され、`TestRegistryNamesUnique` の件数が 61 に更新されている。
- `UPDATE_GOLDEN=1 go test ./internal/contract/` でゴールデンを再生成し、frontend/src/types/__contract__/ECSTaskResource.json の差分が `container_instance_arn` の追加のみであること、frontend/src/types/__contract__/ECSContainerInstanceResource.json が新規に生成されていることを PR 説明に書く。
- frontend の変更は `ECSTaskRaw` への `container_instance_arn: string` (必須、optional にしない) の追加と、frontend/src/lib/normalize.test.ts の `ecsTaskFromRaw` に渡す既存 2 件のリテラルへの同フィールドの追加のみとし、`ECSTaskRow` / `ecsTaskFromRaw` / フック / Drawer タブ / `contract.check.ts` は変更しない。
- `mise run check` が通過する。

## 関連

- docs/issues/0157-feat-ecs-container-instances-drawer-tab.md: 本 issue の API を使って Drawer に `Instances` タブを追加する第 2 段階。本 issue が先行し、0157 は本 issue の close 後に着手する。この前提は技術的な依存で、0157 が `contract.check.ts` に追加する `Contract<typeof ecsContainerInstance, ECSContainerInstanceRaw>` は本 issue が生成する `ECSContainerInstanceResource.json` が無いと import を解決できず `tsc --noEmit` が通らない。
- docs/issues/0155-feat-ecs-task-container-cpu-memory.md: コンテナ単位の CPU と Memory の表示。同じ `ecs_exec.go` の `ecsTaskFromSDK` に手を入れるが、0155 は `ECSTaskContainerDetail` (コンテナ単位の構造体とコンテナのループ)、本 issue は `ECSTaskResource` (タスク単位の構造体と return 直下) で編集箇所が分かれており、どちらを先に実装しても他方は壊れない。

## 着手時の調査結果

- 設計判断と完了条件は frontend/src/lib/normalize.test.ts で `ecsTaskFromRaw` に `ECSTaskRaw` のリテラルを直接渡す既存テストを 2 件としているが、docs/issues/closed/0155 が「コンテナ単位の cpu / memory / memory_reservation を camelCase に写す」テストを追加したため、着手時点では 3 件である。3 件すべてのリテラルに `container_instance_arn` を足す (期待値は変えない)。

## 解決方法

backend/internal/aws/ecs_exec.go に `ECSContainerInstanceResource` (`arn` / `ec2_instance_id` / `status` / `agent_connected` / `running_tasks_count` / `pending_tasks_count` / `registered_cpu` / `registered_memory` / `remaining_cpu` / `remaining_memory`) と `ListECSContainerInstances` / `listECSContainerInstances` を追加した。`ecs.NewListContainerInstancesPaginator` で全ページの ARN を集め、定数 `ecsDescribeContainerInstancesBatchSize = 100` ごとに `DescribeContainerInstances` を呼ぶ。0 件のときは `DescribeContainerInstances` を呼ばず、JSON で `[]` になるよう nil でない空スライスを返す。`ListContainerInstancesInput` に `Status` は渡さない。応答の `Failures` は `slog.Warn("describe ecs container instance failed", "cluster", ..., "arn", ..., "reason", ...)` で 1 件ずつ記録し、結果には `ContainerInstances` に含まれる分だけを入れる。クライアントのインターフェース `ecsContainerInstanceListClient` (`ecs.ListContainerInstancesAPIClient` + `DescribeContainerInstances`) は既存の `ecsTaskListClient` と同じ backend/internal/aws/ecs.go に置いた。

変換は `ecsContainerInstanceFromSDK` が行い、`status` は `DisplayState(ptrStr(ci.Status))` で小文字ハイフン区切りにする。`registered_*` / `remaining_*` は `ecsIntegerResource` が `RegisteredResources` / `RemainingResources` から `Name` が `CPU` / `MEMORY` (定数 `ecsResourceNameCPU` / `ecsResourceNameMemory`) かつ `Type` が `INTEGER` (定数 `ecsResourceTypeInteger`) の要素の `IntegerValue` を `*int32` で返し、該当が無ければ nil (JSON `null`) にする。`omitempty` は付けていない。`ec2_instance_id` は `Ec2InstanceId` をそのまま入れ、`mi-` 始まりの判別は行わない。

`ECSTaskResource` に `ContainerInstanceArn` (JSON `container_instance_arn`) を追加し、`ecsTaskFromSDK` の return 直下で `ptrStr(t.ContainerInstanceArn)` を写す。`listECSTasks` の冒頭に、`DesiredStatus` 未指定 (RUNNING のみ) に Drawer のコンテナインスタンスのタブが依存する旨のコメントを追加した。

backend/internal/api/handlers_aws.go に `handleECSContainerInstances` を追加し、`serveCached` のキャッシュキー `ecs-container-instances` (profile / region / cluster) で `ListECSContainerInstances` を呼ぶ。routes.go に `GET /api/aws/profiles/{profile}/ecs/{cluster}/container-instances` を登録した。handlers_cache_test.go の `knownCacheKeySegments` に `ecs-container-instances` (owner `aws`) を追加し、`wantSegments` とその直前のコメントの件数を 61 にした (先頭コメントの「2026-07-26 抽出時点で 60 種」は抽出日付付きの履歴の記述のため変えていない)。

backend/internal/contract/contract.go の `Registry` に `ECSContainerInstanceResource` を追加し、コメントの件数を 61 に直した。contract_test.go の `TestRegistryNamesUnique` の件数を 61 にし、`TestFillUnsupportedKind` のコメントの「現行 60 型」も 61 に直した。`UPDATE_GOLDEN=1 go test ./internal/contract/` で golden を再生成した。差分は frontend/src/types/__contract__/ECSContainerInstanceResource.json の新規生成、ECSTaskResource.json への `container_instance_arn` キーの追加 (フィラーは連番のため、後続の `containers` 内の値が `s12` 以降から 1 つずつずれる。キーの追加と削除はこの 1 キーのみ)、backend/internal/contract/testdata/tags.golden への `ECSTaskResource.ContainerInstanceArn` と `ECSContainerInstanceResource` の 10 タグの追加である。この差分の内容は PR 作成時に PR 説明へ転記する (PR 作成は本スキルの範囲外)。

frontend の変更は frontend/src/types/aws.ts の `ECSTaskRaw` への必須フィールド `container_instance_arn: string` の追加と、frontend/src/lib/normalize.test.ts で `ecsTaskFromRaw` に `ECSTaskRaw` のリテラルを渡す 3 件 (issue 起票時の 2 件と 0155 で追加された 1 件。「## 着手時の調査結果」に記載) への `container_instance_arn: ''` の追加のみである。期待値は変えていない。`ECSTaskRow` / `ecsTaskFromRaw` / フック / Drawer / contract.check.ts は変更していない。

テストは backend/internal/aws/ecs_exec_test.go に追加した。`mockECSContainerInstanceListClient` は `ListContainerInstances` をページ順に返し、`DescribeContainerInstances` は受け取った ARN のうち用意した instances にあるものを返し、無い ARN は実 API と同じく `Failures` (Reason MISSING) に載せる。`listErr` / `describeErr` で各 API のエラーを注入できる。`TestListECSContainerInstances` は 2 ページのページング (`ListContainerInstancesInput` の `NextToken` 引き継ぎと Describe に全 ARN が渡ること)、101 件時の 100 件 + 1 件の 2 分割、ちょうど 100 件時に Describe が 1 回で空のバッチを送らないこと、0 件時に Describe を呼ばず空スライスを返すこと、Failures 1 件で残り 2 件が返り警告ログ (`describe ecs container instance failed`、cluster / arn / reason 属性) が 1 行だけ出てエラーにならないこと、Failures が全件でもエラーにならず空スライスと警告ログ 2 行になること、102 件で 2 バッチ目に Failures があっても 1 バッチ目の結果と合算されること、`ListContainerInstances` / `DescribeContainerInstances` のエラーが `%w` でラップされ `errors.Is` で辿れることを検証する。100 件、101 件、102 件、Failures 1 件のケースは返却された ARN 列を `assertECSContainerInstanceARNs` で照合する。ログの検証は `captureDefaultLogs` で既定ロガーを `bytes.Buffer` の TextHandler に差し替え、`assertWarnLogLines` でメッセージを含む行の数と各行の属性を確認する (既定ロガーはプロセス共有のため、このパッケージのテストは `t.Parallel` を使わない)。境界値、ログ、合算、エラー経路の各ケースはレビュー観点 2 と観点 3 の指摘で追加した。`TestECSContainerInstanceFromSDK` は CPU / MEMORY のみの取り出し (PORTS を捨てる)、registered に CPU が無いときの nil、remaining に MEMORY が無いときの nil、CPU が DOUBLE 型のときの nil、DRAINING / REGISTERING / REGISTRATION_FAILED / DEREGISTERING の変換、`mi-` 始まりの ID の通過を検証する。`TestListECSContainerInstancesKeepsNonActive` は一覧の経路で 5 状態が除外されず `Status` フィルタも渡されないことを検証する。`TestECSTaskFromSDK` の既存ケースに `ContainerInstanceArn` (Fargate の nil で空文字列、EC2 で ARN) を加えた。

### 完了条件の確認

- エンドポイントの登録と 2 段構成: routes.go と `listECSContainerInstances` の差分、テスト 4 件
- `ECSContainerInstanceResource` の 10 フィールドと `null`: 構造体と `TestECSContainerInstanceFromSDK`
- `Status` フィルタ無しと 5 状態の通過、小文字ハイフン区切り: `TestListECSContainerInstancesKeepsNonActive` と変換テスト
- `ec2_instance_id` の無加工: 変換テスト (`mi-` ケース)
- ecs_exec_test.go の 10 項目: 上記テスト群 (ページング / 2 分割 / CPU と MEMORY のみ / registered_cpu null / remaining_memory null / DOUBLE で null / draining / registering・registration-failed・deregistering / 0 件 / Failures)
- `container_instance_arn` のコピーと Fargate の空文字列: `TestECSTaskFromSDK`
- `listECSTasks` のコメント: 差分で確認
- キャッシュセグメントと `wantSegments`: handlers_cache_test.go の差分、`go test ./internal/api/` 通過
- `Registry` への追加と 61: 差分と `go test ./internal/contract/` 通過
- golden の再生成と PR 説明: 再生成済み。差分内容は上記に記載し PR 作成時に転記する
- frontend の変更範囲: `ECSTaskRaw` と normalize.test.ts の 3 リテラルのみ
- `mise run check`: 通過 (frontend 765 テスト PASS、backend 全パッケージ ok、lint は 0 エラー。警告 10 件はベースラインと同じ)

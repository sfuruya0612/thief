# ECS クラスタの Drawer にコンテナインスタンス (EC2) ごとの稼働タスク一覧を表示する

Created: 2026-08-26
Model: Claude Fable 5
Completed: 2026-08-27

## 背景

docs/issues/TODO.md の次の項目に由来する。

> ECS on EC2 の場合は EC2 ごとにどのタスクが動いているかをみたい

この要望の実現は 2 つの issue に分割した。本 issue はその第 2 段階で、第 1 段階 (docs/issues/0156-feat-ecs-container-instances-api.md) で追加したコンテナインスタンス一覧 API とタスクの `container_instance_arn` を使い、frontend の Drawer に EC2 ごとのタスク一覧を表示する。本 issue は 0156 の close 後に着手する。

ECS クラスタの Drawer (frontend/src/components/Drawer/Drawer.tsx) のタブは `ecs: ['Overview', 'Services', 'Tasks', 'Terminal', 'Tags']` で、コンテナインスタンス (クラスタに登録された EC2) を扱うタブは無い。
Overview の `ecsOverviewRows` (frontend/src/components/Drawer/overviewRows.tsx) は `Registered EC2` として `registeredEc2` の件数だけを表示する。
frontend の `ECSTaskRow` (frontend/src/types/aws.ts) には `containerInstanceArn` が無く (`ECSTaskRaw` の `container_instance_arn` は 0156 が契約検査のために追加する)、コンテナインスタンスの型、normalize、フックも無い。

`Tasks` タブの `DrawerECSTasks.tsx` はタスク起点の一覧であり、EC2 ごとにまとめて見る表示は無い。
これが要望が満たされていない理由である。

## 目的

ECS on EC2 のクラスタで、Drawer からクラスタに登録された EC2 (コンテナインスタンス) の一覧と、各 EC2 の上で動いているタスクを読める。
EC2 の入れ替えやスケールインの前に、どのタスクがどの EC2 に載っているかを Drawer だけで確認できる。

## 設計判断

### Drawer に `Instances` タブを追加する

- `Drawer.tsx` の `ecs` タブに `Instances` を `Tasks` の次に追加し、`DrawerECSContainerInstances.tsx` を新設する。データは新設する `useECSContainerInstances(profile, region, cluster)` (queryKey `['aws', 'ecs-container-instances', profile, region, cluster]`) と、既存の `useECSTasks(profile, region, cluster)` の 2 つを使い、タスクを `containerInstanceArn` でグルーピングして各 EC2 の下に並べる。
- `useECSTasks` は第 4 引数の service を省略し、`DrawerECSTasks.tsx` の呼び出しと同じ queryKey `['aws', 'ecs-tasks', profile, region, cluster, undefined]` を共有する。空文字列を渡すと queryKey が変わり同じデータを二重に取得するため渡さない。
- 型は frontend/src/types/aws.ts に `ECSContainerInstanceRaw` / `ECSContainerInstanceRow` を追加し、`ECSTaskRow` に `containerInstanceArn` を追加する (`ECSTaskRaw` の `container_instance_arn` は 0156 で追加済み)。frontend/src/types/contract.check.ts に `__contract__/ECSContainerInstanceResource.json` (0156 が生成) の import と `Expect<Contract<typeof ecsContainerInstance, ECSContainerInstanceRaw>>` を追加し、Raw とゴールデンのキー集合の一致を `tsc --noEmit` で検査する。変換は frontend/src/lib/normalize.ts の `ecsContainerInstanceFromRaw` (新設) と `ecsTaskFromRaw` (フィールド追加) に置く。API 呼び出しは frontend/src/api/endpoints.ts の `getECSContainerInstances`、フックは frontend/src/api/queries.ts の `useECSContainerInstances` とし、`useECSTasks` / `useECSServices` に並べる。
- 採らなかった案: Fargate のみのクラスタでは `Instances` タブ自体を隠す。タブ構成は `Drawer.tsx` でサービスごとに静的に決まっており、クラスタごとにタブを増減させる仕組みを新設するコストに空表示の利便が見合わないため却下する。
- 採らなかった案: 既存の `Tasks` タブに `Container instance` 列を足すだけにする。タスク起点の一覧では「EC2 ごとに」まとめて見る要望を満たさないため却下する。`Tasks` タブへの列追加は本 issue では行わない。

### 表示内容

- 各 EC2 の見出しに `ec2InstanceId` / `status` / `agentConnected` / running と pending の件数 / CPU と Memory の remaining と registered を表示し、その下にタスクの `group` / `lastStatus` / `cpu` / `memory` / `startedAt` を並べる。`status` は `active` / `draining` / `registering` / `registration-failed` / `deregistering` の区別のために表示する (0156 は INACTIVE 以外の全状態を `DisplayState` を通した小文字ハイフン区切りで返す)。`status` と `lastStatus` の表示は `DrawerECSTasks.tsx` の `ECSTaskDetail` が `lastStatus` に使う `StatusBadge` (frontend/src/components/primitives/StatusBadge.tsx) を再利用し、Tasks タブと見た目を揃える。`StatusBadge` の `MAP` に `draining` / `registering` / `registration-failed` / `deregistering` は無いため、`draining` と `deregistering` を `warn`、`registration-failed` を `err`、`registering` を `info` として `MAP` に追加する (`active` は既存の `ok`)。
- `StatusBadge` の `MAP` は ECS 専用ではなく共有部品で、`draining` を表示し得る他の列が 2 つある。
  - ELB Target Health の `State` 列 (frontend/src/components/tables/columns.tsx の `elbTargetHealthColumns`)。値は backend/internal/aws/elb.go の `elbTargetHealthFromSDK` が `DisplayState(string(d.TargetHealth.State))` で返す `TargetHealthStateEnum` (`initial` / `healthy` / `unhealthy` / `unhealthy.draining` / `unused` / `draining` / `unavailable` の 7 値)。
  - ECS サービス一覧の `State` 列 (`ecsServiceColumns`)。値は backend/internal/aws/ecs_exec.go の `ecsServiceFromSDK` が `DisplayState` を通した `Status` で、ACTIVE / DRAINING / INACTIVE を取る (DRAINING はサービス削除中に現れる)。
- 現状は `MAP` に `draining` が無いため両列ともフォールバックの `muted` で表示されており、本 issue の `MAP` 追加でこの 2 列の `draining` も `warn` に変わる。
- 設計判断: この変更を意図した挙動として受け入れる。ELB の `draining` は登録解除中のターゲットが処理中のリクエストを終えるまでの遷移状態、ECS サービスの DRAINING は削除中で、コンテナインスタンスの DRAINING と同じく「間もなく消える」状態であり、同じ色で示すのが一貫する。
- `unhealthy.draining` は `DisplayState` が `.` を変換しないため `unhealthy.draining` のまま渡り、`draining` とは一致せず本 issue の変更後も `muted` のままである。
- `registering` / `deregistering` / `registration-failed` は `DisplayState` を呼ぶ他の全モジュール (cloudfront / dynamo / ec2 / ecs / elasticache / elb / kinesis / natgw / rds) の状態文字列と衝突しない。
- 採らなかった案: ECS 専用の色分けを `DrawerECSContainerInstances.tsx` 内のローカルな対応表に閉じ、共有の `MAP` を変えない。`draining` の意味が ELB と ECS で同じ (登録解除に向かう遷移状態) であるのに表示だけが分かれ、バッジのクラス付けが 2 か所に重複するため却下する。
- 見出しの running と pending の件数は `DescribeContainerInstances` が返す `RunningTasksCount` / `PendingTasksCount` で、その下に並ぶタスクは別のキャッシュキー (`ecs-tasks`) で取得した一覧である。両者は取得時刻がずれるため件数が一致しないことがある。見出しの件数は `ECS reported` のラベルを付けて ECS 側の申告値であることを示し、一覧側は実際に並べた件数を別に表示する。両者を突き合わせて警告する処理は入れない (キャッシュ TTL 内の正常なずれであり、Refresh で揃う)。
- `registeredCpu` 等が `null` のときは `-` を表示する。
- コンテナインスタンスが 0 件のとき (Fargate のみのクラスタ) は、`DrawerECSTasks.tsx` がコンテナ 0 件のときに出す空表示と同じ形式で、コンテナインスタンスが無いことを示すメッセージを出す。
- `containerInstanceArn` が空文字列のタスク (Fargate。0156 の `ecsTaskFromSDK` が nil を空文字列にする) はどのグループにも表示しない。EC2 と Fargate が混在するクラスタでも、このタブは EC2 上のタスクだけを扱う。
- `containerInstanceArn` が空文字列でなく、該当するコンテナインスタンスが一覧に無いタスク (`DescribeContainerInstances` の後に EC2 が登録解除された等) は、末尾に `Unknown container instance` に相当する見出しでまとめて表示する。
- 一覧のタスクは backend の `listECSTasks` が返す `desiredStatus` RUNNING のものだけである (0156 で `listECSTasks` のコメントに明記)。STOPPED のタスクは表示しない。

### i18n とエラー表示

- タブ名 `Instances` は英語ハードコードとし i18n に載せない (docs/issues/closed/0066 が明文化したタブ名の方針)。
- コンポーネント内のラベルとメッセージは `DrawerECSTasks.tsx` と同じく `useTranslation('drawerAws')` を使い、frontend/src/i18n/locales/{ja,en}/drawerAws.json に `ecsContainerInstances` キーを追加する。
- 2 つのクエリの状態は次のように合成する。どちらか一方でも `isLoading` なら `DrawerLoading` だけを表示する。どちらか一方でも `error` があれば、そのエラーを `DrawerError` で表示し、見出しもタスク一覧も描かない (コンテナインスタンスは取れてタスクだけ 403 のとき、EC2 の見出しの下が 0 件と区別できない空欄になるのを避ける)。両方のエラーがあるときはコンテナインスタンス側のエラーを表示する。両方成功したときだけ一覧を描く。`DrawerECSTasks.tsx` の単一クエリの規則 (`isLoading` でなければ、`error` があっても `data` があれば上部に `DrawerError` を出して本体も描く。docs/issues/closed/0075-feat-waf-rules-drawer.md で確定した表示規則) とは異なり、2 クエリでは `error` を `data` より優先して本体を描かない合成規則を新しく定める。片方の `data` だけで本体を描くと、上記の 0 件と区別できない空欄が生じるためである。2 つのクエリを合成する Drawer タブは本 issue が最初であり、合成規則の共通化 (フックや部品への抽出) は 2 例目が出た時点で検討し、本 issue ではこのコンポーネント内に閉じる。0156 の API が権限不足で 403 `ACCESS_DENIED` を返した場合もこの経路で表示する。

### 権限

- frontend の変更のみで、IAM 権限と依存の追加は無い。

## 完了条件

- frontend に `ECSContainerInstanceRaw` (`registered_cpu` / `registered_memory` / `remaining_cpu` / `remaining_memory` は `number | null`) / `ECSContainerInstanceRow` (対応フィールドは `number | null`)、`ecsContainerInstanceFromRaw`、`getECSContainerInstances`、`useECSContainerInstances` (queryKey `['aws', 'ecs-container-instances', profile, region, cluster]`) が追加され、`ECSTaskRow` / `ecsTaskFromRaw` に `containerInstanceArn` が追加されている (`ECSTaskRaw` の `container_instance_arn` は 0156 で追加済み)。
- frontend/src/types/contract.check.ts に `__contract__/ECSContainerInstanceResource.json` の import と `Expect<Contract<typeof ecsContainerInstance, ECSContainerInstanceRaw>>` が追加され、`tsc --noEmit` が通る。
- frontend/src/lib/normalize.test.ts に、`ecsContainerInstanceFromRaw` の変換 (値あり、`registered_cpu` が `null`) と `ecsTaskFromRaw` の `containerInstanceArn` (値あり、空文字列) のテストが追加されている。
- ECS クラスタの Drawer に `Instances` タブが `Tasks` の次に表示される。タブ名は `Drawer.tsx` の配列に英語で書き、`drawerAws.json` には追加しない。
- `DrawerECSContainerInstances` が `useECSTasks(profile, region, cluster)` を第 4 引数無しで呼ぶ。
- `status` と `lastStatus` が `StatusBadge` で表示され、`StatusBadge` の `MAP` に `draining` (`warn`) / `deregistering` (`warn`) / `registration-failed` (`err`) / `registering` (`info`) が追加されている。この変更で ELB Target Health の `State` 列と ECS サービス一覧の `State` 列の `draining` が `muted` から `warn` に変わることを PR 説明に書く。
- frontend/src/components/primitives/StatusBadge.test.tsx を新設し、`draining` / `deregistering` / `registration-failed` / `registering` の 4 状態がそれぞれ `warn` / `warn` / `err` / `info` のクラスで描画されること、未知の状態が `muted` で描画されることのテストが追加されている。
- コンテナインスタンスごとの見出しに `ec2InstanceId` / `status` / `agentConnected` / running と pending の件数 (`ECS reported` のラベル付き) / CPU と Memory の remaining と registered (`null` のとき `-`) が表示され、一覧側に実際に並べたタスク件数が表示され、その下に `containerInstanceArn` が一致するタスクの `group` / `lastStatus` / `cpu` / `memory` / `startedAt` が表示される。
- どちらかのクエリが `isLoading` のとき `DrawerLoading` だけが表示され、どちらかのクエリが `error` のとき `DrawerError` だけが表示されて見出しとタスク一覧は表示されず、両方が `error` のときコンテナインスタンス側のエラーが表示され、両方成功したときだけ一覧が表示される。
- `containerInstanceArn` が空文字列のタスクはどのグループにも表示されない。
- コンテナインスタンスが 0 件のとき空表示が出る。`containerInstanceArn` が空文字列でなく一致するコンテナインスタンスの無いタスクは `Unknown container instance` に相当する見出し (`drawerAws.json` の `ecsContainerInstances` キー) の下に表示される。
- ja / en の `drawerAws.json` に `ecsContainerInstances` のラベル、`ECS reported`、空表示、未対応見出しの文言が追加されている。
- `DrawerECSContainerInstances.test.tsx` に、2 台に 3 タスクを振り分ける表示、`draining` の 1 台の `status` が `warn` の `StatusBadge` で表示されること、`registration-failed` の 1 台が `err` で表示されること、`registeredCpu` が `null` のとき `-`、0 件の空表示、`Unknown container instance` の表示、EC2 タスク 2 件と Fargate タスク 1 件 (`containerInstanceArn` 空文字列) が混在するときに Fargate タスクがどのグループにも表示されないこと、コンテナインスタンスが成功しタスクだけ 403 のとき `DrawerError` だけが表示され見出しが表示されないこと、タスクが成功しコンテナインスタンスだけ 403 のとき `DrawerError` が表示されること、両方が異なるエラー (コンテナインスタンス 403、タスク 500) のときコンテナインスタンス側のエラーが表示されること、片方だけ `isLoading` のとき `DrawerLoading` だけが表示されること、同一の `QueryClient` に `DrawerECSTasks` と `DrawerECSContainerInstances` を両方マウントしたとき tasks エンドポイントへの GET が 1 回だけ呼ばれること (queryKey の共有) のテストが追加されている。
- EC2 一覧との突合、`Tasks` タブへの列追加、backend の変更は行わない。External (ECS Anywhere) のコンテナインスタンス (`ec2InstanceId` が `mi-` 始まり) を EC2 と区別する表示は行わない (0156 と同じくスコープ外)。
- `mise run check` が通過する。

## 関連

- docs/issues/0156-feat-ecs-container-instances-api.md: 本 issue が使う API とタスクのフィールドを追加する第 1 段階。0156 の close が本 issue の前提である。この前提は技術的な依存で、本 issue が `contract.check.ts` に追加する import は 0156 が生成する `frontend/src/types/__contract__/ECSContainerInstanceResource.json` が無いと解決できず、`ECSTaskRaw` の `container_instance_arn` も 0156 が追加する。モックで一部の表示テストは書けるが、`mise run check` は 0156 無しでは通らない。
- docs/issues/0155-feat-ecs-task-container-cpu-memory.md: `DrawerECSTasks.tsx` の Containers テーブルに列を追加する。本 issue は `DrawerECSTasks.tsx` を変更せず新規コンポーネントを追加する。frontend/src/lib/normalize.ts の `ecsTaskFromRaw` と normalize.test.ts は両 issue が編集するが、本 issue はトップレベルの `containerInstanceArn`、0155 は `containers` の map 内で編集箇所が分かれており、どちらを先に実装しても他方は壊れない。
- docs/issues/closed/0066-feat-rds-elasticache-parameter-groups.md: Drawer のタブ名を英語ハードコードとする方針 (ECR Images / ELB Targets タブの先例に倣ったもの。AGENTS.md frontend 節が参照する issue)。

## 解決方法

frontend のみを変更した。backend は変更していない。

- frontend/src/types/aws.ts に `ECSContainerInstanceRaw` / `ECSContainerInstanceRow` を追加し、`ECSTaskRow` に `containerInstanceArn: string` を追加した。
- frontend/src/lib/normalize.ts に `ecsContainerInstanceFromRaw` を追加し、`ecsTaskFromRaw` が `container_instance_arn` を `containerInstanceArn` に写すようにした。
- frontend/src/api/endpoints.ts に `getECSContainerInstances(profile, region, cluster)` を、frontend/src/api/queries.ts に `useECSContainerInstances(profile, region, cluster)` (queryKey `['aws', 'ecs-container-instances', profile, region, cluster]`) を追加した。
- frontend/src/types/contract.check.ts に `ECSContainerInstanceResource.json` の import と `Expect<Contract<typeof ecsContainerInstance, ECSContainerInstanceRaw>>` を追加した。
- frontend/src/components/Drawer/DrawerECSContainerInstances.tsx を新設した。`useECSContainerInstances` と `useECSTasks(profile, region, cluster)` (service 引数なし、Tasks タブと queryKey を共有) の 2 クエリを合成し、タスクを `containerInstanceArn` でインスタンスごとにグルーピングする。`containerInstanceArn === ''` (Fargate) のタスクはどこにも表示しない。一覧に無い ARN を持つタスクは末尾の `Unknown container instance` 見出しに集める。インスタンスが 0 件のときは空表示を出す (一覧に無い ARN のタスクがあれば空表示の下に `Unknown` の見出しも出す)。どちらかの query が loading なら `DrawerLoading` のみ、どちらかが error なら `DrawerError` のみ (両方 error のときはインスタンス側を優先) を表示し、見出しとタスクは出さない。
- frontend/src/components/Drawer/Drawer.tsx の `ecs` タブ一覧を `['Overview', 'Services', 'Tasks', 'Instances', 'Terminal', 'Tags']` にし、`Instances` タブで `DrawerECSContainerInstances` を描画するようにした。
- frontend/src/components/primitives/StatusBadge.tsx の `MAP` に `draining` (warn) / `deregistering` (warn) / `registration-failed` (err) / `registering` (info) を追加した。`draining` は ELB Target Health と ECS サービスの State 列にも現れるため、これらの表示が muted から warn に変わる。
- frontend/src/i18n/locales/{ja,en}/drawerAws.json に `ecsContainerInstances` キー (見出し、空表示、エージェント接続、`ECS reported`、CPU / Memory ラベル、タスク件数、`Unknown container instance`) を追加した。タブ名 `Instances` は英語ハードコードとし、`Unknown container instance` に相当する見出しは ja では `不明なコンテナインスタンス`、en では `Unknown container instance` とした。
- テスト: frontend/src/lib/normalize.test.ts に `ecsContainerInstanceFromRaw` (値の写しと `null` の保持) と `ecsTaskFromRaw` の `containerInstanceArn` (ARN と空文字列) を追加し、既存の `toEqual` の期待値に `containerInstanceArn: ''` を足した。frontend/src/components/primitives/StatusBadge.test.tsx を新設し 4 状態のクラスと未知の状態の muted を検証した。frontend/src/components/Drawer/DrawerECSContainerInstances.test.tsx を新設し、完了条件に列挙した 11 ケースとインスタンス 0 件かつ一覧に無い ARN のタスクがあるケース (2 インスタンス 3 タスクのグルーピング、draining / registration-failed のバッジ、`registered_cpu` null の `-` 表示、0 件の空表示、Unknown container instance、Fargate タスクの非表示、instances 成功 + tasks 403、tasks 成功 + instances 403、両方エラー時のインスタンス側優先、片方 loading 時の Loading のみ、同じ QueryClient での Tasks タブとの tasks 取得 1 回、0 件かつ Unknown タスクありのときの空表示と Unknown の併記) を検証した。
- `mise run check` は通過した (frontend 786 tests でベースラインの 765 から 21 件増、lint 0 errors / 10 warnings でベースラインと同じ)。
